// Command migrate applies pending database migrations. Fly runs it as the release command, before any machine starts the new version, so a failed migration stops the deploy.
package main

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/coniungo-repo/golang-server/internal/platform/config"
	"github.com/coniungo-repo/golang-server/internal/platform/logger"
	"github.com/coniungo-repo/golang-server/migrations"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
)

const (
	timeout     = 5 * time.Minute
	lockTimeout = "10s"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx); err != nil {
		fmt.Fprintln(os.Stderr, "migrate:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context) error {
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

	db, err := open(cfg.Database)
	if err != nil {
		return err
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	results, err := migrations.Up(ctx, db)
	for _, r := range results {
		log.Info("migration applied", "version", r.Source.Version, "duration", r.Duration)
	}
	if err != nil {
		return err
	}
	if len(results) == 0 {
		log.Info("no pending migrations")
	}
	return nil
}

// open connects with lock_timeout set, so a migration waiting on a busy table fails fast instead of queueing live queries behind it.
func open(cfg config.Database) (*sql.DB, error) {
	cc, err := pgx.ParseConfig(cfg.URL.Reveal())
	if err != nil {
		return nil, fmt.Errorf("parsing database url: %w", err)
	}
	cc.ConnectTimeout = cfg.ConnectTimeout
	cc.RuntimeParams["lock_timeout"] = lockTimeout
	return stdlib.OpenDB(*cc), nil
}
