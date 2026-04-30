package models

import "time"

// Credential represents a username/password pair
type Credential struct {
	Username     string    `json:"username"`
	Password     string    `json:"password"`
	Target       string    `json:"target"`
	Port         int       `json:"port"`
	Valid        bool      `json:"valid"`
	DiscoveredAt time.Time `json:"discovered_at"`
}
