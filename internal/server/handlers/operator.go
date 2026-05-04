package handlers

import (
	"database/sql"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v5"
	"github.com/shardc2/shardc2/internal/database"
	"golang.org/x/crypto/bcrypt"
)

type OperatorHandler struct {
	db     *database.DB
	secret []byte
}

func NewOperatorHandler(db *database.DB, jwtSecret []byte) *OperatorHandler {
	return &OperatorHandler{db: db, secret: jwtSecret}
}

func (h *OperatorHandler) Register(c *fiber.Ctx) error {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
		Role     string `json:"role"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request"})
	}
	if req.Username == "" || len(req.Username) > 64 {
		return c.Status(400).JSON(fiber.Map{"error": "invalid username"})
	}
	if len(req.Password) < 8 {
		return c.Status(400).JSON(fiber.Map{"error": "password must be at least 8 characters"})
	}
	if req.Role == "" {
		req.Role = "operator"
	}
	validRoles := map[string]bool{"admin": true, "operator": true, "viewer": true}
	if !validRoles[req.Role] {
		return c.Status(400).JSON(fiber.Map{"error": "invalid role (admin, operator, viewer)"})
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "failed to hash password"})
	}

	var id string
	err = h.db.QueryRow(`
		INSERT INTO operators (username, password_hash, role)
		VALUES ($1, $2, $3) RETURNING id`,
		req.Username, string(hash), req.Role,
	).Scan(&id)
	if err != nil {
		return c.Status(409).JSON(fiber.Map{"error": "username already exists"})
	}

	return c.Status(201).JSON(fiber.Map{"id": id, "username": req.Username, "role": req.Role})
}

func (h *OperatorHandler) Login(c *fiber.Ctx) error {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request"})
	}

	var id, hash, role string
	var active bool
	err := h.db.QueryRow(`
		SELECT id, password_hash, role, active FROM operators WHERE username = $1`,
		req.Username,
	).Scan(&id, &hash, &role, &active)
	if err == sql.ErrNoRows {
		return c.Status(401).JSON(fiber.Map{"error": "invalid credentials"})
	}
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "internal error"})
	}
	if !active {
		return c.Status(403).JSON(fiber.Map{"error": "account disabled"})
	}

	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(req.Password)); err != nil {
		return c.Status(401).JSON(fiber.Map{"error": "invalid credentials"})
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub":  id,
		"usr":  req.Username,
		"role": role,
		"exp":  time.Now().Add(24 * time.Hour).Unix(),
		"iat":  time.Now().Unix(),
	})

	signed, err := token.SignedString(h.secret)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "failed to generate token"})
	}

	h.db.Exec(`UPDATE operators SET last_login = NOW() WHERE id = $1`, id)

	return c.JSON(fiber.Map{
		"token":    signed,
		"username": req.Username,
		"role":     role,
		"expires":  time.Now().Add(24 * time.Hour).Unix(),
	})
}

func (h *OperatorHandler) List(c *fiber.Ctx) error {
	rows, err := h.db.Query(`
		SELECT id, username, role, active, last_login, created_at
		FROM operators ORDER BY created_at DESC`)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "failed to list operators"})
	}
	defer rows.Close()

	var ops []fiber.Map
	for rows.Next() {
		var id, username, role string
		var active bool
		var lastLogin *time.Time
		var createdAt time.Time
		if err := rows.Scan(&id, &username, &role, &active, &lastLogin, &createdAt); err != nil {
			continue
		}
		ops = append(ops, fiber.Map{
			"id": id, "username": username, "role": role,
			"active": active, "last_login": lastLogin, "created_at": createdAt,
		})
	}
	if err := rows.Err(); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "error reading operators"})
	}
	if ops == nil {
		ops = []fiber.Map{}
	}
	return c.JSON(fiber.Map{"operators": ops})
}

func (h *OperatorHandler) ChangePassword(c *fiber.Ctx) error {
	var req struct {
		CurrentPassword string `json:"current_password"`
		NewPassword     string `json:"new_password"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request"})
	}
	if len(req.NewPassword) < 8 {
		return c.Status(400).JSON(fiber.Map{"error": "new password must be at least 8 characters"})
	}

	operatorID, _ := c.Locals("operator_id").(string)
	if operatorID == "" {
		return c.Status(401).JSON(fiber.Map{"error": "not authenticated"})
	}

	var currentHash string
	err := h.db.QueryRow(`SELECT password_hash FROM operators WHERE id = $1 AND active = true`, operatorID).Scan(&currentHash)
	if err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "operator not found"})
	}

	if err := bcrypt.CompareHashAndPassword([]byte(currentHash), []byte(req.CurrentPassword)); err != nil {
		return c.Status(401).JSON(fiber.Map{"error": "current password is incorrect"})
	}

	newHash, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "failed to hash password"})
	}

	_, err = h.db.Exec(`UPDATE operators SET password_hash = $1 WHERE id = $2`, string(newHash), operatorID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "failed to update password"})
	}

	return c.JSON(fiber.Map{"status": "password changed"})
}

func (h *OperatorHandler) Deactivate(c *fiber.Ctx) error {
	id := c.Params("id")
	result, err := h.db.Exec(`UPDATE operators SET active = false WHERE id = $1`, id)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "failed to deactivate"})
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return c.Status(404).JSON(fiber.Map{"error": "operator not found"})
	}
	return c.JSON(fiber.Map{"status": "deactivated"})
}
