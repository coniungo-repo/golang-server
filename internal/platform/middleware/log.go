package middleware

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/coniungo-repo/golang-server.git/internal/platform/logger"
)

func Log(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		rec := &recorder{ResponseWriter: w}

		ctx, route := withRoute(r.Context())
		r = r.WithContext(ctx)

		next.ServeHTTP(rec, r)

		if *route == "" {
			*route = r.Method + " " + r.URL.Path
		}

		status := rec.status
		if status == 0 {
			status = http.StatusOK
		}

		level := slog.LevelInfo
		if status >= http.StatusInternalServerError {
			level = slog.LevelError
		}

		logger.From(r.Context()).LogAttrs(r.Context(), level, "request finished",
			slog.String("route", *route),
			slog.Int("status", status),
			slog.Int("bytes", rec.bytes),
			slog.Duration("duration", time.Since(start)),
			slog.String("method", r.Method),
		)
	})
}
