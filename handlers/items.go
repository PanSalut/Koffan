package handlers

import (
	"database/sql"
	"log"
	"shopping-list/db"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
)

// retryOnBusy retries a database operation if it fails with SQLITE_BUSY
func retryOnBusy[T any](maxRetries int, operation func() (T, error)) (T, error) {
	var result T
	var err error
	for i := 0; i < maxRetries; i++ {
		result, err = operation()
		if err == nil {
			return result, nil
		}
		// Check if error is database locked (SQLITE_BUSY)
		if !strings.Contains(err.Error(), "database is locked") {
			return result, err
		}
		// Wait before retry with exponential backoff
		time.Sleep(time.Duration(10*(i+1)) * time.Millisecond)
	}
	return result, err
}

// CreateItem creates a new item in a section
func CreateItem(c *fiber.Ctx) error {
	sectionID, err := strconv.ParseInt(c.FormValue("section_id"), 10, 64)
	if err != nil {
		return sendError(c, 400, "error.invalid_section_id")
	}

	name := c.FormValue("name")
	if name == "" {
		return sendError(c, 400, "error.name_required")
	}

	description := c.FormValue("description")

	// Parse quantity (default to 0)
	quantity := 0
	if q := c.FormValue("quantity"); q != "" {
		if parsed, err := strconv.Atoi(q); err == nil && parsed >= 0 {
			quantity = parsed
		}
	}

	// Check if item with same name already exists in this section
	existing, findErr := db.FindItemByNameInSection(sectionID, name)
	if findErr != nil {
		return sendError(c, 500, "error.check_failed")
	}

	if existing != nil {
		if existing.Completed {
			// Reactivate: uncheck and update description/quantity if provided
			desc := description
			if desc == "" {
				desc = existing.Description
			}
			qty := quantity
			if qty == 0 {
				qty = existing.Quantity
			}
			item, err := db.ReactivateItem(existing.ID, desc, qty)
			if err != nil {
				return sendError(c, 500, "error.check_failed")
			}
			db.SaveItemHistory(name, sectionID)
			c.Set("X-Item-Reactivated", "true")
			BroadcastUpdate("item_toggled", item)
			c.Set("HX-Trigger-After-Settle", `{"statsRefresh":"true"}`)
			return c.Render("partials/item", fiber.Map{
				"Item":     item,
				"Sections": getSectionsForDropdown(),
			}, "")
		}
		// Item already active - signal to client
		c.Set("X-Item-Already-Active", "true")
		c.Set("X-Item-Existing-ID", strconv.FormatInt(existing.ID, 10))
		return c.SendStatus(200)
	}

	item, err := db.CreateItem(sectionID, name, description, quantity)
	if err != nil {
		return sendError(c, 500, "error.create_failed")
	}

	// Save to item history for auto-completion
	db.SaveItemHistory(name, sectionID)

	// Broadcast to WebSocket clients
	BroadcastUpdate("item_created", item)

	c.Set("HX-Trigger-After-Settle", `{"statsRefresh":"true"}`)

	// Quick add returns just the item partial for DOM append
	if c.FormValue("quick_add") == "true" {
		return c.Render("partials/item", fiber.Map{
			"Item":     item,
			"Sections": getSectionsForDropdown(),
		}, "")
	}

	// Regular form also returns per-item partial (client handles DOM insertion)
	return c.Render("partials/item", fiber.Map{
		"Item":     item,
		"Sections": getSectionsForDropdown(),
	}, "")
}

