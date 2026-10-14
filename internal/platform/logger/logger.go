package logger

import (
	"io"
	"log/slog"
	"strings"
)

const redactedValue = "[redacted]"

var sensitiveFragments = []string{
	"authorization",
	"cookie",
	"password",
	"passwd",
	"secret",
	"token",
	"api_key",
	"apikey",
	"private_key",
	"dsn",
	"database_url",
}

type Options struct {
	Level        slog.Level
	IsProduction bool
	ServiceName  string
	Environment  string
	Version      string
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
		slog.String("version", opts.Version),
	)
}

func redact(_ []string, a slog.Attr) slog.Attr {
	if a.Value.Kind() == slog.KindGroup {
		return a
	}
	if Sensitive(a.Key) {
		return slog.String(a.Key, redactedValue)
	}
	return a
}

// Sensitive reports whether a log key looks like it holds a credential.
func Sensitive(key string) bool {
	k := strings.ToLower(key)
	for _, f := range sensitiveFragments {
		if strings.Contains(k, f) {
			return true
		}
	}
	return false
}
