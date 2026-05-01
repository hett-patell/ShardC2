package server

import (
	"net/http"
	"strings"
	"testing"

	"github.com/shardc2/shardc2/internal/database"
	"github.com/shardc2/shardc2/internal/testutil"
	"github.com/shardc2/shardc2/pkg/models"
	"github.com/shardc2/shardc2/pkg/policy"
)

func TestOperatorCommandCreateWritesAuditEvent(t *testing.T) {
	db := testutil.TestDB(t)
	cleanupAuditIntegrationData(t, db)
	jwtSecret := []byte("jwt-secret")

	var botID string
	if err := db.QueryRow(`
		INSERT INTO bots (hostname, ip_address, os, architecture, username, status)
		VALUES ('audit-bot', '127.0.0.1', 'linux', 'amd64', 'tester', 'active') RETURNING id`).Scan(&botID); err != nil {
		t.Fatalf("insert bot: %v", err)
	}

	srv := New(db, ServerConfig{ImplantKey: "implant", JWTSecret: jwtSecret, Policy: policy.Default()})
	t.Cleanup(func() { _ = srv.Shutdown() })

	resp := testutil.JSONRequest(t, srv.app, http.MethodPost, "/api/v1/commands", map[string]interface{}{
		"bot_id":  botID,
		"type":    models.CmdTypeShell,
		"payload": "whoami",
	}, map[string]string{"Authorization": "Bearer " + signedOperatorJWT(t, jwtSecret, "11111111-1111-1111-1111-111111111111", "admin")})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("command create status: got %d, want %d", resp.StatusCode, http.StatusCreated)
	}

	assertAuditEvent(t, db, "command.create", "success")
}

func TestOperatorAuthFailureWritesAuditEventWithoutToken(t *testing.T) {
	db := testutil.TestDB(t)
	cleanupAuditIntegrationData(t, db)

	srv := New(db, ServerConfig{OperatorToken: "operator-token", ImplantKey: "implant", JWTSecret: []byte("jwt-secret"), Policy: policy.Default()})
	t.Cleanup(func() { _ = srv.Shutdown() })

	resp := testutil.JSONRequest(t, srv.app, http.MethodGet, "/api/v1/credentials", nil, map[string]string{"Authorization": "Bearer leaked-token-value"})
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("auth failure status: got %d, want %d", resp.StatusCode, http.StatusUnauthorized)
	}

	var details string
	if err := db.QueryRow(`SELECT details::text FROM audit_events WHERE action = 'auth.denied' ORDER BY created_at DESC LIMIT 1`).Scan(&details); err != nil {
		t.Fatalf("query denied audit event: %v", err)
	}
	if strings.Contains(details, "leaked-token-value") {
		t.Fatalf("audit details leaked token: %s", details)
	}
}

func TestCredentialListWritesAuditEvent(t *testing.T) {
	db := testutil.TestDB(t)
	cleanupAuditIntegrationData(t, db)
	jwtSecret := []byte("jwt-secret")

	srv := New(db, ServerConfig{ImplantKey: "implant", JWTSecret: jwtSecret, Policy: policy.Default()})
	t.Cleanup(func() { _ = srv.Shutdown() })

	resp := testutil.JSONRequest(t, srv.app, http.MethodGet, "/api/v1/credentials", nil, map[string]string{"Authorization": "Bearer " + signedOperatorJWT(t, jwtSecret, "11111111-1111-1111-1111-111111111111", "admin")})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("credential list status: got %d, want %d", resp.StatusCode, http.StatusOK)
	}

	assertAuditEvent(t, db, "credential.list", "success")
}

func cleanupAuditIntegrationData(t *testing.T, db *database.DB) {
	t.Helper()
	for _, query := range []string{
		`DELETE FROM audit_events`,
		`DELETE FROM commands`,
		`DELETE FROM bots WHERE hostname = 'audit-bot'`,
	} {
		if _, err := db.Exec(query); err != nil {
			t.Fatalf("cleanup query %q: %v", query, err)
		}
	}
}

func assertAuditEvent(t *testing.T, db *database.DB, action string, outcome string) {
	t.Helper()
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM audit_events WHERE action = $1 AND outcome = $2`, action, outcome).Scan(&count); err != nil {
		t.Fatalf("query audit event %s/%s: %v", action, outcome, err)
	}
	if count == 0 {
		t.Fatalf("expected audit event %s/%s", action, outcome)
	}
}
