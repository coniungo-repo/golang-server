package middleware

import (
	"net/http"

	"github.com/coniungo-repo/golang-server/internal/platform/logger"
	"github.com/coniungo-repo/golang-server/internal/platform/requestid"
)

// RequestID tags each request with an ID. trustedHeader names a header set by a proxy you control; leave it empty to always generate one.
func RequestID(trustedHeader string) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			id := ""
			if trustedHeader != "" {
				if v := r.Header.Get(trustedHeader); requestid.Valid(v) {
					id = v
				}
			}
			if id == "" {
				id = requestid.New()
			}

			w.Header().Set(requestid.Header, id)
			ctx := requestid.Into(r.Context(), id)
			ctx = logger.Into(ctx, logger.From(ctx).With("request_id", id))
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
