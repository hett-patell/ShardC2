package handlers

import (
	"runtime"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/shardc2/shardc2/internal/database"
	"github.com/shardc2/shardc2/pkg/policy"
)

var serverStartTime = time.Now()

type StatusHandler struct {
	db     *database.DB
	policy policy.Policy
}

func NewStatusHandler(db *database.DB, p policy.Policy) *StatusHandler {
	return &StatusHandler{db: db, policy: p}
}

func (h *StatusHandler) SafetyStatus(c *fiber.Ctx) error {
	var runningCampaigns int
	if h.db != nil {
		h.db.QueryRow(`SELECT COUNT(*) FROM campaigns WHERE status = 'running'`).Scan(&runningCampaigns)
	}

	blocked := []string{}
	if h.policy.SafeMode {
		blocked = append(blocked, "auto_resume_campaigns")
	}
	if !h.policy.AllowExternalBrute {
		blocked = append(blocked, "external_brute_campaigns")
	}
	if !h.policy.AllowAutoDeploy {
		blocked = append(blocked, "auto_deploy_agents")
	}

	return c.JSON(fiber.Map{
		"safe_mode":         h.policy.SafeMode,
		"running_campaigns": runningCampaigns,
		"allowed_cidrs":     h.policy.AllowedCIDRs,
		"allowed_hosts":     h.policy.AllowedHosts,
		"blocked_cidrs":     h.policy.BlockedCIDRs,
		"blocked_features":  blocked,
		"external_brute":    h.policy.AllowExternalBrute,
		"auto_deploy":       h.policy.AllowAutoDeploy,
	})
}

func (h *StatusHandler) SystemInfo(c *fiber.Ctx) error {
	var totalBots, activeBots, totalCreds, totalCampaigns, runningCampaigns, totalOperators int
	if h.db != nil {
		h.db.QueryRow(`SELECT
			(SELECT COUNT(*) FROM bots),
			(SELECT COUNT(*) FROM bots WHERE last_seen > NOW() - INTERVAL '5 minutes'),
			(SELECT COUNT(*) FROM credentials),
			(SELECT COUNT(*) FROM campaigns),
			(SELECT COUNT(*) FROM campaigns WHERE status = 'running'),
			(SELECT COUNT(*) FROM operators WHERE active = true)
		`).Scan(&totalBots, &activeBots, &totalCreds, &totalCampaigns, &runningCampaigns, &totalOperators)
	}

	uptime := time.Since(serverStartTime)

	return c.JSON(fiber.Map{
		"version":            "1.0.0",
		"go_version":         runtime.Version(),
		"os":                 runtime.GOOS,
		"arch":               runtime.GOARCH,
		"uptime_seconds":     int(uptime.Seconds()),
		"uptime_human":       formatUptime(uptime),
		"total_bots":         totalBots,
		"active_bots":        activeBots,
		"total_credentials":  totalCreds,
		"total_campaigns":    totalCampaigns,
		"running_campaigns":  runningCampaigns,
		"active_operators":   totalOperators,
		"goroutines":         runtime.NumGoroutine(),
		"policy_safe_mode":   h.policy.SafeMode,
		"external_brute":     h.policy.AllowExternalBrute,
		"auto_deploy":        h.policy.AllowAutoDeploy,
	})
}

func (h *StatusHandler) AuditEvents(c *fiber.Ctx) error {
	limitStr := c.Query("limit", "50")
	limit := 50
	if l := parseInt(limitStr); l > 0 && l <= 200 {
		limit = l
	}

	rows, err := h.db.Query(`
		SELECT id, COALESCE(operator_username, ''), COALESCE(operator_role, ''),
			COALESCE(host(source_ip), ''), action, COALESCE(object_type, ''),
			COALESCE(object_id, ''), outcome, COALESCE(details::text, '{}'), created_at
		FROM audit_events ORDER BY created_at DESC LIMIT $1`, limit)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "failed to query audit events"})
	}
	defer rows.Close()

	var events []fiber.Map
	for rows.Next() {
		var id, username, role, ip, action, objType, objID, outcome, details string
		var createdAt time.Time
		if err := rows.Scan(&id, &username, &role, &ip, &action, &objType, &objID, &outcome, &details, &createdAt); err != nil {
			continue
		}
		events = append(events, fiber.Map{
			"id": id, "operator": username, "role": role, "source_ip": ip,
			"action": action, "object_type": objType, "object_id": objID,
			"outcome": outcome, "details": details, "created_at": createdAt,
		})
	}
	if err := rows.Err(); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "error reading audit events"})
	}
	if events == nil {
		events = []fiber.Map{}
	}
	return c.JSON(fiber.Map{"events": events, "count": len(events)})
}

func (h *StatusHandler) DatabaseStats(c *fiber.Ctx) error {
	type tableSize struct {
		Name  string `json:"name"`
		Rows  int    `json:"rows"`
	}
	tables := []string{"bots", "commands", "credentials", "campaigns", "campaign_tasks", "audit_events", "operators", "exfil_data", "agent_builds"}
	var stats []tableSize
	for _, t := range tables {
		var count int
		h.db.QueryRow(`SELECT COUNT(*) FROM ` + t).Scan(&count)
		stats = append(stats, tableSize{Name: t, Rows: count})
	}

	var dbSize string
	h.db.QueryRow(`SELECT pg_size_pretty(pg_database_size(current_database()))`).Scan(&dbSize)

	var deadBots, staleCommands int
	h.db.QueryRow(`SELECT COUNT(*) FROM bots WHERE last_seen < NOW() - INTERVAL '2 days'`).Scan(&deadBots)
	h.db.QueryRow(`SELECT COUNT(*) FROM commands WHERE status IN ('pending','executing') AND created_at < NOW() - INTERVAL '1 hour'`).Scan(&staleCommands)

	return c.JSON(fiber.Map{
		"tables":          stats,
		"database_size":   dbSize,
		"dead_bots":       deadBots,
		"stale_commands":  staleCommands,
	})
}

func (h *StatusHandler) CleanupDeadBots(c *fiber.Ctx) error {
	result, err := h.db.Exec(`DELETE FROM bots WHERE last_seen < NOW() - INTERVAL '7 days'`)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "cleanup failed"})
	}
	rows, _ := result.RowsAffected()
	return c.JSON(fiber.Map{"deleted": rows})
}

func (h *StatusHandler) CleanupStaleCommands(c *fiber.Ctx) error {
	result, err := h.db.Exec(`UPDATE commands SET status = 'failed', output = 'timed out (stale)' WHERE status IN ('pending','executing') AND created_at < NOW() - INTERVAL '1 hour'`)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "cleanup failed"})
	}
	rows, _ := result.RowsAffected()
	return c.JSON(fiber.Map{"cleaned": rows})
}

func formatUptime(d time.Duration) string {
	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	mins := int(d.Minutes()) % 60
	if days > 0 {
		return time.Duration(d.Nanoseconds()).Truncate(time.Minute).String()
	}
	if hours > 0 {
		return time.Duration(d.Nanoseconds()).Truncate(time.Minute).String()
	}
	_ = mins
	return d.Truncate(time.Second).String()
}

func parseInt(s string) int {
	n := 0
	for _, c := range s {
		if c >= '0' && c <= '9' {
			n = n*10 + int(c-'0')
		}
	}
	return n
}
