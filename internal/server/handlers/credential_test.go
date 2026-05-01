package handlers

import (
	"net/http"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/shardc2/shardc2/internal/database"
	"github.com/shardc2/shardc2/internal/testutil"
)

func TestCredentialListMasksPasswordsByDefault(t *testing.T) {
	db := testutil.TestDB(t)
	cleanupCredentialTestData(t, db)

	db.Exec(`INSERT INTO credentials (username, password, target, port, service, valid) VALUES ('admin', 'secret-pass', '10.0.0.1', 22, 'ssh', true)`)

	app := fiber.New()
	h := NewCredentialHandler(db)
	app.Get("/credentials", h.List)

	resp := testutil.JSONRequest(t, app, http.MethodGet, "/credentials", nil, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status: got %d, want %d", resp.StatusCode, http.StatusOK)
	}

	body := testutil.DecodeJSON[map[string]interface{}](t, resp)
	creds, ok := body["credentials"].([]interface{})
	if !ok || len(creds) == 0 {
		t.Fatal("expected at least one credential")
	}
	first := creds[0].(map[string]interface{})
	pw, _ := first["password"].(string)
	if pw == "secret-pass" {
		t.Fatal("credential list returned plaintext password")
	}
	if pw != "********" {
		t.Fatalf("expected masked password '********', got %q", pw)
	}
}

func TestCredentialRevealReturnsPlaintextPassword(t *testing.T) {
	db := testutil.TestDB(t)
	cleanupCredentialTestData(t, db)

	var credID string
	db.QueryRow(`INSERT INTO credentials (username, password, target, port, service, valid) VALUES ('admin', 'reveal-pass', '10.0.0.1', 22, 'ssh', true) RETURNING id`).Scan(&credID)

	app := fiber.New()
	h := NewCredentialHandler(db)
	app.Get("/credentials/:id/reveal", h.Reveal)

	resp := testutil.JSONRequest(t, app, http.MethodGet, "/credentials/"+credID+"/reveal", nil, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status: got %d, want %d", resp.StatusCode, http.StatusOK)
	}

	body := testutil.DecodeJSON[map[string]interface{}](t, resp)
	pw, _ := body["password"].(string)
	if pw != "reveal-pass" {
		t.Fatalf("expected plaintext password 'reveal-pass', got %q", pw)
	}
}

func cleanupCredentialTestData(t *testing.T, db *database.DB) {
	t.Helper()
	db.Exec(`DELETE FROM credentials WHERE target = '10.0.0.1'`)
}
