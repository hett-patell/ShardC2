package models

import "time"

type Credential struct {
	Username     string    `json:"username"`
	Password     string    `json:"password"`
	Target       string    `json:"target"`
	Port         int       `json:"port"`
	Valid        bool      `json:"valid"`
	BotID        string    `json:"bot_id,omitempty"`
	DiscoveredAt time.Time `json:"discovered_at"`
}
