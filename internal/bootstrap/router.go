package bootstrap

import (
	"net/http"
	"slices"
	"strings"

	"github.com/coniungo-repo/golang-server/api"
	"github.com/coniungo-repo/golang-server/internal/docs"
	"github.com/coniungo-repo/golang-server/internal/platform/metrics"
	"github.com/coniungo-repo/golang-server/internal/platform/middleware"
	"github.com/coniungo-repo/golang-server/internal/platform/ratelimit"
	"github.com/coniungo-repo/golang-server/internal/platform/request"
	"github.com/coniungo-repo/golang-server/internal/platform/requestid"
	"github.com/coniungo-repo/golang-server/internal/platform/respond"
	"github.com/coniungo-repo/golang-server/internal/platform/route"
)

const (
	apiPrefix         = "/api/v1"
	notFoundMessage   = "We couldn't find that. Check the link and try again."
	notAllowedMessage = "That action isn't supported here."
)

var (
	corsAllowHeaders = []string{"Content-Type", "Authorization", requestid.Header, "Idempotency-Key"}
	healthPaths      = []string{"/livez", "/readyz"}
	probeMethods     = []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete}
)

func newRouter(deps *dependencies) (http.Handler, error) {
	mux := http.NewServeMux()
	deps.Health.Register(mux)
	if deps.Config.Docs.Enabled {
		docs.Register(mux, api.OpenAPI)
	}
	registerAPI(route.Prefix(mux, apiPrefix), deps)

	handler := fallback(mux)
	allowHeaders := corsAllowHeaders
	var authPrefixes []string
	if deps.Auth != nil {
		handler = deps.Auth.Middleware(handler)
		allowHeaders = mergeHeaders(corsAllowHeaders, deps.Auth.CORSHeaders())
		authPrefixes = append(authPrefixes, deps.Config.Auth.APIBasePath)
	}

	cors, err := middleware.CORS(deps.Config.CORS.AllowedOrigins, allowHeaders, deps.Config.CORS.MaxAge)
	if err != nil {
		return nil, err
	}

	return middleware.Chain(
		handler,
		middleware.RequestID(deps.Config.Server.RequestIDHeader),
		middleware.Log,
		middleware.Metrics(deps.Metrics),
		middleware.Routes(mux, authPrefixes...),
		middleware.Recover,
		middleware.SecureHeaders(deps.Config.App.IsProduction()),
		cors,
		middleware.RateLimit(metrics.LimitClientIP, deps.IPLimit, clientKey(deps.Config.Server.ClientIPHeader), deps.Metrics),
		middleware.MaxBytes(request.MaxBodyBytes),
		middleware.Timeout(deps.Config.Server.RequestTimeout),
	), nil
}

func registerAPI(mux route.Mux, deps *dependencies) {
	if deps.Auth != nil {
		deps.Auth.Register(mux)
	}
}

func mergeHeaders(sets ...[]string) []string {
	seen := map[string]bool{}
	var out []string
	for _, set := range sets {
		for _, h := range set {
			key := http.CanonicalHeaderKey(h)
			if !seen[key] {
				seen[key] = true
				out = append(out, h)
			}
		}
	}
	return out
}

// fallback serves mux, answering unknown paths with a JSON 404 and known paths with the wrong method with a JSON 405.
func fallback(mux *http.ServeMux) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, pattern := mux.Handler(r); pattern != "" {
			mux.ServeHTTP(w, r)
			return
		}
		if allow := allowedMethods(mux, r); len(allow) > 0 {
			w.Header().Set("Allow", strings.Join(allow, ", "))
			respond.Error(w, r, http.StatusMethodNotAllowed, notAllowedMessage)
			return
		}
		respond.Error(w, r, http.StatusNotFound, notFoundMessage)
	})
}

func allowedMethods(mux *http.ServeMux, r *http.Request) []string {
	var allow []string
	for _, m := range probeMethods {
		probe := r.Clone(r.Context())
		probe.Method = m
		if _, pattern := mux.Handler(probe); pattern != "" {
			allow = append(allow, m)
		}
	}
	return allow
}

// clientKey limits every route by client IP except the health probes, which load balancers call often from shared addresses.
func clientKey(trustedHeader string) func(*http.Request) string {
	return func(r *http.Request) string {
		if slices.Contains(healthPaths, r.URL.Path) {
			return ""
		}
		return ratelimit.ClientKey(request.ClientIP(r, trustedHeader))
	}
}
