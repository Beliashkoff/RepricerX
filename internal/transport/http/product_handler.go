package transport

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/Beliashkoff/RepricerX/internal/domain"
	"github.com/Beliashkoff/RepricerX/internal/repository"
	productsvc "github.com/Beliashkoff/RepricerX/internal/service/product"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type ProductHandler struct {
	svc *productsvc.Service
}

func NewProductHandler(svc *productsvc.Service) *ProductHandler {
	return &ProductHandler{svc: svc}
}

// StartImport godoc
//
//	@Summary		Запустить импорт SKU
//	@Description	Создаёт durable background job импорта товаров магазина и возвращает ID импорта для polling.
//	@Tags			products
//	@Produce		json
//	@Param			id	path		string				true	"UUID магазина"
//	@Success		202	{object}	importStartResponse	"Импорт поставлен в очередь"
//	@Failure		400	{object}	errorResponse
//	@Failure		401	{object}	errorResponse
//	@Failure		403	{object}	errorResponse
//	@Failure		404	{object}	errorResponse
//	@Failure		409	{object}	errorResponse	"Импорт уже выполняется"
//	@Failure		429	{object}	errorResponse	"Cooldown импорта"
//	@Failure		500	{object}	errorResponse
//	@Security		SessionCookie
//	@Router			/api/shops/{id}/products/import [post]
func (h *ProductHandler) StartImport(c *gin.Context) {
	user := mustUser(c)
	shopID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		errResp(c, http.StatusBadRequest, "bad_request", "Неверный формат ID")
		return
	}
	entry, err := h.svc.StartImport(c.Request.Context(), user.ID, shopID)
	if err != nil {
		handleProductErr(c, err)
		return
	}
	var jobID *string
	if entry.JobID != nil {
		value := entry.JobID.String()
		jobID = &value
	}
	c.JSON(http.StatusAccepted, importStartResponse{
		ImportID:  entry.ID.String(),
		JobID:     jobID,
		ShopID:    entry.ShopID.String(),
		Status:    entry.Status,
		StartedAt: entry.StartedAt,
		PollURL:   "/api/imports/" + entry.ID.String(),
	})
}

// GetImport godoc
//
//	@Summary		Статус импорта SKU
//	@Description	Возвращает user-scoped статус импорта, counters и capped список ошибок.
//	@Tags			products
//	@Produce		json
//	@Param			id	path		string					true	"UUID импорта"
//	@Success		200	{object}	importStatusResponse	"Статус импорта"
//	@Failure		400	{object}	errorResponse
//	@Failure		401	{object}	errorResponse
//	@Failure		404	{object}	errorResponse
//	@Failure		500	{object}	errorResponse
//	@Security		SessionCookie
//	@Router			/api/imports/{id} [get]
func (h *ProductHandler) GetImport(c *gin.Context) {
	user := mustUser(c)
	importID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		errResp(c, http.StatusBadRequest, "bad_request", "Неверный формат ID")
		return
	}
	entry, err := h.svc.GetImport(c.Request.Context(), user.ID, importID)
	if err != nil {
		handleProductErr(c, err)
		return
	}
	c.JSON(http.StatusOK, toImportStatusResponse(entry))
}

func (h *ProductHandler) Create(c *gin.Context) {
	user := mustUser(c)
	shopID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		errResp(c, http.StatusBadRequest, "bad_request", "Неверный формат ID")
		return
	}
	var req createProductRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		errResp(c, http.StatusBadRequest, "bad_request", "Неверный формат запроса")
		return
	}
	product, err := h.svc.CreateManual(c.Request.Context(), user.ID, shopID, productsvc.CreateInput{
		ExternalSKU: req.ExternalSKU, Name: req.Name, CurrentPrice: req.CurrentPrice,
		Currency: req.Currency, Status: req.Status,
		MinPrice: req.MinPrice, MaxPrice: req.MaxPrice, CostPrice: req.CostPrice,
	})
	if err != nil {
		handleProductErr(c, err)
		return
	}
	c.JSON(http.StatusCreated, toProductResponse(product))
}

func (h *ProductHandler) List(c *gin.Context) {
	user := mustUser(c)
	filter, ok := parseProductListFilter(c)
	if !ok {
		return
	}
	result, err := h.svc.List(c.Request.Context(), user.ID, filter)
	if err != nil {
		handleProductErr(c, err)
		return
	}
	items := make([]productResponse, 0, len(result.Items))
	for _, product := range result.Items {
		items = append(items, toProductResponse(product))
	}
	c.JSON(http.StatusOK, productListResponse{
		Items: items,
		Pagination: paginationInfo{
			Page: result.Page, PerPage: result.PerPage, Total: result.Total,
		},
	})
}

func (h *ProductHandler) Patch(c *gin.Context) {
	user := mustUser(c)
	productID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		errResp(c, http.StatusBadRequest, "bad_request", "Неверный формат ID")
		return
	}
	patch, ok := parsePricePatch(c)
	if !ok {
		return
	}
	product, err := h.svc.PatchPrices(c.Request.Context(), user.ID, productID, patch)
	if err != nil {
		handleProductErr(c, err)
		return
	}
	c.JSON(http.StatusOK, toProductResponse(product))
}

