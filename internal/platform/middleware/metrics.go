package middleware

import (
	"net/http"
	"time"

	"github.com/coniungo-repo/golang-server/internal/platform/metrics"
)

// Metrics records every request's count and duration by method, matched route and status. Place it inside Log, which provides the route.
func Metrics(m *metrics.Metrics) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rec := &recorder{ResponseWriter: w}
			next.ServeHTTP(rec, r)

			route := unmatchedRoute
			if p, ok := r.Context().Value(routeKey{}).(*string); ok && *p != "" {
				route = *p
			}
			status := rec.status
			if status == 0 {
				status = http.StatusOK
			}
			m.ObserveRequest(r.Method, route, status, time.Since(start))
		})
	}
}
