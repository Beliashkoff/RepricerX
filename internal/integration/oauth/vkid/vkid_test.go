package vkid

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func TestAuthorizationURL_HasRequiredPKCEParams(t *testing.T) {
	c := New("cid", "sec", "https://app/cb")
	authURL := c.AuthorizationURL("st", "ch")
	u, err := url.Parse(authURL)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	q := u.Query()
	if q.Get("response_type") != "code" {
		t.Errorf("response_type = %q", q.Get("response_type"))
	}
	if q.Get("code_challenge") != "ch" {
		t.Errorf("code_challenge = %q", q.Get("code_challenge"))
	}
	// VK ID OAuth 2.1 ОБЯЗАТЕЛЬНО требует PKCE с S256.
	if q.Get("code_challenge_method") != "S256" {
		t.Errorf("code_challenge_method = %q, ожидался S256", q.Get("code_challenge_method"))
	}
	if q.Get("state") != "st" {
		t.Errorf("state = %q", q.Get("state"))
	}
}

func TestExchange_SendsPKCEVerifier(t *testing.T) {
	var seenForm url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		seenForm, _ = url.ParseQuery(string(raw))
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "TKN"})
	}))
	defer srv.Close()

	c := NewWithEndpoints("cid", "sec", "https://app/cb", "x", srv.URL, "x")
	tok, err := c.Exchange(context.Background(), "auth-code", "the-verifier")
	if err != nil {
		t.Fatalf("Exchange: %v", err)
	}
	if tok != "TKN" {
		t.Fatalf("token = %q", tok)
	}
	if seenForm.Get("code_verifier") != "the-verifier" {
		t.Errorf("code_verifier = %q", seenForm.Get("code_verifier"))
	}
	if seenForm.Get("grant_type") != "authorization_code" {
		t.Errorf("grant_type = %q", seenForm.Get("grant_type"))
	}
}

func TestFetchUser_ParsesNestedUser(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"user": map[string]any{
				"user_id":    "1234",
				"email":      "alice@example.com",
				"first_name": "Alice",
				"last_name":  "Petrova",
			},
		})
	}))
	defer srv.Close()

	c := NewWithEndpoints("cid", "sec", "https://app/cb", "x", "x", srv.URL)
	info, err := c.FetchUser(context.Background(), "tok")
	if err != nil {
		t.Fatalf("FetchUser: %v", err)
	}
	if info.ProviderUserID != "1234" {
		t.Errorf("ProviderUserID = %q", info.ProviderUserID)
	}
	if info.Email != "alice@example.com" {
		t.Errorf("Email = %q", info.Email)
	}
	if info.DisplayName != "Alice Petrova" {
		t.Errorf("DisplayName = %q", info.DisplayName)
	}
}

func TestFetchUser_RejectsEmptyUserID(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"user": map[string]any{"user_id": ""},
		})
	}))
	defer srv.Close()
	c := NewWithEndpoints("cid", "sec", "https://app/cb", "x", "x", srv.URL)
	_, err := c.FetchUser(context.Background(), "tok")
	if err == nil {
		t.Fatalf("ожидалась ошибка при пустом user_id")
	}
}
