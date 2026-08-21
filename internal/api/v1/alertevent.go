package v1

import (
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/opsmini/opsmini/internal/pkg/response"
	"github.com/opsmini/opsmini/internal/repository"
)

// AlertEventHandler alert event handler.
type AlertEventHandler struct {
	repo *repository.AlertEventRepo
}

// NewAlertEventHandler constructor.
func NewAlertEventHandler(repo *repository.AlertEventRepo) *AlertEventHandler {
	return &AlertEventHandler{repo: repo}
}

// List GET /alert-events?limit=200 - current + historical alert events (time descending).
func (h *AlertEventHandler) List(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "200"))
	list, err := h.repo.List(limit)
	if err != nil {
		response.Error(c, 500, response.CodeInternal, err.Error())
		return
	}
	active, _ := h.repo.CountActive()
	response.OK(c, gin.H{"active": active, "events": list})
}

// Active GET /alert-events/active - currently unresolved events.
func (h *AlertEventHandler) Active(c *gin.Context) {
	list, err := h.repo.ListActive()
	if err != nil {
		response.Error(c, 500, response.CodeInternal, err.Error())
		return
	}
	response.OK(c, list)
}

// Delete DELETE /alert-events/:id
func (h *AlertEventHandler) Delete(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.Error(c, 400, response.CodeInvalidParam, "invalid id")
		return
	}
	if err := h.repo.Delete(uint(id)); err != nil {
		response.Error(c, 500, response.CodeInternal, err.Error())
		return
	}
	response.OK(c, nil)
}
