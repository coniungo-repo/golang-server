package middleware

import (
	"context"
	"net/http"
)

type routeKey struct{}

// Routes allows the Log middleware to see which pattern matched.
func Routes(mux *http.ServeMux) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if route, ok := r.Context().Value(routeKey{}).(*string); ok {
				_, pattern := mux.Handler(r)
				*route = pattern
			}

			next.ServeHTTP(w, r)
		})
	}
}
func withRoute(ctx context.Context) (context.Context, *string) {
	route := new(string)
	return context.WithValue(ctx, routeKey{}, route), route
}
