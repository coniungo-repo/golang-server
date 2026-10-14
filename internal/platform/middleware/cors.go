package middleware

import (
	"net/http"
	"slices"
	"strings"

	"github.com/coniungo-repo/golang-server.git/internal/platform/respond"
)

const (
	corsMethods          = "GET, POST, PUT, PATCH, DELETE"
	exposeHeaders        = "X-Request-ID, Retry-After, Idempotent-Replayed, Location"
	foreignOriginMessage = "That came from a site we don't know. Please open the official app and try again."
)

// unsafe returns true for methods that can change state.
func unsafe(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return false
	}
	return true
}

// CORS provides a strict Cross-Origin Resource Sharing policy.
// It allows the listed origins to make credentialed calls.
// It also acts as a CSRF defense by blocking unsafe methods from unknown origins.
func CORS(origins []string, allowHeaders []string) Middleware {
	joinedAllowHeaders := strings.Join(allowHeaders, ", ")

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Add("Vary", "Origin")
			origin := r.Header.Get("Origin")
			if origin == "" {
				// Requests with no Origin (like system-to-system or webhooks) pass through.
				next.ServeHTTP(w, r)
				return
			}

			allowed := slices.Contains(origins, origin)

			if allowed {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Access-Control-Allow-Credentials", "true")
				w.Header().Set("Access-Control-Expose-Headers", exposeHeaders)
			}

			// CSRF Protection: If the origin is not allowed and the method is "unsafe"
			// (POST/PUT/etc), we block it even if it's not a preflight.
			if !allowed && unsafe(r.Method) {
				respond.Error(w, r, http.StatusForbidden, foreignOriginMessage)
				return
			}

			// Handle Preflight (OPTIONS)
			if r.Method == http.MethodOptions && r.Header.Get("Access-Control-Request-Method") != "" {
				if allowed {
					w.Header().Add("Vary", "Access-Control-Request-Method")
					w.Header().Add("Vary", "Access-Control-Request-Headers")
					w.Header().Set("Access-Control-Allow-Methods", corsMethods)
					w.Header().Set("Access-Control-Allow-Headers", joinedAllowHeaders)
					w.Header().Set("Access-Control-Max-Age", "600")
				}
				w.WriteHeader(http.StatusNoContent)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
