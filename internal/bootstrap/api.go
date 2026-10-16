package bootstrap

import (
	"context"
	"os"
	"time"

	"github.com/coniungo-repo/golang-server/internal/platform/config"
	"github.com/coniungo-repo/golang-server/internal/platform/httpserver"
	"github.com/coniungo-repo/golang-server/internal/platform/logger"
)

const (
	pruneInterval = time.Hour
	sweepInterval = 5 * time.Minute
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

	for _, w := range startupWarnings(cfg) {
		log.Warn(w)
	}

	deps, err := newDependencies(logger.Into(ctx, log), cfg, log)
	if err != nil {
		log.Error("startup failed", "err", err)
		return err
	}
	defer deps.close()

	go deps.Limiter.RunPruner(ctx, pruneInterval, func(err error) {
		log.Warn("pruning sign-in attempts failed", "err", err)
	})

	go deps.IPLimit.RunSweeper(ctx, sweepInterval)
	go deps.UserLimit.RunSweeper(ctx, sweepInterval)

	if addr := cfg.Server.MetricsAddr; addr != "" {
		if _, err := httpserver.ServeBackground(ctx, addr, deps.Metrics.Handler(), log); err != nil {
			return err
		}
	}

	handler, err := newRouter(deps)
	if err != nil {
		return err
	}
	return httpserver.Run(ctx, cfg.Server, log, handler, deps.Health.Drain)
}

// startupWarnings flags settings that are valid but probably wrong for where the server runs.
func startupWarnings(cfg config.Config) []string {
	var out []string
	if cfg.App.IsProduction() && cfg.Server.ClientIPHeader == "" {
		out = append(out, "HTTP_CLIENT_IP_HEADER is empty: behind a proxy every user shares the proxy's IP and its rate limit. Set it to the proxy's client IP header, e.g. Fly-Client-IP")
	}
	return out
}
