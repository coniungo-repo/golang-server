// Package middleware holds the HTTP middleware chain every request passes through: request IDs, logging, metrics, route labels, panic recovery, security headers, CORS and CSRF protection, per-IP rate limiting, body limits and request deadlines. The order is set in internal/bootstrap/router.go and described in docs/architecture.md.
package middleware

import (
	"net/http"
	"slices"
)

type Middleware func(http.Handler) http.Handler

// Chain wraps h so the first middleware runs first.
func Chain(h http.Handler, mws ...Middleware) http.Handler {
	for _, mw := range slices.Backward(mws) {
		h = mw(h)
	}
	return h
}

type recorder struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (r *recorder) WriteHeader(status int) {
	if r.status == 0 {
		r.status = status
	}
	r.ResponseWriter.WriteHeader(status)
}

func (r *recorder) Write(b []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	n, err := r.ResponseWriter.Write(b)
	r.bytes += n
	return n, err
}

func (r *recorder) Unwrap() http.ResponseWriter { return r.ResponseWriter }
