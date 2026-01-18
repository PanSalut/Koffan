package handlers

import (
	"database/sql"
	"fmt"
	"log"
	"shopping-list/db"
	"shopping-list/i18n"
	"strconv"

	"github.com/gofiber/fiber/v2"
)

// Input length limits
const (
	MaxListNameLength    = 100
	MaxIconLength        = 20 // emoji can be multi-byte
	MaxSectionNameLength = 100
	MaxItemNameLength    = 200
	MaxDescriptionLength = 500
)

// GetListsPage returns the homepage with all lists
func GetListsPage(c *fiber.Ctx) error {
	lists, err := db.GetAllLists()
	if err != nil {
		return c.Status(500).SendString("Failed to fetch lists")
	}

	templates, _ := db.GetAllTemplates()

	return c.Render("home", fiber.Map{
		"Lists":        lists,
		"Templates":    templates,
		"Translations": i18n.GetAllLocales(),
		"Locales":      i18n.AvailableLocales(),
		"DefaultLang":  i18n.GetDefaultLang(),
	})
}

// GetListView returns a single list with its items
func GetListView(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return c.Redirect("/")
	}

	list, err := db.GetListByID(id)
	if err != nil {
		if err == sql.ErrNoRows {
			// List not found - redirect to home
			return c.Redirect("/")
		}
		// Database error - log and show error
		log.Printf("Error fetching list %d: %v", id, err)
		return c.Status(500).SendString("Database error")
	}

	// Set this list as active
	db.SetActiveList(id)

	sections, err := db.GetSectionsByList(id)
	if err != nil {
		return c.Status(500).SendString("Failed to fetch sections")
	}

	stats := db.GetListStats(id)
	lists, _ := db.GetAllLists()

	return c.Render("list", fiber.Map{
		"List":         list,
		"Lists":        lists,
		"Sections":     sections,
		"Stats":        stats,
		"Translations": i18n.GetAllLocales(),
		"Locales":      i18n.AvailableLocales(),
		"DefaultLang":  i18n.GetDefaultLang(),
	})
}

// GetLists returns all lists (JSON API)
func GetLists(c *fiber.Ctx) error {
	lists, err := db.GetAllLists()
	if err != nil {
		return c.Status(500).SendString("Failed to fetch lists")
	}

	// Check if JSON format is requested
	if c.Query("format") == "json" {
		return c.JSON(lists)
	}

	// For HTML, redirect to homepage
	return c.Redirect("/")
}

// CreateList creates a new shopping list
func CreateList(c *fiber.Ctx) error {
	name := c.FormValue("name")
	if name == "" {
		return c.Status(400).SendString("Name is required")
	}
	if len(name) > MaxListNameLength {
		return c.Status(400).SendString("Name too long (max 100 characters)")
	}
	if name == "[HISTORY]" {
		return c.Status(400).SendString("This name is reserved for system use")
	}

	// Check for duplicate name
	exists, err := db.ListNameExists(name, 0)
	if err != nil {
		return c.Status(500).SendString("Failed to check list name")
	}
	if exists {
		return c.Status(400).SendString("list_name_exists")
	}

	icon := c.FormValue("icon")
	if icon == "" {
		icon = "🛒"
	}
	if len(icon) > MaxIconLength {
		return c.Status(400).SendString("Icon too long")
	}

	list, err := db.CreateList(name, icon)
	if err != nil {
		return c.Status(500).SendString("Failed to create list")
	}

	// Broadcast to WebSocket clients
	BroadcastUpdate("list_created", list)

	// Return the new list item partial for HTMX
	return c.Render("partials/list_item", fiber.Map{
		"List": list,
	}, "")
}

// UpdateList updates a list's name and icon
func UpdateList(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return c.Status(400).SendString("Invalid ID")
	}

	name := c.FormValue("name")
	if name == "" {
		return c.Status(400).SendString("Name is required")
	}
	if len(name) > MaxListNameLength {
		return c.Status(400).SendString("Name too long (max 100 characters)")
	}
	if name == "[HISTORY]" {
		return c.Status(400).SendString("This name is reserved for system use")
	}

	// Check for duplicate name (excluding current list)
	exists, err := db.ListNameExists(name, id)
	if err != nil {
		return c.Status(500).SendString("Failed to check list name")
	}
	if exists {
		return c.Status(400).SendString("list_name_exists")
	}

	icon := c.FormValue("icon")
	if len(icon) > MaxIconLength {
		return c.Status(400).SendString("Icon too long")
	}

	list, err := db.UpdateList(id, name, icon)
	if err != nil {
		return c.Status(500).SendString("Failed to update list")
	}

	// Broadcast to WebSocket clients
	BroadcastUpdate("list_updated", list)

	// Return updated list item partial
	return c.Render("partials/list_item", fiber.Map{
		"List": list,
	}, "")
}

// DeleteList deletes a shopping list
func DeleteList(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return c.Status(400).SendString("Invalid ID")
	}

	err = db.DeleteList(id)
	if err != nil {
		return c.Status(400).SendString(err.Error())
	}

	// Broadcast to WebSocket clients
	BroadcastUpdate("list_deleted", map[string]int64{"id": id})

	// Return empty string (HTMX will remove the element)
	return c.SendString("")
}

// SetActiveList sets a list as active
func SetActiveList(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return c.Status(400).SendString("Invalid ID")
	}

	err = db.SetActiveList(id)
	if err != nil {
		return c.Status(500).SendString("Failed to activate list")
	}

	// Broadcast to WebSocket clients
	BroadcastUpdate("list_activated", map[string]int64{"id": id})

	// For regular GET requests (not HTMX), redirect to selected list
	if c.Get("HX-Request") == "" {
		return c.Redirect(fmt.Sprintf("/lists/%d", id))
	}

	// Check if this is from the main page (needs redirect) or lists page
	if c.Get("HX-Current-URL") != "" && !contains(c.Get("HX-Current-URL"), "/lists") {
		c.Set("HX-Redirect", "/")
		return c.SendString("")
	}

	// Return updated lists for the management page
	return returnAllLists(c)
}

// MoveListUp moves a list up in order
func MoveListUp(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return c.Status(400).SendString("Invalid ID")
	}

	err = db.MoveListUp(id)
	if err != nil {
		return c.Status(500).SendString("Failed to move list")
	}

	// Broadcast and return full lists
	BroadcastUpdate("lists_reordered", nil)
	return returnAllLists(c)
}

// MoveListDown moves a list down in order
func MoveListDown(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return c.Status(400).SendString("Invalid ID")
	}

	err = db.MoveListDown(id)
	if err != nil {
		return c.Status(500).SendString("Failed to move list")
	}

	// Broadcast and return full lists
	BroadcastUpdate("lists_reordered", nil)
	return returnAllLists(c)
}

// Helper to return all lists as HTML partials
func returnAllLists(c *fiber.Ctx) error {
	lists, err := db.GetAllLists()
	if err != nil {
		return c.Status(500).SendString("Failed to fetch lists")
	}

	activeList, _ := db.GetActiveList()

	return c.Render("partials/lists_container", fiber.Map{
		"Lists":      lists,
		"ActiveList": activeList,
	}, "")
}

// Helper function to check if string contains substring
func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
