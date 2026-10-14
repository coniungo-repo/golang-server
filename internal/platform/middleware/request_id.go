package middleware

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"

	"github.com/coniungo-repo/golang-server.git/internal/platform/logger"
	"github.com/coniungo-repo/golang-server.git/internal/platform/respond"
)

func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := incomingID(r)

		if id == "" {
			id = newID()
		}

		w.Header().Set(respond.RequestIDHeader, id)
		l := logger.From(r.Context()).With("request_id", id)
		ctx := logger.Into(r.Context(), l)
		ctx, _ = withRoute(ctx)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func incomingID(r *http.Request) string {
	if id := r.Header.Get("Fly-Request-Id"); safeID(id) {
		return id
	}
	return ""
}

func safeID(id string) bool {
	if id == "" || len(id) > 64 {
		return false
	}
	for _, c := range id {
		ok := c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == '_'
		if !ok {
			return false
		}
	}
	return true
}

func newID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return "req_" + hex.EncodeToString(b)
}
