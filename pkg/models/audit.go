package models

import "time"

type AuditEvent struct {
	ID               string    `json:"id"`
	OperatorID       string    `json:"operator_id,omitempty"`
	OperatorUsername string    `json:"operator_username,omitempty"`
	OperatorRole     string    `json:"operator_role,omitempty"`
	SourceIP         string    `json:"source_ip,omitempty"`
	Action           string    `json:"action"`
	ObjectType       string    `json:"object_type,omitempty"`
	ObjectID         string    `json:"object_id,omitempty"`
	Outcome          string    `json:"outcome"`
	Details          string    `json:"details,omitempty"`
	CreatedAt        time.Time `json:"created_at"`
}
