package models

import "time"

const (
	CmdTypeShell    = "shell"
	CmdTypeUpload   = "upload"
	CmdTypeDownload = "download"
	CmdTypeSleep    = "sleep"
	CmdTypePersist  = "persist"
	CmdTypeKill     = "kill"
	CmdTypeProxy    = "proxy"

	StatusPending   = "pending"
	StatusExecuting = "executing"
	StatusCompleted = "completed"
	StatusFailed    = "failed"
)

type Command struct {
	ID         string     `json:"id"`
	BotID      string     `json:"bot_id"`
	CampaignID string     `json:"campaign_id,omitempty"`
	Type       string     `json:"type"`
	Payload    string     `json:"payload"`
	Output     string     `json:"output"`
	Status     string     `json:"status"`
	Timeout    int        `json:"timeout,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
	ExecutedAt *time.Time `json:"executed_at,omitempty"`
}
