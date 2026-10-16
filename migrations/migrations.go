// Package migrations embeds the SQL migrations so the image can apply them without the source tree.
package migrations

import (
	"context"
	"database/sql"
	"embed"
	"fmt"

	"github.com/pressly/goose/v3"
	"github.com/pressly/goose/v3/lock"
)

//go:embed *.sql
var FS embed.FS

// Up applies every pending migration in order, holding a Postgres advisory lock so two deploys never migrate at once.
func Up(ctx context.Context, db *sql.DB) ([]*goose.MigrationResult, error) {
	locker, err := lock.NewPostgresSessionLocker()
	if err != nil {
		return nil, fmt.Errorf("migrations: creating lock: %w", err)
	}
	p, err := goose.NewProvider(goose.DialectPostgres, db, FS, goose.WithSessionLocker(locker))
	if err != nil {
		return nil, fmt.Errorf("migrations: loading: %w", err)
	}
	return p.Up(ctx)
}
