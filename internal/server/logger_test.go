package server

import (
	"bytes"
	"strings"
	"testing"
)

func TestStructuredLogging(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(&buf, "INFO")

	logger.Info("beacon", "Bot connected", map[string]interface{}{"bot_id": "123"})

	output := buf.String()
	if !strings.Contains(output, `"level":"info"`) {
		t.Errorf("Expected JSON log with info level, got: %s", output)
	}
	if !strings.Contains(output, `"category":"beacon"`) {
		t.Errorf("Expected category beacon, got: %s", output)
	}
}

func TestShouldLog(t *testing.T) {
	logger := NewLogger(nil, "INFO")

	if !logger.shouldLog("info") {
		t.Error("Expected info to be logged when level is INFO")
	}
	if logger.shouldLog("debug") {
		t.Error("Expected debug not to be logged when level is INFO")
	}
	if !logger.shouldLog("warn") {
		t.Error("Expected warn to be logged when level is INFO")
	}
	if !logger.shouldLog("error") {
		t.Error("Expected error to be logged when level is INFO")
	}
}
