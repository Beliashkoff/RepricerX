package transport

import (
	"errors"
	"net/http"

	strategysvc "github.com/Beliashkoff/RepricerX/internal/service/strategy"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type strategyHandler struct {
	svc *strategysvc.Service
}

func NewStrategyHandler(svc *strategysvc.Service) *strategyHandler {
	return &strategyHandler{svc: svc}
}

func (h *strategyHandler) List(c *gin.Context) {
	user := mustUser(c)
	items, err := h.svc.List(c.Request.Context(), user.ID)
	if err != nil {
		handleStrategyErr(c, err)
		return
	}
	resp := make([]strategyResponse, 0, len(items))
	for _, st := range items {
		n := h.svc.AssignedCount(c.Request.Context(), st.ID)
		resp = append(resp, toStrategyResponse(st, n))
	}
	c.JSON(http.StatusOK, resp)
}

func (h *strategyHandler) Get(c *gin.Context) {
	user := mustUser(c)
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		errResp(c, http.StatusBadRequest, "invalid_id", "Неверный формат ID")
		return
	}
	st, err := h.svc.Get(c.Request.Context(), user.ID, id)
	if err != nil {
		handleStrategyErr(c, err)
		return
	}
	productIDs, err := h.svc.AssignedProductIDs(c.Request.Context(), user.ID, id)
	if err != nil {
		handleStrategyErr(c, err)
		return
	}
	if productIDs == nil {
		productIDs = []uuid.UUID{}
	}
	c.JSON(http.StatusOK, strategyDetailResponse{
		strategyResponse:   toStrategyResponse(st, len(productIDs)),
		AssignedProductIDs: productIDs,
	})
}

func (h *strategyHandler) Create(c *gin.Context) {
	user := mustUser(c)
	var req createStrategyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		errResp(c, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	st, err := h.svc.Create(c.Request.Context(), user.ID, strategysvc.CreateInput{
		Name:           req.Name,
		Type:           req.Type,
		Params:         req.Params,
		Constraints:    req.Constraints,
		FallbackPolicy: req.FallbackPolicy,
		Priority:       req.Priority,
		Enabled:        req.Enabled,
	})
	if err != nil {
		handleStrategyErr(c, err)
		return
	}
	c.JSON(http.StatusCreated, toStrategyResponse(st, 0))
}

func (h *strategyHandler) Update(c *gin.Context) {
	user := mustUser(c)
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		errResp(c, http.StatusBadRequest, "invalid_id", "Неверный формат ID")
		return
	}
	var req updateStrategyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		errResp(c, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	st, err := h.svc.Update(c.Request.Context(), user.ID, id, strategysvc.UpdatePatch{
		Name:           req.Name,
		Type:           req.Type,
		Params:         req.Params,
		Constraints:    req.Constraints,
		FallbackPolicy: req.FallbackPolicy,
		Priority:       req.Priority,
		Enabled:        req.Enabled,
	})
	if err != nil {
		handleStrategyErr(c, err)
		return
	}
	n := h.svc.AssignedCount(c.Request.Context(), st.ID)
	c.JSON(http.StatusOK, toStrategyResponse(st, n))
}

func (h *strategyHandler) Delete(c *gin.Context) {
	user := mustUser(c)
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		errResp(c, http.StatusBadRequest, "invalid_id", "Неверный формат ID")
		return
	}
	if err := h.svc.Delete(c.Request.Context(), user.ID, id); err != nil {
		handleStrategyErr(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *strategyHandler) Assign(c *gin.Context) {
	user := mustUser(c)
	strategyID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		errResp(c, http.StatusBadRequest, "invalid_id", "Неверный формат ID")
		return
	}
	var req assignStrategyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		errResp(c, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	productIDs, err := parseUUIDs(req.ProductIDs)
	if err != nil {
		errResp(c, http.StatusBadRequest, "invalid_product_id", "Неверный формат productId")
		return
	}
	if err := h.svc.AssignToProducts(c.Request.Context(), user.ID, strategyID, productIDs); err != nil {
		handleStrategyErr(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *strategyHandler) Unassign(c *gin.Context) {
	user := mustUser(c)
	strategyID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		errResp(c, http.StatusBadRequest, "invalid_id", "Неверный формат ID")
		return
	}
	var req assignStrategyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		errResp(c, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	productIDs, err := parseUUIDs(req.ProductIDs)
	if err != nil {
		errResp(c, http.StatusBadRequest, "invalid_product_id", "Неверный формат productId")
		return
	}
	if err := h.svc.UnassignFromProducts(c.Request.Context(), user.ID, strategyID, productIDs); err != nil {
		handleStrategyErr(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func parseUUIDs(raw []string) ([]uuid.UUID, error) {
	ids := make([]uuid.UUID, 0, len(raw))
	for _, s := range raw {
		id, err := uuid.Parse(s)
		if err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, nil
}

func handleStrategyErr(c *gin.Context, err error) {
	switch {
	case errors.Is(err, strategysvc.ErrStrategyNotFound):
		errResp(c, http.StatusNotFound, "strategy_not_found", "Стратегия не найдена")
	case errors.Is(err, strategysvc.ErrInvalidStrategyType):
		errResp(c, http.StatusBadRequest, "invalid_strategy_type", err.Error())
	case errors.Is(err, strategysvc.ErrInvalidStrategyParams):
		errResp(c, http.StatusBadRequest, "invalid_strategy_params", err.Error())
	case errors.Is(err, strategysvc.ErrInvalidConstraints):
		errResp(c, http.StatusBadRequest, "invalid_constraints", err.Error())
	case errors.Is(err, strategysvc.ErrProductNotFound):
		errResp(c, http.StatusBadRequest, "product_not_found", "Один или несколько товаров не найдены")
	default:
		errResp(c, http.StatusInternalServerError, "internal_error", "Внутренняя ошибка сервера")
	}
}
