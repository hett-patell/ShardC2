package middleware

import (
	"bytes"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/shardc2/shardc2/internal/database"
	"github.com/shardc2/shardc2/internal/testutil"
	"github.com/shardc2/shardc2/pkg/crypto"
)

func TestPayloadCryptoAllowsUnsignedRequestsWithoutKey(t *testing.T) {
	app := fiber.New()
	app.Post("/agent", PayloadCrypto(nil), func(c *fiber.Ctx) error {
		return c.SendStatus(http.StatusNoContent)
	})

	req, err := http.NewRequest(http.MethodPost, "/agent", nil)
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("execute request: %v", err)
	}
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("status: got %d, want %d", resp.StatusCode, http.StatusNoContent)
	}
}

func TestPayloadCryptoRequiresSignatureWhenKeySet(t *testing.T) {
	key := crypto.DeriveKey("phase1-hmac-test")

	tests := []struct {
		name    string
		headers map[string]string
	}{
		{name: "missing signature", headers: map[string]string{"X-Timestamp": "123"}},
		{name: "missing timestamp", headers: map[string]string{"X-Signature": "bad"}},
		{name: "missing both", headers: nil},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			app := fiber.New()
			app.Post("/agent", PayloadCrypto(key), func(c *fiber.Ctx) error {
				return c.SendStatus(http.StatusNoContent)
			})

			req, err := http.NewRequest(http.MethodPost, "/agent", nil)
			if err != nil {
				t.Fatalf("create request: %v", err)
			}
			for k, v := range tc.headers {
				req.Header.Set(k, v)
			}
			resp, err := app.Test(req, -1)
			if err != nil {
				t.Fatalf("execute request: %v", err)
			}
			if resp.StatusCode != http.StatusForbidden {
				t.Fatalf("status: got %d, want %d", resp.StatusCode, http.StatusForbidden)
			}
		})
	}
}

func TestPayloadCryptoAcceptsValidSignatureWhenKeySet(t *testing.T) {
	key := crypto.DeriveKey("phase1-valid-hmac-test")
	body := []byte(`{"status":"ok"}`)
	ts := time.Now().Unix()
	sig := crypto.Sign(http.MethodPost, "/agent", body, key, ts)

	app := fiber.New()
	app.Post("/agent", PayloadCrypto(key), func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"ok": true})
	})

	req, err := http.NewRequest(http.MethodPost, "/agent", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	req.Header.Set("X-Signature", sig)
	req.Header.Set("X-Timestamp", strconv.FormatInt(ts, 10))

	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("execute request: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status: got %d, want %d", resp.StatusCode, http.StatusOK)
	}
}

func TestAgentAuthExpiredTokenReturns403(t *testing.T) {
	db := testutil.TestDB(t)
	cleanupAgentAuthTestData(t, db)

	var botID string
	db.QueryRow(`INSERT INTO bots (hostname, ip_address, os, architecture, username, status) VALUES ('expiry-test', '127.0.0.1', 'linux', 'amd64', 'tester', 'active') RETURNING id`).Scan(&botID)
	expired := time.Now().Add(-1 * time.Hour)
	db.Exec(`INSERT INTO bot_tokens (bot_id, token, expires_at) VALUES ($1, 'expired-token-123', $2)`, botID, expired)

	app := fiber.New()
	app.Get("/test", AgentAuth(db), func(c *fiber.Ctx) error {
		return c.SendStatus(http.StatusOK)
	})

	req, _ := http.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("X-Session-Token", "expired-token-123")
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("execute request: %v", err)
	}
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("expired token status: got %d, want %d", resp.StatusCode, http.StatusForbidden)
	}
}

func TestAgentAuthFreshTokenPasses(t *testing.T) {
	db := testutil.TestDB(t)
	cleanupAgentAuthTestData(t, db)

	var botID string
	db.QueryRow(`INSERT INTO bots (hostname, ip_address, os, architecture, username, status) VALUES ('fresh-test', '127.0.0.1', 'linux', 'amd64', 'tester', 'active') RETURNING id`).Scan(&botID)
	future := time.Now().Add(24 * time.Hour)
	db.Exec(`INSERT INTO bot_tokens (bot_id, token, expires_at) VALUES ($1, 'fresh-token-123', $2)`, botID, future)

	app := fiber.New()
	app.Get("/test", AgentAuth(db), func(c *fiber.Ctx) error {
		return c.SendStatus(http.StatusOK)
	})

	req, _ := http.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("X-Session-Token", "fresh-token-123")
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("execute request: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("fresh token status: got %d, want %d", resp.StatusCode, http.StatusOK)
	}
}

func cleanupAgentAuthTestData(t *testing.T, db *database.DB) {
	t.Helper()
	db.Exec(`DELETE FROM bot_tokens WHERE token IN ('expired-token-123', 'fresh-token-123')`)
	db.Exec(`DELETE FROM bots WHERE hostname IN ('expiry-test', 'fresh-test')`)
}
