package models

import "time"

// Command represents a task queued for a bot
type Command struct {
	ID         string     `json:"id"`
	BotID      string     `json:"bot_id"`
	Type       string     `json:"type"`
	Payload    string     `json:"payload"`
	Output     string     `json:"output"`
	Status     string     `json:"status"`
	CreatedAt  time.Time  `json:"created_at"`
	ExecutedAt *time.Time `json:"executed_at"`
}
