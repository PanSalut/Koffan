package handlers

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"shopping-list/db"
	"testing"
	"time"

	"github.com/go-jose/go-jose/v3"
	"github.com/go-jose/go-jose/v3/jwt"
	"github.com/gofiber/fiber/v2"
)

func TestMockOIDCCallbackCreatesAdminAndSession(t *testing.T) {
	initTestDatabase(t)
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: key}, (&jose.SignerOptions{}).WithType("JWT").WithHeader("kid", "test-key"))
	if err != nil {
		t.Fatal(err)
	}
	const nonce = "test-nonce"
	var issuer string
	providerHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/.well-known/openid-configuration":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"issuer": issuer, "authorization_endpoint": issuer + "/authorize",
				"token_endpoint": issuer + "/token", "jwks_uri": issuer + "/keys",
				"response_types_supported": []string{"code"}, "subject_types_supported": []string{"public"},
				"id_token_signing_alg_values_supported": []string{"RS256"},
			})
		case "/keys":
			_ = json.NewEncoder(w).Encode(jose.JSONWebKeySet{Keys: []jose.JSONWebKey{{Key: &key.PublicKey, KeyID: "test-key", Algorithm: "RS256", Use: "sig"}}})
		case "/token":
			now := time.Now()
			raw, signErr := jwt.Signed(signer).Claims(map[string]interface{}{
				"iss": issuer, "sub": "subject-123", "aud": "koffan-test", "exp": now.Add(time.Hour).Unix(), "iat": now.Unix(),
				"nonce": nonce, "preferred_username": "oidc-tester", "name": "OIDC Tester", "email": "oidc@example.test",
				"groups": []string{"Koffan Admins"},
			}).CompactSerialize()
			if signErr != nil {
				http.Error(w, signErr.Error(), 500)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"access_token": "access", "token_type": "Bearer", "expires_in": 3600, "id_token": raw})
		default:
			http.NotFound(w, r)
		}
	})
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Skipf("sandbox does not permit the mocked OIDC listener: %v", err)
	}
	provider := &httptest.Server{Listener: listener, Config: &http.Server{Handler: providerHandler}}
	provider.Start()
	defer provider.Close()
	issuer = provider.URL

	t.Setenv("OIDC_ENABLED", "true")
	t.Setenv("OIDC_ISSUER_URL", issuer)
	t.Setenv("OIDC_CLIENT_ID", "koffan-test")
	t.Setenv("OIDC_CLIENT_SECRET", "secret")
	t.Setenv("OIDC_REDIRECT_URL", "https://koffan.example/auth/oidc/callback")
	t.Setenv("OIDC_ADMIN_GROUP", "Koffan Admins")
	state := "valid-state"
	oidcAttempts.Lock()
	oidcAttempts.m[state] = oidcAttempt{Nonce: nonce, Verifier: "verifier", Expires: time.Now().Add(time.Minute)}
	oidcAttempts.Unlock()

	app := fiber.New()
	app.Get("/auth/oidc/callback", OIDCCallback)
	resp, err := app.Test(httptest.NewRequest(http.MethodGet, "/auth/oidc/callback?state="+state+"&code=valid-code", nil), 5000)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusFound || resp.Header.Get("Location") != "/" {
		t.Fatalf("callback status=%d location=%q", resp.StatusCode, resp.Header.Get("Location"))
	}
	user, err := db.GetUserByUsername("oidc-tester")
	if err != nil || !user.IsAdmin || user.AuthSource != "oidc" {
		t.Fatalf("OIDC user=%#v err=%v", user, err)
	}
	var sessionCookie *http.Cookie
	for _, cookie := range resp.Cookies() {
		if cookie.Name == SessionCookieName {
			sessionCookie = cookie
		}
	}
	if sessionCookie == nil {
		t.Fatal("OIDC callback did not create an application session")
	}
	if _, err = db.GetSession(sessionCookie.Value); err != nil {
		t.Fatalf("OIDC session was not persisted: %v", err)
	}
}

func TestOIDCCallbackRejectsInvalidAndExpiredState(t *testing.T) {
	app := fiber.New()
	app.Get("/auth/oidc/callback", OIDCCallback)
	for _, state := range []string{"missing-state", "expired-state"} {
		if state == "expired-state" {
			oidcAttempts.Lock()
			oidcAttempts.m[state] = oidcAttempt{Expires: time.Now().Add(-time.Minute)}
			oidcAttempts.Unlock()
		}
		resp, err := app.Test(httptest.NewRequest(http.MethodGet, "/auth/oidc/callback?state="+state+"&code=x", nil))
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusFound || resp.Header.Get("Location") != "/login?error=oidc" {
			t.Fatalf("state %q was accepted: status=%d location=%q", state, resp.StatusCode, resp.Header.Get("Location"))
		}
	}
}
