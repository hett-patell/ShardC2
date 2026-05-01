package database_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/shardc2/shardc2/internal/testutil"
	"github.com/shardc2/shardc2/pkg/models"
)

func TestAuditEventsMigrationIsIdempotent(t *testing.T) {
	db := testutil.TestDB(t)

	var event models.AuditEvent
	if event.Outcome != "" {
		t.Fatalf("unexpected zero value outcome: %q", event.Outcome)
	}

	if err := db.RunMigrations(filepath.Join(repoRoot(t), "migrations")); err != nil {
		t.Fatalf("run migrations second time: %v", err)
	}

	var exists bool
	if err := db.QueryRow(`SELECT EXISTS (
		SELECT 1 FROM information_schema.tables
		WHERE table_name = 'audit_events'
	)`).Scan(&exists); err != nil {
		t.Fatalf("query audit_events table: %v", err)
	}
	if !exists {
		t.Fatal("audit_events table does not exist")
	}

	var id string
	if err := db.QueryRow(`
		INSERT INTO audit_events (operator_username, operator_role, source_ip, action, object_type, object_id, outcome, details)
		VALUES ('admin', 'admin', '127.0.0.1', 'test.action', 'test', '123', 'success', '{"safe":true}')
		RETURNING id`).Scan(&id); err != nil {
		t.Fatalf("insert audit event: %v", err)
	}
	if id == "" {
		t.Fatal("audit event id is empty")
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := filepath.Abs(".")
	if err != nil {
		t.Fatalf("resolve cwd: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			matches, _ := filepath.Glob(filepath.Join(dir, "migrations", "*.sql"))
			if len(matches) > 0 {
				return dir
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("repo root not found from %s", dir)
		}
		dir = parent
	}
}
