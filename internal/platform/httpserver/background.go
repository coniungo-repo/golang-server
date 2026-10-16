package httpserver

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"
)

const (
	backgroundShutdown     = 5 * time.Second
	backgroundReadTimeout  = 10 * time.Second
	backgroundWriteTimeout = 30 * time.Second
	backgroundIdleTimeout  = time.Minute
)

// ServeBackground listens on addr now, so a bad address fails startup, then serves handler until ctx is done. It is for internal endpoints such as metrics, never the public API.
func ServeBackground(ctx context.Context, addr string, handler http.Handler, log *slog.Logger) (net.Addr, error) {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("listening on %s: %w", addr, err)
	}
	srv := &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       backgroundReadTimeout,
		WriteTimeout:      backgroundWriteTimeout,
		IdleTimeout:       backgroundIdleTimeout,
		ErrorLog:          slog.NewLogLogger(log.Handler(), slog.LevelWarn),
	}
	go func() {
		if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("internal server stopped", "addr", addr, "err", err)
		}
	}()
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), backgroundShutdown)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()
	log.Info("internal server started", "addr", ln.Addr().String())
	return ln.Addr(), nil
}
