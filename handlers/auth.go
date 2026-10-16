package handlers

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"log"
	"net/url"
	"os"
	"shopping-list/db"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
)

const (
	SessionCookieName      = "session"
	DefaultSessionDuration = 7 * 24 * time.Hour // 7 days
)

// DefaultPassword is used when APP_PASSWORD is unset (development only).
const DefaultPassword = "shopping123"

func getSessionDuration() time.Duration {
	duration := os.Getenv("SESSION_DURATION")
	if duration == "" {
		return DefaultSessionDuration
	}
	d, err := parseSessionDuration(duration)
	if err != nil {
		log.Printf("Invalid SESSION_DURATION: %s", duration)
		return DefaultSessionDuration
	}
	return d
}

func parseSessionDuration(value string) (time.Duration, error) {
	if strings.HasSuffix(value, "d") {
		days, err := strconv.Atoi(strings.TrimSuffix(value, "d"))
		if err != nil {
			return 0, err
		}
		return time.Duration(days) * 24 * time.Hour, nil
	}
	return time.ParseDuration(value)
}

func getAppPassword() string {
	pass := os.Getenv("APP_PASSWORD")
	if pass == "" {
		pass = DefaultPassword
	}
	return pass
}

func isAuthDisabled() bool {
	return os.Getenv("DISABLE_AUTH") == "true"
}

// isSecureConnection checks if the request came over HTTPS
// Works both directly and behind reverse proxies
func isSecureConnection(c *fiber.Ctx) bool {
	// Check X-Forwarded-Proto header (set by reverse proxies)
	if c.Get("X-Forwarded-Proto") == "https" {
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
		if err == nil && session.ExpiresAt > time.Now().Unix() {
			return c.Redirect("/")
		}
	}
	return c.Render("login", withPageI18n(c, fiber.Map{
		"Error":     c.Query("error"),
		"MultiUser": MultiUser(),
	}), "")
}

// Login handles login form submission
func Login(c *fiber.Ctx) error {
	if MultiUser() && crossOriginRequest(c) {
		return sendError(c, fiber.StatusForbidden, "error.forbidden")
	}

	ip := c.IP()
	password := c.FormValue("password")

	// With MULTI_USER the credentials are a username and that user's password;
	// otherwise the single shared APP_PASSWORD.
	var user *db.User
	authenticated := false
	if MultiUser() {
		if u, err := db.Authenticate(c.FormValue("username"), password); err == nil {
			user, authenticated = u, true
		}
	} else {
		authenticated = password == getAppPassword()
	}

	if !authenticated {
		// Record failed attempt
		if loginLimiter != nil {
			if loginLimiter.RecordAttempt(ip) {
				// Limit exceeded, redirect with rate_limited error
				return c.Redirect("/login?error=rate_limited")
			}
		}
		return c.Redirect("/login?error=1")
	}

	// Successful login - reset attempts
	if loginLimiter != nil {
		loginLimiter.ResetAttempts(ip)
	}

	// Create session
	sessionID := generateSessionID()
	expiresAt := time.Now().Add(getSessionDuration())

	var err error
	if user != nil {
		workspaceID, _ := db.ResolveSessionWorkspace(user.ID, 0)
		err = db.CreateUserSession(sessionID, expiresAt.Unix(), user.ID, workspaceID)
	} else {
		err = db.CreateSession(sessionID, expiresAt.Unix())
	}
	if err != nil {
		return sendError(c, 500, "error.session_failed")
	}
	log.Printf("[AUTH] New session created: %s... (expires: %d)", sessionIDPrefix(sessionID), expiresAt.Unix())

	// Set cookie
	c.Cookie(&fiber.Cookie{
		Name:     SessionCookieName,
		Value:    sessionID,
		Expires:  expiresAt,
		HTTPOnly: true,
		Secure:   isSecureConnection(c),
		SameSite: "Lax",
		Path:     "/",
	})

	return c.Redirect("/")
}

// Logout handles logout
func Logout(c *fiber.Ctx) error {
	sessionID := c.Cookies(SessionCookieName)
	if sessionID != "" {
		db.DeleteSession(sessionID)
		closeWebSockets(func(client *webSocketClient) bool { return client.sessionID == sessionID })
	}

	// Clear cookie
	clearSessionCookie(c)

	return c.Redirect("/login")
}

// clearSessionCookie expires the session cookie in the browser.
func clearSessionCookie(c *fiber.Ctx) {
	c.Cookie(&fiber.Cookie{
		Name:     SessionCookieName,
		Value:    "",
		Expires:  time.Now().Add(-time.Hour),
		HTTPOnly: true,
		Secure:   isSecureConnection(c),
		SameSite: "Lax",
		Path:     "/",
	})
}

// redirectToLogin sends a browser to the login page and tells HTMX requests to
// do the same (HTMX ignores plain redirects for partial swaps).
func redirectToLogin(c *fiber.Ctx) error {
	if c.Get("HX-Request") == "true" {
		c.Set("HX-Redirect", "/login")
		return c.SendStatus(401)
	}
	return c.Redirect("/login")
}

