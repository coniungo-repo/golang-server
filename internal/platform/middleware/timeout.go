package middleware

import (
	"context"
	"net/http"
	"time"
)

// Timeout gives each request a deadline. Handlers and database calls must
// respect r.Context() for this to be effective.
func Timeout(d time.Duration) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx, cancel := context.WithTimeout(r.Context(), d)
			defer cancel()

			next.ServeHTTP(w, r.WithContext(ctx))

			// Note: We don't manually send a 504 Gateway Timeout here.
			// Instead, we let the handler or database return context.DeadlineExceeded,
			// which our respond.ServerError or Log middleware will then catch.
		})
	}
}
