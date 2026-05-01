package models

import "time"

const (
	BuildStatusPending   = "pending"
	BuildStatusBuilding  = "building"
	BuildStatusCompleted = "completed"
	BuildStatusFailed    = "failed"
)

type AgentBuild struct {
	ID                    string     `json:"id"`
	GOOS                  string     `json:"goos"`
	GOARCH                string     `json:"goarch"`
	Profile               string     `json:"profile"`
	PayloadKeyFingerprint string     `json:"payload_key_fingerprint,omitempty"`
	RequestedBy           string     `json:"requested_by,omitempty"`
	Status                string     `json:"status"`
	ArtifactPath          string     `json:"artifact_path,omitempty"`
	ErrorMessage          string     `json:"error_message,omitempty"`
	CreatedAt             time.Time  `json:"created_at"`
	CompletedAt           *time.Time `json:"completed_at,omitempty"`
}

var AllowedGOOS = map[string]bool{"linux": true, "darwin": true, "windows": true}
var AllowedGOARCH = map[string]bool{"amd64": true, "arm64": true, "arm": true}
