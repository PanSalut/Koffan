package api

import (
	"database/sql"
	"shopping-list/db"
	"shopping-list/handlers"

	"github.com/gofiber/fiber/v2"
)

// BatchCreate handles batch creation of lists, sections, and items
func BatchCreate(c *fiber.Ctx) error {
	var req BatchCreateRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "invalid_json",
			Message: "Failed to parse request body",
		})
	}

	// Determine which variant we're handling
	if req.List != nil {
		return batchCreateNewList(c, req)
	} else if req.ListID != 0 && len(req.Sections) > 0 {
		return batchAddToList(c, req)
	} else if req.SectionID != 0 && len(req.Items) > 0 {
		return batchAddToSection(c, req)
	}

	return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
		Error:   "validation_error",
		Message: "Request must contain either: list (new list), list_id + sections (add to existing list), or section_id + items (add to existing section)",
	})
}

// batchCreateNewList creates a new list with sections and items
func batchCreateNewList(c *fiber.Ctx, req BatchCreateRequest) error {
	if req.List.Name == "" {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "validation_error",
			Message: "List name is required",
		})
	}

	if len(req.List.Name) > MaxListNameLength {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "validation_error",
			Message: "List name exceeds maximum length of 100 characters",
		})
	}

	// Validate sections and items
	for _, s := range req.List.Sections {
		if s.Name == "" {
			return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
				Error:   "validation_error",
				Message: "Section name is required",
			})
		}
		if len(s.Name) > MaxSectionNameLength {
			return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
				Error:   "validation_error",
				Message: "Section name exceeds maximum length of 100 characters",
			})
		}
		for _, item := range s.Items {
			if item.Name == "" {
				return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
					Error:   "validation_error",
					Message: "Item name is required",
				})
			}
			if len(item.Name) > MaxItemNameLength {
				return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
					Error:   "validation_error",
					Message: "Item name exceeds maximum length of 200 characters",
				})
			}
			if len(item.Description) > MaxDescriptionLength {
				return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
					Error:   "validation_error",
					Message: "Item description exceeds maximum length of 500 characters",
				})
			}
		}
	}

	// Start transaction
	tx, err := db.DB.Begin()
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
			Error:   "db_error",
			Message: "Failed to start transaction",
		})
	}
	defer tx.Rollback()

	// Create list
	icon := NormalizeIcon(req.List.Icon)
	list, err := db.CreateListTx(tx, req.List.Name, icon)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
			Error:   "create_failed",
			Message: "Failed to create list",
		})
	}

	var sections []db.Section
	var items []db.Item

	// Create sections and items
	for sectionOrder, sectionInput := range req.List.Sections {
		section, err := db.CreateSectionForListTx(tx, list.ID, sectionInput.Name, sectionOrder)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
				Error:   "create_failed",
				Message: "Failed to create section: " + sectionInput.Name,
			})
		}

		var sectionItems []db.Item
		for itemOrder, itemInput := range sectionInput.Items {
			item, err := db.CreateItemTx(tx, section.ID, itemInput.Name, itemInput.Description, itemOrder)
			if err != nil {
				return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
					Error:   "create_failed",
					Message: "Failed to create item: " + itemInput.Name,
				})
			}
			sectionItems = append(sectionItems, *item)
			items = append(items, *item)

			// Save to item history
			db.SaveItemHistoryTx(tx, itemInput.Name, section.ID)
		}

		section.Items = sectionItems
		sections = append(sections, *section)
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
			Error:   "commit_failed",
			Message: "Failed to commit transaction",
		})
	}

	// Get list with stats
	list.Stats = db.GetListStats(list.ID)

	// Broadcast WebSocket update
	handlers.BroadcastUpdate("batch_created", map[string]interface{}{
		"list_id": list.ID,
	})

	return c.Status(fiber.StatusCreated).JSON(BatchCreateResponse{
		List:     list,
		Sections: sections,
		Items:    items,
	})
}

