package audit

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/shardc2/shardc2/internal/database"
)

const (
	OutcomeSuccess = "success"
	OutcomeDenied  = "denied"
	OutcomeFailure = "failure"
)

type Details string

func (d Details) String() string { return string(d) }

type Event struct {
	OperatorID       string
	OperatorUsername string
	OperatorRole     string
	SourceIP         string
	Action           string
	ObjectType       string
	ObjectID         string
	Outcome          string
	Details          Details
}

type Recorder struct {
	db *database.DB
}

func NewRecorder(db *database.DB) *Recorder {
	return &Recorder{db: db}
}

func (r *Recorder) Record(c *fiber.Ctx, event Event) error {
	if strings.TrimSpace(event.Action) == "" {
		return errors.New("audit action is required")
	}
	if event.Outcome == "" {
		event.Outcome = OutcomeSuccess
	}
	if event.Details == "" {
		event.Details = SanitizeDetails(fiber.Map{})
	}
	if c != nil {
		if event.SourceIP == "" {
			event.SourceIP = c.IP()
		}
		if event.OperatorID == "" {
			event.OperatorID, _ = c.Locals("operator_id").(string)
		}
		if event.OperatorUsername == "" {
			event.OperatorUsername, _ = c.Locals("operator_username").(string)
		}
		if event.OperatorUsername == "" {
			event.OperatorUsername, _ = c.Locals("operator_user").(string)
		}
		if event.OperatorRole == "" {
			event.OperatorRole, _ = c.Locals("operator_role").(string)
		}
	}
	if r == nil || r.db == nil {
		return errors.New("audit recorder has no database")
	}

	_, err := r.db.Exec(`
		INSERT INTO audit_events (operator_id, operator_username, operator_role, source_ip, action, object_type, object_id, outcome, details)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		nullIfEmpty(event.OperatorID), nullIfEmpty(event.OperatorUsername), nullIfEmpty(event.OperatorRole), nullIfEmpty(event.SourceIP),
		event.Action, nullIfEmpty(event.ObjectType), nullIfEmpty(event.ObjectID), event.Outcome, string(event.Details),
	)
	if err != nil {
		return fmt.Errorf("insert audit event: %w", err)
	}
	return nil
}

func SanitizeDetails(details fiber.Map) Details {
	sanitized := fiber.Map{}
	for key, value := range details {
		if isSensitiveKey(key) {
			sanitized[key] = "[REDACTED]"
			continue
		}
		sanitized[key] = value
	}
	encoded, err := json.Marshal(sanitized)
	if err != nil {
		return Details(`{}`)
	}
	return Details(encoded)
}

func isSensitiveKey(key string) bool {
	normalized := strings.ToLower(key)
	return strings.Contains(normalized, "token") || strings.Contains(normalized, "password") || strings.Contains(normalized, "payload_key") || strings.Contains(normalized, "secret")
}

func nullIfEmpty(value string) interface{} {
	if value == "" {
		return sql.NullString{}
	}
	return value
}
