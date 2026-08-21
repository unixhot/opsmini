package v1

import (
	"github.com/gin-gonic/gin"

	"github.com/opsmini/opsmini/internal/pkg/response"
	"github.com/opsmini/opsmini/internal/service"
)

// SecurityHandler security overview handler.
type SecurityHandler struct {
	svc *service.SecurityService
}

// NewSecurityHandler constructor.
func NewSecurityHandler(svc *service.SecurityService) *SecurityHandler {
	return &SecurityHandler{svc: svc}
}

// Overview GET /security/overview
func (h *SecurityHandler) Overview(c *gin.Context) {
	ov, err := h.svc.Overview()
	if err != nil {
		response.Error(c, 500, response.CodeInternal, err.Error())
		return
	}
	response.OK(c, ov)
}
