package middleware

import (
	"net/http"

	"github.com/coniungo-repo/golang-server.git/internal/platform/respond"
)

const tooLargeMessage = "That's too much to send at once. Try again with less."

// MaxBytes limits the size of the request body.
// It checks the Content-Length header for an early exit and wraps the body
// in a MaxBytesReader to catch streaming overflows.
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
