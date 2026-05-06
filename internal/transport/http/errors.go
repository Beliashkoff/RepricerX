package transport

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/Beliashkoff/RepricerX/internal/repository"
	"github.com/Beliashkoff/RepricerX/internal/service/auth"
	productsvc "github.com/Beliashkoff/RepricerX/internal/service/product"
	shopsvc "github.com/Beliashkoff/RepricerX/internal/service/shop"
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
			"Пароль должен быть от 8 до 128 символов и содержать букву и цифру")
	case auth.ErrEmailTaken:
		errResp(c, http.StatusConflict, "email_taken", "Этот email уже зарегистрирован")
	case auth.ErrInvalidCredentials:
		// Единственный ответ на все ошибки логина — не раскрываем причину.
		errResp(c, http.StatusUnauthorized, "invalid_credentials", "Неверный email или пароль")
	case auth.ErrInvalidResetToken:
		errResp(c, http.StatusBadRequest, "invalid_reset_token", "Ссылка сброса пароля недействительна или истекла")
	case auth.ErrSessionNotFound:
		errResp(c, http.StatusUnauthorized, "unauthorized", "Сессия не найдена или истекла")
	case auth.ErrUserBlocked:
		errResp(c, http.StatusForbidden, "user_blocked", "Аккаунт заблокирован")
	default:
		errResp(c, http.StatusInternalServerError, "internal_error", "Внутренняя ошибка сервера")
	}
}

// handleShopErr маппит ошибки сервиса shop в HTTP-ответы.
func handleShopErr(c *gin.Context, err error) {
	switch err {
	case shopsvc.ErrShopNotFound:
		errResp(c, http.StatusNotFound, "shop_not_found", "Магазин не найден")
	case shopsvc.ErrInvalidMarketplace:
		errResp(c, http.StatusBadRequest, "invalid_marketplace", "Неизвестный маркетплейс")
	case shopsvc.ErrAuthFailed:
		errResp(c, http.StatusUnprocessableEntity, "auth_failed", "Ошибка авторизации в маркетплейсе")
	case shopsvc.ErrRateLimited:
		errResp(c, http.StatusTooManyRequests, "marketplace_rate_limited",
			"Маркетплейс временно ограничил запросы, повторите позже")
	default:
		errResp(c, http.StatusInternalServerError, "internal_error", "Внутренняя ошибка сервера")
	}
}

func handleProductErr(c *gin.Context, err error) {
	var cooldownErr productsvc.ImportCooldownError
	if errors.As(err, &cooldownErr) {
		if cooldownErr.RetryAfter > 0 {
			c.Header("Retry-After", strconv.Itoa(int(cooldownErr.RetryAfter.Seconds())))
		}
		errResp(c, http.StatusTooManyRequests, "import_cooldown", "Повторный импорт для магазина временно недоступен")
		return
	}
	switch {
	case errors.Is(err, productsvc.ErrShopNotFound):
		errResp(c, http.StatusNotFound, "shop_not_found", "Магазин не найден")
	case errors.Is(err, productsvc.ErrProductNotFound):
		errResp(c, http.StatusNotFound, "product_not_found", "Товар не найден")
	case errors.Is(err, productsvc.ErrDuplicateSKU):
		errResp(c, http.StatusConflict, "duplicate_sku", "SKU уже существует в этом магазине")
	case errors.Is(err, productsvc.ErrImportAlreadyRunning):
		errResp(c, http.StatusConflict, "import_already_running", "Импорт для магазина уже выполняется")
	case errors.Is(err, productsvc.ErrImportCooldown):
		errResp(c, http.StatusTooManyRequests, "import_cooldown", "Повторный импорт для магазина временно недоступен")
	case errors.Is(err, productsvc.ErrImportNotFound):
		errResp(c, http.StatusNotFound, "import_not_found", "Импорт не найден")
	case errors.Is(err, productsvc.ErrInvalidProduct):
		errResp(c, http.StatusBadRequest, "invalid_product", "Некорректные данные товара")
	case errors.Is(err, productsvc.ErrInvalidPrice), errors.Is(err, repository.ErrConstraintViolation):
		errResp(c, http.StatusBadRequest, "invalid_price", "Некорректные значения цен")
	case errors.Is(err, productsvc.ErrInvalidMarketplace):
		errResp(c, http.StatusBadRequest, "invalid_marketplace", "Неизвестный маркетплейс")
	case errors.Is(err, productsvc.ErrImportNotCancelable):
		errResp(c, http.StatusConflict, "import_not_cancelable", "Импорт не может быть отменён (уже завершён или не найден)")
	default:
		errResp(c, http.StatusInternalServerError, "internal_error", "Внутренняя ошибка сервера")
	}
}
