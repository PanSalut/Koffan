package handlers

import (
	"shopping-list/db"
	"strconv"

	"github.com/gofiber/fiber/v2"
)

// GetTemplates returns all templates
func GetTemplates(c *fiber.Ctx) error {
	templates, err := db.GetAllTemplates()
	if err != nil {
		return c.Status(500).SendString("Failed to fetch templates")
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
		return c.Status(400).SendString("Invalid ID")
	}

	template, err := db.GetTemplateByID(id)
	if err != nil {
		return c.Status(404).SendString("Template not found")
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
	name := c.FormValue("name")
	if name == "" {
		return c.Status(400).SendString("Name is required")
	}

	description := c.FormValue("description")

	template, err := db.CreateTemplate(name, description)
	if err != nil {
		return c.Status(500).SendString("Failed to create template")
	}

	// Broadcast to WebSocket clients
	BroadcastUpdate("template_created", template)

	// Return the new template partial
	return c.Render("partials/template_item", fiber.Map{
		"Template": template,
	}, "")
}

// UpdateTemplate updates a template's name and description
func UpdateTemplate(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return c.Status(400).SendString("Invalid ID")
	}

	name := c.FormValue("name")
	if name == "" {
		return c.Status(400).SendString("Name is required")
	}

	description := c.FormValue("description")

	template, err := db.UpdateTemplate(id, name, description)
	if err != nil {
		return c.Status(500).SendString("Failed to update template")
	}

	// Broadcast to WebSocket clients
	BroadcastUpdate("template_updated", template)

	// Return updated template partial
	return c.Render("partials/template_item", fiber.Map{
		"Template": template,
	}, "")
}

// DeleteTemplate deletes a template
func DeleteTemplate(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return c.Status(400).SendString("Invalid ID")
	}

	err = db.DeleteTemplate(id)
	if err != nil {
		return c.Status(500).SendString("Failed to delete template")
	}

	// Broadcast to WebSocket clients
	BroadcastUpdate("template_deleted", map[string]int64{"id": id})

	return c.SendString("")
}

// AddTemplateItem adds an item to a template
func AddTemplateItem(c *fiber.Ctx) error {
	templateID, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return c.Status(400).SendString("Invalid template ID")
	}

	sectionName := c.FormValue("section_name")
	if sectionName == "" {
		return c.Status(400).SendString("Section name is required")
	}

	name := c.FormValue("name")
	if name == "" {
		return c.Status(400).SendString("Item name is required")
	}

	description := c.FormValue("description")

	item, err := db.AddTemplateItem(templateID, sectionName, name, description)
	if err != nil {
		return c.Status(500).SendString("Failed to add item to template")
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
		return c.Status(400).SendString("Invalid item ID")
	}

	sectionName := c.FormValue("section_name")
	if sectionName == "" {
		return c.Status(400).SendString("Section name is required")
	}

	name := c.FormValue("name")
	if name == "" {
		return c.Status(400).SendString("Item name is required")
	}

	description := c.FormValue("description")

	item, err := db.UpdateTemplateItem(itemID, sectionName, name, description)
	if err != nil {
		return c.Status(500).SendString("Failed to update template item")
	}

	return c.Render("partials/template_item_row", fiber.Map{
		"Item": item,
	}, "")
}

// DeleteTemplateItem deletes a template item
func DeleteTemplateItem(c *fiber.Ctx) error {
	itemID, err := strconv.ParseInt(c.Params("itemId"), 10, 64)
	if err != nil {
		return c.Status(400).SendString("Invalid item ID")
	}

	err = db.DeleteTemplateItem(itemID)
	if err != nil {
		return c.Status(500).SendString("Failed to delete template item")
	}

	return c.SendString("")
}

// ApplyTemplate applies a template to the active list
func ApplyTemplate(c *fiber.Ctx) error {
	templateID, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return c.Status(400).SendString("Invalid template ID")
	}

	activeList, err := db.GetActiveList()
	if err != nil {
		return c.Status(500).SendString("No active list found")
	}

	err = db.ApplyTemplateToList(templateID, activeList.ID)
	if err != nil {
		return c.Status(500).SendString("Failed to apply template")
	}

	// Broadcast to WebSocket clients
	BroadcastUpdate("template_applied", map[string]interface{}{
		"template_id": templateID,
		"list_id":     activeList.ID,
	})

	// Trigger a full refresh
	c.Set("HX-Trigger", "refreshList, refresh")
	return c.SendString("")
}

// CreateTemplateFromList creates a template from the active list
func CreateTemplateFromList(c *fiber.Ctx) error {
	name := c.FormValue("name")
	if name == "" {
		return c.Status(400).SendString("Template name is required")
	}

	description := c.FormValue("description")

	activeList, err := db.GetActiveList()
	if err != nil {
		return c.Status(500).SendString("No active list found")
	}

	template, err := db.CreateTemplateFromList(activeList.ID, name, description)
	if err != nil {
		return c.Status(500).SendString("Failed to create template from list")
	}

	// Broadcast to WebSocket clients
	BroadcastUpdate("template_created", template)

	// Return the new template partial
	return c.Render("partials/template_item", fiber.Map{
		"Template": template,
	}, "")
}
