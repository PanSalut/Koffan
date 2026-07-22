package handlers

import (
	"database/sql"
	"shopping-list/db"
	"strconv"

	"github.com/gofiber/fiber/v2"
)

func requireTemplateManage(c *fiber.Ctx, templateID int64) (*db.User, error) {
	u, err := CurrentUser(c)
	if err != nil {
		return nil, err
	}
	allowed, err := db.CanManageTemplate(u.ID, templateID, u.IsAdmin)
	if err != nil {
		return nil, err
	}
	if !allowed {
		return nil, fiber.ErrNotFound
	}
	return u, nil
}

func requireTemplateItemManage(c *fiber.Ctx, itemID int64) (*db.User, error) {
	item, err := db.GetTemplateItemByID(itemID)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fiber.ErrNotFound
		}
		return nil, err
	}
	return requireTemplateManage(c, item.TemplateID)
}

// GetTemplates returns all templates
func GetTemplates(c *fiber.Ctx) error {
	u, err := CurrentUser(c)
	if err != nil {
		return err
	}
	templates, err := db.GetTemplatesForUser(u.ID, u.IsAdmin)
	if err != nil {
		return sendError(c, 500, "error.fetch_failed")
	}

	// Check if JSON format is requested
	if c.Query("format") == "json" {
		return c.JSON(templates)
	}

	return c.Render("partials/templates_list", fiber.Map{
		"Templates": templates,
	}, "")
}

// GetTemplate returns a single template with items
func GetTemplate(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return sendError(c, 400, "error.invalid_id")
	}
	if _, err = requireTemplateManage(c, id); err != nil {
		return err
	}

	template, err := db.GetTemplateByID(id)
	if err != nil {
		return sendError(c, 404, "error.template_not_found")
	}

	// Check if JSON format is requested
	if c.Query("format") == "json" {
		return c.JSON(template)
	}

	return c.Render("partials/template_detail", fiber.Map{
		"Template": template,
	}, "")
}

// CreateTemplate creates a new template
func CreateTemplate(c *fiber.Ctx) error {
	u, err := CurrentUser(c)
	if err != nil {
		return err
	}
	name := c.FormValue("name")
	if name == "" {
		return sendError(c, 400, "error.name_required")
	}

	description := c.FormValue("description")

	template, err := db.CreateTemplateForUser(u.ID, name, description)
	if err != nil {
		return sendError(c, 500, "error.create_failed")
	}

	// Return the new template partial
	return c.Render("partials/template_item", fiber.Map{
		"Template": template,
	}, "")
}

// UpdateTemplate updates a template's name and description
func UpdateTemplate(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return sendError(c, 400, "error.invalid_id")
	}
	if _, err = requireTemplateManage(c, id); err != nil {
		return err
	}

	name := c.FormValue("name")
	if name == "" {
		return sendError(c, 400, "error.name_required")
	}

	description := c.FormValue("description")

	template, err := db.UpdateTemplate(id, name, description)
	if err != nil {
		return sendError(c, 500, "error.update_failed")
	}

	// Return updated template partial
	return c.Render("partials/template_item", fiber.Map{
		"Template": template,
	}, "")
}

// DeleteTemplate deletes a template
func DeleteTemplate(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return sendError(c, 400, "error.invalid_id")
	}
	if _, err = requireTemplateManage(c, id); err != nil {
		return err
	}

	err = db.DeleteTemplate(id)
	if err != nil {
		return sendError(c, 500, "error.delete_failed")
	}

	return c.SendString("")
}

// AddTemplateItem adds an item to a template
func AddTemplateItem(c *fiber.Ctx) error {
	templateID, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return sendError(c, 400, "error.invalid_id")
	}
	if _, err = requireTemplateManage(c, templateID); err != nil {
		return err
	}

	sectionName := c.FormValue("section_name")
	if sectionName == "" {
		return sendError(c, 400, "error.section_name_required")
	}

	name := c.FormValue("name")
	if name == "" {
		return sendError(c, 400, "error.item_name_required")
	}

	description := c.FormValue("description")

	item, err := db.AddTemplateItem(templateID, sectionName, name, description)
	if err != nil {
		return sendError(c, 500, "error.create_failed")
	}

	// Return the template item partial
	return c.Render("partials/template_item_row", fiber.Map{
		"Item": item,
	}, "")
}

