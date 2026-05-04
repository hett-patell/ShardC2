package models

import "time"

type Credential struct {
	ID           string    `json:"id"`
	Username     string    `json:"username"`
	Password     string    `json:"password"`
	Target       string    `json:"target"`
	Port         int       `json:"port"`
	Service      string    `json:"service"`
	Category     string    `json:"category"`
	Valid        bool      `json:"valid"`
	BotID        string    `json:"bot_id,omitempty"`
	CampaignID   string    `json:"campaign_id,omitempty"`
	SourcePath   string    `json:"source_path,omitempty"`
	DiscoveredAt time.Time `json:"discovered_at"`
}

const (
	CredCategoryLogin        = "login"
	CredCategorySSHKey       = "ssh_key"
	CredCategoryAPIKey       = "api_key"
	CredCategoryEnvSecret    = "env_secret"
	CredCategoryDBConnection = "db_connection"
	CredCategoryCloudToken   = "cloud_token"
	CredCategoryShellHistory = "shell_history"
	CredCategoryMisc         = "misc"
)
