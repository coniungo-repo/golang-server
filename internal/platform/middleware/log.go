package middleware

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/coniungo-repo/golang-server/internal/platform/logger"
)

func Log(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &recorder{ResponseWriter: w}

		ctx, route := withRoute(r.Context())
		ctx, user := withUser(ctx)
		r = r.WithContext(ctx)

		next.ServeHTTP(rec, r)

		if *route == "" {
			*route = unmatchedRoute
		}
		status := rec.status
		if status == 0 {
			status = http.StatusOK
		}

		level := slog.LevelInfo
		switch {
		case status >= http.StatusInternalServerError:
			level = slog.LevelError
		case status >= http.StatusBadRequest:
			level = slog.LevelWarn
		}

		attrs := []slog.Attr{
			slog.String("method", r.Method),
			slog.String("route", *route),
			slog.Int("status", status),
			slog.Int("bytes", rec.bytes),
			slog.Duration("duration", time.Since(start)),
		}
		if *user != "" {
			attrs = append(attrs, slog.String("user_id", *user))
		}
		logger.From(r.Context()).LogAttrs(r.Context(), level, "request finished", attrs...)
	})
}