// AuthMiddleware checks if user is authenticated
func AuthMiddleware(c *fiber.Ctx) error {
	if isAuthDisabled() {
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
		return redirectToLogin(c)
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
		clearSessionCookie(c)
		return redirectToLogin(c)
	}

	if session.ExpiresAt < time.Now().Unix() {
		log.Printf("[AUTH] Session expired for %s %s (expired: %d, now: %d)", c.Method(), path, session.ExpiresAt, time.Now().Unix())
		db.DeleteSession(sessionID)
		clearSessionCookie(c)
		return redirectToLogin(c)
	}

	c.Locals(localSession, session)

	if MultiUser() {
		if crossOriginRequest(c) {
			return sendError(c, fiber.StatusForbidden, "error.forbidden")
		}
		// Signed-in pages are per person: keep them out of shared caches and
		// the back/forward cache.
		c.Set(fiber.HeaderCacheControl, "private, no-store")
		return attachUserScope(c, session, path)
	}
	// Accounts were switched off again: a session that belongs to an account
	// must not open the shared workspace without the shared password.
	if session.UserID != 0 {
		db.DeleteSession(sessionID)
		clearSessionCookie(c)
		return redirectToLogin(c)
	}
	return c.Next()
}

// crossOriginRequest protects writes and the live feed using Fetch Metadata,
// falling back to Origin/Host checks when metadata is absent (including plain
// HTTP). Like net/http.CrossOriginProtection, requests with neither header are
// allowed. SameSite=Lax alone does not protect against other same-site origins.
func crossOriginRequest(c *fiber.Ctx) bool {
	switch c.Method() {
	case fiber.MethodGet, fiber.MethodHead, fiber.MethodOptions:
		if c.Path() != "/ws" {
			return false
		}
	}

	switch c.Get("Sec-Fetch-Site") {
	case "same-origin", "none":
		return false
	case "":
		// Without Fetch Metadata, compare the Origin with the public host.
	default:
		return true
	}
	origin := c.Get(fiber.HeaderOrigin)
	if origin == "" {
		return false
	}
	host := c.Get(fiber.HeaderHost)
	if host == "" {
		// Absolute-form requests can carry the host in the request URI.
		host = string(c.Context().Host())
	}
	forwardedHost := c.Get(fiber.HeaderXForwardedHost)
	firstForwardedHost, _, _ := strings.Cut(forwardedHost, ",")
	if parsed, err := url.Parse(origin); err == nil && parsed.Host != "" &&
		(strings.EqualFold(parsed.Host, host) || strings.EqualFold(parsed.Host, strings.TrimSpace(firstForwardedHost))) {
		return false
	}
	log.Printf("[AUTH] Cross-origin request refused: method=%q path=%q Origin=%q Host=%q X-Forwarded-Host=%q; reverse proxies must pass the original Host (proxy_set_header Host $host) or X-Forwarded-Host", c.Method(), c.Path(), origin, host, forwardedHost)
	return true
}

// attachUserScope loads the signed-in user and the workspace they are viewing
// into the request. Sessions from before MULTI_USER was enabled carry no user
// and are rejected, so everyone signs in with an account.
func attachUserScope(c *fiber.Ctx, session *db.Session, path string) error {
	reject := func() error {
		db.DeleteSession(session.ID)
		clearSessionCookie(c)
		return redirectToLogin(c)
	}
	if session.UserID == 0 {
		return reject()
	}
	user, err := db.GetUserByID(session.UserID)
	if err != nil {
		return reject()
	}
	workspaceID, err := db.ResolveSessionWorkspace(user.ID, session.WorkspaceID)
	if err != nil {
		return sendError(c, 503, "error.database_unavailable")
	}
	if workspaceID != session.WorkspaceID {
		db.SetSessionWorkspace(session.ID, workspaceID)
	}
	c.Locals(localUser, user)
	c.Locals(localWorkspace, workspaceID)

	// Pages remember the workspace they were rendered for. If the session has
	// since moved to another one (switched in a second tab, or removed from it),
	// refuse the request so the stale page reloads instead of writing into a
	// workspace the user is no longer looking at.
	if claimed := c.Get(WorkspaceHeader); claimed != "" && claimed != strconv.FormatInt(workspaceID, 10) {
		c.Set(WorkspaceMismatchHeader, "1")
		return sendError(c, fiber.StatusConflict, "error.workspace_changed")
	}

	// A user who belongs to no workspace can only reach the pages that create
	// or join one, their own account and, for administrators, other accounts.
	if workspaceID == 0 && !strings.HasPrefix(path, "/workspaces") && !strings.HasPrefix(path, "/admin/") &&
		!strings.HasPrefix(path, "/account/") && path != "/logout" && !strings.HasPrefix(path, "/locales") {
		if c.Method() == fiber.MethodGet && c.Get("HX-Request") != "true" {
			return c.Redirect("/workspaces")
		}
		return sendError(c, 403, "error.no_workspace")
	}
	return c.Next()
}
