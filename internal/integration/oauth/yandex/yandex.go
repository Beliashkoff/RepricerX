// Package yandex реализует OAuth 2.0 поток для Яндекс ID
// (https://yandex.ru/dev/id).
//
// Особенности:
//   - PKCE опционален, но используется (единая логика для VK и Яндекс);
//   - access_token живёт долго (renewable);
//   - user_info — GET https://login.yandex.ru/info с заголовком
//     "Authorization: OAuth <token>" (не "Bearer").
package yandex

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
	defaultAuthorizeURL = "https://oauth.yandex.ru/authorize"
	defaultTokenURL     = "https://oauth.yandex.ru/token"
	defaultUserInfoURL  = "https://login.yandex.ru/info"
	// Минимально нужное: login:email. login:info требует отдельной галочки
	// в кабинете приложения; если её нет — Яндекс отказывает invalid_scope.
	// display_name мы выводим из login (короткий ник), который доступен и
	// без login:info — см. fallback в FetchUser.
	defaultScope = "login:email"
)

// Client — адаптер Яндекс ID.
type Client struct {
	clientID     string
	clientSecret string
	redirectURI  string

	authorizeURL string
	tokenURL     string
	userInfoURL  string

	http *http.Client
}

// New возвращает клиента с production-эндпоинтами.
func New(clientID, clientSecret, redirectURI string) *Client {
	return NewWithEndpoints(clientID, clientSecret, redirectURI,
		defaultAuthorizeURL, defaultTokenURL, defaultUserInfoURL)
}

// NewWithEndpoints позволяет переопределить URL для тестов.
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

func (c *Client) Name() domain.OAuthProvider { return domain.OAuthProviderYandex }

func (c *Client) AuthorizationURL(state, codeChallenge string) string {
	q := url.Values{}
	q.Set("response_type", "code")
	q.Set("client_id", c.clientID)
	q.Set("redirect_uri", c.redirectURI)
	q.Set("scope", defaultScope)
	q.Set("state", state)
	q.Set("code_challenge", codeChallenge)
	q.Set("code_challenge_method", "S256")
	q.Set("force_confirm", "yes")
	return c.authorizeURL + "?" + q.Encode()
}

func (c *Client) Exchange(ctx context.Context, code, codeVerifier string, _ url.Values) (string, error) {
	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("code", code)
	form.Set("client_id", c.clientID)
	form.Set("client_secret", c.clientSecret)
	form.Set("code_verifier", codeVerifier)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.tokenURL,
		strings.NewReader(form.Encode()))
	if err != nil {
		return "", fmt.Errorf("yandex: build token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("yandex: token request: %w",
			errors.Join(oauth.ErrProviderUnavailable, err))
	}
	defer resp.Body.Close() //nolint:errcheck

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("yandex: token http %d: %s: %w",
			resp.StatusCode, string(body), oauth.ErrProviderRejected)
	}

	var tr struct {
		AccessToken string `json:"access_token"`
		Error       string `json:"error"`
	}
	if err = json.Unmarshal(body, &tr); err != nil {
		return "", fmt.Errorf("yandex: parse token response: %w", oauth.ErrInvalidResponse)
	}
	if tr.Error != "" || tr.AccessToken == "" {
		return "", fmt.Errorf("yandex: token error %q: %w", tr.Error, oauth.ErrProviderRejected)
	}
	return tr.AccessToken, nil
}

func (c *Client) FetchUser(ctx context.Context, accessToken string) (domain.OAuthUserInfo, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.userInfoURL+"?format=json", nil)
	if err != nil {
		return domain.OAuthUserInfo{}, fmt.Errorf("yandex: build user_info request: %w", err)
	}
	// ВАЖНО: Yandex использует схему "OAuth", не "Bearer".
	req.Header.Set("Authorization", "OAuth "+accessToken)

	resp, err := c.http.Do(req)
	if err != nil {
		return domain.OAuthUserInfo{}, fmt.Errorf("yandex: user_info request: %w",
			errors.Join(oauth.ErrProviderUnavailable, err))
	}
	defer resp.Body.Close() //nolint:errcheck

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if resp.StatusCode != http.StatusOK {
		return domain.OAuthUserInfo{}, fmt.Errorf("yandex: user_info http %d: %s: %w",
			resp.StatusCode, string(body), oauth.ErrProviderRejected)
	}

	var ur struct {
		ID              string   `json:"id"`
		Login           string   `json:"login"`
		DisplayName     string   `json:"display_name"`
		RealName        string   `json:"real_name"`
		FirstName       string   `json:"first_name"`
		LastName        string   `json:"last_name"`
		DefaultEmail    string   `json:"default_email"`
		Emails          []string `json:"emails"`
	}
	if err = json.Unmarshal(body, &ur); err != nil {
		return domain.OAuthUserInfo{}, fmt.Errorf("yandex: parse user_info: %w", oauth.ErrInvalidResponse)
	}
	if ur.ID == "" {
		return domain.OAuthUserInfo{}, fmt.Errorf("yandex: empty user id: %w", oauth.ErrProviderRejected)
	}

	email := strings.ToLower(strings.TrimSpace(ur.DefaultEmail))
	if email == "" && len(ur.Emails) > 0 {
		email = strings.ToLower(strings.TrimSpace(ur.Emails[0]))
	}

	display := strings.TrimSpace(ur.DisplayName)
	if display == "" {
		display = strings.TrimSpace(ur.RealName)
	}
	if display == "" {
		display = strings.TrimSpace(ur.FirstName + " " + ur.LastName)
	}
	if display == "" {
		display = ur.Login
	}
	if display == "" {
		display = "Пользователь Яндекса"
	}

	return domain.OAuthUserInfo{
		ProviderUserID: ur.ID,
		Email:          email,
		DisplayName:    display,
	}, nil
}
