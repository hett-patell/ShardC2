package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/shardc2/shardc2/internal/database"
	"github.com/shardc2/shardc2/pkg/models"
	"github.com/shardc2/shardc2/pkg/policy"
)

type CampaignHandler struct {
	db     *database.DB
	policy policy.Policy
}

func NewCampaignHandler(db *database.DB, policies ...policy.Policy) *CampaignHandler {
	p := policy.Default()
	if len(policies) > 0 {
		p = policies[0]
	}
	return &CampaignHandler{db: db, policy: p}
}

type bruteCampaignConfig struct {
	Mode    string   `json:"mode"`
	Targets []string `json:"targets"`
}

func ValidateCampaignConfig(p policy.Policy, campaignType string, configJSON string) error {
	if configJSON == "" {
		configJSON = "{}"
	}

	if campaignType != models.CampaignTypeBrute {
		return nil
	}

	var cfg bruteCampaignConfig
	if err := json.Unmarshal([]byte(configJSON), &cfg); err != nil {
		return fmt.Errorf("invalid brute campaign config: %w", err)
	}
	if cfg.Mode == "external" && !p.AllowExternalBrute {
		return fmt.Errorf("external brute campaigns are disabled by policy")
	}
	for _, target := range cfg.Targets {
		if err := p.ValidateTarget(target); err != nil {
			return fmt.Errorf("target %q rejected by policy: %w", target, err)
		}
	}
	return nil
}

func (h *CampaignHandler) Create(c *fiber.Ctx) error {
	var req struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Type        string `json:"type"`
		Config      string `json:"config"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request body"})
	}
	if req.Name == "" || req.Type == "" {
		return c.Status(400).JSON(fiber.Map{"error": "name and type required"})
	}

	validTypes := map[string]bool{
		models.CampaignTypeRecon:   true,
		models.CampaignTypeBrute:   true,
		models.CampaignTypeExfil:   true,
		models.CampaignTypePersist: true,
		models.CampaignTypeCustom:  true,
	}
	if !validTypes[req.Type] {
		return c.Status(400).JSON(fiber.Map{"error": "invalid campaign type"})
	}

	configVal := nilIfEmpty(req.Config)
	if configVal == nil {
		configVal = "{}"
	}
	if err := ValidateCampaignConfig(h.policy, req.Type, configVal.(string)); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}

	var id string
	err := h.db.QueryRow(`
		INSERT INTO campaigns (name, description, type, status, config)
		VALUES ($1, $2, $3, $4, $5) RETURNING id`,
		req.Name, req.Description, req.Type, models.CampaignStatusCreated, configVal,
	).Scan(&id)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "failed to create campaign"})
	}

	return c.Status(201).JSON(fiber.Map{"id": id, "status": models.CampaignStatusCreated})
}

func (h *CampaignHandler) List(c *fiber.Ctx) error {
	query := `SELECT c.id, c.name, COALESCE(c.description, ''), c.type, c.status,
		COALESCE(c.config::text, '{}'), c.total_tasks, c.completed_tasks, c.failed_tasks,
		c.created_at, c.updated_at, COALESCE(bc.bot_count, 0)
		FROM campaigns c
		LEFT JOIN (SELECT campaign_id, COUNT(*) AS bot_count FROM campaign_bots GROUP BY campaign_id) bc
			ON bc.campaign_id = c.id`
	args := []interface{}{}

	if status := c.Query("status"); status != "" {
		query += " WHERE c.status = $1"
		args = append(args, status)
	}
	query += " ORDER BY c.created_at DESC"

	rows, err := h.db.Query(query, args...)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "failed to list campaigns"})
	}
	defer rows.Close()

	var camps []fiber.Map
	for rows.Next() {
		var id, name, desc, cType, status, config string
		var totalTasks, completedTasks, failedTasks, botCount int
		var createdAt, updatedAt time.Time
		if err := rows.Scan(&id, &name, &desc, &cType, &status, &config,
			&totalTasks, &completedTasks, &failedTasks, &createdAt, &updatedAt, &botCount); err != nil {
			continue
		}

		camps = append(camps, fiber.Map{
			"id": id, "name": name, "description": desc, "type": cType,
			"status": status, "config": config, "bot_count": botCount,
			"total_tasks": totalTasks, "completed_tasks": completedTasks, "failed_tasks": failedTasks,
			"created_at": createdAt, "updated_at": updatedAt,
		})
	}
	if camps == nil {
		camps = []fiber.Map{}
	}
	return c.JSON(fiber.Map{"campaigns": camps, "count": len(camps)})
}

func (h *CampaignHandler) Get(c *fiber.Ctx) error {
	id := c.Params("id")

	var name, desc, cType, status, config string
	var totalTasks, completedTasks, failedTasks int
	var createdAt, updatedAt time.Time
	err := h.db.QueryRow(`
		SELECT name, COALESCE(description, ''), type, status, COALESCE(config::text, '{}'),
			total_tasks, completed_tasks, failed_tasks, created_at, updated_at
		FROM campaigns WHERE id = $1`, id).Scan(
		&name, &desc, &cType, &status, &config,
		&totalTasks, &completedTasks, &failedTasks, &createdAt, &updatedAt)
	if err == sql.ErrNoRows {
		return c.Status(404).JSON(fiber.Map{"error": "campaign not found"})
	}
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "failed to get campaign"})
	}

	var botCount int
	h.db.QueryRow(`SELECT COUNT(*) FROM campaign_bots WHERE campaign_id = $1`, id).Scan(&botCount)

	return c.JSON(fiber.Map{
		"id": id, "name": name, "description": desc, "type": cType,
		"status": status, "config": config, "bot_count": botCount,
		"total_tasks": totalTasks, "completed_tasks": completedTasks, "failed_tasks": failedTasks,
		"created_at": createdAt, "updated_at": updatedAt,
	})
}

func (h *CampaignHandler) Update(c *fiber.Ctx) error {
	id := c.Params("id")
	var req struct {
		Status      *string `json:"status"`
		Description *string `json:"description"`
		Config      *string `json:"config"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request body"})
	}

	if req.Status != nil {
		validStatuses := map[string]bool{
			models.CampaignStatusCreated:   true,
			models.CampaignStatusRunning:   true,
			models.CampaignStatusPaused:    true,
			models.CampaignStatusCompleted: true,
			models.CampaignStatusFailed:    true,
		}
		if !validStatuses[*req.Status] {
			return c.Status(400).JSON(fiber.Map{"error": "invalid status"})
		}
	}

	result, err := h.db.Exec(`
		UPDATE campaigns SET
			status = COALESCE($1, status),
			description = COALESCE($2, description),
			config = COALESCE($3, config),
			updated_at = NOW()
		WHERE id = $4`, req.Status, req.Description, req.Config, id)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "failed to update campaign"})
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return c.Status(404).JSON(fiber.Map{"error": "campaign not found"})
	}
	return c.JSON(fiber.Map{"status": "updated"})
}

