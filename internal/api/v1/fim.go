package v1

import (
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/opsmini/opsmini/internal/pkg/response"
	"github.com/opsmini/opsmini/internal/service"
)

// FimHandler file integrity monitoring handler.
type FimHandler struct {
	svc *service.FimService
}

// NewFimHandler constructor.
func NewFimHandler(svc *service.FimService) *FimHandler {
	return &FimHandler{svc: svc}
}

// ListBaselines GET /security/fim/baselines
func (h *FimHandler) ListBaselines(c *gin.Context) {
	list, err := h.svc.ListBaselines()
	if err != nil {
		response.Error(c, 500, response.CodeInternal, err.Error())
		return
	}
	response.OK(c, list)
}

// Rebuild POST /security/fim/rebuild
func (h *FimHandler) Rebuild(c *gin.Context) {
	if err := h.svc.Rebuild(); err != nil {
		response.Error(c, 500, response.CodeInternal, err.Error())
		return
	}
	response.OK(c, nil)
}

// RemoveBaseline DELETE /security/fim/baselines/:id
func (h *FimHandler) RemoveBaseline(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		response.Error(c, 400, response.CodeInvalidParam, "invalid id")
		return
	}
	if err := h.svc.RemoveBaseline(uint(id)); err != nil {
		response.Error(c, 500, response.CodeInternal, err.Error())
		return
	}
	response.OK(c, nil)
}

// ListChanges GET /security/fim/events
func (h *FimHandler) ListChanges(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "200"))
	list, err := h.svc.ListChanges(limit)
	if err != nil {
		response.Error(c, 500, response.CodeInternal, err.Error())
		return
	}
	response.OK(c, list)
}