// UpdateItem updates an item's name, description and quantity
func UpdateItem(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return sendError(c, 400, "error.invalid_id")
	}

	name := c.FormValue("name")
	if name == "" {
		return sendError(c, 400, "error.name_required")
	}

	description := c.FormValue("description")

	// Get existing item to preserve quantity if not provided
	existing, err := db.GetItemByID(id)
	if err != nil {
		return sendError(c, 500, "error.fetch_failed")
	}

	// Parse quantity (preserve existing if not provided)
	quantity := existing.Quantity
	if q := c.FormValue("quantity"); q != "" {
		if parsed, err := strconv.Atoi(q); err == nil && parsed >= 0 {
			quantity = parsed
		}
	}

	item, err := db.UpdateItem(id, name, description, quantity)
	if err != nil {
		return sendError(c, 500, "error.update_failed")
	}

	// Broadcast to WebSocket clients
	BroadcastUpdate("item_updated", item)

	// Return individual item partial for smooth per-item swap
	c.Set("HX-Trigger-After-Settle", `{"statsRefresh":"true"}`)
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

// DeleteItem deletes an item
func DeleteItem(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return sendError(c, 400, "error.invalid_id")
	}

	item, err := db.GetItemByID(id)
	if err != nil {
		return sendError(c, 500, "error.fetch_failed")
	}

	err = db.DeleteItem(id)
	if err != nil {
		return sendError(c, 500, "error.delete_failed")
	}

	BroadcastUpdate("item_deleted", map[string]int64{"id": id, "section_id": item.SectionID})

	return c.SendStatus(200)
}

// DeleteCompletedItems deletes all completed items
func DeleteCompletedItems(c *fiber.Ctx) error {
	count, err := db.DeleteCompletedItems()
	if err != nil {
		return sendError(c, 500, "error.delete_failed")
	}

	// Broadcast to WebSocket clients
	BroadcastUpdate("completed_items_deleted", map[string]int64{"count": count})

	c.Set("HX-Trigger-After-Settle", `{"statsRefresh":"true"}`)
	return c.JSON(fiber.Map{"deleted": count})
}

// ToggleItem toggles the completed status of an item
func ToggleItem(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return sendError(c, 400, "error.invalid_id")
	}

	item, err := db.ToggleItemCompleted(id)
	if err != nil {
		return sendError(c, 500, "error.toggle_failed")
	}

	// Broadcast to WebSocket clients
	BroadcastUpdate("item_toggled", item)

	// Return per-item partial (no section swap - client handles DOM move)
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
		return sendError(c, 400, "error.invalid_id")
	}

	item, err := db.ToggleItemUncertain(id)
	if err != nil {
		return sendError(c, 500, "error.toggle_failed")
	}

	// Broadcast to WebSocket clients
	BroadcastUpdate("item_updated", item)

	// Return individual item partial for smooth per-item swap
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

// MoveItemToSection moves an item to a different section
// Optional parameter: position (index among active items in target section)
func MoveItemToSection(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return sendError(c, 400, "error.invalid_id")
	}

	newSectionID, err := strconv.ParseInt(c.FormValue("section_id"), 10, 64)
	if err != nil {
		return sendError(c, 400, "error.invalid_section_id")
	}

	// Get old section_id BEFORE moving
	oldItem, err := db.GetItemByID(id)
	if err != nil {
		return sendError(c, 404, "error.item_not_found")
	}
	fromSectionID := oldItem.SectionID

	var item *db.Item

	// Check if position parameter is provided (for cross-section drag-and-drop)
	positionStr := c.FormValue("position")
	if positionStr != "" {
		position, err := strconv.Atoi(positionStr)
		if err != nil {
			return sendError(c, 400, "error.invalid_position")
		}
		// Use retry for concurrent access protection
		item, err = retryOnBusy(3, func() (*db.Item, error) {
			return db.MoveItemToSectionAtPosition(id, newSectionID, position)
		})
		if err != nil {
			log.Printf("MoveItemToSection failed after retries: %v", err)
			return sendError(c, 500, "error.move_failed")
		}
	} else {
		item, err = db.MoveItemToSection(id, newSectionID)
		if err != nil {
			return sendError(c, 500, "error.move_failed")
		}
	}

	// Broadcast to WebSocket clients with both section IDs
	BroadcastUpdate("item_moved", map[string]interface{}{
		"id":              item.ID,
		"section_id":      item.SectionID,
		"from_section_id": fromSectionID,
	})

	// Return updated item partial so client can replace stale dropdown
	c.Set("HX-Trigger-After-Settle", `{"statsRefresh":"true"}`)
	return c.Render("partials/item", fiber.Map{
		"Item":     item,
		"Sections": getSectionsForDropdown(),
	}, "")
}