// batchAddToList adds sections and items to an existing list
func batchAddToList(c *fiber.Ctx, req BatchCreateRequest) error {
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

	// Validate sections and items
	for _, s := range req.Sections {
		if s.Name == "" {
			return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
				Error:   "validation_error",
				Message: "Section name is required",
			})
		}
		if len(s.Name) > MaxSectionNameLength {
			return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
				Error:   "validation_error",
				Message: "Section name exceeds maximum length of 100 characters",
			})
		}
		for _, item := range s.Items {
			if item.Name == "" {
				return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
					Error:   "validation_error",
					Message: "Item name is required",
				})
			}
			if len(item.Name) > MaxItemNameLength {
				return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
					Error:   "validation_error",
					Message: "Item name exceeds maximum length of 200 characters",
				})
			}
		}
	}

	// Start transaction
	tx, err := db.DB.Begin()
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
			Error:   "db_error",
			Message: "Failed to start transaction",
		})
	}
	defer tx.Rollback()

	var sections []db.Section
	var items []db.Item

	// Get max section order
	baseSectionOrder := db.GetMaxSectionOrderTx(tx, req.ListID) + 1

	// Create sections and items
	for i, sectionInput := range req.Sections {
		section, err := db.CreateSectionForListTx(tx, req.ListID, sectionInput.Name, baseSectionOrder+i)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
				Error:   "create_failed",
				Message: "Failed to create section: " + sectionInput.Name,
			})
		}

		var sectionItems []db.Item
		for itemOrder, itemInput := range sectionInput.Items {
			item, err := db.CreateItemTx(tx, section.ID, itemInput.Name, itemInput.Description, itemOrder)
			if err != nil {
				return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
					Error:   "create_failed",
					Message: "Failed to create item: " + itemInput.Name,
				})
			}
			sectionItems = append(sectionItems, *item)
			items = append(items, *item)

			db.SaveItemHistoryTx(tx, itemInput.Name, section.ID)
		}

		section.Items = sectionItems
		sections = append(sections, *section)
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
			Error:   "commit_failed",
			Message: "Failed to commit transaction",
		})
	}

	// Broadcast WebSocket update
	handlers.BroadcastUpdate("batch_created", map[string]interface{}{
		"list_id": req.ListID,
	})

	return c.Status(fiber.StatusCreated).JSON(BatchCreateResponse{
		Sections: sections,
		Items:    items,
	})
}

// batchAddToSection adds items to an existing section
func batchAddToSection(c *fiber.Ctx, req BatchCreateRequest) error {
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

	// Validate items
	for _, item := range req.Items {
		if item.Name == "" {
			return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
				Error:   "validation_error",
				Message: "Item name is required",
			})
		}
		if len(item.Name) > MaxItemNameLength {
			return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
				Error:   "validation_error",
				Message: "Item name exceeds maximum length of 200 characters",
			})
		}
	}

	// Start transaction
	tx, err := db.DB.Begin()
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
			Error:   "db_error",
			Message: "Failed to start transaction",
		})
	}
	defer tx.Rollback()

	var items []db.Item

	// Get max item order
	baseItemOrder := db.GetMaxItemOrderTx(tx, req.SectionID) + 1

	// Create items
	for i, itemInput := range req.Items {
		item, err := db.CreateItemTx(tx, req.SectionID, itemInput.Name, itemInput.Description, baseItemOrder+i)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
				Error:   "create_failed",
				Message: "Failed to create item: " + itemInput.Name,
			})
		}
		items = append(items, *item)

		db.SaveItemHistoryTx(tx, itemInput.Name, req.SectionID)
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
			Error:   "commit_failed",
			Message: "Failed to commit transaction",
		})
	}

	// Broadcast WebSocket update
	handlers.BroadcastUpdate("batch_created", map[string]interface{}{
		"section_id": req.SectionID,
	})

	return c.Status(fiber.StatusCreated).JSON(BatchCreateResponse{
		Items: items,
	})
}
