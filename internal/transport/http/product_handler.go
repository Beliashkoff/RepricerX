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

// Create godoc
//
//	@Summary		Добавить товар вручную
//	@Description	Создаёт товар в указанном магазине без обращения к API маркетплейса.
//	@Tags			products
//	@Accept			json
//	@Produce		json
//	@Param			id		path		string					true	"UUID магазина"
//	@Param			body	body		createProductRequest	true	"Данные товара"
//	@Success		201		{object}	productResponse			"Созданный товар"
//	@Failure		400		{object}	errorResponse
//	@Failure		401		{object}	errorResponse
//	@Failure		403		{object}	errorResponse
//	@Failure		404		{object}	errorResponse			"Магазин не найден"
//	@Failure		409		{object}	errorResponse			"SKU уже существует в этом магазине"
//	@Failure		500		{object}	errorResponse
//	@Security		SessionCookie
//	@Router			/api/shops/{id}/products [post]
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

// List godoc
//
//	@Summary		Список товаров
//	@Description	Возвращает постраничный список товаров пользователя с фильтрацией и сортировкой.
//	@Tags			products
//	@Produce		json
//	@Param			q			query		string				false	"Полнотекстовый поиск по названию и SKU"
//	@Param			shopId		query		string				false	"Фильтр по UUID магазина"
//	@Param			status		query		string				false	"Фильтр по статусу: active | archived | out_of_stock"
//	@Param			hasStrategy	query		boolean				false	"Фильтр по наличию стратегии"
//	@Param			sortBy		query		string				false	"Поле сортировки: name | current_price | updated_at (по умолчанию)"
//	@Param			sortDir		query		string				false	"Направление: asc | desc (по умолчанию)"
//	@Param			priceFrom	query		number				false	"Фильтр: цена не ниже"
//	@Param			priceTo		query		number				false	"Фильтр: цена не выше"
//	@Param			page		query		integer				false	"Номер страницы (с 1)"
//	@Param			perPage		query		integer				false	"Размер страницы (1–100, по умолчанию 50)"
//	@Success		200			{object}	productListResponse	"Список товаров с пагинацией"
//	@Failure		400			{object}	errorResponse
//	@Failure		401			{object}	errorResponse
//	@Failure		500			{object}	errorResponse
//	@Security		SessionCookie
//	@Router			/api/products [get]
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

// Patch godoc
//
//	@Summary		Обновить цены товара
//	@Description	Устанавливает min_price, max_price и/или cost_price. Передавайте только изменяемые поля; null сбрасывает значение.
//	@Tags			products
//	@Accept			json
//	@Produce		json
//	@Param			id		path		string			true	"UUID товара"
//	@Param			body	body		object			true	"Поля: minPrice, maxPrice, costPrice (number | null)"
//	@Success		200		{object}	productResponse	"Обновлённый товар"
//	@Failure		400		{object}	errorResponse
//	@Failure		401		{object}	errorResponse
//	@Failure		403		{object}	errorResponse
//	@Failure		404		{object}	errorResponse	"Товар не найден"
//	@Failure		500		{object}	errorResponse
//	@Security		SessionCookie
//	@Router			/api/products/{id} [patch]
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

