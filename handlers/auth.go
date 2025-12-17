package handlers

import (
	"crypto/rand"
	"encoding/hex"
	"os"
	"shopping-list/db"
	"time"

	"github.com/gofiber/fiber/v2"
)

const (
	SessionCookieName = "session"
	SessionDuration   = 7 * 24 * time.Hour // 7 days
)

func getAppPassword() string {
	pass := os.Getenv("APP_PASSWORD")
	if pass == "" {
		pass = "shopping123" // Default password for development
	}
	return pass
}

func generateSessionID() string {
	bytes := make([]byte, 32)
	rand.Read(bytes)
	return hex.EncodeToString(bytes)
}

// LoginPage renders the login page
func LoginPage(c *fiber.Ctx) error {
	// Check if already logged in
	sessionID := c.Cookies(SessionCookieName)
	if sessionID != "" {
		session, err := db.GetSession(sessionID)
		if err == nil && session.ExpiresAt > time.Now().Unix() {
			return c.Redirect("/")
		}
	}
	return c.Render("login", fiber.Map{
		"Error": c.Query("error"),
	}, "")
}

// Login handles login form submission
func Login(c *fiber.Ctx) error {
	password := c.FormValue("password")

	if password != getAppPassword() {
		return c.Redirect("/login?error=1")
	}

	// Create session
	sessionID := generateSessionID()
	expiresAt := time.Now().Add(SessionDuration).Unix()

	err := db.CreateSession(sessionID, expiresAt)
	if err != nil {
		return c.Status(500).SendString("Session creation failed")
	}

	// Set cookie
	c.Cookie(&fiber.Cookie{
		Name:     SessionCookieName,
		Value:    sessionID,
		Expires:  time.Now().Add(SessionDuration),
		HTTPOnly: true,
		Secure:   os.Getenv("APP_ENV") == "production",
		SameSite: "Lax",
	})

	return c.Redirect("/")
}

// Logout handles logout
func Logout(c *fiber.Ctx) error {
	sessionID := c.Cookies(SessionCookieName)
	if sessionID != "" {
		db.DeleteSession(sessionID)
	}

	// Clear cookie
	c.Cookie(&fiber.Cookie{
		Name:     SessionCookieName,
		Value:    "",
		Expires:  time.Now().Add(-time.Hour),
		HTTPOnly: true,
	})

	return c.Redirect("/login")
}

// AuthMiddleware checks if user is authenticated
func AuthMiddleware(c *fiber.Ctx) error {
	// Skip auth for login page and static files
	path := c.Path()
	if path == "/login" || path == "/static" || len(path) > 7 && path[:8] == "/static/" {
		return c.Next()
	}

	sessionID := c.Cookies(SessionCookieName)
	if sessionID == "" {
		if c.Get("HX-Request") == "true" {
			c.Set("HX-Redirect", "/login")
			return c.SendStatus(401)
		}
		return c.Redirect("/login")
	}

	session, err := db.GetSession(sessionID)
	if err != nil || session.ExpiresAt < time.Now().Unix() {
		// Session expired or not found
		if sessionID != "" {
			db.DeleteSession(sessionID)
		}
		c.Cookie(&fiber.Cookie{
			Name:     SessionCookieName,
			Value:    "",
			Expires:  time.Now().Add(-time.Hour),
			HTTPOnly: true,
		})
		if c.Get("HX-Request") == "true" {
			c.Set("HX-Redirect", "/login")
			return c.SendStatus(401)
		}
		return c.Redirect("/login")
	}

	return c.Next()
}
