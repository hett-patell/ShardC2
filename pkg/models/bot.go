package models

import "time"

// Bot represents a compromised host in the botnet
type Bot struct {
	ID           string    `json:"id"`
	Hostname     string    `json:"hostname"`
	IPAddress    string    `json:"ip_address"`
	OS           string    `json:"os"`
	Architecture string    `json:"architecture"`
	Username     string    `json:"username"`
	Privileged   bool      `json:"privileged"`
	LastSeen     time.Time `json:"last_seen"`
	Status       string    `json:"status"`
	CreatedAt    time.Time `json:"created_at"`
}
