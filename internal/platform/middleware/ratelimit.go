package middleware

import (
	"net/http"

	"github.com/coniungo-repo/golang-server/internal/platform/logger"
	"github.com/coniungo-repo/golang-server/internal/platform/metrics"
	"github.com/coniungo-repo/golang-server/internal/platform/ratelimit"
	"github.com/coniungo-repo/golang-server/internal/platform/respond"
)

// RateLimit answers 429 with Retry-After once a key goes over l. Requests whose key is empty are not limited. name labels the log line and the refusal metric.
func RateLimit(name string, l *ratelimit.Limiter, key func(*http.Request) string, m *metrics.Metrics) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if k := key(r); k != "" {
				if ok, wait := l.Allow(k); !ok {
					logger.From(r.Context()).Warn("rate limited", "limit", name, "retry_after", wait)
					m.RateLimited(name)
					respond.TooManyRequests(w, r, wait)
					return
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}
