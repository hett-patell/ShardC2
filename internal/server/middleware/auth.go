package middleware

import (
	"crypto/rand"
	"crypto/subtle"
	"database/sql"
	"encoding/hex"

	"github.com/gofiber/fiber/v2"
	"github.com/shardc2/shardc2/internal/database"
)

func OperatorAuth(token string) fiber.Handler {
	tokenBytes := []byte(token)
	return func(c *fiber.Ctx) error {
		auth := c.Get("Authorization")
		if len(auth) < 8 || auth[:7] != "Bearer " {
			return c.Status(401).JSON(fiber.Map{"error": "unauthorized"})
		}
		if subtle.ConstantTimeCompare([]byte(auth[7:]), tokenBytes) != 1 {
			return c.Status(403).JSON(fiber.Map{"error": "forbidden"})
		}
		return c.Next()
	}
}

func ImplantAuth(key string) fiber.Handler {
	keyBytes := []byte(key)
	return func(c *fiber.Ctx) error {
		provided := c.Get("X-Implant-Key")
		if provided == "" {
			return c.Status(401).JSON(fiber.Map{"error": "unauthorized"})
		}
		if subtle.ConstantTimeCompare([]byte(provided), keyBytes) != 1 {
			return c.Status(403).JSON(fiber.Map{"error": "forbidden"})
		}
		return c.Next()
	}
}

func AgentAuth(db *database.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		token := c.Get("X-Session-Token")
		if token == "" {
			return c.Status(401).JSON(fiber.Map{"error": "unauthorized"})
		}
		var botID string
		err := db.QueryRow("SELECT bot_id FROM bot_tokens WHERE token = $1", token).Scan(&botID)
		if err == sql.ErrNoRows {
			return c.Status(403).JSON(fiber.Map{"error": "forbidden"})
		}
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "internal error"})
		}
		c.Locals("bot_id", botID)
		return c.Next()
	}
}

func GenerateToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
