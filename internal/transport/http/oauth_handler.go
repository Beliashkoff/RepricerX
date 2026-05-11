package transport

import (
	"errors"
	"net/http"
	"net/url"
	"strings"

	"github.com/Beliashkoff/RepricerX/internal/domain"
	"github.com/Beliashkoff/RepricerX/internal/service/auth"
	"github.com/gin-gonic/gin"
)

// OAuthHandler — публичные эндпоинты для OAuth-логина через VK ID и Яндекс ID.
//
// Endpoints:
//   - GET  /api/auth/oauth/:provider/start    — 302 на форму согласия провайдера
//   - GET  /api/auth/oauth/:provider/callback — обмен code + создание сессии или
//                                                редирект на /link-oauth
//   - POST /api/auth/oauth/link               — подтверждение привязки паролем
//
// /start и /callback — публичные GET, защищены state-токеном (CSRF-роль).
// /link — публичный POST, защищён link_token из Redis (одноразовый, TTL 10 мин).
type OAuthHandler struct {
	svc              *auth.Service
	secure           bool
	frontendBaseURL  string // куда редиректить после успеха/ошибки
}

func NewOAuthHandler(svc *auth.Service, secure bool, frontendBaseURL string) *OAuthHandler {
	return &OAuthHandler{
		svc:             svc,
		secure:          secure,
		frontendBaseURL: strings.TrimRight(frontendBaseURL, "/"),
	}
}

// Start godoc
//
//	@Summary		Начало OAuth-логина
//	@Description	Редиректит пользователя на форму согласия выбранного провайдера (VK ID или Яндекс ID).
//	@Description	После авторизации провайдер вернёт пользователя на /api/auth/oauth/:provider/callback.
//	@Tags			auth
//	@Param			provider	path	string	true	"vk или yandex"
//	@Success		302			"Редирект на форму согласия провайдера"
//	@Failure		400			{object}	errorResponse	"Неизвестный провайдер"
//	@Failure		503			{object}	errorResponse	"Провайдер не настроен"
//	@Router			/api/auth/oauth/{provider}/start [get]
func (h *OAuthHandler) Start(c *gin.Context) {
	provider, ok := parseProvider(c.Param("provider"))
	if !ok {
		errResp(c, http.StatusBadRequest, "unknown_provider", "Неизвестный провайдер")
		return
	}
	authURL, err := h.svc.BeginOAuth(c.Request.Context(), provider)
	if err != nil {
		h.handleErr(c, err)
		return
	}
	c.Redirect(http.StatusFound, authURL)
}

// Callback godoc
//
//	@Summary		Callback OAuth-провайдера
//	@Description	Принимает code+state от VK ID / Яндекс ID, обменивает на access_token,
//	@Description	получает данные пользователя и:
//	@Description	- если identity уже привязана → создаёт сессию, редирект на /dashboard;
//	@Description	- если email свободен → создаёт пользователя + identity + сессию;
//	@Description	- если email занят паролем → редирект на /link-oauth?token=...&email=...&provider=...;
//	@Description	- при ошибке провайдера → редирект на /login?oauth_error=...
//	@Tags			auth
//	@Param			provider	path	string	true	"vk или yandex"
//	@Param			code		query	string	false	"authorization code от провайдера"
//	@Param			state		query	string	false	"state из /start (CSRF)"
//	@Param			error		query	string	false	"ошибка от провайдера (если пользователь отказал)"
//	@Success		302	"Редирект на /dashboard или /link-oauth (или /login при ошибке)"
//	@Router			/api/auth/oauth/{provider}/callback [get]
func (h *OAuthHandler) Callback(c *gin.Context) {
	// Базовая валидация провайдера в URL — даже если он не совпадёт с тем,
	// что зашит в state-токене, реальной диспетчеризацией займётся сервис.
	if _, ok := parseProvider(c.Param("provider")); !ok {
		h.redirectError(c, "unknown_provider")
		return
	}

	if errParam := strings.TrimSpace(c.Query("error")); errParam != "" {
		h.redirectError(c, sanitizeErrorCode(errParam))
		return
	}

	state := strings.TrimSpace(c.Query("state"))
	code := strings.TrimSpace(c.Query("code"))
	if state == "" || code == "" {
		h.redirectError(c, "missing_params")
		return
	}

	login, link, err := h.svc.CompleteOAuth(c.Request.Context(), c.Request, state, code, c.Request.URL.Query())
	if err != nil {
		h.redirectFromAuthErr(c, err)
		return
	}

	if link != nil {
		h.redirectToLink(c, link)
		return
	}

	if login == nil {
		h.redirectError(c, "internal_error")
		return
	}
	SetSessionCookie(c, login.Plaintext, login.Session.IdleExpiresAt, h.secure)
	c.Redirect(http.StatusFound, h.frontendBaseURL+"/dashboard")
}

