package server

import (
	"net/http"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/shardc2/shardc2/internal/database"
	"github.com/shardc2/shardc2/internal/testutil"
)

func TestAgentBinaryRequiresOperatorAuth(t *testing.T) {
	jwtSecret := []byte("jwt-secret")
	srv := New(nil, ServerConfig{
		ImplantKey: "implant-key",
		JWTSecret:  jwtSecret,
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
		"Authorization": "Bearer " + signedOperatorJWT(t, jwtSecret, "operator-1", "admin"),
	})
	if valid.StatusCode == http.StatusUnauthorized || valid.StatusCode == http.StatusForbidden {
		t.Fatalf("valid operator token was rejected with status %d", valid.StatusCode)
	}
}

func TestBootstrapTokenOnlyCreatesInitialAdmin(t *testing.T) {
	db := testutil.TestDB(t)
	cleanupServerAuthTestData(t, db)

	srv := New(db, ServerConfig{
		BootstrapToken: "bootstrap-token",
		ImplantKey:     "implant-key",
		JWTSecret:      []byte("jwt-secret"),
	})
	t.Cleanup(func() { _ = srv.Shutdown() })

	stats := testutil.JSONRequest(t, srv.app, http.MethodGet, "/api/v1/stats", nil, map[string]string{
		"Authorization": "Bearer bootstrap-token",
	})
	if stats.StatusCode != http.StatusForbidden {
		t.Fatalf("bootstrap token accessed non-bootstrap route: got %d, want %d", stats.StatusCode, http.StatusForbidden)
	}

	created := testutil.JSONRequest(t, srv.app, http.MethodPost, "/api/v1/operators", map[string]string{
		"username": "first-admin",
		"password": "strong-password",
		"role":     "admin",
	}, map[string]string{"Authorization": "Bearer bootstrap-token"})
	if created.StatusCode != http.StatusCreated {
		t.Fatalf("bootstrap admin create status: got %d, want %d", created.StatusCode, http.StatusCreated)
	}

	second := testutil.JSONRequest(t, srv.app, http.MethodPost, "/api/v1/operators", map[string]string{
		"username": "second-admin",
		"password": "strong-password",
		"role":     "admin",
	}, map[string]string{"Authorization": "Bearer bootstrap-token"})
	if second.StatusCode != http.StatusUnauthorized {
		t.Fatalf("bootstrap token after active admin: got %d, want %d", second.StatusCode, http.StatusUnauthorized)
	}
}

func TestJWTRoleClaimsStillEnforceAdminRoutes(t *testing.T) {
	jwtSecret := []byte("jwt-secret")
	srv := New(nil, ServerConfig{ImplantKey: "implant-key", JWTSecret: jwtSecret})
	t.Cleanup(func() { _ = srv.Shutdown() })

	viewer := testutil.JSONRequest(t, srv.app, http.MethodGet, "/api/v1/operators", nil, map[string]string{
		"Authorization": "Bearer " + signedOperatorJWT(t, jwtSecret, "viewer-1", "viewer"),
	})
	if viewer.StatusCode != http.StatusForbidden {
		t.Fatalf("viewer admin route status: got %d, want %d", viewer.StatusCode, http.StatusForbidden)
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

func signedOperatorJWT(t *testing.T, secret []byte, subject string, role string) string {
	t.Helper()
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub":  subject,
		"usr":  subject,
		"role": role,
		"exp":  time.Now().Add(time.Hour).Unix(),
		"iat":  time.Now().Unix(),
	})
	signed, err := token.SignedString(secret)
	if err != nil {
		t.Fatalf("sign jwt: %v", err)
	}
	return signed
}

func cleanupServerAuthTestData(t *testing.T, db *database.DB) {
	t.Helper()
	if _, err := db.Exec(`DELETE FROM audit_events`); err != nil {
		t.Fatalf("cleanup audit_events: %v", err)
	}
	if _, err := db.Exec(`DELETE FROM operators WHERE username IN ('first-admin', 'second-admin')`); err != nil {
		t.Fatalf("cleanup operators: %v", err)
	}
}
