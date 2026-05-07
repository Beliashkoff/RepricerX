package transport

import (
	"errors"
	"net/http"

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
		ProductID:       productID,
		StrategyID:      strategyID,
		CompetitorPrice: req.CompetitorPrice,
		CostPrice:       req.CostPrice,
	})
	if err != nil {
		handlePricingErr(c, err)
		return
	}
	c.JSON(http.StatusOK, simulatePriceResponse{
		TargetPrice:      result.TargetPrice,
		FinalPrice:       result.FinalPrice,
		ConstraintHit:    result.ConstraintHit,
		Reason:           result.Reason,
		ChangePct:        result.ChangePct,
		CompetitorPrice:  result.CompetitorPrice,
		CompetitorSource: result.CompetitorSource,
	})
}

func handlePricingErr(c *gin.Context, err error) {
	switch {
	case errors.Is(err, pricingsvc.ErrProductNotFound):
		errResp(c, http.StatusNotFound, "product_not_found", "Товар не найден")
	case errors.Is(err, pricingsvc.ErrStrategyNotFound):
		errResp(c, http.StatusNotFound, "strategy_not_found", "Стратегия не найдена")
	case errors.Is(err, pricingsvc.ErrInvalidSimulation):
		errResp(c, http.StatusBadRequest, "invalid_simulation", err.Error())
	default:
		errResp(c, http.StatusInternalServerError, "internal_error", "Внутренняя ошибка сервера")
	}
}
