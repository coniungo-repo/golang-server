package bootstrap

import (
	"net/http"
	"strings"

	"github.com/coniungo-repo/golang-server/internal/platform/middleware"
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
	probeMethods     = []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete}
)

func newRouter(deps *dependencies) (http.Handler, error) {
	mux := http.NewServeMux()
	deps.Health.Register(mux)
	registerAPI(route.Prefix(mux, apiPrefix), deps)

	cors, err := middleware.CORS(deps.Config.CORS.AllowedOrigins, corsAllowHeaders)
	if err != nil {
		return nil, err
	}

	return middleware.Chain(
		fallback(mux),
		middleware.RequestID(deps.Config.Server.RequestIDHeader),
		middleware.Log,
		middleware.Routes(mux),
		middleware.Recover,
		middleware.SecureHeaders(deps.Config.App.IsProduction()),
		cors,
		middleware.MaxBytes(request.MaxBodyBytes),
		middleware.Timeout(deps.Config.Server.RequestTimeout),
	), nil
}

func registerAPI(mux route.Mux, deps *dependencies) {}

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
