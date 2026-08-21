package v1

import (
	"github.com/gin-gonic/gin"

	"github.com/opsmini/opsmini/internal/pkg/response"
	"github.com/opsmini/opsmini/internal/service"
)

// BaselineHandler security baseline check handler.
type BaselineHandler struct {
	svc *service.BaselineService
}

// NewBaselineHandler constructor.
func NewBaselineHandler(svc *service.BaselineService) *BaselineHandler {
	return &BaselineHandler{svc: svc}
}

// Scan POST /security/baseline/scan
func (h *BaselineHandler) Scan(c *gin.Context) {
	results, err := h.svc.RunScan()
	if err != nil {
		response.Error(c, 500, response.CodeInternal, err.Error())
		return
	}
	response.OK(c, results)
}

// Latest GET /security/baseline/latest
func (h *BaselineHandler) Latest(c *gin.Context) {
	results, err := h.svc.Latest()
	if err != nil {
		response.Error(c, 500, response.CodeInternal, err.Error())
		return
	}
	response.OK(c, results)
}
