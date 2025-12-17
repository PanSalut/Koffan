package main

import (
	"html/template"
	"log"
	"os"
	"shopping-list/db"
	"shopping-list/handlers"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/gofiber/template/html/v2"
	"github.com/gofiber/websocket/v2"
)

func main() {
	// Initialize database
	db.Init()
	defer db.Close()

	// Clean expired sessions on startup
	db.CleanExpiredSessions()

	// Initialize template engine
	engine := html.New("./templates", ".html")
	engine.Reload(os.Getenv("APP_ENV") != "production")

	// Add custom template functions
	engine.AddFuncMap(template.FuncMap{
		"dict": func(values ...interface{}) map[string]interface{} {
			if len(values)%2 != 0 {
				return nil
			}
			dict := make(map[string]interface{}, len(values)/2)
			for i := 0; i < len(values); i += 2 {
				key, ok := values[i].(string)
				if !ok {
					continue
				}
				dict[key] = values[i+1]
			}
			return dict
		},
		"add": func(a, b int) int {
			return a + b
		},
		"sub": func(a, b int) int {
			return a - b
		},
		"mul": func(a, b int) int {
			return a * b
		},
		"div": func(a, b int) int {
			if b == 0 {
				return 0
			}
			return a / b
		},
		"gt": func(a, b int) bool {
			return a > b
		},
		"lt": func(a, b int) bool {
			return a < b
		},
		"eq": func(a, b interface{}) bool {
			return a == b
		},
		"ne": func(a, b interface{}) bool {
			return a != b
		},
	})

	// Initialize Fiber app
	app := fiber.New(fiber.Config{
		Views:       engine,
		ViewsLayout: "layout",
	})

	// Middleware
	app.Use(logger.New())
	app.Use(recover.New())

	// Static files
	app.Static("/static", "./static")

	// Auth routes (before middleware)
	app.Get("/login", handlers.LoginPage)
	app.Post("/login", handlers.Login)
	app.Post("/logout", handlers.Logout)

	// Auth middleware for all other routes
	app.Use(handlers.AuthMiddleware)

	// WebSocket upgrade middleware
	app.Use("/ws", func(c *fiber.Ctx) error {
		if websocket.IsWebSocketUpgrade(c) {
			c.Locals("allowed", true)
			return c.Next()
		}
		return fiber.ErrUpgradeRequired
	})

	// WebSocket endpoint
	app.Get("/ws", websocket.New(handlers.WebSocketHandler))

	// Main page
	app.Get("/", handlers.GetSections)

	// Sections API
	app.Get("/sections/list", handlers.GetSectionsListForModal)
	app.Post("/sections", handlers.CreateSection)
	app.Put("/sections/:id", handlers.UpdateSection)
	app.Delete("/sections/:id", handlers.DeleteSection)
	app.Post("/sections/:id/move-up", handlers.MoveSectionUp)
	app.Post("/sections/:id/move-down", handlers.MoveSectionDown)

	// Items API
	app.Post("/items", handlers.CreateItem)
	app.Put("/items/:id", handlers.UpdateItem)
	app.Delete("/items/:id", handlers.DeleteItem)
	app.Post("/items/:id/toggle", handlers.ToggleItem)
	app.Post("/items/:id/uncertain", handlers.ToggleUncertain)
	app.Post("/items/:id/move", handlers.MoveItemToSection)
	app.Post("/items/:id/move-up", handlers.MoveItemUp)
	app.Post("/items/:id/move-down", handlers.MoveItemDown)

	// Stats API
	app.Get("/stats", handlers.GetStats)

	// Preferences API
	app.Get("/preferences", handlers.GetPreferences)
	app.Post("/preferences", handlers.UpdatePreferences)
	app.Post("/preferences/toggle-mobile-helper", handlers.ToggleMobileHelper)

	// Batch operations
	app.Post("/sections/batch-delete", handlers.BatchDeleteSections)

	// Get port from env or default to 3000
	port := os.Getenv("PORT")
	if port == "" {
		port = "3000"
	}

	log.Printf("Starting server on port %s", port)
	log.Fatal(app.Listen(":" + port))
}
