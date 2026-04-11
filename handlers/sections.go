package handlers

import (
	"errors"
	"fmt"
	"shopping-list/db"
	"shopping-list/i18n"
	"strconv"

	"github.com/gofiber/fiber/v2"
)

var ErrInvalidListID = errors.New("invalid list_id")

// GetSectionHTML returns a single section rendered as HTML partial
func GetSectionHTML(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return sendError(c, 400, "error.invalid_id")
	}

	section, err := db.GetSectionByID(id)
	if err != nil {
		return sendError(c, 404, "error.section_not_found")
	}

	return c.Render("partials/section", sectionRenderMap(section), "")
}

// GetSections returns all sections with items (for full page render)
func GetSections(c *fiber.Ctx) error {
	sections, err := db.GetAllSections()
	if err != nil {
		return sendError(c, 500, "error.fetch_failed")
	}

	stats := db.GetStats()

	// Get lists for dropdown
	lists, _ := db.GetAllLists()
	activeList, _ := db.GetActiveList()

	return c.Render("list", fiber.Map{
		"Sections":     sections,
		"Stats":        stats,
		"Lists":        lists,
		"ActiveList":   activeList,
		"Translations": i18n.GetAllLocales(),
		"Locales":      i18n.AvailableLocales(),
		"DefaultLang":  i18n.GetDefaultLang(),
	})
}

// CreateSection creates a new section
func CreateSection(c *fiber.Ctx) error {
	name := c.FormValue("name")
	if name == "" {
		return sendError(c, 400, "error.name_required")
	}
	if len(name) > MaxSectionNameLength {
		return sendError(c, 400, "error.name_too_long")
	}
	if name == "[HISTORY]" {
		return sendError(c, 400, "common.reserved_name")
	}

	section, err := db.CreateSection(name)
	if err != nil {
		return sendError(c, 500, "error.create_failed")
	}

	// Broadcast to WebSocket clients
	BroadcastUpdate("section_created", section)

	// Return the new section partial for HTMX
	return c.Render("partials/section", sectionRenderMap(section), "")
}

// UpdateSection updates a section's name
func UpdateSection(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return sendError(c, 400, "error.invalid_id")
	}

	name := c.FormValue("name")
	if name == "" {
		return sendError(c, 400, "error.name_required")
	}
	if len(name) > MaxSectionNameLength {
		return sendError(c, 400, "error.name_too_long")
	}
	if name == "[HISTORY]" {
		return sendError(c, 400, "common.reserved_name")
	}

	section, err := db.UpdateSection(id, name)
	if err != nil {
		return sendError(c, 500, "error.update_failed")
	}

	// Broadcast to WebSocket clients
	BroadcastUpdate("section_updated", section)

	// Return appropriate partial based on context
	if c.Get("HX-Target") == "manage-sections-list" {
		return returnSectionsForModal(c)
	}

	// Return updated section partial for main list
	return c.Render("partials/section", sectionRenderMap(section), "")
}

// DeleteSection deletes a section and all its items
func DeleteSection(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return sendError(c, 400, "error.invalid_id")
	}

	err = db.DeleteSection(id)
	if err != nil {
		return sendError(c, 500, "error.delete_failed")
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
		return sendError(c, 400, "error.invalid_id")
	}

	err = db.MoveSectionUp(id)
	if err != nil {
		return sendError(c, 500, "error.move_failed")
	}

	BroadcastUpdate("sections_reordered", nil)

	// Modal expects full list, main page handles reorder via WS
	if c.Get("HX-Target") == "manage-sections-list" {
		return returnSectionsForModal(c)
	}
	return c.SendStatus(200)
}

// MoveSectionDown moves a section down in order
func MoveSectionDown(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return sendError(c, 400, "error.invalid_id")
	}

	err = db.MoveSectionDown(id)
	if err != nil {
		return sendError(c, 500, "error.move_failed")
	}

	BroadcastUpdate("sections_reordered", nil)

	if c.Get("HX-Target") == "manage-sections-list" {
		return returnSectionsForModal(c)
	}
	return c.SendStatus(200)
}

// UpdateSectionSortMode updates the sort mode of a section
func UpdateSectionSortMode(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return sendError(c, 400, "error.invalid_id")
	}

	sortMode := c.FormValue("sort_mode")
	if sortMode == "" {
		return sendError(c, 400, "error.sort_mode_required")
	}

	section, err := db.UpdateSectionSortMode(id, sortMode)
	if err != nil {
		return sendError(c, 500, "error.update_failed")
	}

	BroadcastUpdate("section_sort_changed", map[string]interface{}{"section_id": id, "sort_mode": sortMode})

	return c.Render("partials/section", sectionRenderMap(section), "")
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
		return sendError(c, 400, "error.no_ids")
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
		return sendError(c, 400, "error.no_valid_ids")
	}

	err := db.DeleteSections(ids)
	if err != nil {
		return sendError(c, 500, "error.delete_failed")
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

// getSectionsForList returns sections for a specific list (by list_id query param) or falls back to the active list.
func getSectionsForList(c *fiber.Ctx) ([]db.Section, error) {
	if listIDStr := c.Query("list_id"); listIDStr != "" {
		listID, err := strconv.ParseInt(listIDStr, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("%w: %s", ErrInvalidListID, listIDStr)
		}
		return db.GetSectionsByList(listID)
	}
	return db.GetAllSections()
}

// Helper to return sections for modal
func returnSectionsForModal(c *fiber.Ctx) error {
	sections, err := getSectionsForList(c)
	if err != nil {
		if errors.Is(err, ErrInvalidListID) {
			return sendError(c, 400, "error.invalid_list_id")
		}
		return sendError(c, 500, "error.fetch_failed")
	}

	return c.Render("partials/manage_sections_list", fiber.Map{
		"Sections": sections,
	}, "")
}

// GetSectionsListForModal returns sections list for the management modal
func GetSectionsListForModal(c *fiber.Ctx) error {
	// Check if JSON format is requested
	if c.Query("format") == "json" {
		sections, err := getSectionsForList(c)
		if err != nil {
			if errors.Is(err, ErrInvalidListID) {
				return c.Status(400).JSON(fiber.Map{"error": "Invalid list_id parameter"})
			}
			return c.Status(500).JSON(fiber.Map{"error": "Failed to fetch sections"})
		}
		// Return simplified JSON for select options
		type SectionOption struct {
			ID   int64  `json:"id"`
			Name string `json:"name"`
		}
		var options []SectionOption
		for _, s := range sections {
			options = append(options, SectionOption{ID: s.ID, Name: s.Name})
		}
		return c.JSON(options)
	}
	return returnSectionsForModal(c)
}
