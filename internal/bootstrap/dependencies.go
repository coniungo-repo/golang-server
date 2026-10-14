package bootstrap

import (
	"context"
	"log/slog"

	"github.com/coniungo-repo/golang-server/internal/health"
	"github.com/coniungo-repo/golang-server/internal/platform/clock"
	"github.com/coniungo-repo/golang-server/internal/platform/config"
	"github.com/coniungo-repo/golang-server/internal/platform/database"
	"github.com/jackc/pgx/v5/pgxpool"
)

type dependencies struct {
	Config config.Config
	Log    *slog.Logger
	DB     *pgxpool.Pool
	Clock  clock.Clock
	Health *health.Probe
}

func newDependencies(ctx context.Context, cfg config.Config, log *slog.Logger) (*dependencies, error) {
	pool, err := database.Open(ctx, cfg.Database)
	if err != nil {
		return nil, err
	}

	return &dependencies{
		Config: cfg,
		Log:    log,
		DB:     pool,
		Clock:  clock.New(),
		Health: health.New(pool),
	}, nil
}

func (d *dependencies) close() {
	d.Log.Info("closing dependencies")
	d.DB.Close()
}
