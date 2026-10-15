package middleware

import (
	"context"
	"net/http"
	"strings"
)

const unmatchedRoute = "unmatched"

type routeKey struct{}

// Routes records the matched mux pattern so Log can label requests without high-cardinality paths. Unmatched paths under one of prefixes are labelled prefix+"/*".
func Routes(mux *http.ServeMux, prefixes ...string) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if route, ok := r.Context().Value(routeKey{}).(*string); ok {
				if _, pattern := mux.Handler(r); pattern != "" {
					*route = pattern
				} else if p, ok := matchPrefix(r.URL.Path, prefixes); ok {
					*route = p + "/*"
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

func matchPrefix(path string, prefixes []string) (string, bool) {
	for _, p := range prefixes {
		if path == p || strings.HasPrefix(path, p+"/") {
			return p, true
		}
	}
	return "", false
}
