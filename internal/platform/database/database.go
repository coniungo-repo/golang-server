package database

import (
	"context"
	"fmt"
	"strconv"

	"github.com/coniungo-repo/golang-server/internal/platform/config"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Open builds a pool from cfg and verifies it with a ping before returning.
func Open(ctx context.Context, cfg config.Database) (*pgxpool.Pool, error) {
	poolCfg, err := pgxpool.ParseConfig(cfg.URL.Reveal())
	if err != nil {
		return nil, fmt.Errorf("parsing database url: %w", err)
	}

	configure(poolCfg, cfg)

	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return nil, fmt.Errorf("creating database pool: %w", err)
	}

	pingCtx, cancel := context.WithTimeout(ctx, cfg.ConnectTimeout)
	defer cancel()

	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("pinging database: %w", err)
	}
	return pool, nil
}

// configure applies cfg to the pool. statement_timeout and idle_in_transaction_session_timeout are a server-side cap, so a runaway query or abandoned transaction can't hold a connection after the client has given up.
func configure(poolCfg *pgxpool.Config, cfg config.Database) {
	poolCfg.MaxConns = cfg.MaxConns
	poolCfg.MinConns = cfg.MinConns
	poolCfg.MaxConnIdleTime = cfg.ConnMaxIdle
	poolCfg.HealthCheckPeriod = cfg.HealthCheckPeriod
	poolCfg.ConnConfig.ConnectTimeout = cfg.ConnectTimeout

	ms := strconv.FormatInt(cfg.StatementTimeout.Milliseconds(), 10)
	poolCfg.ConnConfig.RuntimeParams["statement_timeout"] = ms
	poolCfg.ConnConfig.RuntimeParams["idle_in_transaction_session_timeout"] = ms
}