// Delete godoc
//
//	@Summary		Архивировать товар (soft-delete)
//	@Description	Переводит товар в статус archived. Данные не удаляются.
//	@Tags			products
//	@Produce		json
//	@Param			id	path	string	true	"UUID товара"
//	@Success		204	"Товар архивирован"
//	@Failure		401	{object}	errorResponse
//	@Failure		403	{object}	errorResponse
//	@Failure		404	{object}	errorResponse	"Товар не найден"
//	@Failure		500	{object}	errorResponse
//	@Security		SessionCookie
//	@Router			/api/products/{id} [delete]
func (h *ProductHandler) Delete(c *gin.Context) {
	user := mustUser(c)
	productID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		errResp(c, http.StatusBadRequest, "bad_request", "Неверный формат ID")
		return
	}
	if err := h.svc.SoftDelete(c.Request.Context(), user.ID, productID); err != nil {
		handleProductErr(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// BulkPatch godoc
//
//	@Summary		Массовое обновление цен
//	@Description	Обновляет min_price/max_price/cost_price для набора товаров (до 100) за один запрос.
//	@Tags			products
//	@Accept			json
//	@Produce		json
//	@Param			body	body		bulkPatchRequest	true	"Список товаров с изменениями"
//	@Success		200		{object}	bulkPatchResponse	"Количество обновлённых товаров"
//	@Failure		400		{object}	errorResponse
//	@Failure		401		{object}	errorResponse
//	@Failure		403		{object}	errorResponse
//	@Failure		500		{object}	errorResponse
//	@Security		SessionCookie
//	@Router			/api/products/bulk-patch [post]
func (h *ProductHandler) BulkPatch(c *gin.Context) {
	user := mustUser(c)
	var req bulkPatchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		errResp(c, http.StatusBadRequest, "bad_request", "Неверный формат запроса")
		return
	}
	items := make([]productsvc.BulkPatchItem, 0, len(req.Products))
	for _, p := range req.Products {
		productID, err := uuid.Parse(p.ID)
		if err != nil {
			errResp(c, http.StatusBadRequest, "bad_request", "Неверный формат ID товара: "+p.ID)
			return
		}
		items = append(items, productsvc.BulkPatchItem{
			ProductID: productID,
			MinPrice:  repository.OptionalFloat64{Set: p.MinPrice != nil, Value: p.MinPrice},
			MaxPrice:  repository.OptionalFloat64{Set: p.MaxPrice != nil, Value: p.MaxPrice},
			CostPrice: repository.OptionalFloat64{Set: p.CostPrice != nil, Value: p.CostPrice},
		})
	}
	updated, err := h.svc.BulkPatch(c.Request.Context(), user.ID, items)
	if err != nil {
		handleProductErr(c, err)
		return
	}
	c.JSON(http.StatusOK, bulkPatchResponse{Updated: updated})
}

// Export godoc
//
//	@Summary		Экспорт каталога в CSV
//	@Description	Возвращает все товары пользователя (с учётом фильтров) в виде CSV-файла. Лимит 10 000 строк.
//	@Tags			products
//	@Produce		text/csv
//	@Param			shopId		query	string	false	"Фильтр по UUID магазина"
//	@Param			status		query	string	false	"Фильтр по статусу: active | archived | out_of_stock"
//	@Param			q			query	string	false	"Полнотекстовый поиск"
//	@Param			sortBy		query	string	false	"Поле сортировки"
//	@Param			sortDir		query	string	false	"Направление: asc | desc"
//	@Param			priceFrom	query	number	false	"Цена не ниже"
//	@Param			priceTo		query	number	false	"Цена не выше"
//	@Success		200	{string}	string	"CSV-файл с товарами"
//	@Failure		400	{object}	errorResponse
//	@Failure		401	{object}	errorResponse
//	@Failure		500	{object}	errorResponse
//	@Security		SessionCookie
//	@Router			/api/products/export [get]
func (h *ProductHandler) Export(c *gin.Context) {
	user := mustUser(c)
	filter, ok := parseProductListFilter(c)
	if !ok {
		return
	}
	csvBytes, err := h.svc.ExportCSV(c.Request.Context(), user.ID, filter)
	if err != nil {
		handleProductErr(c, err)
		return
	}
	c.Header("Content-Disposition", "attachment; filename=\"products.csv\"")
	c.Data(http.StatusOK, "text/csv; charset=utf-8", csvBytes)
}

// CancelImport godoc
//
//	@Summary		Отменить импорт
//	@Description	Отменяет импорт со статусом pending или running. Если импорт уже завершён — возвращает 409.
//	@Tags			products
//	@Produce		json
//	@Param			id	path	string	true	"UUID импорта"
//	@Success		204	"Импорт отменён"
//	@Failure		400	{object}	errorResponse
//	@Failure		401	{object}	errorResponse
//	@Failure		403	{object}	errorResponse
//	@Failure		409	{object}	errorResponse	"Импорт уже завершён или не найден"
//	@Failure		500	{object}	errorResponse
//	@Security		SessionCookie
//	@Router			/api/imports/{id} [delete]
func (h *ProductHandler) CancelImport(c *gin.Context) {
	user := mustUser(c)
	importID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		errResp(c, http.StatusBadRequest, "bad_request", "Неверный формат ID")
		return
	}
	if err := h.svc.CancelImport(c.Request.Context(), user.ID, importID); err != nil {
		handleProductErr(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// GetImportErrors godoc
//
//	@Summary		Детали ошибок импорта
//	@Description	Возвращает постраничный список ошибок конкретного импорта (только для импортов пользователя).
//	@Tags			products
//	@Produce		json
//	@Param			id		path		string					true	"UUID импорта"
//	@Param			page	query		integer					false	"Номер страницы (с 1)"
//	@Param			perPage	query		integer					false	"Размер страницы (1–100, по умолчанию 20)"
//	@Success		200		{object}	importErrorsResponse	"Список ошибок с пагинацией"
//	@Failure		400		{object}	errorResponse
//	@Failure		401		{object}	errorResponse
//	@Failure		404		{object}	errorResponse			"Импорт не найден"
//	@Failure		500		{object}	errorResponse
//	@Security		SessionCookie
//	@Router			/api/imports/{id}/errors [get]
func (h *ProductHandler) GetImportErrors(c *gin.Context) {
	user := mustUser(c)
	importID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		errResp(c, http.StatusBadRequest, "bad_request", "Неверный формат ID")
		return
	}
	page := parsePositiveInt(c.Query("page"))
	perPage := parsePositiveInt(firstQuery(c, "per_page", "perPage"))
	if page < 1 {
		page = 1
	}
	if perPage < 1 {
		perPage = 20
	}
	errs, total, err := h.svc.GetImportErrors(c.Request.Context(), user.ID, importID, page, perPage)
	if err != nil {
		handleProductErr(c, err)
		return
	}
	items := make([]importErrorDTO, 0, len(errs))
	for _, e := range errs {
		items = append(items, importErrorDTO{ExternalSKU: e.ExternalSKU, Code: e.Code, Message: e.Message})
	}
	c.JSON(http.StatusOK, importErrorsResponse{Items: items, Total: total, Page: page, PerPage: perPage})
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
	filter.SortBy = firstQuery(c, "sort_by", "sortBy")
	filter.SortDir = firstQuery(c, "sort_dir", "sortDir")
	if raw := firstQuery(c, "price_from", "priceFrom"); raw != "" {
		v, err := strconv.ParseFloat(raw, 64)
		if err != nil {
			errResp(c, http.StatusBadRequest, "bad_request", "Неверный формат price_from")
			return filter, false
		}
		filter.PriceFrom = &v
	}
	if raw := firstQuery(c, "price_to", "priceTo"); raw != "" {
		v, err := strconv.ParseFloat(raw, 64)
		if err != nil {
			errResp(c, http.StatusBadRequest, "bad_request", "Неверный формат price_to")
			return filter, false
		}
		filter.PriceTo = &v
	}
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
