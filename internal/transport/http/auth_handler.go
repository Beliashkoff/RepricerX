package transport

import (
	"net/http"

	"github.com/Beliashkoff/RepricerX/internal/service/auth"
	"github.com/gin-gonic/gin"
)

// AuthHandler обрабатывает все /api/auth/* эндпоинты.
type AuthHandler struct {
	svc    *auth.Service
	secure bool   // Secure-флаг для cookie: true в prod
	frontendURL string // куда редиректить после verify
}

func NewAuthHandler(svc *auth.Service, secure bool, frontendURL string) *AuthHandler {
	return &AuthHandler{svc: svc, secure: secure, frontendURL: frontendURL}
}

// POST /api/auth/register
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

// POST /api/auth/login
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

// POST /api/auth/logout
func (h *AuthHandler) Logout(c *gin.Context) {
	plaintext, err := c.Cookie(sessionCookieName)
	if err == nil && plaintext != "" {
		h.svc.Logout(c.Request.Context(), plaintext)
	}
	ClearSessionCookie(c, h.secure)
	c.Status(http.StatusNoContent)
}

// GET /api/auth/me
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

// PATCH /api/auth/me
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

// GET /api/auth/verify?token=<plaintext>
func (h *AuthHandler) VerifyEmail(c *gin.Context) {
	plaintextToken := c.Query("token")
	result := h.svc.VerifyEmail(c.Request.Context(), plaintextToken)
	if result.Success {
		c.Redirect(http.StatusFound, h.frontendURL+"/login?verified=1")
	} else {
		c.Redirect(http.StatusFound, h.frontendURL+"/login?verified=0")
	}
}

// POST /api/auth/verification/resend
func (h *AuthHandler) ResendVerification(c *gin.Context) {
	var req resendRequest
	if err := c.ShouldBindJSON(&req); err == nil {
		_ = h.svc.ResendVerification(c.Request.Context(), req.Email)
	}
	// Всегда 202 — не раскрываем существование email.
	c.Status(http.StatusAccepted)
}
