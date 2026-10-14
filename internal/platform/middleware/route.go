package middleware

import (
	"context"
	"net/http"
)

const unmatchedRoute = "unmatched"

type routeKey struct{}

// Routes records the matched mux pattern so Log can label requests without high-cardinality paths.
func Routes(mux *http.ServeMux) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if route, ok := r.Context().Value(routeKey{}).(*string); ok {
				if _, pattern := mux.Handler(r); pattern != "" {
					*route = pattern
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}

func withRoute(ctx context.Context) (context.Context, *string) {
	route := new(string)
	return context.WithValue(ctx, routeKey{}, route), route
}
