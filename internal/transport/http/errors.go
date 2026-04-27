package transport

import (
	"net/http"

	"github.com/Beliashkoff/RepricerX/internal/service/auth"
	"github.com/gin-gonic/gin"
)

// errResp записывает унифицированный JSON-ответ с кодом ошибки.
func errResp(c *gin.Context, httpStatus int, code, message string) {
	c.JSON(httpStatus, errorResponse{
		Error: errorDetail{Code: code, Message: message},
	})
}

// handleAuthErr маппит ошибки сервиса auth в HTTP-ответы.
func handleAuthErr(c *gin.Context, err error) {
	switch err {
	case auth.ErrInvalidEmail:
		errResp(c, http.StatusBadRequest, "invalid_email", "Неверный формат email")
	case auth.ErrWeakPassword:
		errResp(c, http.StatusBadRequest, "weak_password",
			"Пароль должен быть от 12 до 128 символов и содержать букву и цифру")
	case auth.ErrEmailTaken:
		errResp(c, http.StatusConflict, "email_taken", "Этот email уже зарегистрирован")
	case auth.ErrInvalidCredentials:
		// Единственный ответ на все ошибки логина — не раскрываем причину.
		errResp(c, http.StatusUnauthorized, "invalid_credentials", "Неверный email или пароль")
	case auth.ErrSessionNotFound:
		errResp(c, http.StatusUnauthorized, "unauthorized", "Сессия не найдена или истекла")
	case auth.ErrUserBlocked:
		errResp(c, http.StatusForbidden, "user_blocked", "Аккаунт заблокирован")
	default:
		errResp(c, http.StatusInternalServerError, "internal_error", "Внутренняя ошибка сервера")
	}
}
