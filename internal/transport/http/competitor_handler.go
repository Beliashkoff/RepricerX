package transport

import (
	"errors"
	"net/http"

	"github.com/Beliashkoff/RepricerX/internal/domain"
	competitorsvc "github.com/Beliashkoff/RepricerX/internal/service/competitor"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type CompetitorHandler struct {
	svc *competitorsvc.Service
}

func NewCompetitorHandler(svc *competitorsvc.Service) *CompetitorHandler {
	return &CompetitorHandler{svc: svc}
}

// List godoc
//
//	@Summary		Список конкурентов товара
//	@Description	Возвращает привязанные к товару Ozon competitor URL/public product ID. Доступ только к товарам текущего пользователя.
//	@Tags			competitors
//	@Produce		json
//	@Param			id	path		string					true	"UUID товара"
//	@Success		200	{array}		competitorResponse		"Конкуренты товара"
//	@Failure		400	{object}	errorResponse
//	@Failure		401	{object}	errorResponse
//	@Failure		500	{object}	errorResponse
//	@Security		SessionCookie
//	@Router			/api/products/{id}/competitors [get]
func (h *CompetitorHandler) List(c *gin.Context) {
	user := mustUser(c)
	productID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		errResp(c, http.StatusBadRequest, "bad_request", "Неверный формат ID товара")
		return
	}
	items, err := h.svc.List(c.Request.Context(), user.ID, productID)
	if err != nil {
		handleCompetitorErr(c, err)
		return
	}
	resp := make([]competitorResponse, 0, len(items))
	for _, item := range items {
		resp = append(resp, toCompetitorResponse(item))
	}
	c.JSON(http.StatusOK, resp)
}

// Create godoc
//
//	@Summary		Добавить конкурента к товару
//	@Description	Принимает ссылку на товар Ozon или публичный ID товара Ozon. Цена обновляется отдельным refresh-запросом.
//	@Tags			competitors
//	@Accept			json
//	@Produce		json
//	@Param			id		path		string				true	"UUID товара"
//	@Param			body	body		competitorRequest	true	"target: Ozon URL или public product ID"
//	@Success		201		{object}	competitorResponse	"Созданный конкурент"
//	@Failure		400		{object}	errorResponse		"Некорректный target"
//	@Failure		401		{object}	errorResponse
//	@Failure		403		{object}	errorResponse
//	@Failure		404		{object}	errorResponse		"Товар не найден"
//	@Failure		409		{object}	errorResponse		"Дубликат конкурента"
//	@Failure		500		{object}	errorResponse
//	@Security		SessionCookie
//	@Router			/api/products/{id}/competitors [post]
func (h *CompetitorHandler) Create(c *gin.Context) {
	user := mustUser(c)
	productID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		errResp(c, http.StatusBadRequest, "bad_request", "Неверный формат ID товара")
		return
	}
	var req competitorRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		errResp(c, http.StatusBadRequest, "bad_request", "Неверный формат запроса")
		return
	}
	item, err := h.svc.Create(c.Request.Context(), user.ID, competitorsvc.CreateInput{
		ProductID: productID,
		Target:    req.Target,
	})
	if err != nil {
		handleCompetitorErr(c, err)
		return
	}
	c.JSON(http.StatusCreated, toCompetitorResponse(item))
}

// Update godoc
//
//	@Summary		Обновить ссылку конкурента
//	@Description	Сбрасывает последнюю цену и статус проверки, затем сохраняет новый Ozon URL/public product ID.
//	@Tags			competitors
//	@Accept			json
//	@Produce		json
//	@Param			competitorId	path		string				true	"UUID конкурента"
//	@Param			body			body		competitorRequest	true	"target: Ozon URL или public product ID"
//	@Success		200				{object}	competitorResponse	"Обновлённый конкурент"
//	@Failure		400				{object}	errorResponse
//	@Failure		401				{object}	errorResponse
//	@Failure		403				{object}	errorResponse
//	@Failure		404				{object}	errorResponse
//	@Failure		409				{object}	errorResponse
//	@Failure		500				{object}	errorResponse
//	@Security		SessionCookie
//	@Router			/api/competitors/{competitorId} [patch]
func (h *CompetitorHandler) Update(c *gin.Context) {
	user := mustUser(c)
	competitorID, err := uuid.Parse(c.Param("competitorId"))
	if err != nil {
		errResp(c, http.StatusBadRequest, "bad_request", "Неверный формат ID конкурента")
		return
	}
	var req competitorRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		errResp(c, http.StatusBadRequest, "bad_request", "Неверный формат запроса")
		return
	}
	item, err := h.svc.Update(c.Request.Context(), user.ID, competitorID, competitorsvc.UpdateInput{Target: req.Target})
	if err != nil {
		handleCompetitorErr(c, err)
		return
	}
	c.JSON(http.StatusOK, toCompetitorResponse(item))
}

