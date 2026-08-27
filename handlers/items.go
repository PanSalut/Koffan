package handlers

import (
	"database/sql"
	"log"
	"shopping-list/db"
	"shopping-list/webhook"
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

func attributeItem(c *fiber.Ctx, item *db.Item, created bool) (*db.Item, error) {
	u, err := CurrentUser(c)
	if err != nil {
		return nil, err
	}
	if created {
		err = db.SetItemCreatedBy(item.ID, u.ID)
	} else {
		err = db.SetItemUpdatedBy(item.ID, u.ID)
	}
	if err != nil {
		return nil, err
	}
	return db.GetItemByID(item.ID)
}

func recordListActivity(c *fiber.Ctx, listID, itemID int64, action string, metadata interface{}) {
	u, err := CurrentUser(c)
	if err != nil {
		return
	}
	if err = db.LogListActivity(listID, itemID, u.ID, action, metadata); err != nil {
		log.Printf("Failed to record list activity %s: %v", action, err)
	}
}

func sectionsForItem(item *db.Item) []db.Section {
	listID, err := db.ListIDForSection(item.SectionID)
	if err != nil {
		return nil
	}
	return getSectionsForDropdown(listID)
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
	if len(name) > MaxItemNameLength {
		return c.Status(fiber.StatusBadRequest).SendString("Item name too long (max 200 characters)")
	}

	description := c.FormValue("description")
	if len(description) > MaxDescriptionLength {
		return c.Status(fiber.StatusBadRequest).SendString("Item description too long (max 500 characters)")
	}
	u, err := CurrentUser(c)
	if err != nil {
		return err
	}
	listID, err := db.ListIDForSection(sectionID)
	if err != nil {
		return fiber.ErrNotFound
	}
	allowed, err := db.CanEditList(u.ID, listID, u.IsAdmin)
	if err != nil || !allowed {
		return fiber.ErrForbidden
	}

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
			item, err = attributeItem(c, item, false)
			if err != nil {
				return sendError(c, 500, "error.update_failed")
			}
			recordListActivity(c, listID, item.ID, "item_reactivated", map[string]interface{}{"name": item.Name, "section_id": item.SectionID})
			db.SaveItemHistoryForUser(u.ID, name, sectionID)
			c.Set("X-Item-Reactivated", "true")
			BroadcastUpdate("item_toggled", item)
			NotifyItemWebhook(webhook.EventItemUpdated, item)
			c.Set("HX-Trigger-After-Settle", `{"statsRefresh":"true"}`)
			return c.Render("partials/item", fiber.Map{
				"Item":     item,
				"Sections": sectionsForItem(item),
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
	item, err = attributeItem(c, item, true)
	if err != nil {
		return sendError(c, 500, "error.update_failed")
	}
	recordListActivity(c, listID, item.ID, "item_created", map[string]interface{}{"name": item.Name, "section_id": item.SectionID, "quantity": item.Quantity})

	// Save to item history for auto-completion
	db.SaveItemHistoryForUser(u.ID, name, sectionID)

	// Broadcast to WebSocket clients
	BroadcastUpdate("item_created", item)
	NotifyItemWebhook(webhook.EventItemCreated, item)

	c.Set("HX-Trigger-After-Settle", `{"statsRefresh":"true"}`)

	// Quick add returns just the item partial for DOM append
	if c.FormValue("quick_add") == "true" {
		return c.Render("partials/item", fiber.Map{
			"Item":     item,
			"Sections": sectionsForItem(item),
		}, "")
	}

	// Regular form also returns per-item partial (client handles DOM insertion)
	return c.Render("partials/item", fiber.Map{
		"Item":     item,
		"Sections": sectionsForItem(item),
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
	if len(name) > MaxItemNameLength {
		return c.Status(fiber.StatusBadRequest).SendString("Item name too long (max 200 characters)")
	}

	description := c.FormValue("description")
	if len(description) > MaxDescriptionLength {
		return c.Status(fiber.StatusBadRequest).SendString("Item description too long (max 500 characters)")
	}

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
	item, err = attributeItem(c, item, false)
	if err != nil {
		return sendError(c, 500, "error.update_failed")
	}
	listID, _ := db.ListIDForSection(item.SectionID)
	recordListActivity(c, listID, item.ID, "item_updated", map[string]interface{}{"name": item.Name, "previous_name": existing.Name, "section_id": item.SectionID, "quantity": item.Quantity})

	// Broadcast to WebSocket clients
	BroadcastUpdate("item_updated", item)
	NotifyItemWebhook(webhook.EventItemUpdated, item)

	// Return individual item partial for smooth per-item swap
	c.Set("HX-Trigger-After-Settle", `{"statsRefresh":"true"}`)
	if item.Completed {
		return c.Render("partials/item_completed", fiber.Map{
			"Item":     item,
			"Sections": sectionsForItem(item),
		}, "")
	}
	return c.Render("partials/item", fiber.Map{
		"Item":     item,
		"Sections": sectionsForItem(item),
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
	listID, err := db.ListIDForSection(item.SectionID)
	if err != nil {
		return sendError(c, 500, "error.fetch_failed")
	}

	recordListActivity(c, listID, item.ID, "item_deleted", map[string]interface{}{"name": item.Name, "section_id": item.SectionID})
	err = db.DeleteItem(id)
	if err != nil {
		return sendError(c, 500, "error.delete_failed")
	}

	BroadcastListUpdate(listID, "item_deleted", map[string]int64{"id": id, "section_id": item.SectionID, "list_id": listID})
	NotifyItemWebhook(webhook.EventItemDeleted, item)

	return c.SendStatus(200)
}

// DeleteCompletedItems deletes all completed items
func DeleteCompletedItems(c *fiber.Ctx) error {
	u, err := CurrentUser(c)
	if err != nil {
		return err
	}
	activeList, err := db.GetActiveListForUser(u.ID, u.IsAdmin)
	if err != nil {
		return sendError(c, 404, "error.no_active_list")
	}
	canEdit, err := db.CanEditList(u.ID, activeList.ID, u.IsAdmin)
	if err != nil || !canEdit {
		return fiber.ErrForbidden
	}
	deletedItems, err := db.DeleteCompletedItemsForList(activeList.ID)
	if err != nil {
		return sendError(c, 500, "error.delete_failed")
	}
	count := len(deletedItems)

	// Broadcast to WebSocket clients
	BroadcastListUpdate(activeList.ID, "completed_items_deleted", map[string]interface{}{"count": count, "list_id": activeList.ID})
	NotifyItemWebhooks(webhook.EventItemDeleted, deletedItems)

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
	item, err = attributeItem(c, item, false)
	if err != nil {
		return sendError(c, 500, "error.update_failed")
	}
	listID, _ := db.ListIDForSection(item.SectionID)
	action := "item_uncompleted"
	if item.Completed {
		action = "item_completed"
	}
	recordListActivity(c, listID, item.ID, action, map[string]interface{}{"name": item.Name, "section_id": item.SectionID})
	// Broadcast to WebSocket clients
	BroadcastUpdate("item_toggled", item)
	event := webhook.EventItemUpdated
	if item.Completed {
		event = webhook.EventItemCompleted
	}
	NotifyItemWebhook(event, item)

	// Return per-item partial (no section swap - client handles DOM move)
	if item.Completed {
		return c.Render("partials/item_completed", fiber.Map{
			"Item":     item,
			"Sections": sectionsForItem(item),
		}, "")
	}
	return c.Render("partials/item", fiber.Map{
		"Item":     item,
		"Sections": sectionsForItem(item),
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
	item, err = attributeItem(c, item, false)
	if err != nil {
		return sendError(c, 500, "error.update_failed")
	}
	listID, _ := db.ListIDForSection(item.SectionID)
	recordListActivity(c, listID, item.ID, "item_uncertain_changed", map[string]interface{}{"name": item.Name, "uncertain": item.Uncertain, "section_id": item.SectionID})

	// Broadcast to WebSocket clients
	BroadcastUpdate("item_updated", item)
	NotifyItemWebhook(webhook.EventItemUpdated, item)

	// Return individual item partial for smooth per-item swap
	if item.Completed {
		return c.Render("partials/item_completed", fiber.Map{
			"Item":     item,
			"Sections": sectionsForItem(item),
		}, "")
	}
	return c.Render("partials/item", fiber.Map{
		"Item":     item,
		"Sections": sectionsForItem(item),
	}, "")
}

// AdjustItemQuantity adjusts an item's quantity via delta or absolute value.
// Body: {"delta": 1} | {"delta": -1} | {"quantity": 5}. Clamped to >= 0.
func AdjustItemQuantity(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return sendError(c, 400, "error.invalid_id")
	}

	var req struct {
		Delta    int  `json:"delta,omitempty"`
		Quantity *int `json:"quantity,omitempty"`
	}
	if err := c.BodyParser(&req); err != nil {
		return sendError(c, 400, "error.invalid_request")
	}

	if req.Quantity == nil && req.Delta == 0 {
		return sendError(c, 400, "error.invalid_request")
	}

	item, err := db.AdjustItemQuantity(id, req.Delta, req.Quantity)
	if err != nil {
		if err == sql.ErrNoRows {
			return sendError(c, 404, "error.item_not_found")
		}
		return sendError(c, 500, "error.update_failed")
	}
	item, err = attributeItem(c, item, false)
	if err != nil {
		return sendError(c, 500, "error.update_failed")
	}
	listID, _ := db.ListIDForSection(item.SectionID)
	recordListActivity(c, listID, item.ID, "item_quantity_changed", map[string]interface{}{"name": item.Name, "quantity": item.Quantity, "section_id": item.SectionID})

	BroadcastUpdate("item_updated", item)
	NotifyItemWebhook(webhook.EventItemUpdated, item)

	if item.Completed {
		return c.Render("partials/item_completed", fiber.Map{
			"Item":     item,
			"Sections": sectionsForItem(item),
		}, "")
	}
	return c.Render("partials/item", fiber.Map{
		"Item":     item,
		"Sections": sectionsForItem(item),
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
	fromListID, err := db.ListIDForSection(fromSectionID)
	if err != nil {
		return sendError(c, 404, "error.item_not_found")
	}
	toListID, err := db.ListIDForSection(newSectionID)
	if err != nil {
		return sendError(c, 404, "error.section_not_found")
	}
	if fromListID != toListID {
		return fiber.ErrForbidden
	}

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
	item, err = attributeItem(c, item, false)
	if err != nil {
		return sendError(c, 500, "error.update_failed")
	}
	listID, _ := db.ListIDForSection(item.SectionID)
	recordListActivity(c, listID, item.ID, "item_moved", map[string]interface{}{"name": item.Name, "from_section_id": fromSectionID, "section_id": item.SectionID})

	// Broadcast to WebSocket clients with both section IDs
	BroadcastListUpdate(listID, "item_moved", map[string]interface{}{
		"id":              item.ID,
		"section_id":      item.SectionID,
		"from_section_id": fromSectionID,
		"list_id":         listID,
	})
	NotifyItemWebhook(webhook.EventItemUpdated, item)

	// Return updated item partial so client can replace stale dropdown
	c.Set("HX-Trigger-After-Settle", `{"statsRefresh":"true"}`)
	return c.Render("partials/item", fiber.Map{
		"Item":     item,
		"Sections": sectionsForItem(item),
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
	if item, e := db.GetItemByID(id); e == nil {
		_, _ = attributeItem(c, item, false)
	}

	item, _ := db.GetItemByID(id)
	if item != nil {
		listID, _ := db.ListIDForSection(item.SectionID)
		BroadcastListUpdate(listID, "items_reordered", map[string]int64{"section_id": item.SectionID, "list_id": listID})
		NotifyItemWebhook(webhook.EventItemUpdated, item)
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
	if item, e := db.GetItemByID(id); e == nil {
		_, _ = attributeItem(c, item, false)
	}

	item, _ := db.GetItemByID(id)
	if item != nil {
		listID, _ := db.ListIDForSection(item.SectionID)
		BroadcastListUpdate(listID, "items_reordered", map[string]int64{"section_id": item.SectionID, "list_id": listID})
		NotifyItemWebhook(webhook.EventItemUpdated, item)
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
		"Sections": sectionsForItem(item),
	}, "")
}

// CheckAllItems marks all active items in a section as completed
func CheckAllItems(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return sendError(c, 400, "error.invalid_section_id")
	}

	u, err := CurrentUser(c)
	if err != nil {
		return err
	}
	items, err := db.CheckAllItemsByUser(id, u.ID)
	if err != nil {
		return sendError(c, 500, "error.check_failed")
	}
	count := len(items)

	listID, _ := db.ListIDForSection(id)
	BroadcastListUpdate(listID, "section_items_checked", map[string]interface{}{"section_id": id, "count": count, "list_id": listID})
	NotifyItemWebhooks(webhook.EventItemCompleted, items)

	return c.JSON(fiber.Map{"count": count, "section_id": id})
}

// UncheckAllItems marks all completed items in a section as active
func UncheckAllItems(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return sendError(c, 400, "error.invalid_section_id")
	}

	u, err := CurrentUser(c)
	if err != nil {
		return err
	}
	items, err := db.UncheckAllItemsByUser(id, u.ID)
	if err != nil {
		return sendError(c, 500, "error.check_failed")
	}
	count := len(items)

	listID, _ := db.ListIDForSection(id)
	BroadcastListUpdate(listID, "section_items_unchecked", map[string]interface{}{"section_id": id, "count": count, "list_id": listID})
	NotifyItemWebhooks(webhook.EventItemUpdated, items)

	return c.JSON(fiber.Map{"count": count, "section_id": id})
}

// GetStats returns current stats as JSON (for Alpine.js updates)
func GetStats(c *fiber.Ctx) error {
	u, err := CurrentUser(c)
	if err != nil {
		return err
	}
	list, err := db.GetActiveListForUser(u.ID, u.IsAdmin)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "No accessible list"})
	}
	stats := db.GetListStats(list.ID)
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
