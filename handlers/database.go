package handlers

import (
	"shopping-list/db"

	"github.com/gofiber/fiber/v2"
)

// ClearDatabaseRequest represents the request body for clearing the database
type ClearDatabaseRequest struct {
	Confirmation string `json:"confirmation" form:"confirmation"`
}

// ClearDatabase handles the database clear operation
// Requires confirmation word "DELETE" to proceed
func ClearDatabase(c *fiber.Ctx) error {
	var req ClearDatabaseRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"success": false,
			"error":   "Invalid request",
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
