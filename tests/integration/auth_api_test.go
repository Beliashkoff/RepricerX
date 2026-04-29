//go:build integration

package integration

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
)

// --- Register ---

func TestRegister_Success(t *testing.T) {
	truncate(t)
	client := newClient()

	resp := doJSON(t, client, http.MethodPost, "/api/auth/register", map[string]any{
		"email": testEmail, "password": testPassword, "displayName": testName,
	})
	mustStatus(t, resp, http.StatusCreated)

	var body struct{ Email string }
	mustDecode(t, resp, &body)
	if body.Email != testEmail {
		t.Fatalf("want email %q, got %q", testEmail, body.Email)
	}
}

func TestRegister_DuplicateEmail(t *testing.T) {
	truncate(t)
	client := newClient()

	payload := map[string]any{
		"email": testEmail, "password": testPassword, "displayName": testName,
	}
	mustStatus(t, doJSON(t, client, http.MethodPost, "/api/auth/register", payload), http.StatusCreated)
	resp := doJSON(t, client, http.MethodPost, "/api/auth/register", payload)
	mustStatus(t, resp, http.StatusConflict)
	mustErrorCode(t, resp, "email_taken")
}

func TestRegister_WeakPassword(t *testing.T) {
	truncate(t)
	client := newClient()

	resp := doJSON(t, client, http.MethodPost, "/api/auth/register", map[string]any{
		"email": testEmail, "password": "short", "displayName": testName,
	})
	mustStatus(t, resp, http.StatusBadRequest)
	mustErrorCode(t, resp, "weak_password")
}

func TestRegister_InvalidEmail(t *testing.T) {
	truncate(t)
	client := newClient()

	resp := doJSON(t, client, http.MethodPost, "/api/auth/register", map[string]any{
		"email": "not-an-email", "password": testPassword, "displayName": testName,
	})
	mustStatus(t, resp, http.StatusBadRequest)
	mustErrorCode(t, resp, "invalid_email")
}

func TestRegister_BadJSON(t *testing.T) {
	truncate(t)
	client := newClient()

	resp := doJSON(t, client, http.MethodPost, "/api/auth/register", map[string]any{
		"email": testEmail, // missing required password and displayName
	})
	mustStatus(t, resp, http.StatusBadRequest)
}

// --- Login ---

func TestLogin_UnverifiedUser(t *testing.T) {
	truncate(t)
	client := newClient()

	doJSON(t, client, http.MethodPost, "/api/auth/register", map[string]any{
		"email": testEmail, "password": testPassword, "displayName": testName,
	})

	resp := doJSON(t, client, http.MethodPost, "/api/auth/login", map[string]any{
		"email": testEmail, "password": testPassword,
	})
	mustStatus(t, resp, http.StatusUnauthorized)
	mustErrorCode(t, resp, "invalid_credentials")
}

func TestLogin_WrongPassword(t *testing.T) {
	truncate(t)
	client := newClient()

	doJSON(t, client, http.MethodPost, "/api/auth/register", map[string]any{
		"email": testEmail, "password": testPassword, "displayName": testName,
	})
	activateUser(t, testEmail)

	resp := doJSON(t, client, http.MethodPost, "/api/auth/login", map[string]any{
		"email": testEmail, "password": "WrongPass456!",
	})
	mustStatus(t, resp, http.StatusUnauthorized)
	mustErrorCode(t, resp, "invalid_credentials")
}

func TestLogin_NonExistentUser(t *testing.T) {
	truncate(t)
	client := newClient()

	resp := doJSON(t, client, http.MethodPost, "/api/auth/login", map[string]any{
		"email": "nobody@example.com", "password": testPassword,
	})
	mustStatus(t, resp, http.StatusUnauthorized)
	mustErrorCode(t, resp, "invalid_credentials")
}

