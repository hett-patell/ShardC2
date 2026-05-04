package handlers

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/shardc2/shardc2/internal/database"
	"github.com/shardc2/shardc2/internal/server/builds"
	"github.com/shardc2/shardc2/pkg/models"
)

type BuildHandler struct {
	db      *database.DB
	builder builds.Builder
}

func NewBuildHandler(db *database.DB, builder builds.Builder) *BuildHandler {
	return &BuildHandler{db: db, builder: builder}
}

func (h *BuildHandler) Create(c *fiber.Ctx) error {
	var req builds.Request
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request body"})
	}
	if err := req.Validate(); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}

	operator, _ := c.Locals("operator_user").(string)
	if operator == "" {
		operator, _ = c.Locals("operator_username").(string)
	}
	req.RequestedBy = operator

	var buildID string
	err := h.db.QueryRow(`
		INSERT INTO agent_builds (goos, goarch, profile, payload_key_fingerprint, requested_by, status)
		VALUES ($1, $2, $3, $4, $5, $6) RETURNING id`,
		req.GOOS, req.GOARCH, req.Profile, nilIfEmpty(req.PayloadKeyFingerprint), nilIfEmpty(req.RequestedBy), models.BuildStatusBuilding,
	).Scan(&buildID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "failed to create build record"})
	}

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()

		artifact, buildErr := h.builder.Build(ctx, req)
		if buildErr != nil {
			h.db.Exec(`UPDATE agent_builds SET status = $1, error_message = $2, completed_at = NOW() WHERE id = $3`,
				models.BuildStatusFailed, artifact.Error, buildID)
			return
		}
		h.db.Exec(`UPDATE agent_builds SET status = $1, artifact_path = $2, completed_at = NOW() WHERE id = $3`,
			models.BuildStatusCompleted, artifact.Path, buildID)
	}()

	return c.Status(202).JSON(fiber.Map{"id": buildID, "status": models.BuildStatusBuilding})
}

func (h *BuildHandler) Get(c *fiber.Ctx) error {
	id := c.Params("id")

	var b models.AgentBuild
	var artifactPath, errorMessage, payloadFP, requestedBy sql.NullString
	var completedAt *time.Time
	err := h.db.QueryRow(`
		SELECT id, goos, goarch, profile, payload_key_fingerprint, requested_by, status, artifact_path, error_message, created_at, completed_at
		FROM agent_builds WHERE id = $1`, id,
	).Scan(&b.ID, &b.GOOS, &b.GOARCH, &b.Profile, &payloadFP, &requestedBy, &b.Status, &artifactPath, &errorMessage, &b.CreatedAt, &completedAt)
	if err == sql.ErrNoRows {
		return c.Status(404).JSON(fiber.Map{"error": "build not found"})
	}
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "failed to get build"})
	}

	b.ArtifactPath = artifactPath.String
	b.ErrorMessage = errorMessage.String
	b.PayloadKeyFingerprint = payloadFP.String
	b.RequestedBy = requestedBy.String
	b.CompletedAt = completedAt

	return c.JSON(b)
}

func (h *BuildHandler) CreateStager(c *fiber.Ctx) error {
	var req builds.StagerRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request body"})
	}
	if err := req.Validate(); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}

	var stageStatus string
	err := h.db.QueryRow("SELECT status FROM agent_builds WHERE id = $1", req.StageID).Scan(&stageStatus)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "stage build not found"})
	}
	if stageStatus != models.BuildStatusCompleted {
		return c.Status(400).JSON(fiber.Map{"error": "stage build not completed yet"})
	}

	var buildID string
	err = h.db.QueryRow(`
		INSERT INTO agent_builds (goos, goarch, profile, requested_by, status)
		VALUES ($1, $2, $3, $4, $5) RETURNING id`,
		req.GOOS, req.GOARCH, "stager", nil, models.BuildStatusBuilding,
	).Scan(&buildID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "failed to create build record"})
	}

	lb, ok := h.builder.(*builds.LocalBuilder)
	if !ok {
		return c.Status(500).JSON(fiber.Map{"error": "stager builds require local builder"})
	}

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()

		artifact, buildErr := lb.BuildStager(ctx, req)
		if buildErr != nil {
			h.db.Exec(`UPDATE agent_builds SET status = $1, error_message = $2, completed_at = NOW() WHERE id = $3`,
				models.BuildStatusFailed, artifact.Error, buildID)
			return
		}
		h.db.Exec(`UPDATE agent_builds SET status = $1, artifact_path = $2, completed_at = NOW() WHERE id = $3`,
			models.BuildStatusCompleted, artifact.Path, buildID)
	}()

	return c.Status(202).JSON(fiber.Map{"id": buildID, "status": models.BuildStatusBuilding})
}

func (h *BuildHandler) Download(c *fiber.Ctx) error {
	id := c.Params("id")

	var path string
	var status string
	err := h.db.QueryRow(`SELECT COALESCE(artifact_path, ''), status FROM agent_builds WHERE id = $1`, id).Scan(&path, &status)
	if err == sql.ErrNoRows {
		return c.Status(404).JSON(fiber.Map{"error": "build not found"})
	}
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "failed to get build"})
	}
	if status != models.BuildStatusCompleted || path == "" {
		return c.Status(400).JSON(fiber.Map{"error": "build not ready", "status": status})
	}

	clean := filepath.Clean(path)
	if strings.Contains(clean, "..") || filepath.IsAbs(clean) && !strings.HasPrefix(clean, filepath.Clean("bin")) {
		return c.Status(403).JSON(fiber.Map{"error": "invalid artifact path"})
	}

	return c.SendFile(clean, false)
}
