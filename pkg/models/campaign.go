package models

import "time"

const (
	CampaignTypeRecon   = "recon"
	CampaignTypeBrute   = "brute"
	CampaignTypeExfil   = "exfil"
	CampaignTypePersist = "persist"
	CampaignTypeCustom  = "custom"

	CampaignStatusCreated   = "created"
	CampaignStatusRunning   = "running"
	CampaignStatusPaused    = "paused"
	CampaignStatusCompleted = "completed"
	CampaignStatusFailed    = "failed"
)

type Campaign struct {
	ID             string    `json:"id"`
	Name           string    `json:"name"`
	Description    string    `json:"description"`
	Type           string    `json:"type"`
	Status         string    `json:"status"`
	Config         string    `json:"config"`
	TotalTasks     int       `json:"total_tasks"`
	CompletedTasks int       `json:"completed_tasks"`
	FailedTasks    int       `json:"failed_tasks"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type CampaignBot struct {
	CampaignID string    `json:"campaign_id"`
	BotID      string    `json:"bot_id"`
	AssignedAt time.Time `json:"assigned_at"`
}

type CampaignTask struct {
	ID          string     `json:"id"`
	CampaignID  string     `json:"campaign_id"`
	BotID       string     `json:"bot_id"`
	CommandID   string     `json:"command_id"`
	TaskName    string     `json:"task_name"`
	Status      string     `json:"status"`
	Output      string     `json:"output"`
	CreatedAt   time.Time  `json:"created_at"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
}
