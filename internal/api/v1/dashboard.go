package v1

import (
	"time"

	"github.com/gin-gonic/gin"

	"github.com/opsmini/opsmini/internal/pkg/response"
	"github.com/opsmini/opsmini/internal/service"
)

// DashboardHandler dashboard and monitoring handler.
type DashboardHandler struct {
	sys      *service.SystemService
	metrics  *service.MetricCollector
}

// NewDashboardHandler constructor.
func NewDashboardHandler(sys *service.SystemService, metrics *service.MetricCollector) *DashboardHandler {
	return &DashboardHandler{sys: sys, metrics: metrics}
}

// Overview GET /dashboard/overview
func (h *DashboardHandler) Overview(c *gin.Context) {
	o, err := h.sys.Overview()
	if err != nil {
		response.Error(c, 500, response.CodeInternal, err.Error())
		return
	}
	response.OK(c, o)
}

// Metrics GET /dashboard/metrics?range=1h|24h|7d
func (h *DashboardHandler) Metrics(c *gin.Context) {
	rng := c.DefaultQuery("range", "1h")
	var (
		pts    []service.MetricPoint
		window time.Duration
		limit  int
	)
	switch rng {
	case "30m":
		window, limit = 30*time.Minute, 360
	case "6h":
		window, limit = 6*time.Hour, 720
	case "24h":
		window, limit = 24*time.Hour, 720
	case "7d":
		window, limit = 7*24*time.Hour, 720
	default:
		window, limit = time.Hour, 360
	}

	pts = h.metrics.Since(window)
	pts = service.Downsample(pts, limit)
	response.OK(c, gin.H{"range": rng, "points": pts})
}
