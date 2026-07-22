package handlers

import (
	"shopping-list/db"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"
)

// GetAllData returns all sections with items and stats for offline caching
func GetAllData(c *fiber.Ctx) error {
	u, err := CurrentUser(c)
	if err != nil {
		return err
	}
	activeList, err := db.GetActiveListForUser(u.ID, u.IsAdmin)
	if err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "No accessible list"})
	}
	sections, err := db.GetSectionsByList(activeList.ID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to fetch data"})
	}

	stats := db.GetListStats(activeList.ID)

	return c.JSON(fiber.Map{
		"sections":  sections,
		"stats":     stats,
		"timestamp": time.Now().Unix(),
		"list_id":   activeList.ID,
		"user_id":   u.ID,
		"can_edit":  activeList.AccessRole != "viewer",
	})
}

func GetListActivity(c *fiber.Ctx) error {
	listID, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return fiber.ErrBadRequest
	}
	activity, err := db.GetListActivity(listID, 100)
	if err != nil {
		return err
	}
	return c.JSON(activity)
}
