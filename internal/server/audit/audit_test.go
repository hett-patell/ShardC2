package audit

import (
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
)

func TestEventDetailsRedactsSensitiveValues(t *testing.T) {
	details := SanitizeDetails(fiber.Map{
		"token":       "secret-token",
		"password":    "secret-password",
		"payload_key": "secret-payload-key",
		"command":     "whoami",
	})

	encoded := details.String()
	for _, secret := range []string{"secret-token", "secret-password", "secret-payload-key"} {
		if strings.Contains(encoded, secret) {
			t.Fatalf("details leaked sensitive value %q in %s", secret, encoded)
		}
	}
	if !strings.Contains(encoded, "whoami") {
		t.Fatalf("details removed non-sensitive value: %s", encoded)
	}
}

func TestRecorderRecordRejectsMissingAction(t *testing.T) {
	recorder := NewRecorder(nil)
	if err := recorder.Record(nil, Event{Outcome: OutcomeSuccess}); err == nil {
		t.Fatal("expected missing action to be rejected")
	}
}
