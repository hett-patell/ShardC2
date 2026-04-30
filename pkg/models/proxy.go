package models

import "time"

type Proxy struct {
	ID         string    `json:"id"`
	BotID      string    `json:"bot_id"`
	Type       string    `json:"type"`
	ListenPort int       `json:"listen_port"`
	Status     string    `json:"status"`
	CreatedAt  time.Time `json:"created_at"`
}