// UpdateTemplateItem updates a template item
func UpdateTemplateItem(c *fiber.Ctx) error {
	itemID, err := strconv.ParseInt(c.Params("itemId"), 10, 64)
	if err != nil {
		return sendError(c, 400, "error.invalid_id")
	}
	if _, err = requireTemplateItemManage(c, itemID); err != nil {
		return err
	}

	sectionName := c.FormValue("section_name")
	if sectionName == "" {
		return sendError(c, 400, "error.section_name_required")
	}

	name := c.FormValue("name")
	if name == "" {
		return sendError(c, 400, "error.item_name_required")
	}

	description := c.FormValue("description")

	item, err := db.UpdateTemplateItem(itemID, sectionName, name, description)
	if err != nil {
		return sendError(c, 500, "error.update_failed")
	}

	return c.Render("partials/template_item_row", fiber.Map{
		"Item": item,
	}, "")
}

// DeleteTemplateItem deletes a template item
func DeleteTemplateItem(c *fiber.Ctx) error {
	itemID, err := strconv.ParseInt(c.Params("itemId"), 10, 64)
	if err != nil {
		return sendError(c, 400, "error.invalid_id")
	}
	if _, err = requireTemplateItemManage(c, itemID); err != nil {
		return err
	}

	err = db.DeleteTemplateItem(itemID)
	if err != nil {
		return sendError(c, 500, "error.delete_failed")
	}

	return c.SendString("")
}

// ApplyTemplate applies a template to the active list
func ApplyTemplate(c *fiber.Ctx) error {
	templateID, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return sendError(c, 400, "error.invalid_id")
	}
	u, err := requireTemplateManage(c, templateID)
	if err != nil {
		return err
	}

	activeList, err := db.GetActiveListForUser(u.ID, u.IsAdmin)
	if err != nil {
		return sendError(c, 500, "error.no_active_list")
	}

	canEdit, err := db.CanEditList(u.ID, activeList.ID, u.IsAdmin)
	if err != nil || !canEdit {
		return fiber.ErrForbidden
	}
	err = db.ApplyTemplateToListAsUser(templateID, activeList.ID, u.ID)
	if err != nil {
		return sendError(c, 500, "error.apply_failed")
	}

	// Broadcast to WebSocket clients
	BroadcastUpdate("template_applied", map[string]int64{
		"template_id": templateID,
		"list_id":     activeList.ID,
	})

	// Trigger full refresh - template adds items to multiple sections
	c.Set("HX-Trigger-After-Settle", `{"statsRefresh":"true","refreshList":"true"}`)
	return c.SendString("")
}

// CreateTemplateFromList creates a template from the active list
func CreateTemplateFromList(c *fiber.Ctx) error {
	u, err := CurrentUser(c)
	if err != nil {
		return err
	}
	name := c.FormValue("name")
	if name == "" {
		return sendError(c, 400, "error.template_name_required")
	}

	description := c.FormValue("description")

	activeList, err := db.GetActiveListForUser(u.ID, u.IsAdmin)
	if err != nil {
		return sendError(c, 500, "error.no_active_list")
	}

	canEdit, err := db.CanEditList(u.ID, activeList.ID, u.IsAdmin)
	if err != nil || !canEdit {
		return fiber.ErrForbidden
	}
	template, err := db.CreateTemplateFromListForUser(activeList.ID, u.ID, name, description)
	if err != nil {
		return sendError(c, 500, "error.create_failed")
	}

	// Return the new template partial
	return c.Render("partials/template_item", fiber.Map{
		"Template": template,
	}, "")
}
