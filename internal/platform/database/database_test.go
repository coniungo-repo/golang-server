package database

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/coniungo-repo/golang-server/internal/platform/config"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestConfigureSetsServerTimeouts(t *testing.T) {
	poolCfg, err := pgxpool.ParseConfig("postgres://u:p@localhost:5432/db?statement_timeout=0")
	if err != nil {
		t.Fatal(err)
	}
	configure(poolCfg, config.Database{MaxConns: 5, StatementTimeout: 1500 * time.Millisecond})
	for _, key := range []string{"statement_timeout", "idle_in_transaction_session_timeout"} {
		if got := poolCfg.ConnConfig.RuntimeParams[key]; got != "1500" {
			t.Errorf("%s = %q, want 1500", key, got)
		}
	}
}

// TestOpenAppliesStatementTimeout needs TEST_DATABASE_URL.
func TestOpenAppliesStatementTimeout(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	pool, err := Open(ctx, config.Database{
		URL: config.Secret(url), MaxConns: 2, ConnMaxIdle: time.Minute, ConnectTimeout: 5 * time.Second,
		HealthCheckPeriod: time.Minute, StatementTimeout: 200 * time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	var got string
	if err := pool.QueryRow(ctx, "SHOW statement_timeout").Scan(&got); err != nil || got != "200ms" {
		t.Fatalf("statement_timeout %q, err %v", got, err)
	}
	if _, err := pool.Exec(ctx, "SELECT pg_sleep(1)"); err == nil {
		t.Fatal("a statement longer than the timeout should be cancelled")
	}
}
