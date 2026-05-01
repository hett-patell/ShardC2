package middleware

import (
	"crypto/rand"
	"crypto/subtle"
	"database/sql"
	"encoding/hex"
	"fmt"
	"log"
	"strconv"

	"github.com/gofiber/fiber/v2"
	"github.com/shardc2/shardc2/internal/database"
	"github.com/shardc2/shardc2/pkg/crypto"
)

func OperatorAuth(token string) fiber.Handler {
	tokenBytes := []byte(token)
	return func(c *fiber.Ctx) error {
		auth := c.Get("Authorization")
		if len(auth) < 8 || auth[:7] != "Bearer " {
			log.Printf("[!] Auth failure: operator missing token from %s %s %s", c.IP(), c.Method(), c.Path())
			return c.Status(401).JSON(fiber.Map{"error": "unauthorized"})
		}
		if subtle.ConstantTimeCompare([]byte(auth[7:]), tokenBytes) != 1 {
			log.Printf("[!] Auth failure: operator invalid token from %s %s %s", c.IP(), c.Method(), c.Path())
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
			log.Printf("[!] Auth failure: implant missing key from %s %s %s", c.IP(), c.Method(), c.Path())
			return c.Status(401).JSON(fiber.Map{"error": "unauthorized"})
		}
		if subtle.ConstantTimeCompare([]byte(provided), keyBytes) != 1 {
			log.Printf("[!] Auth failure: implant invalid key from %s %s %s", c.IP(), c.Method(), c.Path())
			return c.Status(403).JSON(fiber.Map{"error": "forbidden"})
		}
		return c.Next()
	}
}

func AgentAuth(db *database.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		token := c.Get("X-Session-Token")
		if token == "" {
			log.Printf("[!] Auth failure: agent missing token from %s %s %s", c.IP(), c.Method(), c.Path())
			return c.Status(401).JSON(fiber.Map{"error": "unauthorized"})
		}
		var botID string
		err := db.QueryRow("SELECT bot_id FROM bot_tokens WHERE token = $1", token).Scan(&botID)
		if err == sql.ErrNoRows {
			log.Printf("[!] Auth failure: agent invalid token from %s %s %s", c.IP(), c.Method(), c.Path())
			return c.Status(403).JSON(fiber.Map{"error": "forbidden"})
		}
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "internal error"})
		}
		c.Locals("bot_id", botID)
		return c.Next()
	}
}

func PayloadCrypto(key []byte) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if len(key) == 0 {
			return c.Next()
		}

		// Verify HMAC signature if present
		sig := c.Get("X-Signature")
		tsStr := c.Get("X-Timestamp")
		if sig != "" && tsStr != "" {
			ts, err := strconv.ParseInt(tsStr, 10, 64)
			if err != nil || !crypto.Verify(c.Method(), c.Path(), c.Body(), key, ts, sig) {
				log.Printf("[!] Auth failure: invalid HMAC signature from %s %s %s", c.IP(), c.Method(), c.Path())
				return c.Status(403).JSON(fiber.Map{"error": "invalid signature"})
			}
		}

		// Decrypt request body if encrypted
		if c.Get("Content-Type") == "application/octet-stream" && len(c.Body()) > 0 {
			plaintext, err := crypto.Decrypt(c.Body(), key)
			if err != nil {
				log.Printf("[!] Decrypt failure from %s %s %s: %v", c.IP(), c.Method(), c.Path(), err)
				return c.Status(400).JSON(fiber.Map{"error": "decryption failed"})
			}
			c.Request().SetBody(plaintext)
			c.Request().Header.Set("Content-Type", "application/json")
		}

		// Process the request
		err := c.Next()
		if err != nil {
			return err
		}

		// Encrypt response body for successful responses
		if c.Response().StatusCode() >= 200 && c.Response().StatusCode() < 300 {
			respBody := c.Response().Body()
			if len(respBody) > 0 {
				encrypted, encErr := crypto.Encrypt(respBody, key)
				if encErr != nil {
					return fmt.Errorf("encrypt response: %w", encErr)
				}
				c.Response().SetBody(encrypted)
				c.Response().Header.Set("Content-Type", "application/octet-stream")
			}
		}

		return nil
	}
}

func GenerateToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
