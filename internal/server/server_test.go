package server

import (
	"net/http"
	"testing"
	"time"

	"github.com/shardc2/shardc2/internal/testutil"
)

func TestAgentBinaryRequiresOperatorAuth(t *testing.T) {
	srv := New(nil, ServerConfig{
		OperatorToken: "operator-token",
		ImplantKey:    "implant-key",
		JWTSecret:     []byte("jwt-secret"),
	})
	t.Cleanup(func() { _ = srv.Shutdown() })

	missing := testutil.JSONRequest(t, srv.app, http.MethodGet, "/api/v1/agent/binary", nil, nil)
	if missing.StatusCode != http.StatusUnauthorized {
		t.Fatalf("missing auth status: got %d, want %d", missing.StatusCode, http.StatusUnauthorized)
	}

	invalid := testutil.JSONRequest(t, srv.app, http.MethodGet, "/api/v1/agent/binary", nil, map[string]string{
		"Authorization": "Bearer wrong-token",
	})
	if invalid.StatusCode != http.StatusUnauthorized {
		t.Fatalf("invalid auth status: got %d, want %d", invalid.StatusCode, http.StatusUnauthorized)
	}

	valid := testutil.JSONRequest(t, srv.app, http.MethodGet, "/api/v1/agent/binary", nil, map[string]string{
		"Authorization": "Bearer operator-token",
	})
	if valid.StatusCode == http.StatusUnauthorized || valid.StatusCode == http.StatusForbidden {
		t.Fatalf("valid operator token was rejected with status %d", valid.StatusCode)
	}
}

func TestLoginRateLimit(t *testing.T) {
	srv := New(nil, ServerConfig{
		OperatorToken:        "operator-token",
		ImplantKey:           "implant-key",
		JWTSecret:            []byte("jwt-secret"),
		LoginRateLimitMax:    2,
		LoginRateLimitWindow: time.Minute,
	})
	t.Cleanup(func() { _ = srv.Shutdown() })

	for i := 0; i < 2; i++ {
		resp := testutil.JSONRequest(t, srv.app, http.MethodPost, "/api/v1/auth/login", map[string]string{
			"username": "missing",
			"password": "wrong-password",
		}, nil)
		if resp.StatusCode == http.StatusTooManyRequests {
			t.Fatalf("request %d was rate limited before threshold", i+1)
		}
	}

	limited := testutil.JSONRequest(t, srv.app, http.MethodPost, "/api/v1/auth/login", map[string]string{
		"username": "missing",
		"password": "wrong-password",
	}, nil)
	if limited.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("status: got %d, want %d", limited.StatusCode, http.StatusTooManyRequests)
	}
}