// MoveItemUp moves an item up in its section
func MoveItemUp(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return sendError(c, 400, "error.invalid_id")
	}

	err = db.MoveItemUp(id)
	if err != nil {
		return sendError(c, 500, "error.move_failed")
	}

	item, _ := db.GetItemByID(id)
	if item != nil {
		BroadcastUpdate("items_reordered", map[string]int64{"section_id": item.SectionID})
	}

	return c.SendStatus(200)
}

// MoveItemDown moves an item down in its section
func MoveItemDown(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return sendError(c, 400, "error.invalid_id")
	}

	err = db.MoveItemDown(id)
	if err != nil {
		return sendError(c, 500, "error.move_failed")
	}

	item, _ := db.GetItemByID(id)
	if item != nil {
		BroadcastUpdate("items_reordered", map[string]int64{"section_id": item.SectionID})
	}

	return c.SendStatus(200)
}

// Helper to return all items in a section
func returnSectionItems(c *fiber.Ctx, sectionID int64) error {
	section, err := db.GetSectionByID(sectionID)
	if err != nil {
		return sendError(c, 500, "error.fetch_failed")
	}

	return c.Render("partials/section", sectionRenderMap(section), "")
}

// GetItemHTML returns a single item rendered as HTML partial
func GetItemHTML(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return sendError(c, 400, "error.invalid_id")
	}

	item, err := db.GetItemByID(id)
	if err != nil {
		return sendError(c, 404, "error.item_not_found")
	}

	tmpl := "partials/item"
	if item.Completed {
		tmpl = "partials/item_completed"
	}

	return c.Render(tmpl, fiber.Map{
		"Item":     item,
		"Sections": getSectionsForDropdown(),
	}, "")
}

// CheckAllItems marks all active items in a section as completed
func CheckAllItems(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return sendError(c, 400, "error.invalid_section_id")
	}

	count, err := db.CheckAllItems(id)
	if err != nil {
		return sendError(c, 500, "error.check_failed")
	}

	BroadcastUpdate("section_items_checked", map[string]interface{}{"section_id": id, "count": count})

	return c.JSON(fiber.Map{"count": count, "section_id": id})
}

// UncheckAllItems marks all completed items in a section as active
func UncheckAllItems(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return sendError(c, 400, "error.invalid_section_id")
	}

	count, err := db.UncheckAllItems(id)
	if err != nil {
		return sendError(c, 500, "error.check_failed")
	}

	BroadcastUpdate("section_items_unchecked", map[string]interface{}{"section_id": id, "count": count})

	return c.JSON(fiber.Map{"count": count, "section_id": id})
}

// GetStats returns current stats as JSON (for Alpine.js updates)
func GetStats(c *fiber.Ctx) error {
	stats := db.GetStats()
	return c.JSON(stats)
}

// GetItemVersion returns the current updated_at timestamp for an item (for offline sync conflict resolution)
func GetItemVersion(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid ID"})
	}

	item, err := db.GetItemByID(id)
	if err != nil {
		if err == sql.ErrNoRows {
			return c.Status(404).JSON(fiber.Map{"error": "Item not found"})
		}
		log.Printf("GetItemVersion database error for item %d: %v", id, err)
		return c.Status(500).JSON(fiber.Map{"error": "Database error"})
	}

	return c.JSON(fiber.Map{
		"id":         item.ID,
		"updated_at": item.UpdatedAt,
		"completed":  item.Completed,
	})
}
