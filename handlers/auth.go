package handlers

import (
	"crypto/rand"
	"crypto/subtle"
	"database/sql"
	"encoding/hex"
	"errors"
	"log"
	"os"
	"shopping-list/db"
	"shopping-list/i18n"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
)

const (
	SessionCookieName = "session"
	SessionDuration   = 7 * 24 * time.Hour // 7 days
)

func isAuthDisabled() bool {
	return os.Getenv("DISABLE_AUTH") == "true"
}

// isSecureConnection checks if the request came over HTTPS
// Works both directly and behind reverse proxies
func isSecureConnection(c *fiber.Ctx) bool {
	// Check X-Forwarded-Proto header (set by reverse proxies)
	if strings.EqualFold(os.Getenv("TRUST_PROXY_HEADERS"), "true") && c.Get("X-Forwarded-Proto") == "https" {
		return true
	}
	// Check direct connection protocol
	return c.Protocol() == "https"
}

func generateSessionID() string {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		log.Fatal("Failed to generate secure random bytes:", err)
	}
	return hex.EncodeToString(bytes)
}

func sessionIDPrefix(sessionID string) string {
	const prefixLength = 8
	if len(sessionID) <= prefixLength {
		return sessionID
	}
	return sessionID[:prefixLength]
}

// LoginPage renders the login page
func LoginPage(c *fiber.Ctx) error {
	// Check if already logged in
	sessionID := c.Cookies(SessionCookieName)
	if sessionID != "" {
		session, err := db.GetSession(sessionID)
		if err == nil && session.UserID > 0 && session.ExpiresAt > time.Now().Unix() {
			return c.Redirect("/")
		}
	}
	return c.Render("login", fiber.Map{
		"Error":        c.Query("error"),
		"Translations": i18n.GetAllLocales(),
		"Locales":      i18n.AvailableLocales(),
		"DefaultLang":  i18n.GetDefaultLang(),
		"OIDCEnabled":  OIDCEnabled(),
		"OIDCName":     envDefault("OIDC_DISPLAY_NAME", "OpenID Connect"),
	}, "")
}

// Login handles login form submission
func Login(c *fiber.Ctx) error {
	ip := c.IP()
	username := strings.ToLower(strings.TrimSpace(c.FormValue("username")))
	password := c.FormValue("password")
	limitKeys := []string{"ip:" + ip, "user:" + username}

	user, err := db.GetUserByUsername(username)
	valid := err == nil && user.PasswordHash != nil && user.AuthSource == "local" && !user.Disabled && db.VerifyPassword(*user.PasswordHash, password)
	if !valid {
		// Record failed attempt
		if loginLimiter != nil {
			blocked := false
			for _, key := range limitKeys {
				if loginLimiter.RecordAttempt(key) {
					blocked = true
				}
			}
			if blocked {
				// Limit exceeded, redirect with rate_limited error
				return c.Redirect("/login?error=rate_limited")
			}
		}
		return c.Redirect("/login?error=1")
	}

	// Successful login - reset attempts
	if loginLimiter != nil {
		for _, key := range limitKeys {
			loginLimiter.ResetAttempts(key)
		}
	}

	err = createApplicationSession(c, user)
	if err != nil {
		return sendError(c, 500, "error.session_failed")
	}

	return c.Redirect("/")
}

func createApplicationSession(c *fiber.Ctx, user *db.User) error {
	sessionID := generateSessionID()
	csrfToken := generateSessionID()
	expires := time.Now().Add(SessionDuration)
	if err := db.CreateUserSession(sessionID, user.ID, expires.Unix(), csrfToken); err != nil {
		return err
	}
	c.Cookie(&fiber.Cookie{Name: SessionCookieName, Value: sessionID, Expires: expires, HTTPOnly: true, Secure: isSecureConnection(c), SameSite: "Lax", Path: "/"})
	c.Cookie(&fiber.Cookie{Name: "csrf", Value: csrfToken, Expires: expires, HTTPOnly: false, Secure: isSecureConnection(c), SameSite: "Lax", Path: "/"})
	_ = db.MarkUserLogin(user.ID)
	log.Printf("[AUTH] User %d session created: %s...", user.ID, sessionIDPrefix(sessionID))
	return nil
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
		Secure:   isSecureConnection(c),
		SameSite: "Lax",
		Path:     "/",
	})
	c.Cookie(&fiber.Cookie{Name: "csrf", Value: "", Expires: time.Now().Add(-time.Hour), HTTPOnly: false, Secure: isSecureConnection(c), SameSite: "Lax", Path: "/"})

	return c.Redirect("/login")
}

