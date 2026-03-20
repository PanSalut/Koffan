package handlers

import (
	"crypto/subtle"
	"shopping-list/db"
	"sync"
	"time"

	"github.com/gofiber/fiber/v2"
)

// ClearDatabaseRequest represents the request body for clearing the database
type ClearDatabaseRequest struct {
	Confirmation string `json:"confirmation" form:"confirmation"`
	CSRFToken    string `json:"csrf_token" form:"csrf_token"`
}

// csrfToken stores a single-use CSRF token with expiry
type csrfToken struct {
	Token     string
	ExpiresAt time.Time
}

var (
	csrfTokens   = make(map[string]csrfToken) // sessionID -> token
	csrfTokensMu sync.Mutex
)

func init() {
	go func() {
		ticker := time.NewTicker(10 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			now := time.Now()
			csrfTokensMu.Lock()
			for id, t := range csrfTokens {
				if now.After(t.ExpiresAt) {
					delete(csrfTokens, id)
				}
			}
			csrfTokensMu.Unlock()
		}
	}()
}

// GenerateCSRFToken generates a single-use CSRF token for the clear database operation
func GenerateCSRFToken(c *fiber.Ctx) error {
	sessionID := c.Cookies(SessionCookieName)
	tokenStr := generateSessionID()

	csrfTokensMu.Lock()
	csrfTokens[sessionID] = csrfToken{
		Token:     tokenStr,
		ExpiresAt: time.Now().Add(5 * time.Minute),
	}
	csrfTokensMu.Unlock()

	return c.JSON(fiber.Map{"csrf_token": tokenStr})
}

// validateCSRFToken validates and consumes a CSRF token (single-use)
func validateCSRFToken(sessionID, token string) bool {
	csrfTokensMu.Lock()
	defer csrfTokensMu.Unlock()

	stored, exists := csrfTokens[sessionID]
	if !exists {
		return false
	}

	// Always delete the token (single-use)
	delete(csrfTokens, sessionID)

	if time.Now().After(stored.ExpiresAt) {
		return false
	}

	return subtle.ConstantTimeCompare([]byte(stored.Token), []byte(token)) == 1
}

// ClearDatabase handles the database clear operation
// Requires confirmation word "DELETE" and a valid CSRF token to proceed
func ClearDatabase(c *fiber.Ctx) error {
	var req ClearDatabaseRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"success": false,
			"error":   "Invalid request",
		})
	}

	// Verify CSRF token
	sessionID := c.Cookies(SessionCookieName)
	if !validateCSRFToken(sessionID, req.CSRFToken) {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
			"success": false,
			"error":   "invalid_csrf_token",
		})
	}

	// Verify confirmation word
	if req.Confirmation != "DELETE" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"success": false,
			"error":   "invalid_confirmation",
		})
	}

	// Clear all data
	if err := db.ClearAllData(); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"success": false,
			"error":   "Failed to clear database: " + err.Error(),
		})
	}

	// Broadcast update to all connected clients
	BroadcastUpdate("database_cleared", nil)

	return c.JSON(fiber.Map{
		"success": true,
	})
}
