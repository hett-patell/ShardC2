package models

import "testing"

func TestAgentModeConstants(t *testing.T) {
	if AgentModeBeacon != "beacon" {
		t.Fatalf("beacon mode: got %q", AgentModeBeacon)
	}
	if AgentModeSession != "session" {
		t.Fatalf("session mode: got %q", AgentModeSession)
	}
}

func TestBotHasModeField(t *testing.T) {
	b := Bot{Mode: AgentModeBeacon}
	if b.Mode != "beacon" {
		t.Fatalf("bot mode: got %q", b.Mode)
	}
}
