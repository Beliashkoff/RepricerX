package transport

import (
	"net/http"

	"github.com/Beliashkoff/RepricerX/internal/domain"
	shopsvc "github.com/Beliashkoff/RepricerX/internal/service/shop"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// ShopHandler обрабатывает все /api/shops/* эндпоинты.
type ShopHandler struct {
	svc *shopsvc.Service
}

func NewShopHandler(svc *shopsvc.Service) *ShopHandler {
	return &ShopHandler{svc: svc}
}

// List godoc
//
//	@Summary		Список магазинов
//	@Description	Возвращает все магазины, принадлежащие аутентифицированному пользователю.
//	@Tags			shops
//	@Produce		json
//	@Success		200	{array}		shopResponse
//	@Failure		401	{object}	errorResponse	"Сессия отсутствует или истекла"
//	@Failure		500	{object}	errorResponse
//	@Security		SessionCookie
//	@Router			/api/shops [get]
func (h *ShopHandler) List(c *gin.Context) {
	user := mustUser(c)
	shops, err := h.svc.List(c.Request.Context(), user.ID)
	if err != nil {
		handleShopErr(c, err)
		return
	}
	resp := make([]shopResponse, 0, len(shops))
	for _, s := range shops {
		resp = append(resp, toShopResponse(s))
	}
	c.JSON(http.StatusOK, resp)
}

// Get godoc
//
//	@Summary		Получить магазин
//	@Description	Возвращает магазин по UUID. Доступен только владельцу.
//	@Tags			shops
//	@Produce		json
//	@Param			id	path		string			true	"UUID магазина"	example(550e8400-e29b-41d4-a716-446655440000)
//	@Success		200	{object}	shopResponse
//	@Failure		400	{object}	errorResponse	"Неверный формат UUID"
//	@Failure		401	{object}	errorResponse	"Не аутентифицирован"
//	@Failure		404	{object}	errorResponse	"Магазин не найден (код: shop_not_found)"
//	@Failure		500	{object}	errorResponse
//	@Security		SessionCookie
//	@Router			/api/shops/{id} [get]
func (h *ShopHandler) Get(c *gin.Context) {
	user := mustUser(c)
	shopID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		errResp(c, http.StatusBadRequest, "bad_request", "Неверный формат ID")
		return
	}
	shop, err := h.svc.Get(c.Request.Context(), user.ID, shopID)
	if err != nil {
		handleShopErr(c, err)
		return
	}
	c.JSON(http.StatusOK, toShopResponse(shop))
}

// Create godoc
//
//	@Summary		Подключить магазин
//	@Description	Создаёт новый магазин, сохраняет API-ключи в зашифрованном виде (AES-GCM).
//	@Description
//	@Description	Формат поля `credentials` зависит от маркетплейса:
//	@Description	- **wb** (Wildberries): `{"api_key": "<API-токен>"}`
//	@Description	  Токен можно получить в личном кабинете WB: Настройки → Доступ к API.
//	@Description	- **ozon**: `{"client_id": "<Client-ID>", "api_key": "<API-ключ>"}`
//	@Description	  Ключи доступны в кабинете Ozon Seller: Настройки → API-ключи.
//	@Tags			shops
//	@Accept			json
//	@Produce		json
//	@Param			body	body		createShopRequest	true	"Данные нового магазина"
//	@Success		201		{object}	shopResponse		"Магазин создан"
//	@Failure		400		{object}	errorResponse		"Неверный формат или неизвестный маркетплейс (код: bad_request / invalid_marketplace)"
//	@Failure		401		{object}	errorResponse		"Не аутентифицирован"
//	@Failure		403		{object}	errorResponse		"CSRF — Origin не совпадает"
//	@Failure		500		{object}	errorResponse
//	@Security		SessionCookie
//	@Router			/api/shops [post]
func (h *ShopHandler) Create(c *gin.Context) {
	var req createShopRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		errResp(c, http.StatusBadRequest, "bad_request", "Неверный формат запроса")
		return
	}
	user := mustUser(c)
	shop, err := h.svc.Create(c.Request.Context(), user.ID, req.Marketplace, req.Name, req.Credentials)
	if err != nil {
		handleShopErr(c, err)
		return
	}
	c.JSON(http.StatusCreated, toShopResponse(shop))
}

