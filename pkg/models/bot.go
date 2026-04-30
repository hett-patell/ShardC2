package models

import "time"

type Bot struct {
	ID             string    `json:"id"`
	Hostname       string    `json:"hostname"`
	IPAddress      string    `json:"ip_address"`
	ExternalIP     string    `json:"external_ip,omitempty"`
	OS             string    `json:"os"`
	Architecture   string    `json:"architecture"`
	Username       string    `json:"username"`
	Privileged     bool      `json:"privileged"`
	LastSeen       time.Time `json:"last_seen"`
	Status         string    `json:"status"`
	BeaconInterval int       `json:"beacon_interval"`
	CreatedAt      time.Time `json:"created_at"`
}
