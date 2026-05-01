package middleware

import (
	"bytes"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
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
