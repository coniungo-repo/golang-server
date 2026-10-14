package logger

import (
	"context"
	"log/slog"
)

type ctxKey struct{}

// Into returns a context carrying l, so a request's logger (with its request ID) follows it through calls.
func Into(ctx context.Context, l *slog.Logger) context.Context {
	return context.WithValue(ctx, ctxKey{}, l)
}

// From returns the context's logger, or slog.Default when it has none.
func From(ctx context.Context) *slog.Logger {
	if l, ok := ctx.Value(ctxKey{}).(*slog.Logger); ok {
		return l
	}
	return slog.Default()
}
