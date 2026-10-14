package middleware

import (
	"net/http"

	"github.com/coniungo-repo/golang-server/internal/platform/respond"
)

const tooLargeMessage = "That's too much to send at once. Try again with less."

// MaxBytes rejects bodies over n bytes, by Content-Length up front and by a capped reader for streamed bodies.
func MaxBytes(n int64) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.ContentLength > n {
				respond.Error(w, r, http.StatusRequestEntityTooLarge, tooLargeMessage)
				return
			}
			r.Body = http.MaxBytesReader(w, r.Body, n)
			next.ServeHTTP(w, r)
		})
	}
}
