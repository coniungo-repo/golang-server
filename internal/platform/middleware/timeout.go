package middleware

import (
	"context"
	"net/http"
	"time"
)

// Timeout sets a deadline on the request context. Handlers must pass r.Context() to I/O and report its error through respond.ServerError.
func Timeout(d time.Duration) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx, cancel := context.WithTimeout(r.Context(), d)
			defer cancel()
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
