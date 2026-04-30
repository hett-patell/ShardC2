package agent

// Agent represents a ShardC2 bot agent
type Agent struct {
	ServerURL string
	BotID     string
}

// New creates a new agent instance
func New(serverURL string) *Agent {
	return &Agent{ServerURL: serverURL}
}
