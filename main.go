package main

import (
	"encoding/json"
	"html/template"
	"log"
	"os"
	"shopping-list/api"
	"shopping-list/db"
	"shopping-list/handlers"
	"shopping-list/i18n"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/gofiber/template/html/v2"
	"github.com/gofiber/websocket/v2"
)

func main() {
	// Initialize i18n first (before db, so migrations can use translations)
	if err := i18n.Init(); err != nil {
		log.Fatal("Failed to initialize i18n:", err)
	}

	// Set default language from env var (if specified)
	if lang := os.Getenv("DEFAULT_LANG"); lang != "" {
		i18n.SetDefaultLang(lang)
	}

	// Initialize database
	db.Init()
	defer db.Close()

	// Clean expired sessions on startup
	db.CleanExpiredSessions()

	// Initialize login rate limiter
	handlers.InitLoginRateLimiter()

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
		// i18n functions
		"T": i18n.T,
		"toJSON": func(v interface{}) template.JS {
			b, err := json.Marshal(v)
			if err != nil {
				return template.JS("{}")
			}
			return template.JS(b)
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
	app.Post("/login", handlers.LoginRateLimitMiddleware, handlers.Login)
	app.Post("/logout", handlers.Logout)

	// i18n API (before auth middleware - needed for login page)
	app.Get("/locales", handlers.GetLocales)

	// REST API (before auth middleware - uses token auth)
	api.Register(app)

	// Public endpoints (no auth required)
	app.Get("/api/version", handlers.GetVersion)

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

	// Main page - shows all lists
	app.Get("/", handlers.GetListsPage)

	// Single list view - shows items
	app.Get("/lists/:id", handlers.GetListView)

	// Sections API
	app.Get("/sections/list", handlers.GetSectionsListForModal)
	app.Post("/sections", handlers.CreateSection)
	app.Put("/sections/:id", handlers.UpdateSection)
	app.Delete("/sections/:id", handlers.DeleteSection)
	app.Post("/sections/:id/move-up", handlers.MoveSectionUp)
	app.Post("/sections/:id/move-down", handlers.MoveSectionDown)

	// Lists API
	app.Get("/lists", handlers.GetLists)
	app.Post("/lists", handlers.CreateList)
	app.Put("/lists/:id", handlers.UpdateList)
	app.Delete("/lists/:id", handlers.DeleteList)
	app.Post("/lists/:id/activate", handlers.SetActiveList)
	app.Get("/lists/:id/activate", handlers.SetActiveList)
	app.Post("/lists/:id/move-up", handlers.MoveListUp)
	app.Post("/lists/:id/move-down", handlers.MoveListDown)

	// Templates API
	app.Get("/templates", handlers.GetTemplates)
	app.Get("/templates/:id", handlers.GetTemplate)
	app.Post("/templates", handlers.CreateTemplate)
	app.Put("/templates/:id", handlers.UpdateTemplate)
	app.Delete("/templates/:id", handlers.DeleteTemplate)
	app.Post("/templates/:id/items", handlers.AddTemplateItem)
	app.Put("/templates/:id/items/:itemId", handlers.UpdateTemplateItem)
	app.Delete("/templates/:id/items/:itemId", handlers.DeleteTemplateItem)
	app.Post("/templates/:id/apply", handlers.ApplyTemplate)
	app.Post("/templates/from-list", handlers.CreateTemplateFromList)

	// Items API
	app.Post("/items", handlers.CreateItem)
	app.Post("/items/delete-completed", handlers.DeleteCompletedItems)
	app.Put("/items/:id", handlers.UpdateItem)
	app.Delete("/items/:id", handlers.DeleteItem)
	app.Post("/items/:id/toggle", handlers.ToggleItem)
	app.Post("/items/:id/uncertain", handlers.ToggleUncertain)
	app.Post("/items/:id/move", handlers.MoveItemToSection)
	app.Post("/items/:id/move-up", handlers.MoveItemUp)
	app.Post("/items/:id/move-down", handlers.MoveItemDown)

	// Stats API
	app.Get("/stats", handlers.GetStats)

	// Offline data API
	app.Get("/api/data", handlers.GetAllData)
	app.Get("/api/item/:id/version", handlers.GetItemVersion)
	app.Get("/api/suggestions", handlers.GetSuggestions)

	// History management API
	app.Get("/api/history", handlers.GetHistory)
	app.Delete("/api/history/:id", handlers.DeleteHistoryItem)
	app.Post("/api/history/batch-delete", handlers.BatchDeleteHistory)

	// Batch operations
	app.Post("/sections/batch-delete", handlers.BatchDeleteSections)

	// Import/Export
	app.Get("/export", handlers.ExportAllData)
	app.Get("/export/list/:id", handlers.ExportSingleList)
	app.Get("/export/preview", handlers.GetExportPreview)
	app.Post("/import", handlers.ImportData)
	app.Post("/import/preview", handlers.PreviewImport)

	// Database management
	app.Post("/api/database/clear", handlers.ClearDatabase)

	// Get port from env or default to 3000
	port := os.Getenv("PORT")
	if port == "" {
		port = "3000"
	}

	log.Printf("Starting server on port %s", port)
	log.Fatal(app.Listen(":" + port))
}
