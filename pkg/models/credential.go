package models

import "time"

type Credential struct {
	ID           string    `json:"id"`
	Username     string    `json:"username"`
	Password     string    `json:"password"`
	Target       string    `json:"target"`
	Port         int       `json:"port"`
	Service      string    `json:"service"`
	Valid        bool      `json:"valid"`
	BotID        string    `json:"bot_id,omitempty"`
	DiscoveredAt time.Time `json:"discovered_at"`
}