func TestLogin_Success(t *testing.T) {
	truncate(t)
	client := newClient()

	doJSON(t, client, http.MethodPost, "/api/auth/register", map[string]any{
		"email": testEmail, "password": testPassword, "displayName": testName,
	})
	activateUser(t, testEmail)

	resp := doJSON(t, client, http.MethodPost, "/api/auth/login", map[string]any{
		"email": testEmail, "password": testPassword,
	})
	mustStatus(t, resp, http.StatusOK)

	var body struct {
		Email       string `json:"email"`
		DisplayName string `json:"displayName"`
	}
	mustDecode(t, resp, &body)
	if body.Email != testEmail {
		t.Fatalf("want email %q, got %q", testEmail, body.Email)
	}
	if body.DisplayName != testName {
		t.Fatalf("want displayName %q, got %q", testName, body.DisplayName)
	}

	// session cookie must be set
	cookies := resp.Cookies()
	var found bool
	for _, c := range cookies {
		if c.Name == "rx_session" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("rx_session cookie not set after login")
	}
}

// --- /api/auth/me ---

func TestMe_Unauthorized(t *testing.T) {
	truncate(t)
	client := newClient()

	resp := doJSON(t, client, http.MethodGet, "/api/auth/me", nil)
	mustStatus(t, resp, http.StatusUnauthorized)
}

func TestMe_Success(t *testing.T) {
	truncate(t)
	client := newClient()

	doJSON(t, client, http.MethodPost, "/api/auth/register", map[string]any{
		"email": testEmail, "password": testPassword, "displayName": testName,
	})
	activateUser(t, testEmail)
	doJSON(t, client, http.MethodPost, "/api/auth/login", map[string]any{
		"email": testEmail, "password": testPassword,
	})

	resp := doJSON(t, client, http.MethodGet, "/api/auth/me", nil)
	mustStatus(t, resp, http.StatusOK)

	var body struct {
		Email       string `json:"email"`
		DisplayName string `json:"displayName"`
		Status      string `json:"status"`
	}
	mustDecode(t, resp, &body)
	if body.Email != testEmail {
		t.Fatalf("want email %q, got %q", testEmail, body.Email)
	}
	if body.Status != "active" {
		t.Fatalf("want status %q, got %q", "active", body.Status)
	}
}

// --- PATCH /api/auth/me ---

func TestUpdateMe_CSRFBlocked(t *testing.T) {
	truncate(t)
	client := newClient()

	doJSON(t, client, http.MethodPost, "/api/auth/register", map[string]any{
		"email": testEmail, "password": testPassword, "displayName": testName,
	})
	activateUser(t, testEmail)
	doJSON(t, client, http.MethodPost, "/api/auth/login", map[string]any{
		"email": testEmail, "password": testPassword,
	})

	// no Origin header → CSRF blocked
	resp := doJSON(t, client, http.MethodPatch, "/api/auth/me", map[string]any{
		"displayName": "New Name",
	})
	mustStatus(t, resp, http.StatusForbidden)
	mustErrorCode(t, resp, "csrf_blocked")
}

func TestUpdateMe_Success(t *testing.T) {
	truncate(t)
	client := newClient()

	doJSON(t, client, http.MethodPost, "/api/auth/register", map[string]any{
		"email": testEmail, "password": testPassword, "displayName": testName,
	})
	activateUser(t, testEmail)
	doJSON(t, client, http.MethodPost, "/api/auth/login", map[string]any{
		"email": testEmail, "password": testPassword,
	})

	newName := "Updated Name"
	resp := doJSON(t, client, http.MethodPatch, "/api/auth/me",
		map[string]any{"displayName": newName},
		withOrigin(),
	)
	mustStatus(t, resp, http.StatusOK)

	var body struct{ DisplayName string `json:"displayName"` }
	mustDecode(t, resp, &body)
	if body.DisplayName != newName {
		t.Fatalf("want displayName %q, got %q", newName, body.DisplayName)
	}
}

// --- Logout ---

