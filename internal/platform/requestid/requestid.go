// Package requestid generates request IDs, validates ones received from a trusted proxy, and stores them on the context, so logs, error bodies and the X-Request-ID header all carry the same ID.
package requestid

import (
	"context"
	"crypto/rand"
	"encoding/hex"
)

const (
	Header = "X-Request-ID"
	maxLen = 64
)

type ctxKey struct{}

func Into(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, ctxKey{}, id)
}

func From(ctx context.Context) string {
	id, _ := ctx.Value(ctxKey{}).(string)
	return id
}

func New() string {
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	return "req_" + hex.EncodeToString(b)
}

// Valid reports whether id is safe to log and echo: 1 to 64 characters of [A-Za-z0-9_-].
func Valid(id string) bool {
	if id == "" || len(id) > maxLen {
		return false
	}
	for _, c := range id {
		if (c < 'a' || c > 'z') && (c < 'A' || c > 'Z') && (c < '0' || c > '9') && c != '-' && c != '_' {
			return false
		}
	}
	return true
}
