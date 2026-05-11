// Package vkid реализует OAuth 2.1 поток для VK ID (https://id.vk.com).
//
// VK ID — современный поток на смену устаревшему oauth.vk.com.
// Особенности:
//   - PKCE с S256 — обязателен;
//   - токен живёт сутки (refresh-token-ы — отдельная тема, в этом
//     приложении они не нужны: после логина ведём собственную сессию).
package vkid

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Beliashkoff/RepricerX/internal/domain"
	"github.com/Beliashkoff/RepricerX/internal/integration/oauth"
)

const (
	defaultAuthorizeURL = "https://id.vk.com/authorize"
	defaultTokenURL     = "https://id.vk.com/oauth2/auth"
	defaultUserInfoURL  = "https://id.vk.com/oauth2/user_info"
	defaultScope        = "email"
)

// Client — адаптер VK ID. Конструктор New() использует production-эндпоинты;
// тестовые конструкторы могут подменять URL через NewWithEndpoints.
type Client struct {
	clientID     string
	clientSecret string
	redirectURI  string

	authorizeURL string
	tokenURL     string
	userInfoURL  string

	http *http.Client
}

// New возвращает клиента VK ID с production-эндпоинтами.
func New(clientID, clientSecret, redirectURI string) *Client {
	return NewWithEndpoints(clientID, clientSecret, redirectURI,
		defaultAuthorizeURL, defaultTokenURL, defaultUserInfoURL)
}

// NewWithEndpoints позволяет переопределить URL — для тестов.
func NewWithEndpoints(clientID, clientSecret, redirectURI, authorizeURL, tokenURL, userInfoURL string) *Client {
	return &Client{
		clientID:     clientID,
		clientSecret: clientSecret,
		redirectURI:  redirectURI,
		authorizeURL: authorizeURL,
		tokenURL:     tokenURL,
		userInfoURL:  userInfoURL,
		http:         &http.Client{Timeout: 10 * time.Second},
	}
}

// Name — идентификатор провайдера.
func (c *Client) Name() domain.OAuthProvider { return domain.OAuthProviderVK }

// AuthorizationURL строит URL для редиректа на форму согласия VK ID.
func (c *Client) AuthorizationURL(state, codeChallenge string) string {
	q := url.Values{}
	q.Set("response_type", "code")
	q.Set("client_id", c.clientID)
	q.Set("redirect_uri", c.redirectURI)
	q.Set("scope", defaultScope)
	q.Set("state", state)
	q.Set("code_challenge", codeChallenge)
	q.Set("code_challenge_method", "S256")
	return c.authorizeURL + "?" + q.Encode()
}

// Exchange обменивает authorization code на access_token.
// VK ID требует PKCE: code_verifier должен совпадать с тем, что
// использовался при генерации code_challenge для AuthorizationURL.
func (c *Client) Exchange(ctx context.Context, code, codeVerifier string) (string, error) {
	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("code", code)
	form.Set("client_id", c.clientID)
	form.Set("client_secret", c.clientSecret)
	form.Set("redirect_uri", c.redirectURI)
	form.Set("code_verifier", codeVerifier)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.tokenURL,
		strings.NewReader(form.Encode()))
	if err != nil {
		return "", fmt.Errorf("vkid: build token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("vkid: token request: %w", errors.Join(oauth.ErrProviderUnavailable, err))
	}
	defer resp.Body.Close() //nolint:errcheck

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("vkid: token http %d: %s: %w", resp.StatusCode, string(body), oauth.ErrProviderRejected)
	}

	var tr struct {
		AccessToken string `json:"access_token"`
		Error       string `json:"error"`
	}
	if err = json.Unmarshal(body, &tr); err != nil {
		return "", fmt.Errorf("vkid: parse token response: %w", oauth.ErrInvalidResponse)
	}
	if tr.Error != "" || tr.AccessToken == "" {
		return "", fmt.Errorf("vkid: token error %q: %w", tr.Error, oauth.ErrProviderRejected)
	}
	return tr.AccessToken, nil
}

// FetchUser получает данные пользователя через user_info endpoint VK ID.
func (c *Client) FetchUser(ctx context.Context, accessToken string) (domain.OAuthUserInfo, error) {
	form := url.Values{}
	form.Set("access_token", accessToken)
	form.Set("client_id", c.clientID)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.userInfoURL,
		strings.NewReader(form.Encode()))
	if err != nil {
		return domain.OAuthUserInfo{}, fmt.Errorf("vkid: build user_info request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.http.Do(req)
	if err != nil {
		return domain.OAuthUserInfo{}, fmt.Errorf("vkid: user_info request: %w",
			errors.Join(oauth.ErrProviderUnavailable, err))
	}
	defer resp.Body.Close() //nolint:errcheck

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if resp.StatusCode != http.StatusOK {
		return domain.OAuthUserInfo{}, fmt.Errorf("vkid: user_info http %d: %s: %w",
			resp.StatusCode, string(body), oauth.ErrProviderRejected)
	}

	var ur struct {
		User struct {
			UserID    string `json:"user_id"`
			Email     string `json:"email"`
			FirstName string `json:"first_name"`
			LastName  string `json:"last_name"`
		} `json:"user"`
		Error string `json:"error"`
	}
	if err = json.Unmarshal(body, &ur); err != nil {
		return domain.OAuthUserInfo{}, fmt.Errorf("vkid: parse user_info: %w", oauth.ErrInvalidResponse)
	}
	if ur.Error != "" || ur.User.UserID == "" {
		return domain.OAuthUserInfo{}, fmt.Errorf("vkid: user_info error %q: %w",
			ur.Error, oauth.ErrProviderRejected)
	}

	display := strings.TrimSpace(ur.User.FirstName + " " + ur.User.LastName)
	if display == "" {
		display = "Пользователь VK"
	}
	return domain.OAuthUserInfo{
		ProviderUserID: ur.User.UserID,
		Email:          strings.ToLower(strings.TrimSpace(ur.User.Email)),
		DisplayName:    display,
	}, nil
}
