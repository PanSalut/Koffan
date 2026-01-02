package api

import (
	"os"
	"strings"

	"github.com/gofiber/fiber/v2"
)

// GetAPIToken returns the API token from environment, empty if not set
func GetAPIToken() string {
	return os.Getenv("API_TOKEN")
}

// IsAPIEnabled returns true if API_TOKEN is set
func IsAPIEnabled() bool {
	return GetAPIToken() != ""
}

// TokenAuthMiddleware validates Bearer token in Authorization header
func TokenAuthMiddleware(c *fiber.Ctx) error {
	expectedToken := GetAPIToken()
	if expectedToken == "" {
		return c.Status(fiber.StatusServiceUnavailable).JSON(ErrorResponse{
			Error:   "api_disabled",
			Message: "API is not enabled on this server",
		})
	}

	authHeader := c.Get("Authorization")
	if authHeader == "" {
		return c.Status(fiber.StatusUnauthorized).JSON(ErrorResponse{
			Error:   "missing_token",
			Message: "Authorization header is required",
		})
	}

	// Expect "Bearer <token>"
	parts := strings.SplitN(authHeader, " ", 2)
	if len(parts) != 2 || strings.ToLower(parts[0]) != "bearer" {
		return c.Status(fiber.StatusUnauthorized).JSON(ErrorResponse{
			Error:   "invalid_format",
			Message: "Authorization header must be in format: Bearer <token>",
		})
	}

	if parts[1] != expectedToken {
		return c.Status(fiber.StatusUnauthorized).JSON(ErrorResponse{
			Error:   "invalid_token",
			Message: "Invalid API token",
		})
	}

	return c.Next()
}
