package yandex

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestAuthorizationURL_HasRequiredParams(t *testing.T) {
	c := New("cid", "secret", "https://app/cb")
	authURL := c.AuthorizationURL("st-1", "ch-1")
	u, err := url.Parse(authURL)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	q := u.Query()
	cases := map[string]string{
		"response_type":         "code",
		"client_id":             "cid",
		"redirect_uri":          "https://app/cb",
		"state":                 "st-1",
		"code_challenge":        "ch-1",
		"code_challenge_method": "S256",
	}
	for k, want := range cases {
		if got := q.Get(k); got != want {
			t.Errorf("%s = %q, ожидался %q", k, got, want)
		}
	}
}

func TestExchange_FormsRequestAndParsesResponse(t *testing.T) {
	var capturedBody string
	var capturedCT string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedCT = r.Header.Get("Content-Type")
		raw, _ := io.ReadAll(r.Body)
		capturedBody = string(raw)
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "tok-abc"})
	}))
	defer srv.Close()

	c := NewWithEndpoints("cid", "sec", "https://app/cb", "x", srv.URL, "x")
	tok, err := c.Exchange(context.Background(), "code-123", "ver-456")
	if err != nil {
		t.Fatalf("Exchange: %v", err)
	}
	if tok != "tok-abc" {
		t.Fatalf("token = %q", tok)
	}
	if !strings.Contains(capturedCT, "application/x-www-form-urlencoded") {
		t.Errorf("Content-Type = %q", capturedCT)
	}
	form, _ := url.ParseQuery(capturedBody)
	if form.Get("grant_type") != "authorization_code" {
		t.Errorf("grant_type = %q", form.Get("grant_type"))
	}
	if form.Get("code") != "code-123" {
		t.Errorf("code = %q", form.Get("code"))
	}
	if form.Get("code_verifier") != "ver-456" {
		t.Errorf("code_verifier = %q", form.Get("code_verifier"))
	}
	if form.Get("client_id") != "cid" || form.Get("client_secret") != "sec" {
		t.Errorf("client creds mismatch: %q / %q", form.Get("client_id"), form.Get("client_secret"))
	}
}

func TestExchange_RejectsNon200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "invalid", http.StatusBadRequest)
	}))
	defer srv.Close()

	c := NewWithEndpoints("cid", "sec", "https://app/cb", "x", srv.URL, "x")
	_, err := c.Exchange(context.Background(), "c", "v")
	if err == nil {
		t.Fatalf("ожидалась ошибка")
	}
}

func TestFetchUser_UsesOAuthAuthHeader(t *testing.T) {
	var seenAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenAuth = r.Header.Get("Authorization")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":            "777",
			"login":         "alice",
			"display_name":  "Alice",
			"default_email": "alice@yandex.ru",
		})
	}))
	defer srv.Close()

	c := NewWithEndpoints("cid", "sec", "https://app/cb", "x", "x", srv.URL)
	info, err := c.FetchUser(context.Background(), "TOKEN-XYZ")
	if err != nil {
		t.Fatalf("FetchUser: %v", err)
	}
	if seenAuth != "OAuth TOKEN-XYZ" {
		t.Errorf("Authorization = %q, ожидался %q", seenAuth, "OAuth TOKEN-XYZ")
	}
	if info.ProviderUserID != "777" {
		t.Errorf("ProviderUserID = %q", info.ProviderUserID)
	}
	if info.Email != "alice@yandex.ru" {
		t.Errorf("Email = %q", info.Email)
	}
	if info.DisplayName != "Alice" {
		t.Errorf("DisplayName = %q", info.DisplayName)
	}
}

func TestFetchUser_FallsBackToEmailsArray(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":     "1",
			"login":  "bob",
			"emails": []string{"bob@yandex.ru"},
		})
	}))
	defer srv.Close()

	c := NewWithEndpoints("cid", "sec", "https://app/cb", "x", "x", srv.URL)
	info, err := c.FetchUser(context.Background(), "t")
	if err != nil {
		t.Fatalf("FetchUser: %v", err)
	}
	if info.Email != "bob@yandex.ru" {
		t.Errorf("Email = %q", info.Email)
	}
	if info.DisplayName != "bob" {
		t.Errorf("DisplayName = %q, ожидался login", info.DisplayName)
	}
}
