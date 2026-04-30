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
