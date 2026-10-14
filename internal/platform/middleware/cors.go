package middleware

import (
	"fmt"
	"net/http"
	"slices"
	"strings"

	"github.com/coniungo-repo/golang-server/internal/platform/respond"
)

const (
	corsMethods          = "GET, HEAD, POST, PUT, PATCH, DELETE"
	corsMaxAge           = "600"
	exposeHeaders        = "X-Request-ID, Retry-After, Idempotent-Replayed, Location"
	foreignOriginMessage = "That came from a site we don't know. Please open the official app and try again."
)

// CORS lets the listed origins make credentialed calls and rejects cross-origin state-changing requests from anywhere else, using Origin and Sec-Fetch-Site.
func CORS(origins, allowHeaders []string) (Middleware, error) {
	csrf := http.NewCrossOriginProtection()
	for _, o := range origins {
		if err := csrf.AddTrustedOrigin(o); err != nil {
			return nil, fmt.Errorf("cors origin %q: %w", o, err)
		}
	}
	csrf.SetDenyHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respond.Error(w, r, http.StatusForbidden, foreignOriginMessage)
	}))
	joinedAllowHeaders := strings.Join(allowHeaders, ", ")

	return func(next http.Handler) http.Handler {
		protected := csrf.Handler(next)

		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			h := w.Header()
			h.Add("Vary", "Origin")

			origin := r.Header.Get("Origin")
			allowed := origin != "" && slices.Contains(origins, origin)
			if allowed {
				h.Set("Access-Control-Allow-Origin", origin)
				h.Set("Access-Control-Allow-Credentials", "true")
				h.Set("Access-Control-Expose-Headers", exposeHeaders)
			}

			if r.Method == http.MethodOptions && r.Header.Get("Access-Control-Request-Method") != "" {
				h.Add("Vary", "Access-Control-Request-Method")
				h.Add("Vary", "Access-Control-Request-Headers")
				if allowed {
					h.Set("Access-Control-Allow-Methods", corsMethods)
					h.Set("Access-Control-Allow-Headers", joinedAllowHeaders)
					h.Set("Access-Control-Max-Age", corsMaxAge)
				}
				w.WriteHeader(http.StatusNoContent)
				return
			}

			protected.ServeHTTP(w, r)
		})
	}, nil
}
