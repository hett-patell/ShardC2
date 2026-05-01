package handlers

import (
	"github.com/gofiber/fiber/v2"
	"github.com/shardc2/shardc2/internal/database"
	"github.com/shardc2/shardc2/pkg/policy"
)

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
