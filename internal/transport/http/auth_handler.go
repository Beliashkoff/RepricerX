package transport

import (
	"net/http"

	"github.com/Beliashkoff/RepricerX/internal/service/auth"
	"github.com/gin-gonic/gin"
)

// AuthHandler обрабатывает все /api/auth/* эндпоинты.
type AuthHandler struct {
	svc         *auth.Service
	secure      bool   // Secure-флаг для cookie: true в prod
	frontendURL string // куда редиректить после verify
}

func NewAuthHandler(svc *auth.Service, secure bool, frontendURL string) *AuthHandler {
	return &AuthHandler{svc: svc, secure: secure, frontendURL: frontendURL}
}

// Register godoc
//
//	@Summary		Регистрация
//	@Description	Создаёт нового пользователя. На указанный email отправляется письмо для подтверждения.
//	@Description	Войти можно только после перехода по ссылке из письма (активация аккаунта).
//	@Tags			auth
//	@Accept			json
//	@Produce		json
//	@Param			body	body		registerRequest		true	"Данные для регистрации"
//	@Success		201		{object}	registerResponse	"Пользователь создан, письмо отправлено"
//	@Failure		400		{object}	errorResponse		"Неверный формат запроса, некорректный email или слабый пароль (код: bad_request / invalid_email / weak_password)"
//	@Failure		409		{object}	errorResponse		"Email уже зарегистрирован (код: email_taken)"
//	@Failure		500		{object}	errorResponse
//	@Router			/api/auth/register [post]
func (h *AuthHandler) Register(c *gin.Context) {
	var req registerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		errResp(c, http.StatusBadRequest, "bad_request", "Неверный формат запроса")
		return
	}

	result, err := h.svc.Register(c.Request.Context(), req.Email, req.Password, req.DisplayName)
	if err != nil {
		handleAuthErr(c, err)
		return
	}
	c.JSON(http.StatusCreated, registerResponse{Email: result.Email})
}

// Login godoc
//
//	@Summary		Вход
//	@Description	Проверяет email и пароль. При успехе создаёт сессию и устанавливает HttpOnly-cookie `rx_session`.
//	@Description	Аккаунт должен быть активирован (email подтверждён через письмо).
//	@Description	После 5 неудачных попыток аккаунт блокируется на 15 минут.
//	@Tags			auth
//	@Accept			json
//	@Produce		json
//	@Param			body	body		loginRequest	true	"Учётные данные"
//	@Success		200		{object}	loginResponse	"Успешный вход, cookie rx_session выставлен"
//	@Failure		401		{object}	errorResponse	"Неверные учётные данные или аккаунт не активирован (код: invalid_credentials)"
//	@Failure		403		{object}	errorResponse	"Аккаунт заблокирован (код: user_blocked)"
//	@Failure		500		{object}	errorResponse
//	@Router			/api/auth/login [post]
func (h *AuthHandler) Login(c *gin.Context) {
	var req loginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		// Даже на плохой JSON — generic 401, чтобы не раскрывать причину.
		errResp(c, http.StatusUnauthorized, "invalid_credentials", "Неверный email или пароль")
		return
	}

	result, err := h.svc.Login(c.Request.Context(), c.Request, req.Email, req.Password)
	if err != nil {
		handleAuthErr(c, err)
		return
	}

	SetSessionCookie(c, result.Plaintext, result.Session.IdleExpiresAt, h.secure)
	c.JSON(http.StatusOK, loginResponse{
		ID:          result.User.ID.String(),
		Email:       result.User.Email,
		DisplayName: result.User.DisplayName,
	})
}

// Logout godoc
//
//	@Summary		Выход
//	@Description	Завершает текущую сессию, инвалидирует cookie `rx_session`.
//	@Description	Требует валидной сессии и совпадения заголовка Origin (CSRF-защита).
//	@Tags			auth
//	@Produce		json
//	@Success		204	"Сессия завершена"
//	@Failure		401	{object}	errorResponse	"Сессия не найдена или истекла"
//	@Failure		403	{object}	errorResponse	"CSRF — Origin не совпадает с разрешёнными"
//	@Security		SessionCookie
//	@Router			/api/auth/logout [post]
func (h *AuthHandler) Logout(c *gin.Context) {
	plaintext, err := c.Cookie(sessionCookieName)
	if err == nil && plaintext != "" {
		h.svc.Logout(c.Request.Context(), plaintext)
	}
	ClearSessionCookie(c, h.secure)
	c.Status(http.StatusNoContent)
}

