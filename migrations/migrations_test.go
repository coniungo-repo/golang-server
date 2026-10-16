package migrations

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func TestEmbedsEveryMigration(t *testing.T) {
	onDisk, err := filepath.Glob("*.sql")
	if err != nil {
		t.Fatal(err)
	}
	embedded, err := FS.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	if len(onDisk) == 0 || len(embedded) != len(onDisk) {
		t.Fatalf("embedded %d migrations, found %d on disk", len(embedded), len(onDisk))
	}
}

// TestUpIsIdempotent needs TEST_DATABASE_URL pointing at a migrated database; it checks that a second deploy finds nothing to apply.
func TestUpIsIdempotent(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	db, err := sql.Open("pgx", url)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	results, err := Up(context.Background(), db)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 0 {
		t.Fatalf("applied %d migrations to an up-to-date database", len(results))
	}
}
