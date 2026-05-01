package transport

import (
	"net/http"

	shopsvc "github.com/Beliashkoff/RepricerX/internal/service/shop"
	"github.com/Beliashkoff/RepricerX/internal/domain"
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

// GET /api/shops
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

// GET /api/shops/:id
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

// POST /api/shops
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

// PATCH /api/shops/:id
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

// DELETE /api/shops/:id
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

// POST /api/shops/:id/test
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
	c.JSON(http.StatusOK, gin.H{"status": domain.ShopStatusActive})
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
		CreatedAt:         s.CreatedAt,
	}
}
