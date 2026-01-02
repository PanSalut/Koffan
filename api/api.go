package api

import (
	"log"

	"github.com/gofiber/fiber/v2"
)

// Register conditionally registers the API routes if API_TOKEN is set
func Register(app *fiber.App) {
	if !IsAPIEnabled() {
		log.Println("REST API is disabled (API_TOKEN not set)")
		// Register catch-all handler that returns 503 for all API requests
		app.All("/api/v1/*", func(c *fiber.Ctx) error {
			return c.Status(fiber.StatusServiceUnavailable).JSON(ErrorResponse{
				Error:   "api_disabled",
				Message: "API is not enabled on this server",
			})
		})
		return
	}

	log.Println("REST API is enabled")

	// Create API group with version prefix and token auth middleware
	v1 := app.Group("/api/v1", TokenAuthMiddleware)

	// Lists endpoints
	v1.Get("/lists", GetLists)
	v1.Get("/lists/:id", GetList)
	v1.Post("/lists", CreateList)
	v1.Put("/lists/:id", UpdateList)
	v1.Delete("/lists/:id", DeleteList)
	v1.Get("/lists/:id/sections", GetListSections)
	v1.Post("/lists/:id/move-up", MoveListUp)
	v1.Post("/lists/:id/move-down", MoveListDown)

	// Sections endpoints
	v1.Get("/sections/:id", GetSection)
	v1.Post("/sections", CreateSection)
	v1.Put("/sections/:id", UpdateSection)
	v1.Delete("/sections/:id", DeleteSection)
	v1.Get("/sections/:id/items", GetSectionItems)
	v1.Post("/sections/:id/move-up", MoveSectionUp)
	v1.Post("/sections/:id/move-down", MoveSectionDown)

	// Items endpoints
	v1.Get("/items/:id", GetItem)
	v1.Post("/items", CreateItem)
	v1.Put("/items/:id", UpdateItem)
	v1.Delete("/items/:id", DeleteItem)
	v1.Post("/items/:id/toggle", ToggleItemCompleted)
	v1.Post("/items/:id/uncertain", ToggleItemUncertain)
	v1.Post("/items/:id/move", MoveItem)
	v1.Post("/items/:id/move-up", MoveItemUp)
	v1.Post("/items/:id/move-down", MoveItemDown)

	// Batch endpoint
	v1.Post("/batch", BatchCreate)

	// History endpoints (suggestions)
	v1.Get("/history", GetHistory)
	v1.Post("/history", CreateHistory)
	v1.Delete("/history/:id", DeleteHistory)
	v1.Post("/history/batch-delete", BatchDeleteHistory)
}