// Delete godoc
//
//	@Summary		Удалить конкурента
//	@Description	Удаляет competitor link и историю цен. Доступ ограничен владельцем товара.
//	@Tags			competitors
//	@Param			competitorId	path	string	true	"UUID конкурента"
//	@Success		204				"Конкурент удалён"
//	@Failure		400				{object}	errorResponse
//	@Failure		401				{object}	errorResponse
//	@Failure		403				{object}	errorResponse
//	@Failure		404				{object}	errorResponse
//	@Failure		500				{object}	errorResponse
//	@Security		SessionCookie
//	@Router			/api/competitors/{competitorId} [delete]
func (h *CompetitorHandler) Delete(c *gin.Context) {
	user := mustUser(c)
	competitorID, err := uuid.Parse(c.Param("competitorId"))
	if err != nil {
		errResp(c, http.StatusBadRequest, "bad_request", "Неверный формат ID конкурента")
		return
	}
	if err := h.svc.Delete(c.Request.Context(), user.ID, competitorID); err != nil {
		handleCompetitorErr(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// Refresh godoc
//
//	@Summary		Обновить цену конкурента
//	@Description	Проверяет публичную страницу Ozon через изолированный adapter, сохраняет safe status/error code и snapshot. Raw parser/external errors не возвращаются.
//	@Tags			competitors
//	@Produce		json
//	@Param			competitorId	path		string				true	"UUID конкурента"
//	@Success		200				{object}	competitorResponse	"Цена обновлена"
//	@Success		202				{object}	competitorResponse	"Проверка выполнена с безопасным кодом ошибки"
//	@Failure		400				{object}	errorResponse
//	@Failure		401				{object}	errorResponse
//	@Failure		403				{object}	errorResponse
//	@Failure		404				{object}	errorResponse
//	@Failure		500				{object}	errorResponse
//	@Security		SessionCookie
//	@Router			/api/competitors/{competitorId}/refresh [post]
func (h *CompetitorHandler) Refresh(c *gin.Context) {
	user := mustUser(c)
	competitorID, err := uuid.Parse(c.Param("competitorId"))
	if err != nil {
		errResp(c, http.StatusBadRequest, "bad_request", "Неверный формат ID конкурента")
		return
	}
	item, err := h.svc.Refresh(c.Request.Context(), user.ID, competitorID)
	if err != nil && !errors.Is(err, competitorsvc.ErrRefreshFailed) {
		handleCompetitorErr(c, err)
		return
	}
	status := http.StatusOK
	if errors.Is(err, competitorsvc.ErrRefreshFailed) {
		status = http.StatusAccepted
	}
	c.JSON(status, toCompetitorResponse(item))
}

func handleCompetitorErr(c *gin.Context, err error) {
	switch {
	case errors.Is(err, competitorsvc.ErrProductNotFound), errors.Is(err, competitorsvc.ErrCompetitorNotFound):
		errResp(c, http.StatusNotFound, "competitor_not_found", "Конкурент не найден")
	case errors.Is(err, competitorsvc.ErrInvalidTarget):
		errResp(c, http.StatusBadRequest, "invalid_competitor_target", "Укажите ссылку на товар Ozon или публичный ID товара")
	case errors.Is(err, competitorsvc.ErrDuplicateCompetitor):
		errResp(c, http.StatusConflict, "duplicate_competitor", "Этот конкурент уже добавлен к товару")
	case errors.Is(err, competitorsvc.ErrRefreshFailed):
		errResp(c, http.StatusAccepted, "competitor_refresh_failed", "Не удалось обновить цену конкурента")
	default:
		errResp(c, http.StatusInternalServerError, "internal_error", "Внутренняя ошибка сервера")
	}
}

func toCompetitorResponse(item *domain.ProductCompetitor) competitorResponse {
	return competitorResponse{
		ID: item.ID.String(), ProductID: item.ProductID.String(),
		Marketplace: item.Marketplace, Source: item.Source,
		CompetitorURL:       item.CompetitorURL,
		OzonPublicProductID: item.OzonPublicProductID,
		LastPrice:           item.LastPrice, LastAvailability: item.LastAvailability,
		LastCheckedAt: item.LastCheckedAt, LastErrorCode: item.LastErrorCode,
		LastStatus: item.LastStatus, CreatedAt: item.CreatedAt, UpdatedAt: item.UpdatedAt,
	}
}
