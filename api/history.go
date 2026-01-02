package api

import (
	"shopping-list/db"

	"github.com/gofiber/fiber/v2"
)

// HistoryResponse wraps multiple history items
type HistoryResponse struct {
	Items []db.HistoryItem `json:"items"`
}

// CreateHistoryRequest for adding a new history entry
type CreateHistoryRequest struct {
	Name      string `json:"name"`
	SectionID int64  `json:"section_id,omitempty"`
}

// BatchDeleteHistoryRequest for deleting multiple history entries
type BatchDeleteHistoryRequest struct {
	IDs []int64 `json:"ids"`
}

// GetHistory returns all history items
func GetHistory(c *fiber.Ctx) error {
	items, err := db.GetItemHistoryList()
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
			Error:   "db_error",
			Message: "Failed to fetch history",
		})
	}

	if items == nil {
		items = []db.HistoryItem{}
	}

	return c.JSON(HistoryResponse{Items: items})
}

// CreateHistory adds a new item to history
func CreateHistory(c *fiber.Ctx) error {
	var req CreateHistoryRequest
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

	if len(req.Name) > MaxItemNameLength {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "validation_error",
			Message: "Name exceeds maximum length of 200 characters",
		})
	}

	// If section_id provided, verify it exists
	if req.SectionID != 0 {
		_, err := db.GetSectionByID(req.SectionID)
		if err != nil {
			return c.Status(fiber.StatusNotFound).JSON(ErrorResponse{
				Error:   "not_found",
				Message: "Section not found",
			})
		}
	}

	if err := db.SaveItemHistory(req.Name, req.SectionID); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
			Error:   "create_failed",
			Message: "Failed to save history",
		})
	}

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"message": "History entry created",
		"name":    req.Name,
	})
}

// DeleteHistory deletes a single history entry
func DeleteHistory(c *fiber.Ctx) error {
	id, err := c.ParamsInt("id")
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "invalid_id",
			Message: "Invalid history ID",
		})
	}

	if err := db.DeleteItemHistory(int64(id)); err != nil {
		return c.Status(fiber.StatusNotFound).JSON(ErrorResponse{
			Error:   "not_found",
			Message: "History entry not found",
		})
	}

	return c.SendStatus(fiber.StatusNoContent)
}

// BatchDeleteHistory deletes multiple history entries
func BatchDeleteHistory(c *fiber.Ctx) error {
	var req BatchDeleteHistoryRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "invalid_json",
			Message: "Failed to parse request body",
		})
	}

	if len(req.IDs) == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "validation_error",
			Message: "IDs array is required",
		})
	}

	deleted, err := db.DeleteItemHistoryBatch(req.IDs)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
			Error:   "delete_failed",
			Message: "Failed to delete history entries",
		})
	}

	return c.JSON(fiber.Map{
		"deleted": deleted,
	})
}