// Update godoc
//
//	@Summary		Обновить магазин
//	@Description	Изменяет имя, учётные данные и/или настройки автообновления и расписания.
//	@Description	Все поля опциональны — передавайте только то, что хотите изменить.
//	@Description
//	@Description	Поле `scheduleCron` задаёт расписание пересчёта цен в формате cron (5 полей).
//	@Description	Примеры: `"0 * * * *"` — каждый час, `"0 */4 * * *"` — каждые 4 часа.
//	@Tags			shops
//	@Accept			json
//	@Produce		json
//	@Param			id		path		string				true	"UUID магазина"
//	@Param			body	body		updateShopRequest	true	"Обновляемые поля"
//	@Success		200		{object}	shopResponse
//	@Failure		400		{object}	errorResponse	"Неверный формат"
//	@Failure		401		{object}	errorResponse	"Не аутентифицирован"
//	@Failure		403		{object}	errorResponse	"CSRF или нет доступа к магазину"
//	@Failure		404		{object}	errorResponse	"Магазин не найден"
//	@Failure		500		{object}	errorResponse
//	@Security		SessionCookie
//	@Router			/api/shops/{id} [patch]
func (h *ShopHandler) Update(c *gin.Context) {
	user := mustUser(c)
	shopID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		errResp(c, http.StatusBadRequest, "bad_request", "Неверный формат ID")
		return
	}
	var req updateShopRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		errResp(c, http.StatusBadRequest, "bad_request", "Неверный формат запроса")
		return
	}
	shop, err := h.svc.Update(c.Request.Context(), user.ID, shopID, shopsvc.UpdatePatch{
		Name:              req.Name,
		Credentials:       req.Credentials,
		AutoUpdateEnabled: req.AutoUpdateEnabled,
		ScheduleCron:      req.ScheduleCron,
	})
	if err != nil {
		handleShopErr(c, err)
		return
	}
	c.JSON(http.StatusOK, toShopResponse(shop))
}

// Delete godoc
//
//	@Summary		Удалить магазин
//	@Description	Удаляет магазин и все связанные данные.
//	@Description	Если за последние 30 дней есть записи в журнале изменений цен — операция будет отклонена.
//	@Tags			shops
//	@Produce		json
//	@Param			id	path	string	true	"UUID магазина"
//	@Success		204	"Магазин удалён"
//	@Failure		400	{object}	errorResponse	"Неверный формат UUID"
//	@Failure		401	{object}	errorResponse	"Не аутентифицирован"
//	@Failure		403	{object}	errorResponse	"CSRF — Origin не совпадает"
//	@Failure		404	{object}	errorResponse	"Магазин не найден"
//	@Failure		500	{object}	errorResponse
//	@Security		SessionCookie
//	@Router			/api/shops/{id} [delete]
func (h *ShopHandler) Delete(c *gin.Context) {
	user := mustUser(c)
	shopID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		errResp(c, http.StatusBadRequest, "bad_request", "Неверный формат ID")
		return
	}
	if err := h.svc.Delete(c.Request.Context(), user.ID, shopID); err != nil {
		handleShopErr(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// TestConnection godoc
//
//	@Summary		Проверить подключение к маркетплейсу
//	@Description	Выполняет тестовый запрос к API маркетплейса с сохранёнными учётными данными.
//	@Description	При успехе статус магазина становится `active`, при ошибке авторизации — `error`.
//	@Description	Результат проверки фиксируется в поле `lastCheckedAt`.
//	@Tags			shops
//	@Produce		json
//	@Param			id	path		string					true	"UUID магазина"
//	@Success		200	{object}	testConnectionResponse	"Подключение успешно"
//	@Failure		400	{object}	errorResponse			"Неверный формат UUID"
//	@Failure		401	{object}	errorResponse			"Не аутентифицирован"
//	@Failure		403	{object}	errorResponse			"CSRF — Origin не совпадает"
//	@Failure		404	{object}	errorResponse			"Магазин не найден"
//	@Failure		422	{object}	errorResponse			"Ошибка авторизации в маркетплейсе (код: auth_failed)"
//	@Failure		500	{object}	errorResponse
//	@Security		SessionCookie
//	@Router			/api/shops/{id}/test [post]
func (h *ShopHandler) TestConnection(c *gin.Context) {
	user := mustUser(c)
	shopID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		errResp(c, http.StatusBadRequest, "bad_request", "Неверный формат ID")
		return
	}
	if err := h.svc.TestConnection(c.Request.Context(), user.ID, shopID); err != nil {
		handleShopErr(c, err)
		return
	}
	c.JSON(http.StatusOK, testConnectionResponse{Status: "active"})
}

func toShopResponse(s *domain.Shop) shopResponse {
	return shopResponse{
		ID:                s.ID.String(),
		Marketplace:       s.Marketplace,
		Name:              s.Name,
		Status:            s.Status,
		AutoUpdateEnabled: s.AutoUpdateEnabled,
		ScheduleCron:      s.ScheduleCron,
		LastCheckedAt:     s.LastCheckedAt,
		LastRecalcAt:      s.LastRecalcAt,
		CreatedAt:         s.CreatedAt,
	}
}