func (h *CampaignHandler) Delete(c *fiber.Ctx) error {
	id := c.Params("id")
	result, err := h.db.Exec(`DELETE FROM campaigns WHERE id = $1`, id)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "failed to delete campaign"})
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return c.Status(404).JSON(fiber.Map{"error": "campaign not found"})
	}
	return c.JSON(fiber.Map{"status": "deleted"})
}

func (h *CampaignHandler) AssignBots(c *fiber.Ctx) error {
	campID := c.Params("id")
	var req struct {
		BotIDs []string `json:"bot_ids"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request body"})
	}
	if len(req.BotIDs) == 0 {
		return c.Status(400).JSON(fiber.Map{"error": "bot_ids required"})
	}

	var exists bool
	h.db.QueryRow(`SELECT EXISTS(SELECT 1 FROM campaigns WHERE id = $1)`, campID).Scan(&exists)
	if !exists {
		return c.Status(404).JSON(fiber.Map{"error": "campaign not found"})
	}

	assigned := 0
	for _, botID := range req.BotIDs {
		_, err := h.db.Exec(`
			INSERT INTO campaign_bots (campaign_id, bot_id)
			VALUES ($1, $2)
			ON CONFLICT (campaign_id, bot_id) DO NOTHING`, campID, botID)
		if err == nil {
			assigned++
		}
	}

	return c.JSON(fiber.Map{"status": "assigned", "count": assigned})
}

func (h *CampaignHandler) RemoveBot(c *fiber.Ctx) error {
	campID := c.Params("id")
	botID := c.Params("bot_id")

	result, err := h.db.Exec(`DELETE FROM campaign_bots WHERE campaign_id = $1 AND bot_id = $2`, campID, botID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "failed to remove bot"})
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return c.Status(404).JSON(fiber.Map{"error": "assignment not found"})
	}
	return c.JSON(fiber.Map{"status": "removed"})
}

func (h *CampaignHandler) ListBots(c *fiber.Ctx) error {
	campID := c.Params("id")

	rows, err := h.db.Query(`
		SELECT b.id, b.hostname, b.ip_address, COALESCE(b.external_ip, ''),
			b.os, b.architecture, COALESCE(b.username, ''), b.privileged, b.status, b.last_seen
		FROM campaign_bots cb
		JOIN bots b ON b.id = cb.bot_id
		WHERE cb.campaign_id = $1
		ORDER BY cb.assigned_at`, campID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "failed to list campaign bots"})
	}
	defer rows.Close()

	var bots []fiber.Map
	for rows.Next() {
		var id, hostname, ip, extIP, osName, arch, username, status string
		var privileged bool
		var lastSeen time.Time
		if err := rows.Scan(&id, &hostname, &ip, &extIP, &osName, &arch, &username, &privileged, &status, &lastSeen); err != nil {
			continue
		}
		bots = append(bots, fiber.Map{
			"id": id, "hostname": hostname, "ip_address": ip, "external_ip": extIP,
			"os": osName, "architecture": arch, "username": username, "privileged": privileged,
			"status": status, "last_seen": lastSeen,
		})
	}
	if bots == nil {
		bots = []fiber.Map{}
	}
	return c.JSON(fiber.Map{"bots": bots, "count": len(bots)})
}

func (h *CampaignHandler) Launch(c *fiber.Ctx) error {
	campID := c.Params("id")

	var status, campaignType, config string
	err := h.db.QueryRow(`SELECT status, type, COALESCE(config::text, '{}') FROM campaigns WHERE id = $1`, campID).Scan(&status, &campaignType, &config)
	if err == sql.ErrNoRows {
		return c.Status(404).JSON(fiber.Map{"error": "campaign not found"})
	}
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "failed to get campaign"})
	}

	if status == models.CampaignStatusRunning {
		return c.Status(400).JSON(fiber.Map{"error": "campaign already running"})
	}
	if status == models.CampaignStatusCompleted {
		return c.Status(400).JSON(fiber.Map{"error": "campaign already completed"})
	}
	if err := ValidateCampaignConfig(h.policy, campaignType, config); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}

	var botCount int
	h.db.QueryRow(`SELECT COUNT(*) FROM campaign_bots WHERE campaign_id = $1`, campID).Scan(&botCount)
	isExternalBrute := false
	if campaignType == models.CampaignTypeBrute {
		var cfg bruteCampaignConfig
		if json.Unmarshal([]byte(config), &cfg) == nil && cfg.Mode == "external" {
			isExternalBrute = true
		}
	}
	if botCount == 0 && !isExternalBrute {
		return c.Status(400).JSON(fiber.Map{"error": "no bots assigned to campaign"})
	}

	_, err = h.db.Exec(`UPDATE campaigns SET status = $1, updated_at = NOW() WHERE id = $2`,
		models.CampaignStatusRunning, campID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "failed to launch campaign"})
	}

	return c.JSON(fiber.Map{"status": "launched", "bot_count": botCount})
}

func (h *CampaignHandler) Progress(c *fiber.Ctx) error {
	campID := c.Params("id")

	var name, cType, status string
	var totalTasks, completedTasks, failedTasks int
	err := h.db.QueryRow(`
		SELECT name, type, status, total_tasks, completed_tasks, failed_tasks
		FROM campaigns WHERE id = $1`, campID).Scan(
		&name, &cType, &status, &totalTasks, &completedTasks, &failedTasks)
	if err == sql.ErrNoRows {
		return c.Status(404).JSON(fiber.Map{"error": "campaign not found"})
	}
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "failed to get progress"})
	}

	pending := totalTasks - completedTasks - failedTasks
	if pending < 0 {
		pending = 0
	}

	var pct float64
	if totalTasks > 0 {
		pct = float64(completedTasks+failedTasks) / float64(totalTasks) * 100
	}

	return c.JSON(fiber.Map{
		"name": name, "type": cType, "status": status,
		"total": totalTasks, "completed": completedTasks, "failed": failedTasks,
		"pending": pending, "percent": pct,
	})
}

func (h *CampaignHandler) Results(c *fiber.Ctx) error {
	campID := c.Params("id")

	rows, err := h.db.Query(`
		SELECT ct.id, COALESCE(ct.bot_id::text, ''), ct.task_name, ct.status,
			COALESCE(ct.output, ''), ct.created_at, ct.completed_at,
			COALESCE(b.hostname, 'C2-SERVER'), COALESCE(b.ip_address, 'server-side')
		FROM campaign_tasks ct
		LEFT JOIN bots b ON b.id = ct.bot_id
		WHERE ct.campaign_id = $1
		ORDER BY ct.created_at ASC`, campID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "failed to get results"})
	}
	defer rows.Close()

	var tasks []fiber.Map
	for rows.Next() {
		var id, botID, taskName, status, output, hostname, ip string
		var createdAt time.Time
		var completedAt *time.Time
		if err := rows.Scan(&id, &botID, &taskName, &status, &output, &createdAt, &completedAt, &hostname, &ip); err != nil {
			continue
		}
		tasks = append(tasks, fiber.Map{
			"id": id, "bot_id": botID, "task_name": taskName, "status": status,
			"output": output, "created_at": createdAt, "completed_at": completedAt,
			"hostname": hostname, "ip_address": ip,
		})
	}
	if tasks == nil {
		tasks = []fiber.Map{}
	}
	return c.JSON(fiber.Map{"tasks": tasks, "count": len(tasks)})
}

func (h *CampaignHandler) Replay(c *fiber.Ctx) error {
	srcID := c.Params("id")

	var name, desc, cType, config string
	err := h.db.QueryRow(`
		SELECT name, COALESCE(description, ''), type, COALESCE(config::text, '{}')
		FROM campaigns WHERE id = $1`, srcID).Scan(&name, &desc, &cType, &config)
	if err == sql.ErrNoRows {
		return c.Status(404).JSON(fiber.Map{"error": "campaign not found"})
	}
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "failed to read campaign"})
	}

	if err := ValidateCampaignConfig(h.policy, cType, config); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}

	var req struct {
		Name      string `json:"name"`
		AutoStart bool   `json:"auto_start"`
	}
	c.BodyParser(&req)
	newName := req.Name
	if newName == "" {
		newName = name + " (replay)"
	}

	var newID string
	err = h.db.QueryRow(`
		INSERT INTO campaigns (name, description, type, status, config)
		VALUES ($1, $2, $3, $4, $5) RETURNING id`,
		newName, desc, cType, models.CampaignStatusCreated, config,
	).Scan(&newID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "failed to create replay campaign"})
	}

	result, err := h.db.Exec(`
		INSERT INTO campaign_bots (campaign_id, bot_id)
		SELECT $1, bot_id FROM campaign_bots WHERE campaign_id = $2`, newID, srcID)
	var botCount int64
	if err == nil {
		botCount, _ = result.RowsAffected()
	}

	isExternalBrute := false
	if cType == models.CampaignTypeBrute {
		var cfg bruteCampaignConfig
		if json.Unmarshal([]byte(config), &cfg) == nil && cfg.Mode == "external" {
			isExternalBrute = true
		}
	}

	if req.AutoStart && (botCount > 0 || isExternalBrute) {
		h.db.Exec(`UPDATE campaigns SET status = $1, updated_at = NOW() WHERE id = $2`,
			models.CampaignStatusRunning, newID)
		return c.Status(201).JSON(fiber.Map{
			"id": newID, "status": models.CampaignStatusRunning,
			"bot_count": botCount, "source_id": srcID,
		})
	}

	return c.Status(201).JSON(fiber.Map{
		"id": newID, "status": models.CampaignStatusCreated,
		"bot_count": botCount, "source_id": srcID,
	})
}

type DryRunResult struct {
	TotalTargets   int      `json:"total_targets"`
	BlockedTargets int      `json:"blocked_targets"`
	PolicyWarnings []string `json:"policy_warnings"`
}

func DryRunValidate(p policy.Policy, campaignType string, configJSON string) DryRunResult {
	result := DryRunResult{}

	if campaignType != models.CampaignTypeBrute {
		return result
	}

	var cfg bruteCampaignConfig
	if err := json.Unmarshal([]byte(configJSON), &cfg); err != nil {
		result.PolicyWarnings = append(result.PolicyWarnings, fmt.Sprintf("invalid config: %v", err))
		return result
	}

	result.TotalTargets = len(cfg.Targets)

	if cfg.Mode == "external" && !p.AllowExternalBrute {
		result.PolicyWarnings = append(result.PolicyWarnings, "external brute campaigns are disabled by policy")
	}

	for _, target := range cfg.Targets {
		if err := p.ValidateTarget(target); err != nil {
			result.BlockedTargets++
			result.PolicyWarnings = append(result.PolicyWarnings, fmt.Sprintf("target %q: %v", target, err))
		}
	}

	return result
}

func (h *CampaignHandler) Validate(c *fiber.Ctx) error {
	var req struct {
		Type   string `json:"type"`
		Config string `json:"config"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request body"})
	}
	if req.Type == "" {
		return c.Status(400).JSON(fiber.Map{"error": "type required"})
	}

	configJSON := req.Config
	if configJSON == "" {
		configJSON = "{}"
	}

	result := DryRunValidate(h.policy, req.Type, configJSON)
	return c.JSON(fiber.Map{
		"total_targets":   result.TotalTargets,
		"blocked_targets": result.BlockedTargets,
		"policy_warnings": result.PolicyWarnings,
		"can_launch":      result.BlockedTargets == 0 && len(result.PolicyWarnings) == 0,
	})
}