func TestLogout_ClearsSession(t *testing.T) {
	truncate(t)
	client := newClient()

	doJSON(t, client, http.MethodPost, "/api/auth/register", map[string]any{
		"email": testEmail, "password": testPassword, "displayName": testName,
	})
	activateUser(t, testEmail)
	doJSON(t, client, http.MethodPost, "/api/auth/login", map[string]any{
		"email": testEmail, "password": testPassword,
	})

	// pre-logout: me should work
	mustStatus(t, doJSON(t, client, http.MethodGet, "/api/auth/me", nil), http.StatusOK)

	// logout
	mustStatus(t,
		doJSON(t, client, http.MethodPost, "/api/auth/logout", nil, withOrigin()),
		http.StatusNoContent,
	)

	// post-logout: me should be unauthorized
	mustStatus(t, doJSON(t, client, http.MethodGet, "/api/auth/me", nil), http.StatusUnauthorized)
}

// --- Email verification full flow ---

func TestVerifyEmail_FullFlow(t *testing.T) {
	truncate(t)
	client := newClient()

	// 1. Register
	mustStatus(t, doJSON(t, client, http.MethodPost, "/api/auth/register", map[string]any{
		"email": testEmail, "password": testPassword, "displayName": testName,
	}), http.StatusCreated)

	// 2. Login before verify → should fail
	mustStatus(t, doJSON(t, client, http.MethodPost, "/api/auth/login", map[string]any{
		"email": testEmail, "password": testPassword,
	}), http.StatusUnauthorized)

	// 3. Extract token from captured email
	email := testMailer.last()
	if email == nil {
		t.Fatal("no verification email captured")
	}
	tok := extractVerificationToken(t, email)

	// 4. Verify email via GET /api/auth/verify?token=...
	// The endpoint redirects to frontend; don't follow redirects
	resp := doJSON(t, client, http.MethodGet,
		"/api/auth/verify?token="+tok, nil)
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("want redirect 302, got %d", resp.StatusCode)
	}
	loc := resp.Header.Get("Location")
	if loc == "" {
		t.Fatal("no Location header in verify response")
	}
	if !strings.Contains(loc, "verified=1") {
		t.Fatalf("expected verified=1 in redirect Location %q", loc)
	}

	// 5. Login after verify → should succeed
	mustStatus(t, doJSON(t, client, http.MethodPost, "/api/auth/login", map[string]any{
		"email": testEmail, "password": testPassword,
	}), http.StatusOK)
}

// --- Multiple sessions / isolation ---

func TestMultipleSessions_Independent(t *testing.T) {
	truncate(t)

	// Two independent clients = two independent sessions
	c1 := newClient()
	c2 := newClient()

	for i, email := range []string{
		fmt.Sprintf("user%d@example.com", 1),
		fmt.Sprintf("user%d@example.com", 2),
	} {
		client := []*http.Client{c1, c2}[i]
		doJSON(t, client, http.MethodPost, "/api/auth/register", map[string]any{
			"email": email, "password": testPassword, "displayName": testName,
		})
		activateUser(t, email)
		mustStatus(t, doJSON(t, client, http.MethodPost, "/api/auth/login", map[string]any{
			"email": email, "password": testPassword,
		}), http.StatusOK)
	}

	// Each client can access /me independently
	mustStatus(t, doJSON(t, c1, http.MethodGet, "/api/auth/me", nil), http.StatusOK)
	mustStatus(t, doJSON(t, c2, http.MethodGet, "/api/auth/me", nil), http.StatusOK)

	// Logout c1 doesn't affect c2
	doJSON(t, c1, http.MethodPost, "/api/auth/logout", nil, withOrigin())
	mustStatus(t, doJSON(t, c1, http.MethodGet, "/api/auth/me", nil), http.StatusUnauthorized)
	mustStatus(t, doJSON(t, c2, http.MethodGet, "/api/auth/me", nil), http.StatusOK)
}

