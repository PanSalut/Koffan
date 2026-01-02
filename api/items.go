package api

import (
	"database/sql"
	"shopping-list/db"
	"shopping-list/handlers"

	"github.com/gofiber/fiber/v2"
)

const (
	MaxItemNameLength    = 200
	MaxDescriptionLength = 500
)

// GetItem returns a single item by ID
func GetItem(c *fiber.Ctx) error {
	id, err := c.ParamsInt("id")
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "invalid_id",
			Message: "Invalid item ID",
		})
	}

	item, err := db.GetItemByID(int64(id))
	if err != nil {
		if err == sql.ErrNoRows {
			return c.Status(fiber.StatusNotFound).JSON(ErrorResponse{
				Error:   "not_found",
				Message: "Item not found",
			})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
			Error:   "db_error",
			Message: "Failed to fetch item",
		})
	}

	return c.JSON(item)
}

// CreateItem creates a new item
func CreateItem(c *fiber.Ctx) error {
	var req CreateItemRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "invalid_json",
			Message: "Failed to parse request body",
		})
	}

	if req.Name == "" {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "validation_error",
			Message: "Name is required",
		})
	}

	if req.SectionID == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "validation_error",
			Message: "section_id is required",
		})
	}

	if len(req.Name) > MaxItemNameLength {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "validation_error",
			Message: "Name exceeds maximum length of 200 characters",
		})
	}

	if len(req.Description) > MaxDescriptionLength {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "validation_error",
			Message: "Description exceeds maximum length of 500 characters",
		})
	}

	// Check if section exists
	_, err := db.GetSectionByID(req.SectionID)
	if err != nil {
		if err == sql.ErrNoRows {
			return c.Status(fiber.StatusNotFound).JSON(ErrorResponse{
				Error:   "not_found",
				Message: "Section not found",
			})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
			Error:   "db_error",
			Message: "Failed to fetch section",
		})
	}

	item, err := db.CreateItem(req.SectionID, req.Name, req.Description)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
			Error:   "create_failed",
			Message: "Failed to create item",
		})
	}

	// Save to item history for suggestions
	db.SaveItemHistory(req.Name, req.SectionID)

	handlers.BroadcastUpdate("item_created", item)
	return c.Status(fiber.StatusCreated).JSON(item)
}

// UpdateItem updates an item
func UpdateItem(c *fiber.Ctx) error {
	id, err := c.ParamsInt("id")
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "invalid_id",
			Message: "Invalid item ID",
		})
	}

	var req UpdateItemRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "invalid_json",
			Message: "Failed to parse request body",
		})
	}

	// Get existing item
	existing, err := db.GetItemByID(int64(id))
	if err != nil {
		if err == sql.ErrNoRows {
			return c.Status(fiber.StatusNotFound).JSON(ErrorResponse{
				Error:   "not_found",
				Message: "Item not found",
			})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
			Error:   "db_error",
			Message: "Failed to fetch item",
		})
	}

	name := req.Name
	if name == "" {
		name = existing.Name
	}
	description := req.Description
	if description == "" && req.Name != "" {
		description = existing.Description
	}

	if len(name) > MaxItemNameLength {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "validation_error",
			Message: "Name exceeds maximum length of 200 characters",
		})
	}

	if len(description) > MaxDescriptionLength {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "validation_error",
			Message: "Description exceeds maximum length of 500 characters",
		})
	}

	item, err := db.UpdateItem(int64(id), name, description)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
			Error:   "update_failed",
			Message: "Failed to update item",
		})
	}

	handlers.BroadcastUpdate("item_updated", item)
	return c.JSON(item)
}

// DeleteItem deletes an item
func DeleteItem(c *fiber.Ctx) error {
	id, err := c.ParamsInt("id")
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "invalid_id",
			Message: "Invalid item ID",
		})
	}

	// Check if item exists
	_, err = db.GetItemByID(int64(id))
	if err != nil {
		if err == sql.ErrNoRows {
			return c.Status(fiber.StatusNotFound).JSON(ErrorResponse{
				Error:   "not_found",
				Message: "Item not found",
			})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
			Error:   "db_error",
			Message: "Failed to fetch item",
		})
	}

	if err := db.DeleteItem(int64(id)); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
			Error:   "delete_failed",
			Message: "Failed to delete item",
		})
	}

	handlers.BroadcastUpdate("item_deleted", map[string]int64{"id": int64(id)})
	return c.SendStatus(fiber.StatusNoContent)
}

