package handlers

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"shopping-list/db"
	"strings"
	"sync"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/gofiber/fiber/v2"
	"golang.org/x/oauth2"
)

type oidcAttempt struct {
	Nonce, Verifier string
	Expires         time.Time
}

var oidcAttempts = struct {
	sync.Mutex
	m map[string]oidcAttempt
}{m: map[string]oidcAttempt{}}

func OIDCEnabled() bool { return strings.EqualFold(os.Getenv("OIDC_ENABLED"), "true") }
func randomURLToken() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}
func oidcProvider(ctx context.Context) (*oidc.Provider, *oauth2.Config, error) {
	if !OIDCEnabled() {
		return nil, nil, errors.New("OIDC is disabled")
	}
	// OIDC issuer identifiers are exact, security-sensitive strings. In
	// particular, Authentik's issuer ends with a slash; do not normalize it.
	issuer := strings.TrimSpace(os.Getenv("OIDC_ISSUER_URL"))
	clientID := os.Getenv("OIDC_CLIENT_ID")
	redirect := os.Getenv("OIDC_REDIRECT_URL")
	if issuer == "" || clientID == "" || redirect == "" {
		return nil, nil, errors.New("OIDC configuration is incomplete")
	}
	p, e := oidc.NewProvider(ctx, issuer)
	if e != nil {
		return nil, nil, e
	}
	scopes := strings.Fields(os.Getenv("OIDC_SCOPES"))
	if len(scopes) == 0 {
		scopes = []string{oidc.ScopeOpenID, "profile", "email"}
	}
	return p, &oauth2.Config{ClientID: clientID, ClientSecret: os.Getenv("OIDC_CLIENT_SECRET"), Endpoint: p.Endpoint(), RedirectURL: redirect, Scopes: scopes}, nil
}
func OIDCLogin(c *fiber.Ctx) error {
	p, cfg, e := oidcProvider(c.Context())
	_ = p
	if e != nil {
		return c.Status(503).SendString(e.Error())
	}
	state, nonce, verifier := randomURLToken(), randomURLToken(), randomURLToken()
	sum := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(sum[:])
	oidcAttempts.Lock()
	oidcAttempts.m[state] = oidcAttempt{nonce, verifier, time.Now().Add(10 * time.Minute)}
	for k, v := range oidcAttempts.m {
		if time.Now().After(v.Expires) {
			delete(oidcAttempts.m, k)
		}
	}
	oidcAttempts.Unlock()
	url := cfg.AuthCodeURL(state, oidc.Nonce(nonce), oauth2.SetAuthURLParam("code_challenge", challenge), oauth2.SetAuthURLParam("code_challenge_method", "S256"))
	return c.Redirect(url)
}
func claimString(m map[string]json.RawMessage, key string) string {
	var s string
	if raw := m[key]; raw != nil {
		_ = json.Unmarshal(raw, &s)
	}
	return s
}
func claimStrings(m map[string]json.RawMessage, key string) []string {
	var out []string
	if raw := m[key]; raw != nil {
		if json.Unmarshal(raw, &out) != nil {
			var s string
			if json.Unmarshal(raw, &s) == nil {
				out = []string{s}
			}
		}
	}
	return out
}
func envDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
func containsString(values []string, want string) bool {
	for _, v := range values {
		if v == want {
			return true
		}
	}
	return false
}
func OIDCCallback(c *fiber.Ctx) error {
	state := c.Query("state")
	oidcAttempts.Lock()
	attempt, ok := oidcAttempts.m[state]
	delete(oidcAttempts.m, state)
	oidcAttempts.Unlock()
	if !ok || time.Now().After(attempt.Expires) {
		return c.Redirect("/login?error=oidc")
	}
	provider, cfg, e := oidcProvider(c.Context())
	if e != nil {
		return c.Redirect("/login?error=oidc")
	}
	tok, e := cfg.Exchange(c.Context(), c.Query("code"), oauth2.SetAuthURLParam("code_verifier", attempt.Verifier))
	if e != nil {
		return c.Redirect("/login?error=oidc")
	}
	raw, ok := tok.Extra("id_token").(string)
	if !ok {
		return c.Redirect("/login?error=oidc")
	}
	idToken, e := provider.Verifier(&oidc.Config{ClientID: cfg.ClientID}).Verify(c.Context(), raw)
	if e != nil || idToken.Nonce != attempt.Nonce {
		return c.Redirect("/login?error=oidc")
	}
	claims := map[string]json.RawMessage{}
	if idToken.Claims(&claims) != nil {
		return c.Redirect("/login?error=oidc")
	}
	groups := claimStrings(claims, envDefault("OIDC_GROUPS_CLAIM", "groups"))
	if allowed := os.Getenv("OIDC_ALLOWED_GROUP"); allowed != "" && !containsString(groups, allowed) {
		return c.Redirect("/login?error=oidc")
	}
	username := claimString(claims, envDefault("OIDC_USERNAME_CLAIM", "preferred_username"))
	display := claimString(claims, envDefault("OIDC_DISPLAY_NAME_CLAIM", "name"))
	emailValue := claimString(claims, envDefault("OIDC_EMAIL_CLAIM", "email"))
	var email *string
	if emailValue != "" {
		email = &emailValue
	}
	auto := !strings.EqualFold(os.Getenv("OIDC_AUTO_CREATE_USERS"), "false")
	grantAdmin := os.Getenv("OIDC_ADMIN_GROUP") != "" && containsString(groups, os.Getenv("OIDC_ADMIN_GROUP"))
	user, e := db.UpsertOIDCUser(idToken.Issuer, idToken.Subject, username, display, email, auto, grantAdmin)
	if e != nil || user.Disabled {
		return c.Redirect("/login?error=oidc")
	}
	autoGroups, settingErr := db.OIDCAutoCreateGroups()
	if settingErr != nil {
		return c.Redirect("/login?error=oidc")
	}
	if e = db.SyncOIDCGroups(user.ID, groups, autoGroups); e != nil {
		return c.Redirect("/login?error=oidc")
	}
	// End any older session whose cached administrator state or list access may
	// no longer match the freshly synchronized identity-provider claims.
	DisconnectUserWebSockets(user.ID)
	if e = createApplicationSession(c, user); e != nil {
		return fiber.ErrInternalServerError
	}
	return c.Redirect("/")
}
