package client

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHealthSendsNoAuth(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/health" {
			w.WriteHeader(404)
			return
		}
		json.NewEncoder(w).Encode(HealthResponse{Status: "ok", Service: "shardc2"})
	}))
	defer srv.Close()

	c := New(srv.URL, "")
	resp, err := c.Health()
	if err != nil {
		t.Fatalf("health: %v", err)
	}
	if resp.Status != "ok" || resp.Service != "shardc2" {
		t.Fatalf("unexpected: %+v", resp)
	}
}

func TestStatsSendsAuthHeader(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-token" {
			w.WriteHeader(401)
			return
		}
		json.NewEncoder(w).Encode(StatsResponse{TotalBots: 5, ActiveBots: 3})
	}))
	defer srv.Close()

	c := New(srv.URL, "test-token")
	resp, err := c.Stats()
	if err != nil {
		t.Fatalf("stats: %v", err)
	}
	if resp.TotalBots != 5 || resp.ActiveBots != 3 {
		t.Fatalf("unexpected: %+v", resp)
	}
}

func TestListBotsReturnsSlice(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok" {
			w.WriteHeader(401)
			return
		}
		json.NewEncoder(w).Encode(BotsResponse{Bots: []Bot{{ID: "b1", Hostname: "host1"}}})
	}))
	defer srv.Close()

	c := New(srv.URL, "tok")
	bots, err := c.ListBots()
	if err != nil {
		t.Fatalf("list bots: %v", err)
	}
	if len(bots) != 1 || bots[0].ID != "b1" {
		t.Fatalf("unexpected: %+v", bots)
	}
}

func TestValidateCampaignPostsJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(405)
			return
		}
		var req ValidateRequest
		json.NewDecoder(r.Body).Decode(&req)
		if req.Type != "brute" {
			w.WriteHeader(400)
			return
		}
		json.NewEncoder(w).Encode(ValidateResponse{TotalTargets: 1, BlockedTargets: 0, CanLaunch: true})
	}))
	defer srv.Close()

	c := New(srv.URL, "tok")
	resp, err := c.ValidateCampaign(ValidateRequest{Type: "brute", Config: `{"targets":["127.0.0.1"]}`})
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	if !resp.CanLaunch || resp.TotalTargets != 1 {
		t.Fatalf("unexpected: %+v", resp)
	}
}

func TestAPIErrorReturnsStatusAndBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(403)
		w.Write([]byte(`{"error":"forbidden"}`))
	}))
	defer srv.Close()

	c := New(srv.URL, "bad")
	_, err := c.Stats()
	if err == nil {
		t.Fatal("expected error for 403")
	}
}
