package handlers

import (
	"shopping-list/db"
	"strconv"

	"github.com/gofiber/fiber/v2"
)

// CreateItem creates a new item in a section
func CreateItem(c *fiber.Ctx) error {
	sectionID, err := strconv.ParseInt(c.FormValue("section_id"), 10, 64)
	if err != nil {
		return c.Status(400).SendString("Invalid section ID")
	}

	name := c.FormValue("name")
	if name == "" {
		return c.Status(400).SendString("Name is required")
	}

	description := c.FormValue("description")

	item, err := db.CreateItem(sectionID, name, description)
	if err != nil {
		return c.Status(500).SendString("Failed to create item")
	}

	// Broadcast to WebSocket clients
	BroadcastUpdate("item_created", item)

	// Return the new item partial for HTMX
	return c.Render("partials/item", fiber.Map{
		"Item":     item,
		"Sections": getSectionsForDropdown(),
	}, "")
}

// UpdateItem updates an item's name and description
func UpdateItem(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return c.Status(400).SendString("Invalid ID")
	}

	name := c.FormValue("name")
	if name == "" {
		return c.Status(400).SendString("Name is required")
	}

	description := c.FormValue("description")

	item, err := db.UpdateItem(id, name, description)
	if err != nil {
		return c.Status(500).SendString("Failed to update item")
	}

	// Broadcast to WebSocket clients
	BroadcastUpdate("item_updated", item)

	// Return updated item partial
	return c.Render("partials/item", fiber.Map{
		"Item":     item,
		"Sections": getSectionsForDropdown(),
	}, "")
}

// DeleteItem deletes an item
func DeleteItem(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return c.Status(400).SendString("Invalid ID")
	}

	err = db.DeleteItem(id)
	if err != nil {
		return c.Status(500).SendString("Failed to delete item")
	}

	// Broadcast to WebSocket clients
	BroadcastUpdate("item_deleted", map[string]int64{"id": id})

	// Return empty string (HTMX will remove the element)
	return c.SendString("")
}

// ToggleItem toggles the completed status of an item
func ToggleItem(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return c.Status(400).SendString("Invalid ID")
	}

	item, err := db.ToggleItemCompleted(id)
	if err != nil {
		return c.Status(500).SendString("Failed to toggle item")
	}

	// Broadcast to WebSocket clients
	BroadcastUpdate("item_toggled", item)

	// Return the appropriate item partial based on completed status
	if item.Completed {
		return c.Render("partials/item_completed", fiber.Map{
			"Item":     item,
			"Sections": getSectionsForDropdown(),
		}, "")
	}
	return c.Render("partials/item", fiber.Map{
		"Item":     item,
		"Sections": getSectionsForDropdown(),
	}, "")
}

// ToggleUncertain toggles the uncertain status of an item
func ToggleUncertain(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return c.Status(400).SendString("Invalid ID")
	}

	item, err := db.ToggleItemUncertain(id)
	if err != nil {
		return c.Status(500).SendString("Failed to toggle uncertain")
	}

	// Broadcast to WebSocket clients
	BroadcastUpdate("item_updated", item)

	// Return updated item partial
	return c.Render("partials/item", fiber.Map{
		"Item":     item,
		"Sections": getSectionsForDropdown(),
	}, "")
}

// MoveItemToSection moves an item to a different section
func MoveItemToSection(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return c.Status(400).SendString("Invalid ID")
	}

	newSectionID, err := strconv.ParseInt(c.FormValue("section_id"), 10, 64)
	if err != nil {
		return c.Status(400).SendString("Invalid section ID")
	}

	item, err := db.MoveItemToSection(id, newSectionID)
	if err != nil {
		return c.Status(500).SendString("Failed to move item")
	}

	// Broadcast to WebSocket clients
	BroadcastUpdate("item_moved", item)

	// Trigger full refresh for simplicity (item moved between sections)
	c.Set("HX-Trigger", "refreshList")
	return c.SendString("")
}

// MoveItemUp moves an item up in its section
func MoveItemUp(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return c.Status(400).SendString("Invalid ID")
	}

	err = db.MoveItemUp(id)
	if err != nil {
		return c.Status(500).SendString("Failed to move item")
	}

	// Get the item's section and return all items in that section
	item, _ := db.GetItemByID(id)
	if item != nil {
		BroadcastUpdate("items_reordered", map[string]int64{"section_id": item.SectionID})
		return returnSectionItems(c, item.SectionID)
	}

	return c.SendString("")
}

// MoveItemDown moves an item down in its section
func MoveItemDown(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return c.Status(400).SendString("Invalid ID")
	}

	err = db.MoveItemDown(id)
	if err != nil {
		return c.Status(500).SendString("Failed to move item")
	}

	// Get the item's section and return all items in that section
	item, _ := db.GetItemByID(id)
	if item != nil {
		BroadcastUpdate("items_reordered", map[string]int64{"section_id": item.SectionID})
		return returnSectionItems(c, item.SectionID)
	}

	return c.SendString("")
}

// Helper to return all items in a section
func returnSectionItems(c *fiber.Ctx, sectionID int64) error {
	section, err := db.GetSectionByID(sectionID)
	if err != nil {
		return c.Status(500).SendString("Failed to fetch section")
	}

	return c.Render("partials/section", fiber.Map{
		"Section":  section,
		"Sections": getSectionsForDropdown(),
	}, "")
}

// GetStats returns current stats as JSON (for Alpine.js updates)
func GetStats(c *fiber.Ctx) error {
	stats := db.GetStats()
	return c.JSON(stats)
}
