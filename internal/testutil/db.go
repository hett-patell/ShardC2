package testutil

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/shardc2/shardc2/internal/database"
)

func TestDB(t *testing.T) *database.DB {
	t.Helper()

	connStr := os.Getenv("SHARDC2_TEST_DB")
	if connStr == "" {
		t.Skip("SHARDC2_TEST_DB is not set")
	}

	db, err := database.New(connStr)
	if err != nil {
		t.Fatalf("connect test database: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	root := repoRoot(t)
	if err := db.RunMigrations(filepath.Join(root, "migrations")); err != nil {
		t.Fatalf("run migrations: %v", err)
	}

	return db
}

func repoRoot(t *testing.T) string {
	t.Helper()

	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}

	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			if _, err := os.Stat(filepath.Join(dir, "migrations")); err == nil {
				return dir
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("repo root with go.mod and migrations not found from %s", dir)
		}
		dir = parent
	}
}