// AuthMiddleware checks if user is authenticated
func AuthMiddleware(c *fiber.Ctx) error {
	if isAuthDisabled() {
		users, err := db.ListUsers()
		if err == nil && len(users) > 0 {
			u := &users[0]
			u.IsAdmin = true
			c.Locals("user", u)
			c.Locals("userID", u.ID)
			c.Locals("isAdmin", true)
		}
		return c.Next()
	}

	// Skip auth for login page and static files
	path := c.Path()
	if path == "/login" || path == "/static" || len(path) > 7 && path[:8] == "/static/" {
		return c.Next()
	}

	sessionID := c.Cookies(SessionCookieName)
	if sessionID == "" {
		log.Printf("[AUTH] No session cookie for %s %s (HX-Request: %s)", c.Method(), path, c.Get("HX-Request"))
		if c.Get("HX-Request") == "true" {
			c.Set("HX-Redirect", "/login")
			return c.SendStatus(401)
		}
		return c.Redirect("/login")
	}

	session, err := db.GetSession(sessionID)
	if err != nil {
		// Check if it's a "not found" error vs database error
		if errors.Is(err, sql.ErrNoRows) {
			log.Printf("[AUTH] Session not found in DB for %s %s (sessionID: %s...)", c.Method(), path, sessionIDPrefix(sessionID))
			// Only delete if session truly doesn't exist
			db.DeleteSession(sessionID)
		} else {
			// Database error (e.g., locked) - don't delete session, just log and retry
			log.Printf("[AUTH] Database error for %s %s: %v", c.Method(), path, err)
			// Return 503 Service Unavailable for temporary DB issues
			return sendError(c, 503, "error.database_unavailable")
		}
		c.Cookie(&fiber.Cookie{
			Name:     SessionCookieName,
			Value:    "",
			Expires:  time.Now().Add(-time.Hour),
			HTTPOnly: true,
			Secure:   isSecureConnection(c),
			SameSite: "Lax",
			Path:     "/",
		})
		if c.Get("HX-Request") == "true" {
			c.Set("HX-Redirect", "/login")
			return c.SendStatus(401)
		}
		return c.Redirect("/login")
	}

	if session.ExpiresAt < time.Now().Unix() {
		log.Printf("[AUTH] Session expired for %s %s (expired: %d, now: %d)", c.Method(), path, session.ExpiresAt, time.Now().Unix())
		db.DeleteSession(sessionID)
		c.Cookie(&fiber.Cookie{
			Name:     SessionCookieName,
			Value:    "",
			Expires:  time.Now().Add(-time.Hour),
			HTTPOnly: true,
			Secure:   isSecureConnection(c),
			SameSite: "Lax",
			Path:     "/",
		})
		if c.Get("HX-Request") == "true" {
			c.Set("HX-Redirect", "/login")
			return c.SendStatus(401)
		}
		return c.Redirect("/login")
	}

	user, err := db.GetUserByID(session.UserID)
	if err != nil || user.Disabled {
		db.DeleteSession(sessionID)
		if c.Get("HX-Request") == "true" {
			c.Set("HX-Redirect", "/login")
			return c.SendStatus(401)
		}
		return c.Redirect("/login")
	}
	effectiveAdmin, err := db.IsUserAdministrator(user.ID)
	if err != nil {
		return sendError(c, 503, "error.database_unavailable")
	}
	user.IsAdmin = effectiveAdmin
	c.Locals("user", user)
	c.Locals("userID", user.ID)
	c.Locals("isAdmin", effectiveAdmin)
	c.Locals("csrfToken", session.CSRFToken)

	return c.Next()
}

func CurrentUser(c *fiber.Ctx) (*db.User, error) {
	u, ok := c.Locals("user").(*db.User)
	if !ok || u == nil {
		return nil, fiber.ErrUnauthorized
	}
	return u, nil
}

func RequireAdmin(c *fiber.Ctx) error {
	u, err := CurrentUser(c)
	if err != nil {
		return err
	}
	if !u.IsAdmin {
		return fiber.ErrForbidden
	}
	return nil
}

func CSRFMiddleware(c *fiber.Ctx) error {
	if isAuthDisabled() {
		return c.Next()
	}
	switch c.Method() {
	case fiber.MethodPost, fiber.MethodPut, fiber.MethodPatch, fiber.MethodDelete:
		sessionToken, _ := c.Locals("csrfToken").(string)
		supplied := c.Get("X-CSRF-Token")
		if supplied == "" {
			supplied = c.FormValue("_csrf")
		}
		if sessionToken == "" || subtle.ConstantTimeCompare([]byte(sessionToken), []byte(supplied)) != 1 {
			return fiber.ErrForbidden
		}
	}
	return c.Next()
}
