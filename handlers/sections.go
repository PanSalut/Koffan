package handlers

import (
	"shopping-list/db"
	"strconv"

	"github.com/gofiber/fiber/v2"
)

// GetSections returns all sections with items (for full page render)
func GetSections(c *fiber.Ctx) error {
	sections, err := db.GetAllSections()
	if err != nil {
		return c.Status(500).SendString("Failed to fetch sections")
	}

	stats := db.GetStats()
	prefs := db.GetPreferences()

	return c.Render("list", fiber.Map{
		"Sections":    sections,
		"Stats":       stats,
		"Preferences": prefs,
	})
}

// CreateSection creates a new section
func CreateSection(c *fiber.Ctx) error {
	name := c.FormValue("name")
	if name == "" {
		return c.Status(400).SendString("Name is required")
	}

	section, err := db.CreateSection(name)
	if err != nil {
		return c.Status(500).SendString("Failed to create section")
	}

	// Broadcast to WebSocket clients
	BroadcastUpdate("section_created", section)

	// Return the new section partial for HTMX
	return c.Render("partials/section", fiber.Map{
		"Section":  section,
		"Sections": getSectionsForDropdown(),
	}, "")
}

// UpdateSection updates a section's name
func UpdateSection(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return c.Status(400).SendString("Invalid ID")
	}

	name := c.FormValue("name")
	if name == "" {
		return c.Status(400).SendString("Name is required")
	}

	section, err := db.UpdateSection(id, name)
	if err != nil {
		return c.Status(500).SendString("Failed to update section")
	}

	// Broadcast to WebSocket clients
	BroadcastUpdate("section_updated", section)

	// Return updated section partial
	return c.Render("partials/section", fiber.Map{
		"Section":  section,
		"Sections": getSectionsForDropdown(),
	}, "")
}

// DeleteSection deletes a section and all its items
func DeleteSection(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return c.Status(400).SendString("Invalid ID")
	}

	err = db.DeleteSection(id)
	if err != nil {
		return c.Status(500).SendString("Failed to delete section")
	}

	// Broadcast to WebSocket clients
	BroadcastUpdate("section_deleted", map[string]int64{"id": id})

	// Return empty string (HTMX will remove the element)
	return c.SendString("")
}

// MoveSectionUp moves a section up in order
func MoveSectionUp(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return c.Status(400).SendString("Invalid ID")
	}

	err = db.MoveSectionUp(id)
	if err != nil {
		return c.Status(500).SendString("Failed to move section")
	}

	// Broadcast and return full sections list
	BroadcastUpdate("sections_reordered", nil)
	return returnAllSections(c)
}

// MoveSectionDown moves a section down in order
func MoveSectionDown(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return c.Status(400).SendString("Invalid ID")
	}

	err = db.MoveSectionDown(id)
	if err != nil {
		return c.Status(500).SendString("Failed to move section")
	}

	// Broadcast and return full sections list
	BroadcastUpdate("sections_reordered", nil)
	return returnAllSections(c)
}

// Helper to return all sections as HTML partials
func returnAllSections(c *fiber.Ctx) error {
	sections, err := db.GetAllSections()
	if err != nil {
		return c.Status(500).SendString("Failed to fetch sections")
	}

	return c.Render("partials/sections_list", fiber.Map{
		"Sections": sections,
	}, "")
}

// Helper to get sections for dropdown
func getSectionsForDropdown() []db.Section {
	sections, _ := db.GetAllSections()
	return sections
}

// BatchDeleteSections deletes multiple sections
func BatchDeleteSections(c *fiber.Ctx) error {
	// Get IDs from form (comma-separated or multiple values)
	idsStr := c.FormValue("ids")
	if idsStr == "" {
		return c.Status(400).SendString("No IDs provided")
	}

	// Parse IDs
	var ids []int64
	for _, idStr := range splitAndTrim(idsStr, ",") {
		id, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil {
			continue
		}
		ids = append(ids, id)
	}

	if len(ids) == 0 {
		return c.Status(400).SendString("No valid IDs provided")
	}

	err := db.DeleteSections(ids)
	if err != nil {
		return c.Status(500).SendString("Failed to delete sections")
	}

	// Broadcast to WebSocket clients
	BroadcastUpdate("sections_deleted", map[string]interface{}{"ids": ids})

	// Return updated sections list for modal
	return returnSectionsForModal(c)
}

// Helper to split and trim string
func splitAndTrim(s, sep string) []string {
	var result []string
	for _, part := range splitString(s, sep) {
		trimmed := trimSpace(part)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

func splitString(s, sep string) []string {
	var result []string
	start := 0
	for i := 0; i < len(s); i++ {
		if i+len(sep) <= len(s) && s[i:i+len(sep)] == sep {
			result = append(result, s[start:i])
			start = i + len(sep)
			i += len(sep) - 1
		}
	}
	result = append(result, s[start:])
	return result
}

func trimSpace(s string) string {
	start := 0
	end := len(s)
	for start < end && (s[start] == ' ' || s[start] == '\t' || s[start] == '\n' || s[start] == '\r') {
		start++
	}
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t' || s[end-1] == '\n' || s[end-1] == '\r') {
		end--
	}
	return s[start:end]
}

// Helper to return sections for modal
func returnSectionsForModal(c *fiber.Ctx) error {
	sections, err := db.GetAllSections()
	if err != nil {
		return c.Status(500).SendString("Failed to fetch sections")
	}

	return c.Render("partials/manage_sections_list", fiber.Map{
		"Sections": sections,
	}, "")
}

// GetSectionsListForModal returns sections list for the management modal
func GetSectionsListForModal(c *fiber.Ctx) error {
	return returnSectionsForModal(c)
}
