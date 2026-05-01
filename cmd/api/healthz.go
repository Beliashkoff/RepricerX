package main

import (
	"net/http"

	"github.com/Beliashkoff/RepricerX/internal/pkg/redischeck"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

type healthHandlers struct {
	pool      *pgxpool.Pool
	redisAddr string
}

// healthz godoc
//
//	@Summary		Liveness probe
//	@Description	Возвращает 200, если процесс запущен. Не проверяет зависимости.
//	@Tags			system
//	@Produce		plain
//	@Success		200
//	@Router			/healthz [get]
func (h *healthHandlers) healthz(c *gin.Context) {
	c.Status(http.StatusOK)
}

// ready godoc
//
//	@Summary		Readiness probe
//	@Description	Возвращает 200, если Postgres и Redis доступны.
//	@Description	При недоступности любого из них — 503 с описанием причины.
//	@Description	Используется как healthcheck в Docker Compose и CI.
//	@Tags			system
//	@Produce		json
//	@Success		200
//	@Failure		503	{object}	map[string]string	"db unavailable / redis unavailable"
//	@Router			/ready [get]
func (h *healthHandlers) ready(c *gin.Context) {
	if err := h.pool.Ping(c.Request.Context()); err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "db unavailable"})
		return
	}
	if err := redischeck.Ping(c.Request.Context(), h.redisAddr); err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "redis unavailable"})
		return
	}
	c.Status(http.StatusOK)
}
