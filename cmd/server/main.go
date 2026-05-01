package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/shardc2/shardc2/internal/database"
	"github.com/shardc2/shardc2/internal/server"
	"github.com/shardc2/shardc2/internal/server/middleware"
	"github.com/shardc2/shardc2/pkg/crypto"
	"github.com/shardc2/shardc2/pkg/policy"
	"github.com/shardc2/shardc2/pkg/profiles"
)

const banner = `
  ____  _                   _  ____ ____
 / ___|| |__   __ _ _ __ __| |/ ___|___ \
 \___ \| '_ \ / _' | '__/ _' | |     __) |
  ___) | | | | (_| | | | (_| | |___ / __/
 |____/|_| |_|\__,_|_|  \__,_|\____|_____|

 Command & Control Framework v0.2.0
`

func main() {
	var (
		addr          = flag.String("addr", ":8443", "Server listen address")
		dbConn        = flag.String("db", envOrDefault("SHARDC2_DB", "postgres://shardc2:shardc2_secret@localhost:5432/shardc2?sslmode=disable"), "Database connection string")
		migrate       = flag.Bool("migrate", false, "Run database migrations on startup")
		operatorToken = flag.String("operator-token", os.Getenv("SHARDC2_OPERATOR_TOKEN"), "Operator authentication token")
		implantKey    = flag.String("implant-key", os.Getenv("SHARDC2_IMPLANT_KEY"), "Agent implant authentication key")
		c2URL         = flag.String("c2-url", os.Getenv("SHARDC2_C2_URL"), "External C2 URL for agent auto-deployment (e.g. http://10.0.0.5:8443)")
		payloadKey    = flag.String("payload-key", os.Getenv("SHARDC2_PAYLOAD_KEY"), "Payload encryption key (hex, 32 bytes)")
		tlsCert       = flag.String("tls-cert", "", "TLS certificate file")
		tlsKey        = flag.String("tls-key", "", "TLS private key file")
		generateCert  = flag.Bool("generate-cert", false, "Generate self-signed TLS certificate and exit")
		profileName   = flag.String("profile", "default", "Malleable C2 profile (default, cloudfront, wordpress, or path to JSON)")
		jwtSecret     = flag.String("jwt-secret", os.Getenv("SHARDC2_JWT_SECRET"), "JWT signing secret for operator auth")
		policyFile    = flag.String("policy-file", os.Getenv("SHARDC2_POLICY_FILE"), "Safety policy JSON file")
	)
	flag.Parse()

	fmt.Print(banner)
	fmt.Printf("[*] PID: %d\n", os.Getpid())

	if *generateCert {
		if err := server.GenerateSelfSignedCert("server.crt", "server.key"); err != nil {
			log.Fatalf("[-] Certificate generation failed: %v", err)
		}
		fmt.Println("[+] Generated server.crt and server.key")
		return
	}

	derivedOperatorToken, jwtSecretBytes, generatedOperatorToken, err := deriveServerSecrets(*operatorToken, *jwtSecret, os.Stdout)
	if err != nil {
		log.Fatalf("[-] Secret configuration failed: %v", err)
	}
	*operatorToken = derivedOperatorToken
	if generatedOperatorToken {
		fmt.Println("[!] No operator token set. Generated a temporary operator token for this process.")
		fmt.Println("[!] Set SHARDC2_OPERATOR_TOKEN or --operator-token to persist a known value")
	}

	if *implantKey == "" {
		key, err := middleware.GenerateToken()
		if err != nil {
			log.Fatalf("[-] Failed to generate implant key: %v", err)
		}
		*implantKey = key
		fmt.Printf("[!] No implant key set. Generated: %s\n", key)
		fmt.Println("[!] Set SHARDC2_IMPLANT_KEY or --implant-key to persist this")
	}

	db, err := database.New(*dbConn)
	if err != nil {
		log.Fatalf("[-] Database connection failed: %v", err)
	}
	defer db.Close()
	fmt.Println("[+] Database connected")

	if *migrate {
		if err := db.RunMigrations("migrations"); err != nil {
			log.Fatalf("[-] Migration failed: %v", err)
		}
		fmt.Println("[+] Migrations applied")
	}

	if *c2URL != "" {
		fmt.Printf("[+] C2 URL for auto-deploy: %s\n", *c2URL)
	}

	var payloadKeyBytes []byte
	if *payloadKey != "" {
		var err error
		payloadKeyBytes, err = crypto.ParseHexKey(*payloadKey)
		if err != nil {
			payloadKeyBytes = crypto.DeriveKey(*payloadKey)
		}
		fmt.Println("[+] Payload encryption enabled")
	}

	profile, err := profiles.Load(*profileName)
	if err != nil {
		log.Fatalf("[-] Failed to load profile: %v", err)
	}
	if *profileName != "default" {
		fmt.Printf("[+] Malleable profile: %s\n", profile.Name)
	}
	safetyPolicy, err := loadServerPolicy(*policyFile)
	if err != nil {
		log.Fatalf("[-] Failed to load safety policy: %v", err)
	}
	if safetyPolicy.SafeMode {
		fmt.Println("[+] Safety policy: safe mode enabled")
	}

	cfg := server.ServerConfig{
		OperatorToken: *operatorToken,
		ImplantKey:    *implantKey,
		PayloadKey:    payloadKeyBytes,
		C2URL:         *c2URL,
		Profile:       profile,
		JWTSecret:     jwtSecretBytes,
		Policy:        safetyPolicy,
	}
	srv := server.New(db, cfg)

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-quit
		fmt.Println("\n[*] Shutting down...")
		srv.Shutdown()
	}()

	if *tlsCert != "" && *tlsKey != "" {
		if err := srv.StartTLS(*addr, *tlsCert, *tlsKey); err != nil {
			log.Fatalf("[-] Server error: %v", err)
		}
	} else {
		if err := srv.Start(*addr); err != nil {
			log.Fatalf("[-] Server error: %v", err)
		}
	}
}

func deriveServerSecrets(operatorTokenFlag, jwtSecretFlag string, notice io.Writer) (string, []byte, bool, error) {
	operatorToken := operatorTokenFlag
	generatedOperatorToken := false
	if operatorToken == "" {
		token, err := middleware.GenerateToken()
		if err != nil {
			return "", nil, false, fmt.Errorf("generate operator token: %w", err)
		}
		operatorToken = token
		generatedOperatorToken = true
	}

	jwtSecret := jwtSecretFlag
	if jwtSecret == "" {
		secret, err := middleware.GenerateToken()
		if err != nil {
			return "", nil, false, fmt.Errorf("generate jwt secret: %w", err)
		}
		jwtSecret = secret
		if notice != nil {
			fmt.Fprintln(notice, "[!] No JWT secret set. Generated a temporary JWT signing secret for this process.")
			fmt.Fprintln(notice, "[!] Set SHARDC2_JWT_SECRET or --jwt-secret to persist a known value")
		}
	}

	if operatorToken == jwtSecret {
		return "", nil, false, errors.New("operator token and JWT secret must be different")
	}

	return operatorToken, []byte(jwtSecret), generatedOperatorToken, nil
}

func loadServerPolicy(path string) (policy.Policy, error) {
	return policy.LoadFile(path)
}

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
