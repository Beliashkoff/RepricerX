package transport

import (
	"net/http"

	"github.com/Beliashkoff/RepricerX/internal/domain"
	auditsvc "github.com/Beliashkoff/RepricerX/internal/service/audit"
	"github.com/gin-gonic/gin"
)

type auditHandler struct {
	svc *auditsvc.Service
}

func NewAuditHandler(svc *auditsvc.Service) *auditHandler {
	return &auditHandler{svc: svc}
}

func (h *auditHandler) ListChanges(c *gin.Context) {
	user := mustUser(c)
	changes, err := h.svc.ListChanges(c.Request.Context(), user.ID)
	if err != nil {
		errResp(c, http.StatusInternalServerError, "internal_error", "Внутренняя ошибка сервера")
		return
	}
	resp := make([]priceChangeResponse, 0, len(changes))
	for _, change := range changes {
		resp = append(resp, toPriceChangeResponse(change))
	}
	c.JSON(http.StatusOK, resp)
}

func (h *auditHandler) Summary(c *gin.Context) {
	user := mustUser(c)
	summary, err := h.svc.Summary(c.Request.Context(), user.ID)
	if err != nil {
		errResp(c, http.StatusInternalServerError, "internal_error", "Внутренняя ошибка сервера")
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

func (h *auditHandler) ExportCSV(c *gin.Context) {
	user := mustUser(c)
	csvBytes, err := h.svc.ExportCSV(c.Request.Context(), user.ID)
	if err != nil {
		errResp(c, http.StatusInternalServerError, "internal_error", "Внутренняя ошибка сервера")
		return
	}
	c.Header("Content-Disposition", "attachment; filename=\"price-changes.csv\"")
	c.Data(http.StatusOK, "text/csv; charset=utf-8", csvBytes)
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
