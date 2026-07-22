package main

import (
	"embed"
	"encoding/json"
	"flag"
	"html/template"
	"io/fs"
	"log"
	"net/http"
	"os"
	"shopping-list/api"
	"shopping-list/db"
	"shopping-list/handlers"
	"shopping-list/i18n"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/compress"
	"github.com/gofiber/fiber/v2/middleware/filesystem"
	"github.com/gofiber/fiber/v2/middleware/helmet"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/gofiber/template/html/v2"
	"github.com/gofiber/websocket/v2"
)

//go:embed templates/*
var embeddedTemplatesFS embed.FS

//go:embed static/*
var embeddedStaticFS embed.FS

func main() {
	adminRecovery := flag.Bool("admin-recovery", false, "run interactive administrator recovery and exit")
	envFile := flag.String("env-file", "", "load missing environment variables from this file")
	flag.Parse()
	loadedFile, loadedCount, err := loadStartupEnvironment(*envFile)
	if err != nil {
		log.Fatalf("Failed to load environment file: %v", err)
	}
	if loadedFile != "" {
		log.Printf("Loaded %d environment variable(s) from %s", loadedCount, loadedFile)
	}
	if *adminRecovery {
		db.InitForAdminRecovery()
		err := runAdminRecovery(os.Stdin, os.Stdout)
		db.Close()
		if err != nil {
			log.Printf("Administrator recovery failed: %v", err)
			os.Exit(1)
		}
		return
	}
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
	if strings.EqualFold(os.Getenv("DISABLE_AUTH"), "true") {
		log.Print("SECURITY WARNING: DISABLE_AUTH=true grants every request administrator access. Use only on an isolated, trusted network.")
	}

	// Clean expired sessions on startup
	db.CleanExpiredSessions()

	// Initialize login rate limiter
	handlers.InitLoginRateLimiter()

	// Initialize template engine
	templatesRootFS, err := fs.Sub(embeddedTemplatesFS, "templates")
	if err != nil {
		log.Fatalf("Embedded templates directory missing: %v", err)
	}

	engine := html.NewFileSystem(http.FS(templatesRootFS), ".html")
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
		"asset": func(path string) string {
			return "/static/" + path + "?v=" + handlers.AssetHash
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
	app.Use(helmet.New(helmet.Config{
		ContentSecurityPolicy: "default-src 'self'; script-src 'self' 'unsafe-inline' 'unsafe-eval'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; font-src 'self' data:; connect-src 'self' ws: wss:; object-src 'none'; base-uri 'self'; frame-ancestors 'self'; form-action 'self'",
		ReferrerPolicy:        "no-referrer",
		PermissionPolicy:      "camera=(), microphone=(), geolocation=()",
	}))
	app.Use(func(c *fiber.Ctx) error {
		secure := c.Protocol() == "https" || (strings.EqualFold(os.Getenv("TRUST_PROXY_HEADERS"), "true") && strings.EqualFold(strings.TrimSpace(strings.Split(c.Get("X-Forwarded-Proto"), ",")[0]), "https"))
		if secure {
			c.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}
		return c.Next()
	})
	app.Use(compress.New(compress.Config{Level: compress.LevelBestSpeed}))

	// Static files
	staticRootFS, err := fs.Sub(embeddedStaticFS, "static")
	if err != nil {
		log.Fatalf("Embedded static directory missing: %v", err)
	}

	// Compute content hash across embedded static FS. Used as ?v=<hash>
	// cache-buster in templates and injected into sw.js placeholders so
	// any change to static/ automatically invalidates browser + SW caches.
	hash, err := handlers.ComputeAssetHash(staticRootFS)
	if err != nil {
		log.Fatalf("Failed to compute asset hash: %v", err)
	}
	handlers.AssetHash = hash
	log.Printf("Asset hash: %s", hash)

	swBytes, err := handlers.BuildServiceWorker(staticRootFS, hash)
	if err != nil {
		log.Fatalf("Failed to build service worker: %v", err)
	}
	handlers.ServiceWorkerBytes = swBytes

	// SW must be served by a dedicated handler (not the filesystem middleware)
	// so placeholders get replaced and Cache-Control is no-cache instead of
	// the 30-day max-age applied to other static assets.
	app.Get("/static/sw.js", handlers.ServeServiceWorker)

	app.Use("/static", filesystem.New(filesystem.Config{
		Root:   http.FS(staticRootFS),
		Browse: false,
		MaxAge: 86400 * 30, // 30 days - files are embedded and versioned at build time
	}))

	// Auth routes (before middleware)
	app.Get("/login", handlers.LoginPage)
	app.Post("/login", handlers.LoginRateLimitMiddleware, handlers.Login)
	app.Get("/auth/oidc/login", handlers.OIDCLogin)
	app.Get("/auth/oidc/callback", handlers.OIDCCallback)

	// i18n API (before auth middleware - needed for login page)
	app.Get("/locales", handlers.GetLocales)

	// REST API (before auth middleware - uses token auth)
	api.Register(app)

	// Public endpoints (no auth required)
	app.Get("/api/version", handlers.GetVersion)

	// Auth middleware for all other routes
	app.Use(handlers.AuthMiddleware)
	app.Use(handlers.CSRFMiddleware)
	app.Post("/logout", handlers.Logout)

	// WebSocket upgrade middleware
	app.Use("/ws", func(c *fiber.Ctx) error {
		if websocket.IsWebSocketUpgrade(c) {
			return handlers.ValidateWebSocketOrigin(c)
		}
		return fiber.ErrUpgradeRequired
	})

	// WebSocket endpoint
	app.Get("/ws", websocket.New(handlers.WebSocketHandler))

	// Main page - shows all lists
	app.Get("/", handlers.GetListsPage)

	// Single list view - shows items
	app.Get("/lists/:id", handlers.RequireListView, handlers.GetListView)

	// Sections API
	app.Get("/sections/list", handlers.GetSectionsListForModal)
	app.Get("/sections/:id/html", handlers.RequireSectionView, handlers.GetSectionHTML)
	app.Post("/sections", handlers.CreateSection)
	app.Put("/sections/:id", handlers.RequireSectionEdit, handlers.UpdateSection)
	app.Delete("/sections/:id", handlers.RequireSectionEdit, handlers.DeleteSection)
	app.Post("/sections/:id/move-up", handlers.RequireSectionEdit, handlers.MoveSectionUp)
	app.Post("/sections/:id/move-down", handlers.RequireSectionEdit, handlers.MoveSectionDown)
	app.Post("/sections/:id/check-all", handlers.RequireSectionEdit, handlers.CheckAllItems)
	app.Post("/sections/:id/uncheck-all", handlers.RequireSectionEdit, handlers.UncheckAllItems)
	app.Post("/sections/:id/sort-mode", handlers.RequireSectionEdit, handlers.UpdateSectionSortMode)

	// Lists API
	app.Get("/lists", handlers.GetLists)
	app.Post("/lists", handlers.CreateList)
	app.Put("/lists/:id", handlers.RequireListOwner, handlers.UpdateList)
	app.Delete("/lists/:id", handlers.RequireListOwner, handlers.DeleteList)
	app.Post("/lists/:id/activate", handlers.RequireListView, handlers.SetActiveList)
	app.Post("/lists/:id/move-up", handlers.RequireListView, handlers.MoveListUp)
	app.Post("/lists/:id/move-down", handlers.RequireListView, handlers.MoveListDown)
	app.Post("/lists/:id/toggle-completed", handlers.RequireListOwner, handlers.ToggleShowCompleted)
	app.Get("/lists/:id/members", handlers.RequireListManage, handlers.GetListMembers)
	app.Get("/lists/:id/access", handlers.RequireListManage, handlers.ListAccessPage)
	app.Put("/lists/:id/owner", handlers.RequireListOwner, handlers.TransferListOwnership)
	app.Put("/lists/:id/groups/:groupId", handlers.RequireListManage, handlers.SetListGroupAccess)
	app.Delete("/lists/:id/groups/:groupId", handlers.RequireListManage, handlers.DeleteListGroupAccess)
	app.Post("/lists/:id/members", handlers.RequireListManage, handlers.AddListMember)
	app.Put("/lists/:id/members/:userId", handlers.RequireListManage, handlers.UpdateListMember)
	app.Delete("/lists/:id/members/:userId", handlers.RequireListManage, handlers.DeleteListMember)
	app.Get("/users/search", handlers.SearchUsers)
	app.Get("/admin/users", handlers.AdminUsersPage)
	app.Get("/admin/users.json", handlers.GetUsers)
	app.Post("/admin/users", handlers.CreateUser)
	app.Get("/admin", handlers.AdminUsersPage)
	app.Put("/admin/users/:id", handlers.UpdateUser)
	app.Post("/admin/users/:id/invalidate-sessions", handlers.InvalidateUserSessions)
	app.Delete("/admin/users/:id", handlers.DeleteUser)
	app.Get("/admin/groups", handlers.GetGroups)
	app.Post("/admin/groups", handlers.CreateGroup)
	app.Put("/admin/groups/:id", handlers.UpdateGroupDetails)
	app.Delete("/admin/groups/:id", handlers.DeleteGroup)
	app.Put("/admin/groups/:id/admin", handlers.UpdateGroupAdmin)
	app.Post("/admin/groups/:id/users/:userId", handlers.AddGroupUser)
	app.Delete("/admin/groups/:id/users/:userId", handlers.RemoveGroupUser)
	app.Put("/admin/groups/:id/lists/:listId", handlers.SetGroupListAccess)
	app.Delete("/admin/groups/:id/lists/:listId", handlers.DeleteGroupListAccess)

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
	app.Get("/items/:id/html", handlers.RequireItemView, handlers.GetItemHTML)
	app.Post("/items", handlers.CreateItem)
	app.Post("/items/delete-completed", handlers.DeleteCompletedItems)
	app.Put("/items/:id", handlers.RequireItemEdit, handlers.UpdateItem)
	app.Delete("/items/:id", handlers.RequireItemEdit, handlers.DeleteItem)
	app.Post("/items/:id/toggle", handlers.RequireItemEdit, handlers.ToggleItem)
	app.Post("/items/:id/quantity", handlers.RequireItemEdit, handlers.AdjustItemQuantity)
	app.Post("/items/:id/uncertain", handlers.RequireItemEdit, handlers.ToggleUncertain)
	app.Post("/items/:id/move", handlers.RequireItemEdit, handlers.MoveItemToSection)
	app.Post("/items/:id/move-up", handlers.RequireItemEdit, handlers.MoveItemUp)
	app.Post("/items/:id/move-down", handlers.RequireItemEdit, handlers.MoveItemDown)

	// Stats API
	app.Get("/stats", handlers.GetStats)

	// Offline data API
	app.Get("/api/data", handlers.GetAllData)
	app.Get("/api/item/:id/version", handlers.RequireItemView, handlers.GetItemVersion)
	app.Get("/lists/:id/activity", handlers.RequireListView, handlers.GetListActivity)
	app.Get("/api/suggestions", handlers.GetSuggestions)

	// History management API
	app.Get("/api/history", handlers.GetHistory)
	app.Delete("/api/history/:id", handlers.DeleteHistoryItem)
	app.Post("/api/history/batch-delete", handlers.BatchDeleteHistory)

	// Batch operations
	app.Post("/sections/batch-delete", handlers.BatchDeleteSections)

	// Import/Export
	app.Get("/export", handlers.ExportAllData)
	app.Get("/export/list/:id", handlers.RequireListView, handlers.ExportSingleList)
	app.Get("/export/preview", handlers.GetExportPreview)
	app.Post("/import", handlers.ImportData)
	app.Post("/import/preview", handlers.PreviewImport)

	// Database management
	app.Get("/api/database/csrf-token", handlers.GenerateCSRFToken)
	app.Post("/api/database/clear", handlers.ClearDatabase)

	// Get port from env or default to 3000
	port := os.Getenv("PORT")
	if port == "" {
		port = "3000"
	}

	log.Printf("Starting server on port %s", port)
	log.Fatal(app.Listen(":" + port))
}