// Me godoc
//
//	@Summary		Профиль текущего пользователя
//	@Description	Возвращает данные аутентифицированного пользователя: id, email, displayName, статус, дату регистрации.
//	@Tags			auth
//	@Produce		json
//	@Success		200	{object}	meResponse
//	@Failure		401	{object}	errorResponse	"Сессия отсутствует или истекла"
//	@Security		SessionCookie
//	@Router			/api/auth/me [get]
func (h *AuthHandler) Me(c *gin.Context) {
	user := mustUser(c)
	c.JSON(http.StatusOK, meResponse{
		ID:          user.ID.String(),
		Email:       user.Email,
		DisplayName: user.DisplayName,
		Status:      user.Status,
		CreatedAt:   user.CreatedAt,
	})
}

// UpdateMe godoc
//
//	@Summary		Обновить профиль
//	@Description	Изменяет отображаемое имя (displayName) текущего пользователя.
//	@Description	Требует валидной сессии и совпадения Origin (CSRF-защита).
//	@Tags			auth
//	@Accept			json
//	@Produce		json
//	@Param			body	body		updateMeRequest	true	"Новое отображаемое имя"
//	@Success		200		{object}	loginResponse	"Обновлённые данные пользователя"
//	@Failure		400		{object}	errorResponse	"Неверный формат запроса"
//	@Failure		401		{object}	errorResponse	"Не аутентифицирован"
//	@Failure		403		{object}	errorResponse	"CSRF — Origin не совпадает"
//	@Failure		500		{object}	errorResponse
//	@Security		SessionCookie
//	@Router			/api/auth/me [patch]
func (h *AuthHandler) UpdateMe(c *gin.Context) {
	var req updateMeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		errResp(c, http.StatusBadRequest, "bad_request", "Неверный формат запроса")
		return
	}

	user := mustUser(c)
	if err := h.svc.UpdateDisplayName(c.Request.Context(), user.ID, req.DisplayName); err != nil {
		errResp(c, http.StatusInternalServerError, "internal_error", "Ошибка обновления профиля")
		return
	}
	user.DisplayName = req.DisplayName
	c.JSON(http.StatusOK, loginResponse{
		ID:          user.ID.String(),
		Email:       user.Email,
		DisplayName: user.DisplayName,
	})
}

// VerifyEmail godoc
//
//	@Summary		Подтверждение email
//	@Description	Принимает одноразовый токен из письма, активирует аккаунт и перенаправляет на фронтенд.
//	@Description	При успехе — редирект на `/login?verified=1`, при ошибке или истёкшем токене — на `/login?verified=0`.
//	@Tags			auth
//	@Param			token	query	string	true	"Одноразовый токен из письма (plaintext)"
//	@Success		302		"Редирект на фронтенд с verified=1"
//	@Failure		302		"Редирект на фронтенд с verified=0 (токен не найден, уже использован или истёк)"
//	@Router			/api/auth/verify [get]
func (h *AuthHandler) VerifyEmail(c *gin.Context) {
	plaintextToken := c.Query("token")
	result := h.svc.VerifyEmail(c.Request.Context(), plaintextToken)
	if result.Success {
		c.Redirect(http.StatusFound, h.frontendURL+"/login?verified=1")
	} else {
		c.Redirect(http.StatusFound, h.frontendURL+"/login?verified=0")
	}
}

// ResendVerification godoc
//
//	@Summary		Повторная отправка письма
//	@Description	Отправляет новое письмо с токеном верификации.
//	@Description	Всегда возвращает 202 — сервер не раскрывает, существует ли аккаунт с таким email (защита от перебора).
//	@Tags			auth
//	@Accept			json
//	@Param			body	body	resendRequest	true	"Email пользователя"
//	@Success		202		"Письмо отправлено (или будет отправлено, если email зарегистрирован)"
//	@Router			/api/auth/verification/resend [post]
func (h *AuthHandler) ResendVerification(c *gin.Context) {
	var req resendRequest
	if err := c.ShouldBindJSON(&req); err == nil {
		_ = h.svc.ResendVerification(c.Request.Context(), req.Email)
	}
	// Всегда 202 — не раскрываем существование email.
	c.Status(http.StatusAccepted)
}
