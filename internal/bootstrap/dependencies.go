package bootstrap

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/coniungo-repo/golang-server/internal/health"
	"github.com/coniungo-repo/golang-server/internal/platform/clock"
	"github.com/coniungo-repo/golang-server/internal/platform/config"
	"github.com/coniungo-repo/golang-server/internal/platform/database"
	"github.com/coniungo-repo/golang-server/internal/platform/phone"
	"github.com/jackc/pgx/v5/pgxpool"
)

type dependencies struct {
	Config config.Config
	Log    *slog.Logger
	DB     *pgxpool.Pool
	Clock  clock.Clock
	Health *health.Probe
	Phone  *phone.Validator
}

func newDependencies(ctx context.Context, cfg config.Config, log *slog.Logger) (*dependencies, error) {
	phones, err := newPhoneValidator(cfg.Phone)
	if err != nil {
		return nil, err
	}

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
		Phone:  phones,
	}, nil
}

func newPhoneValidator(cfg config.Phone) (*phone.Validator, error) {
	var opts []phone.Option
	if cfg.DefaultRegion != "" {
		opts = append(opts, phone.WithDefaultRegion(cfg.DefaultRegion))
	}
	if len(cfg.AllowedRegions) > 0 {
		opts = append(opts, phone.WithAllowedRegions(cfg.AllowedRegions...))
	}
	v, err := phone.New(opts...)
	if err != nil {
		return nil, fmt.Errorf("configuring phone validator: %w", err)
	}
	return v, nil
}

func (d *dependencies) close() {
	d.Log.Info("closing dependencies")
	d.DB.Close()
}
