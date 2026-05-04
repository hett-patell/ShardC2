package handlers

import (
	"encoding/json"
	"os"
	"sync"

	"github.com/gofiber/fiber/v2"
	"github.com/shardc2/shardc2/pkg/policy"
)

type PolicyHandler struct {
	mu         sync.RWMutex
	policy     *policy.Policy
	policyPath string
}

func NewPolicyHandler(p *policy.Policy, path string) *PolicyHandler {
	return &PolicyHandler{policy: p, policyPath: path}
}

func (h *PolicyHandler) Get(c *fiber.Ctx) error {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return c.JSON(h.policy)
}

func (h *PolicyHandler) Update(c *fiber.Ctx) error {
	var incoming policy.Policy
	if err := c.BodyParser(&incoming); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request body"})
	}
	if err := incoming.Validate(); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}

	if h.policyPath != "" {
		data, _ := json.MarshalIndent(incoming, "", "  ")
		if err := os.WriteFile(h.policyPath, data, 0644); err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "failed to write policy file"})
		}
	}

	h.mu.Lock()
	*h.policy = incoming
	h.mu.Unlock()

	return c.JSON(fiber.Map{"status": "updated"})
}
