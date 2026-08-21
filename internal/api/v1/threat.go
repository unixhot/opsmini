package v1

import (
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/opsmini/opsmini/internal/pkg/response"
	"github.com/opsmini/opsmini/internal/service"
)

// ThreatHandler threat detection handler.
type ThreatHandler struct {
	svc *service.ThreatService
}

// NewThreatHandler constructor.
func NewThreatHandler(svc *service.ThreatService) *ThreatHandler {
	return &ThreatHandler{svc: svc}
}

// List GET /security/threats
func (h *ThreatHandler) List(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "200"))
	list, err := h.svc.List(limit)
	if err != nil {
		response.Error(c, 500, response.CodeInternal, err.Error())
		return
	}
	response.OK(c, list)
}

// Scan POST /security/threats/scan
func (h *ThreatHandler) Scan(c *gin.Context) {
	list, err := h.svc.Scan()
	if err != nil {
		response.Error(c, 500, response.CodeInternal, err.Error())
		return
	}
	response.OK(c, list)
}

// Resolve POST /security/threats/:id/resolve
func (h *ThreatHandler) Resolve(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		response.Error(c, 400, response.CodeInvalidParam, "invalid id")
		return
	}
	if err := h.svc.Resolve(uint(id)); err != nil {
		response.Error(c, 500, response.CodeInternal, err.Error())
		return
	}
	response.OK(c, nil)
}