// ToggleItemCompleted toggles the completed status
func ToggleItemCompleted(c *fiber.Ctx) error {
	id, err := c.ParamsInt("id")
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "invalid_id",
			Message: "Invalid item ID",
		})
	}

	// Check if item exists
	_, err = db.GetItemByID(int64(id))
	if err != nil {
		if err == sql.ErrNoRows {
			return c.Status(fiber.StatusNotFound).JSON(ErrorResponse{
				Error:   "not_found",
				Message: "Item not found",
			})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
			Error:   "db_error",
			Message: "Failed to fetch item",
		})
	}

	item, err := db.ToggleItemCompleted(int64(id))
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
			Error:   "toggle_failed",
			Message: "Failed to toggle item",
		})
	}

	handlers.BroadcastUpdate("item_toggled", item)
	return c.JSON(item)
}

// ToggleItemUncertain toggles the uncertain status
func ToggleItemUncertain(c *fiber.Ctx) error {
	id, err := c.ParamsInt("id")
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "invalid_id",
			Message: "Invalid item ID",
		})
	}

	// Check if item exists
	_, err = db.GetItemByID(int64(id))
	if err != nil {
		if err == sql.ErrNoRows {
			return c.Status(fiber.StatusNotFound).JSON(ErrorResponse{
				Error:   "not_found",
				Message: "Item not found",
			})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
			Error:   "db_error",
			Message: "Failed to fetch item",
		})
	}

	item, err := db.ToggleItemUncertain(int64(id))
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
			Error:   "toggle_failed",
			Message: "Failed to toggle item",
		})
	}

	handlers.BroadcastUpdate("item_updated", item)
	return c.JSON(item)
}

// MoveItem moves an item to a different section
func MoveItem(c *fiber.Ctx) error {
	id, err := c.ParamsInt("id")
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "invalid_id",
			Message: "Invalid item ID",
		})
	}

	var req MoveItemRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "invalid_json",
			Message: "Failed to parse request body",
		})
	}

	if req.SectionID == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "validation_error",
			Message: "section_id is required",
		})
	}

	// Check if item exists
	_, err = db.GetItemByID(int64(id))
	if err != nil {
		if err == sql.ErrNoRows {
			return c.Status(fiber.StatusNotFound).JSON(ErrorResponse{
				Error:   "not_found",
				Message: "Item not found",
			})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
			Error:   "db_error",
			Message: "Failed to fetch item",
		})
	}

	// Check if target section exists
	_, err = db.GetSectionByID(req.SectionID)
	if err != nil {
		if err == sql.ErrNoRows {
			return c.Status(fiber.StatusNotFound).JSON(ErrorResponse{
				Error:   "not_found",
				Message: "Target section not found",
			})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
			Error:   "db_error",
			Message: "Failed to fetch section",
		})
	}

	item, err := db.MoveItemToSection(int64(id), req.SectionID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
			Error:   "move_failed",
			Message: "Failed to move item",
		})
	}

	handlers.BroadcastUpdate("item_moved", item)
	return c.JSON(item)
}

// MoveItemUp moves an item up in sort order
func MoveItemUp(c *fiber.Ctx) error {
	id, err := c.ParamsInt("id")
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "invalid_id",
			Message: "Invalid item ID",
		})
	}

	// Check if item exists
	item, err := db.GetItemByID(int64(id))
	if err != nil {
		if err == sql.ErrNoRows {
			return c.Status(fiber.StatusNotFound).JSON(ErrorResponse{
				Error:   "not_found",
				Message: "Item not found",
			})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
			Error:   "db_error",
			Message: "Failed to fetch item",
		})
	}

	if err := db.MoveItemUp(int64(id)); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
			Error:   "move_failed",
			Message: "Failed to move item",
		})
	}

	handlers.BroadcastUpdate("items_reordered", map[string]int64{"section_id": item.SectionID})

	updatedItem, _ := db.GetItemByID(int64(id))
	return c.JSON(updatedItem)
}

// MoveItemDown moves an item down in sort order
func MoveItemDown(c *fiber.Ctx) error {
	id, err := c.ParamsInt("id")
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "invalid_id",
			Message: "Invalid item ID",
		})
	}

	// Check if item exists
	item, err := db.GetItemByID(int64(id))
	if err != nil {
		if err == sql.ErrNoRows {
			return c.Status(fiber.StatusNotFound).JSON(ErrorResponse{
				Error:   "not_found",
				Message: "Item not found",
			})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
			Error:   "db_error",
			Message: "Failed to fetch item",
		})
	}

	if err := db.MoveItemDown(int64(id)); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
			Error:   "move_failed",
			Message: "Failed to move item",
		})
	}

	handlers.BroadcastUpdate("items_reordered", map[string]int64{"section_id": item.SectionID})

	updatedItem, _ := db.GetItemByID(int64(id))
	return c.JSON(updatedItem)
}
