package api

import (
	"database/sql"
	"shopping-list/db"
	"shopping-list/handlers"

	"github.com/gofiber/fiber/v2"
)

const (
	MaxListNameLength = 100
	MaxIconLength     = 20
)

// GetLists returns all lists
func GetLists(c *fiber.Ctx) error {
	lists, err := db.GetAllLists()
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
			Error:   "db_error",
			Message: "Failed to fetch lists",
		})
	}
	return c.JSON(ListsResponse{Lists: lists})
}

// GetList returns a single list by ID
func GetList(c *fiber.Ctx) error {
	id, err := c.ParamsInt("id")
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "invalid_id",
			Message: "Invalid list ID",
		})
	}

	list, err := db.GetListByID(int64(id))
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

	return c.JSON(list)
}

// CreateList creates a new list
func CreateList(c *fiber.Ctx) error {
	var req CreateListRequest
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

	if len(req.Name) > MaxListNameLength {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "validation_error",
			Message: "Name exceeds maximum length of 100 characters",
		})
	}

	if len(req.Icon) > MaxIconLength {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "validation_error",
			Message: "Icon exceeds maximum length of 20 characters",
		})
	}

	icon := NormalizeIcon(req.Icon)
	list, err := db.CreateList(req.Name, icon)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
			Error:   "create_failed",
			Message: "Failed to create list",
		})
	}

	handlers.BroadcastUpdate("list_created", list)
	return c.Status(fiber.StatusCreated).JSON(list)
}

// UpdateList updates a list
func UpdateList(c *fiber.Ctx) error {
	id, err := c.ParamsInt("id")
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "invalid_id",
			Message: "Invalid list ID",
		})
	}

	var req UpdateListRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "invalid_json",
			Message: "Failed to parse request body",
		})
	}

	// Get existing list to check if it exists and for default values
	existing, err := db.GetListByID(int64(id))
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

	name := req.Name
	if name == "" {
		name = existing.Name
	}
	icon := req.Icon
	if icon == "" {
		icon = existing.Icon
	} else {
		icon = NormalizeIcon(icon)
	}

	if len(name) > MaxListNameLength {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "validation_error",
			Message: "Name exceeds maximum length of 100 characters",
		})
	}

	list, err := db.UpdateList(int64(id), name, icon)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
			Error:   "update_failed",
			Message: "Failed to update list",
		})
	}

	handlers.BroadcastUpdate("list_updated", list)
	return c.JSON(list)
}

// DeleteList deletes a list
func DeleteList(c *fiber.Ctx) error {
	id, err := c.ParamsInt("id")
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "invalid_id",
			Message: "Invalid list ID",
		})
	}

	// Check if list exists
	_, err = db.GetListByID(int64(id))
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

	if err := db.DeleteList(int64(id)); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
			Error:   "delete_failed",
			Message: "Failed to delete list",
		})
	}

	handlers.BroadcastUpdate("list_deleted", map[string]int64{"id": int64(id)})
	return c.SendStatus(fiber.StatusNoContent)
}

// GetListSections returns all sections for a list
func GetListSections(c *fiber.Ctx) error {
	id, err := c.ParamsInt("id")
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "invalid_id",
			Message: "Invalid list ID",
		})
	}

	// Check if list exists
	_, err = db.GetListByID(int64(id))
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

	sections, err := db.GetSectionsByList(int64(id))
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
			Error:   "db_error",
			Message: "Failed to fetch sections",
		})
	}

	return c.JSON(SectionsResponse{Sections: sections})
}

// MoveListUp moves a list up in sort order
func MoveListUp(c *fiber.Ctx) error {
	id, err := c.ParamsInt("id")
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "invalid_id",
			Message: "Invalid list ID",
		})
	}

	// Check if list exists
	_, err = db.GetListByID(int64(id))
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

	if err := db.MoveListUp(int64(id)); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
			Error:   "move_failed",
			Message: "Failed to move list",
		})
	}

	handlers.BroadcastUpdate("lists_reordered", nil)

	list, _ := db.GetListByID(int64(id))
	return c.JSON(list)
}

// MoveListDown moves a list down in sort order
func MoveListDown(c *fiber.Ctx) error {
	id, err := c.ParamsInt("id")
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "invalid_id",
			Message: "Invalid list ID",
		})
	}

	// Check if list exists
	_, err = db.GetListByID(int64(id))
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

	if err := db.MoveListDown(int64(id)); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
			Error:   "move_failed",
			Message: "Failed to move list",
		})
	}

	handlers.BroadcastUpdate("lists_reordered", nil)

	list, _ := db.GetListByID(int64(id))
	return c.JSON(list)
}
