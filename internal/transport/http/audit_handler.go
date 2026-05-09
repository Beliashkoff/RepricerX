package transport

import (
	"errors"
	"net/http"
	"time"

	"github.com/Beliashkoff/RepricerX/internal/domain"
	auditsvc "github.com/Beliashkoff/RepricerX/internal/service/audit"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type auditHandler struct {
	svc *auditsvc.Service
}

func NewAuditHandler(svc *auditsvc.Service) *auditHandler {
	return &auditHandler{svc: svc}
}

// ListChanges godoc
//
//	@Summary		Журнал изменений цен
//	@Description	Пагинированный список изменений цен с фильтрами. Хранение записей — 180 дней.
//	@Tags			audit
//	@Produce		json
//	@Param			shop_id			query		string	false	"UUID магазина"
//	@Param			product_id		query		string	false	"UUID товара"
//	@Param			external_sku	query		string	false	"Подстрока external_sku товара (ILIKE)"
//	@Param			status			query		string	false	"success | failed | skipped"
//	@Param			from			query		string	false	"Начало периода (RFC3339)"
//	@Param			to				query		string	false	"Конец периода (RFC3339)"
//	@Param			page			query		int		false	"Страница (>=1)"
//	@Param			per_page		query		int		false	"Размер страницы (1..200)"
//	@Param			sort_dir		query		string	false	"asc | desc (default desc)"
//	@Success		200	{object}	priceChangeListResponse
//	@Failure		400	{object}	errorResponse
//	@Failure		401	{object}	errorResponse
//	@Failure		500	{object}	errorResponse
//	@Security		SessionCookie
//	@Router			/api/audit/price-changes [get]
func (h *auditHandler) ListChanges(c *gin.Context) {
	user := mustUser(c)
	filter, ok := parseAuditFilter(c)
	if !ok {
		return
	}
	changes, total, err := h.svc.ListChanges(c.Request.Context(), user.ID, filter)
	if err != nil {
		handleAuditErr(c, err)
		return
	}
	items := make([]priceChangeResponse, 0, len(changes))
	for _, change := range changes {
		items = append(items, toPriceChangeResponse(change))
	}
	page, perPage := filter.Page, filter.PerPage
	if page < 1 {
		page = 1
	}
	if perPage < 1 {
		perPage = 50
	}
	c.JSON(http.StatusOK, priceChangeListResponse{
		Items:      items,
		Pagination: paginationInfo{Page: page, PerPage: perPage, Total: total},
	})
}

// Summary godoc
//
//	@Summary		Сводка по изменениям цен
//	@Description	Агрегированные метрики (всего/успех/ошибки/среднее изменение) за параметризуемый период (по умолчанию — 30 дней). Поддерживает те же фильтры, что и список.
//	@Tags			reports
//	@Produce		json
//	@Param			shop_id			query		string	false	"UUID магазина"
//	@Param			product_id		query		string	false	"UUID товара"
//	@Param			external_sku	query		string	false	"Подстрока external_sku товара"
//	@Param			status			query		string	false	"success | failed | skipped"
//	@Param			from			query		string	false	"Начало периода (RFC3339)"
//	@Param			to				query		string	false	"Конец периода (RFC3339)"
//	@Success		200	{object}	summaryResponse
//	@Failure		400	{object}	errorResponse
//	@Failure		401	{object}	errorResponse
//	@Failure		500	{object}	errorResponse
//	@Security		SessionCookie
//	@Router			/api/reports/summary [get]
func (h *auditHandler) Summary(c *gin.Context) {
	user := mustUser(c)
	filter, ok := parseAuditFilter(c)
	if !ok {
		return
	}
	summary, err := h.svc.Summary(c.Request.Context(), user.ID, filter)
	if err != nil {
		handleAuditErr(c, err)
		return
	}
	c.JSON(http.StatusOK, summaryResponse{
		TotalUpdates:      summary.TotalUpdates,
		SuccessfulUpdates: summary.SuccessfulUpdates,
		FailedUpdates:     summary.FailedUpdates,
		AvgChangePct:      summary.AvgChangePct,
		PeriodStart:       summary.PeriodStart,
		PeriodEnd:         summary.PeriodEnd,
	})
}

// ExportCSV godoc
//
//	@Summary		CSV-экспорт журнала
//	@Description	CSV-выгрузка журнала с теми же фильтрами, что и список. Лимит 10 000 строк — при большем объёме нужно сужать фильтр.
//	@Tags			audit
//	@Produce		text/csv
//	@Param			shop_id			query		string	false	"UUID магазина"
//	@Param			product_id		query		string	false	"UUID товара"
//	@Param			external_sku	query		string	false	"Подстрока external_sku товара"
//	@Param			status			query		string	false	"success | failed | skipped"
//	@Param			from			query		string	false	"Начало периода (RFC3339)"
//	@Param			to				query		string	false	"Конец периода (RFC3339)"
//	@Param			sort_dir		query		string	false	"asc | desc (default desc)"
//	@Success		200	{string}	string	"CSV-файл"
//	@Failure		400	{object}	errorResponse
//	@Failure		401	{object}	errorResponse
//	@Failure		500	{object}	errorResponse
//	@Security		SessionCookie
//	@Router			/api/audit/price-changes.csv [get]
func (h *auditHandler) ExportCSV(c *gin.Context) {
	user := mustUser(c)
	filter, ok := parseAuditFilter(c)
	if !ok {
		return
	}
	csvBytes, err := h.svc.ExportCSV(c.Request.Context(), user.ID, filter)
	if err != nil {
		handleAuditErr(c, err)
		return
	}
	c.Header("Content-Disposition", "attachment; filename=\"price-changes.csv\"")
	c.Data(http.StatusOK, "text/csv; charset=utf-8", csvBytes)
}

func parseAuditFilter(c *gin.Context) (auditsvc.ListFilter, bool) {
	var f auditsvc.ListFilter

	if raw := firstQuery(c, "shop_id", "shopId"); raw != "" {
		id, err := uuid.Parse(raw)
		if err != nil {
			errResp(c, http.StatusBadRequest, "bad_request", "Неверный формат shop_id")
			return f, false
		}
		f.ShopID = &id
	}
	if raw := firstQuery(c, "product_id", "productId"); raw != "" {
		id, err := uuid.Parse(raw)
		if err != nil {
			errResp(c, http.StatusBadRequest, "bad_request", "Неверный формат product_id")
			return f, false
		}
		f.ProductID = &id
	}
	f.ExternalSKU = firstQuery(c, "external_sku", "externalSku")

	if raw := c.Query("status"); raw != "" {
		switch raw {
		case "success", "failed", "skipped":
			f.Status = raw
		default:
			errResp(c, http.StatusBadRequest, "bad_request", "Неверный статус: ожидается success | failed | skipped")
			return f, false
		}
	}

	if raw := c.Query("from"); raw != "" {
		t, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			errResp(c, http.StatusBadRequest, "bad_request", "Неверный формат from (ожидается RFC3339)")
			return f, false
		}
		f.From = t
	}
	if raw := c.Query("to"); raw != "" {
		t, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			errResp(c, http.StatusBadRequest, "bad_request", "Неверный формат to (ожидается RFC3339)")
			return f, false
		}
		f.Until = t
	}

	f.Page = parsePositiveInt(c.Query("page"))
	f.PerPage = parsePositiveInt(firstQuery(c, "per_page", "perPage"))
	if dir := firstQuery(c, "sort_dir", "sortDir"); dir != "" {
		switch dir {
		case "asc", "desc":
			f.SortDir = dir
		default:
			errResp(c, http.StatusBadRequest, "bad_request", "Неверный sort_dir: ожидается asc | desc")
			return f, false
		}
	}
	return f, true
}

func handleAuditErr(c *gin.Context, err error) {
	switch {
	case errors.Is(err, auditsvc.ErrInvalidFilter):
		errResp(c, http.StatusBadRequest, "bad_request", err.Error())
	default:
		errResp(c, http.StatusInternalServerError, "internal_error", "Внутренняя ошибка сервера")
	}
}

func toPriceChangeResponse(c *domain.PriceChange) priceChangeResponse {
	var strategyID *string
	if c.StrategyID != nil {
		id := c.StrategyID.String()
		strategyID = &id
	}
	return priceChangeResponse{
		ID:            c.ID.String(),
		ShopID:        c.ShopID.String(),
		ProductID:     c.ProductID.String(),
		ProductName:   c.ProductName,
		StrategyID:    strategyID,
		OldPrice:      c.OldPrice,
		NewPrice:      c.NewPrice,
		TargetPrice:   c.TargetPrice,
		Reason:        c.Reason,
		ConstraintHit: c.ConstraintHit,
		Status:        c.Status,
		CreatedAt:     c.CreatedAt,
	}
}
