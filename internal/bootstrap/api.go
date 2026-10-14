package bootstrap

import (
	"context"
	"os"

	"github.com/coniungo-repo/golang-server/internal/platform/config"
	"github.com/coniungo-repo/golang-server/internal/platform/httpserver"
	"github.com/coniungo-repo/golang-server/internal/platform/logger"
)

func RunAPI(ctx context.Context) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	log := logger.New(os.Stdout, logger.Options{
		Level:        cfg.Log.Level,
		IsProduction: cfg.App.IsProduction(),
		ServiceName:  cfg.App.Name,
		Environment:  string(cfg.App.Environment),
		Version:      cfg.App.Version,
	})

	deps, err := newDependencies(logger.Into(ctx, log), cfg, log)
	if err != nil {
		log.Error("startup failed", "err", err)
		return err
	}
	defer deps.close()

	handler, err := newRouter(deps)
	if err != nil {
		return err
	}
	return httpserver.Run(ctx, cfg.Server, log, handler, deps.Health.Drain)
}