func parseProductListFilter(c *gin.Context) (productsvc.ListFilter, bool) {
	var filter productsvc.ListFilter
	filter.Query = c.Query("q")
	if shopIDRaw := firstQuery(c, "shop_id", "shopId"); shopIDRaw != "" {
		shopID, err := uuid.Parse(shopIDRaw)
		if err != nil {
			errResp(c, http.StatusBadRequest, "bad_request", "Неверный формат shop_id")
			return filter, false
		}
		filter.ShopID = &shopID
	}
	filter.Status = c.Query("status")
	if raw := firstQuery(c, "has_strategy", "hasStrategy"); raw != "" {
		value, err := strconv.ParseBool(raw)
		if err != nil {
			errResp(c, http.StatusBadRequest, "bad_request", "Неверный формат has_strategy")
			return filter, false
		}
		filter.HasStrategy = &value
	}
	filter.Page = parsePositiveInt(c.Query("page"))
	filter.PerPage = parsePositiveInt(firstQuery(c, "per_page", "perPage"))
	return filter, true
}

func firstQuery(c *gin.Context, names ...string) string {
	for _, name := range names {
		if value := c.Query(name); value != "" {
			return value
		}
	}
	return ""
}

func parsePositiveInt(raw string) int {
	if raw == "" {
		return 0
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return 0
	}
	return value
}

func parsePricePatch(c *gin.Context) (productsvc.PricePatch, bool) {
	var raw map[string]json.RawMessage
	if err := c.ShouldBindJSON(&raw); err != nil {
		errResp(c, http.StatusBadRequest, "bad_request", "Неверный формат запроса")
		return productsvc.PricePatch{}, false
	}
	patch := productsvc.PricePatch{}
	if value, exists := raw["minPrice"]; exists {
		parsed, ok := parseNullableFloat(c, value)
		if !ok {
			return productsvc.PricePatch{}, false
		}
		patch.MinPrice = repository.OptionalFloat64{Set: true, Value: parsed}
	}
	if value, exists := raw["maxPrice"]; exists {
		parsed, ok := parseNullableFloat(c, value)
		if !ok {
			return productsvc.PricePatch{}, false
		}
		patch.MaxPrice = repository.OptionalFloat64{Set: true, Value: parsed}
	}
	if value, exists := raw["costPrice"]; exists {
		parsed, ok := parseNullableFloat(c, value)
		if !ok {
			return productsvc.PricePatch{}, false
		}
		patch.CostPrice = repository.OptionalFloat64{Set: true, Value: parsed}
	}
	if !patch.MinPrice.Set && !patch.MaxPrice.Set && !patch.CostPrice.Set {
		errResp(c, http.StatusBadRequest, "bad_request", "Нет полей для обновления")
		return productsvc.PricePatch{}, false
	}
	return patch, true
}

func parseNullableFloat(c *gin.Context, raw json.RawMessage) (*float64, bool) {
	if string(raw) == "null" {
		return nil, true
	}
	var value float64
	if err := json.Unmarshal(raw, &value); err != nil {
		errResp(c, http.StatusBadRequest, "bad_request", "Неверный формат цены")
		return nil, false
	}
	return &value, true
}

func toProductResponse(p *domain.Product) productResponse {
	return productResponse{
		ID: p.ID.String(), ShopID: p.ShopID.String(),
		ExternalSKU: p.ExternalSKU, Name: p.Name, CurrentPrice: p.CurrentPrice,
		Currency: p.Currency, Status: p.Status,
		MinPrice: p.MinPrice, MaxPrice: p.MaxPrice, CostPrice: p.CostPrice,
		StockCount: p.StockCount, Rating: p.Rating, ReviewsCount: p.ReviewsCount,
		LastSyncedAt: p.LastSyncedAt, HasStrategy: p.HasStrategy,
		CreatedAt: p.CreatedAt, UpdatedAt: p.UpdatedAt,
	}
}

func toImportStatusResponse(entry *domain.ImportLogEntry) importStatusResponse {
	var jobID *string
	if entry.JobID != nil {
		value := entry.JobID.String()
		jobID = &value
	}
	errors := make([]importErrorDTO, 0, len(entry.Errors))
	for _, err := range entry.Errors {
		errors = append(errors, importErrorDTO{
			ExternalSKU: err.ExternalSKU,
			Code:        err.Code,
			Message:     err.Message,
		})
	}
	return importStatusResponse{
		ID: entry.ID.String(), JobID: jobID, ShopID: entry.ShopID.String(),
		Status: entry.Status, JobStatus: entry.JobStatus,
		Total: entry.Total, Added: entry.Added, Updated: entry.Updated,
		Skipped: entry.Skipped, Failed: entry.Failed, Errors: errors,
		StartedAt: entry.StartedAt, FinishedAt: entry.FinishedAt,
	}
}
