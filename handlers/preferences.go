package handlers

import (
	"shopping-list/db"

	"github.com/gofiber/fiber/v2"
)

// GetPreferences returns user preferences
func GetPreferences(c *fiber.Ctx) error {
	prefs := db.GetPreferences()
	return c.JSON(prefs)
}

// UpdatePreferences updates user preferences
func UpdatePreferences(c *fiber.Ctx) error {
	mobileHelper := c.FormValue("mobile_helper")
	if mobileHelper != "button" && mobileHelper != "progress" {
		return c.Status(400).SendString("Invalid mobile_helper value")
	}

	err := db.UpdatePreferences(mobileHelper)
	if err != nil {
		return c.Status(500).SendString("Failed to update preferences")
	}

	// Broadcast to WebSocket clients
	BroadcastUpdate("preferences_updated", db.GetPreferences())

	return c.JSON(db.GetPreferences())
}

// ToggleMobileHelper toggles between button and progress
func ToggleMobileHelper(c *fiber.Ctx) error {
	prefs := db.GetPreferences()
	newValue := "progress"
	if prefs.MobileHelper == "progress" {
		newValue = "button"
	}

	err := db.UpdatePreferences(newValue)
	if err != nil {
		return c.Status(500).SendString("Failed to update preferences")
	}

	// Broadcast to WebSocket clients
	BroadcastUpdate("preferences_updated", db.GetPreferences())

	// Return updated preferences as JSON
	return c.JSON(db.GetPreferences())
}
