package logger

import (
	"io"
	"log/slog"
	"strings"
)

const redactedValue = "[redacted]"

// Pre-compute sensitive keys for O(1) lookup
var sensitiveKeys = map[string]struct{}{
	"cookie":        {},
	"authorization": {},
	"set-cookie":    {},
	"password":      {},
	"token":         {},
	"api_key":       {},
	"secret":        {},
}

// Options allows the logger to be configured without depending on the config package
type Options struct {
	Level        slog.Level
	IsProduction bool
	ServiceName  string
	Environment  string
}

func New(w io.Writer, opts Options) *slog.Logger {
	handlerOpts := &slog.HandlerOptions{
		Level:       opts.Level,
		AddSource:   !opts.IsProduction,
		ReplaceAttr: redact,
	}

	var h slog.Handler
	if opts.IsProduction {
		h = slog.NewJSONHandler(w, handlerOpts)
	} else {
		h = slog.NewTextHandler(w, handlerOpts)
	}

	return slog.New(h).With(
		slog.String("service", opts.ServiceName),
		slog.String("env", opts.Environment),
	)
}

func redact(_ []string, a slog.Attr) slog.Attr {
	// Only check if it's a string or sensitive key
	// We lowercase the key for comparison to catch "Cookie", "COOKIE", etc.
	if _, ok := sensitiveKeys[strings.ToLower(a.Key)]; ok {
		return slog.String(a.Key, redactedValue)
	}

	return a
}
