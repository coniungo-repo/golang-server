package httpserver

import (
	"context"
	"errors"
	"net/http"

	"github.com/coniungo-repo/golang-server.git/internal/platform/config"
	"github.com/coniungo-repo/golang-server.git/internal/platform/logger"
)

// Run starts the HTTP server and handles graceful shutdown based on context.
func Run(ctx context.Context, cfg config.Server, handler http.Handler) error {
	log := logger.From(ctx)

	srv := &http.Server{
		Addr:              cfg.Addr(),
		Handler:           handler,
		ReadHeaderTimeout: cfg.ReadHeaderTimeout,
		ReadTimeout:       cfg.ReadTimeout,
		WriteTimeout:      cfg.WriteTimeout,
		IdleTimeout:       cfg.IdleTimeout,
	}

	errCh := make(chan error, 1)

	log.Info("server starting", "addr", srv.Addr)

	go func() {
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case err := <-errCh:
		log.Error("server failed to start", "err", err)
		return err

	case <-ctx.Done():
		log.Info("server shutting down")

		shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
		defer cancel()

		if err := srv.Shutdown(shutdownCtx); err != nil {
			log.Error("graceful shutdown failed", "err", err)
			return err
		}

		log.Info("server stopped")
		return nil
	}
}