// Link godoc
//
//	@Summary		Подтверждение привязки OAuth-аккаунта
//	@Description	Пользователь, у которого email уже зарегистрирован обычным способом,
//	@Description	подтверждает паролем привязку OAuth-провайдера. При успехе создаётся
//	@Description	OAuth-идентичность и устанавливается сессия (HttpOnly cookie rx_session).
//	@Tags			auth
//	@Accept			json
//	@Produce		json
//	@Param			body	body		oauthLinkRequest	true	"link_token (из URL после OAuth-callback) и пароль аккаунта"
//	@Success		200		{object}	loginResponse	"Привязка подтверждена, сессия выдана"
//	@Failure		400		{object}	errorResponse	"Неверный запрос или истёкший link_token"
//	@Failure		401		{object}	errorResponse	"Неверный пароль"
//	@Router			/api/auth/oauth/link [post]
func (h *OAuthHandler) Link(c *gin.Context) {
	var req oauthLinkRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		errResp(c, http.StatusBadRequest, "bad_request", "Неверный формат запроса")
		return
	}
	if strings.TrimSpace(req.LinkToken) == "" || req.Password == "" {
		errResp(c, http.StatusBadRequest, "bad_request", "Неверный формат запроса")
		return
	}

	login, err := h.svc.ConfirmOAuthLink(c.Request.Context(), c.Request, req.LinkToken, req.Password)
	if err != nil {
		h.handleErr(c, err)
		return
	}
	SetSessionCookie(c, login.Plaintext, login.Session.IdleExpiresAt, h.secure)
	c.JSON(http.StatusOK, loginResponse{
		ID:          login.User.ID.String(),
		Email:       login.User.Email,
		DisplayName: login.User.DisplayName,
	})
}

func (h *OAuthHandler) handleErr(c *gin.Context, err error) {
	switch {
	case errors.Is(err, auth.ErrOAuthDisabled):
		errResp(c, http.StatusServiceUnavailable, "oauth_disabled", "OAuth-провайдер не настроен на сервере")
	case errors.Is(err, auth.ErrUnknownOAuthProvider):
		errResp(c, http.StatusBadRequest, "unknown_provider", "Неизвестный провайдер")
	case errors.Is(err, auth.ErrInvalidOAuthState):
		errResp(c, http.StatusBadRequest, "invalid_oauth_state", "Сессия OAuth истекла, попробуйте снова")
	case errors.Is(err, auth.ErrUserBlocked):
		errResp(c, http.StatusForbidden, "user_blocked", "Аккаунт заблокирован")
	case errors.Is(err, auth.ErrInvalidCredentials):
		errResp(c, http.StatusUnauthorized, "invalid_credentials", "Неверный пароль")
	case errors.Is(err, auth.ErrOAuthProviderFailed):
		errResp(c, http.StatusBadGateway, "oauth_provider_failed", "Не удалось получить данные у провайдера")
	default:
		errResp(c, http.StatusInternalServerError, "internal_error", "Внутренняя ошибка сервера")
	}
}

// redirectFromAuthErr редиректит на фронт с понятным кодом ошибки.
// Для случаев, когда ответ — 302 (Callback), а не JSON.
func (h *OAuthHandler) redirectFromAuthErr(c *gin.Context, err error) {
	switch {
	case errors.Is(err, auth.ErrOAuthDisabled):
		h.redirectError(c, "oauth_disabled")
	case errors.Is(err, auth.ErrUnknownOAuthProvider):
		h.redirectError(c, "unknown_provider")
	case errors.Is(err, auth.ErrInvalidOAuthState):
		h.redirectError(c, "invalid_state")
	case errors.Is(err, auth.ErrUserBlocked):
		h.redirectError(c, "user_blocked")
	case errors.Is(err, auth.ErrOAuthProviderFailed):
		h.redirectError(c, "provider_failed")
	default:
		h.redirectError(c, "internal_error")
	}
}

func (h *OAuthHandler) redirectError(c *gin.Context, code string) {
	target := h.frontendBaseURL + "/login?oauth_error=" + url.QueryEscape(code)
	c.Redirect(http.StatusFound, target)
}

func (h *OAuthHandler) redirectToLink(c *gin.Context, link *auth.OAuthLinkRequest) {
	q := url.Values{}
	q.Set("token", link.LinkToken)
	q.Set("email", link.Email)
	q.Set("provider", string(link.Provider))
	c.Redirect(http.StatusFound, h.frontendBaseURL+"/link-oauth?"+q.Encode())
}

func parseProvider(raw string) (domain.OAuthProvider, bool) {
	p := domain.OAuthProvider(strings.ToLower(strings.TrimSpace(raw)))
	if !p.IsValid() {
		return "", false
	}
	return p, true
}

// sanitizeErrorCode защищает от наполнения URL чем попало из строки error
// провайдера (могут быть пробелы, кириллица). На фронте мы и так не пытаемся
// показывать оригинал — только маппим в локализованный текст.
func sanitizeErrorCode(raw string) string {
	out := make([]byte, 0, len(raw))
	for i := 0; i < len(raw) && i < 64; i++ {
		c := raw[i]
		switch {
		case c >= 'a' && c <= 'z',
			c >= 'A' && c <= 'Z',
			c >= '0' && c <= '9',
			c == '_', c == '-':
			out = append(out, c)
		}
	}
	if len(out) == 0 {
		return "provider_error"
	}
	return "provider_" + string(out)
}
