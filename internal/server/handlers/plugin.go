package handlers

import (
	"github.com/gofiber/fiber/v2"
	"github.com/shardc2/shardc2/pkg/plugins"
)

type PluginHandler struct {
	pluginsDir string
}

func NewPluginHandler(pluginsDir string) *PluginHandler {
	return &PluginHandler{pluginsDir: pluginsDir}
}

func (h *PluginHandler) List(c *fiber.Ctx) error {
	manifests, err := plugins.LoadAllManifests(h.pluginsDir)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "failed to list plugins"})
	}
	if manifests == nil {
		manifests = []plugins.Manifest{}
	}

	items := make([]fiber.Map, 0, len(manifests))
	for _, m := range manifests {
		items = append(items, fiber.Map{
			"name":        m.Name,
			"version":     m.Version,
			"description": m.Description,
			"commands":    m.Commands,
			"permissions": m.Permissions,
			"execution":   "disabled",
		})
	}

	return c.JSON(fiber.Map{"plugins": items, "count": len(items)})
}
