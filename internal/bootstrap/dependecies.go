package bootstrap

import (
	"context"
	"log/slog"
	"os"

	"github.com/coniungo-repo/golang-server.git/internal/platform/clock"
	"github.com/coniungo-repo/golang-server.git/internal/platform/config"
	"github.com/coniungo-repo/golang-server.git/internal/platform/database"
	"github.com/coniungo-repo/golang-server.git/internal/platform/logger"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Dependencies holds the core resources required by the application.
type dependencies struct {
	Config config.Config
	Log    *slog.Logger
	DB     *pgxpool.Pool
	Clock  clock.Clock
}

func NewDependencies(ctx context.Context, cfg config.Config) (*dependencies, error) {

	log := logger.New(os.Stdout, logger.Options{
		Level:        cfg.Log.Level,
		IsProduction: cfg.App.IsProduction(),
		ServiceName:  cfg.App.Name,
		Environment:  string(cfg.App.Environment),
	})

	slog.SetDefault(log)

	dbCtx := logger.Into(ctx, log)
	pool, err := database.Open(dbCtx, cfg.Database)
	if err != nil {
		log.Error("database connection failed", "err", err)
		return nil, err
	}

	return &dependencies{
		Config: cfg,
		Log:    log,
		DB:     pool,
		Clock:  clock.New(),
	}, nil
}

func (d *dependencies) close() {
	d.Log.Info("shutting down dependencies")
	d.DB.Close()
}
