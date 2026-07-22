package handlers

import (
	"shopping-list/db"
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v2"
)

// GetSuggestions returns item name suggestions for auto-completion
func GetSuggestions(c *fiber.Ctx) error {
	u, err := CurrentUser(c)
	if err != nil {
		return err
	}
	query := c.Query("q")
	limitStr := c.Query("limit", "10")

	limit, err := strconv.Atoi(limitStr)
	if err != nil || limit <= 0 {
		limit = 10
	} else if limit > 100 {
		limit = 100 // Cap at reasonable maximum
	}

	// If no query, return all suggestions (for offline cache)
	if query == "" {
		suggestions, err := db.GetAllItemSuggestionsForUser(u.ID, limit)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "Failed to fetch suggestions"})
		}
		if suggestions == nil {
			suggestions = []db.ItemSuggestion{}
		}
		return c.JSON(suggestions)
	}

	suggestions, err := db.GetItemSuggestionsForUser(u.ID, query, limit)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to fetch suggestions"})
	}

	if suggestions == nil {
		suggestions = []db.ItemSuggestion{}
	}

	return c.JSON(suggestions)
}

// GetHistory returns all history items for management UI
func GetHistory(c *fiber.Ctx) error {
	u, err := CurrentUser(c)
	if err != nil {
		return err
	}
	items, err := db.GetItemHistoryListForUser(u.ID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to fetch history"})
	}

	if items == nil {
		items = []db.HistoryItem{}
	}

	return c.JSON(items)
}

// DeleteHistoryItem deletes a single item from history
func DeleteHistoryItem(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid ID"})
	}

	u, err := CurrentUser(c)
	if err != nil {
		return err
	}
	err = db.DeleteItemHistoryForUser(u.ID, id)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to delete history item"})
	}

	return c.JSON(fiber.Map{"success": true})
}

// BatchDeleteHistory deletes multiple items from history
func BatchDeleteHistory(c *fiber.Ctx) error {
	idsStr := c.FormValue("ids")
	if idsStr == "" {
		return c.Status(400).JSON(fiber.Map{"error": "No IDs provided"})
	}

	idStrings := strings.Split(idsStr, ",")
	if len(idStrings) > 100 {
		return c.Status(400).JSON(fiber.Map{"error": "Too many IDs (max 100)"})
	}
	ids := make([]int64, 0, len(idStrings))

	for _, idStr := range idStrings {
		id, err := strconv.ParseInt(strings.TrimSpace(idStr), 10, 64)
		if err != nil {
			continue
		}
		ids = append(ids, id)
	}

	if len(ids) == 0 {
		return c.Status(400).JSON(fiber.Map{"error": "No valid IDs provided"})
	}

	u, err := CurrentUser(c)
	if err != nil {
		return err
	}
	deleted, err := db.DeleteItemHistoryBatchForUser(u.ID, ids)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to delete history items"})
	}

	return c.JSON(fiber.Map{"deleted": deleted})
}
