package agent

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestBeacon(t *testing.T) {
	// Create a mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/beacon" && r.Method == "POST" {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"status": "ok", "pending_commands": 0}`))
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	agent := New(server.URL)
	agent.BotID = "test-bot"

	// Mock server response or check if beacon sends request
	err := agent.Beacon()
	if err != nil {
		t.Fatalf("Expected beacon to succeed, got error: %v", err)
	}
}
