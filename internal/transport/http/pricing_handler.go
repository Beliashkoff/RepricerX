package transport

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/Beliashkoff/RepricerX/internal/domain"
	pricingsvc "github.com/Beliashkoff/RepricerX/internal/service/pricing"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type pricingHandler struct {
	svc *pricingsvc.Service
}

func NewPricingHandler(svc *pricingsvc.Service) *pricingHandler {
	return &pricingHandler{svc: svc}
}

func (h *pricingHandler) Simulate(c *gin.Context) {
	user := mustUser(c)
	var req simulatePriceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		errResp(c, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	productID, err := uuid.Parse(req.ProductID)
	if err != nil {
		errResp(c, http.StatusBadRequest, "invalid_product_id", "Неверный формат product_id")
		return
	}
	strategyID, err := uuid.Parse(req.StrategyID)
	if err != nil {
		errResp(c, http.StatusBadRequest, "invalid_strategy_id", "Неверный формат strategy_id")
		return
	}
	result, err := h.svc.Simulate(c.Request.Context(), user.ID, pricingsvc.SimulateInput{
		ProductID:        productID,
		StrategyID:       strategyID,
		CompetitorPrice:  req.CompetitorPrice,
		CompetitorPrices: req.CompetitorPrices,
		CostPrice:        req.CostPrice,
	})
	if err != nil {
		handlePricingErr(c, err)
		return
	}
	c.JSON(http.StatusOK, simulatePriceResponse{
		TargetPrice:      result.TargetPrice,
		FinalPrice:       result.FinalPrice,
		ConstraintHit:    result.ConstraintHit,
		Status:           result.Status,
		Reason:           result.Reason,
		ChangePct:        result.ChangePct,
		CompetitorPrice:  result.CompetitorPrice,
		CompetitorSource: result.CompetitorSource,
	})
}

func (h *pricingHandler) Recalculate(c *gin.Context) {
	user := mustUser(c)
	var req recalculateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		errResp(c, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	shopID, err := uuid.Parse(req.ShopID)
	if err != nil {
		errResp(c, http.StatusBadRequest, "invalid_shop_id", "Неверный формат shop_id")
		return
	}
	productIDs, err := parseUUIDs(req.ProductIDs)
	if err != nil {
		errResp(c, http.StatusBadRequest, "invalid_product_id", "Неверный формат product_id")
		return
	}
	plan, job, err := h.svc.Recalculate(c.Request.Context(), user.ID, shopID, productIDs)
	if err != nil {
		handlePricingErr(c, err)
		return
	}
	c.JSON(http.StatusAccepted, recalculateResponse{
		Plan:  toPlanResponse(plan),
		JobID: job.ID.String(),
	})
}

func (h *pricingHandler) ListPlans(c *gin.Context) {
	user := mustUser(c)
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
	plans, total, err := h.svc.ListPlans(c.Request.Context(), user.ID, limit, offset)
	if err != nil {
		handlePricingErr(c, err)
		return
	}
	items := make([]pricePlanResponse, 0, len(plans))
	for _, p := range plans {
		items = append(items, toPlanResponse(p))
	}
	c.JSON(http.StatusOK, pricePlanListResponse{
		Items:  items,
		Total:  total,
		Limit:  limit,
		Offset: offset,
	})
}

func (h *pricingHandler) GetPlan(c *gin.Context) {
	user := mustUser(c)
	planID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		errResp(c, http.StatusBadRequest, "invalid_id", "Неверный формат id")
		return
	}
	plan, items, err := h.svc.GetPlan(c.Request.Context(), user.ID, planID)
	if err != nil {
		handlePricingErr(c, err)
		return
	}
	resp := pricePlanDetailResponse{
		Plan:  toPlanResponse(plan),
		Items: make([]pricePlanItemResponse, 0, len(items)),
	}
	for _, it := range items {
		var stratID *string
		if it.StrategyID != nil {
			s := it.StrategyID.String()
			stratID = &s
		}
		resp.Items = append(resp.Items, pricePlanItemResponse{
			ID:            it.ID.String(),
			ProductID:     it.ProductID.String(),
			ProductName:   it.ProductName,
			StrategyID:    stratID,
			CurrentPrice:  it.CurrentPrice,
			TargetPrice:   it.TargetPrice,
			FinalPrice:    it.FinalPrice,
			ConstraintHit: it.ConstraintHit,
			Status:        it.Status,
			Error:         it.Error,
		})
	}
	c.JSON(http.StatusOK, resp)
}

func toPlanResponse(p *domain.PricePlan) pricePlanResponse {
	return pricePlanResponse{
		ID:        p.ID.String(),
		ShopID:    p.ShopID.String(),
		Status:    p.Status,
		Total:     p.Total,
		CreatedAt: p.CreatedAt,
		UpdatedAt: p.UpdatedAt,
	}
}

func handlePricingErr(c *gin.Context, err error) {
	switch {
	case errors.Is(err, pricingsvc.ErrProductNotFound):
		errResp(c, http.StatusNotFound, "product_not_found", "Товар не найден")
	case errors.Is(err, pricingsvc.ErrStrategyNotFound):
		errResp(c, http.StatusNotFound, "strategy_not_found", "Стратегия не найдена")
	case errors.Is(err, pricingsvc.ErrShopNotFound):
		errResp(c, http.StatusNotFound, "shop_not_found", "Магазин не найден")
	case errors.Is(err, pricingsvc.ErrPlanNotFound):
		errResp(c, http.StatusNotFound, "plan_not_found", "План не найден")
	case errors.Is(err, pricingsvc.ErrInvalidSimulation):
		errResp(c, http.StatusBadRequest, "invalid_simulation", err.Error())
	default:
		errResp(c, http.StatusInternalServerError, "internal_error", "Внутренняя ошибка сервера")
	}
}
