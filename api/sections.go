package api

import (
	"database/sql"
	"shopping-list/db"
	"shopping-list/handlers"

	"github.com/gofiber/fiber/v2"
)

const (
	MaxSectionNameLength = 100
)

// GetSection returns a single section by ID
func GetSection(c *fiber.Ctx) error {
	id, err := c.ParamsInt("id")
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "invalid_id",
			Message: "Invalid section ID",
		})
	}

	section, err := db.GetSectionByID(int64(id))
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

	return c.JSON(section)
}

// CreateSection creates a new section
func CreateSection(c *fiber.Ctx) error {
	var req CreateSectionRequest
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

	if req.ListID == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "validation_error",
			Message: "list_id is required",
		})
	}

	if len(req.Name) > MaxSectionNameLength {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "validation_error",
			Message: "Name exceeds maximum length of 100 characters",
		})
	}

	// Check if list exists
	_, err := db.GetListByID(req.ListID)
	if err != nil {
		if err == sql.ErrNoRows {
			return c.Status(fiber.StatusNotFound).JSON(ErrorResponse{
				Error:   "not_found",
				Message: "List not found",
			})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
			Error:   "db_error",
			Message: "Failed to fetch list",
		})
	}

	section, err := db.CreateSectionForList(req.ListID, req.Name)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
			Error:   "create_failed",
			Message: "Failed to create section",
		})
	}

	handlers.BroadcastUpdate("section_created", section)
	return c.Status(fiber.StatusCreated).JSON(section)
}

// UpdateSection updates a section
func UpdateSection(c *fiber.Ctx) error {
	id, err := c.ParamsInt("id")
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "invalid_id",
			Message: "Invalid section ID",
		})
	}

	var req UpdateSectionRequest
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

	if len(req.Name) > MaxSectionNameLength {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "validation_error",
			Message: "Name exceeds maximum length of 100 characters",
		})
	}

	// Check if section exists
	_, err = db.GetSectionByID(int64(id))
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

	section, err := db.UpdateSection(int64(id), req.Name)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
			Error:   "update_failed",
			Message: "Failed to update section",
		})
	}

	handlers.BroadcastUpdate("section_updated", section)
	return c.JSON(section)
}

// DeleteSection deletes a section
func DeleteSection(c *fiber.Ctx) error {
	id, err := c.ParamsInt("id")
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "invalid_id",
			Message: "Invalid section ID",
		})
	}

	// Check if section exists
	_, err = db.GetSectionByID(int64(id))
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

	if err := db.DeleteSection(int64(id)); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
			Error:   "delete_failed",
			Message: "Failed to delete section",
		})
	}

	handlers.BroadcastUpdate("section_deleted", map[string]int64{"id": int64(id)})
	return c.SendStatus(fiber.StatusNoContent)
}

// GetSectionItems returns all items for a section
func GetSectionItems(c *fiber.Ctx) error {
	id, err := c.ParamsInt("id")
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "invalid_id",
			Message: "Invalid section ID",
		})
	}

	// Check if section exists
	_, err = db.GetSectionByID(int64(id))
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

	items, err := db.GetItemsBySection(int64(id))
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
			Error:   "db_error",
			Message: "Failed to fetch items",
		})
	}

	return c.JSON(ItemsResponse{Items: items})
}

// MoveSectionUp moves a section up in sort order
func MoveSectionUp(c *fiber.Ctx) error {
	id, err := c.ParamsInt("id")
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "invalid_id",
			Message: "Invalid section ID",
		})
	}

	// Check if section exists
	_, err = db.GetSectionByID(int64(id))
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

	if err := db.MoveSectionUp(int64(id)); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
			Error:   "move_failed",
			Message: "Failed to move section",
		})
	}

	handlers.BroadcastUpdate("sections_reordered", nil)

	section, _ := db.GetSectionByID(int64(id))
	return c.JSON(section)
}

// MoveSectionDown moves a section down in sort order
func MoveSectionDown(c *fiber.Ctx) error {
	id, err := c.ParamsInt("id")
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "invalid_id",
			Message: "Invalid section ID",
		})
	}

	// Check if section exists
	_, err = db.GetSectionByID(int64(id))
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

	if err := db.MoveSectionDown(int64(id)); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
			Error:   "move_failed",
			Message: "Failed to move section",
		})
	}

	handlers.BroadcastUpdate("sections_reordered", nil)

	section, _ := db.GetSectionByID(int64(id))
	return c.JSON(section)
}
