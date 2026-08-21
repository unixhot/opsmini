package v1

import (
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/opsmini/opsmini/internal/pkg/response"
	"github.com/opsmini/opsmini/internal/service"
)

// CronHandler scheduled task handler.
type CronHandler struct {
	svc *service.CronService
}

// NewCronHandler constructor.
func NewCronHandler(svc *service.CronService) *CronHandler {
	return &CronHandler{svc: svc}
}

// List GET /cron-jobs
func (h *CronHandler) List(c *gin.Context) {
	list, err := h.svc.List()
	if err != nil {
		response.Error(c, 500, response.CodeInternal, err.Error())
		return
	}
	response.OK(c, list)
}

// Create POST /cron-jobs
func (h *CronHandler) Create(c *gin.Context) {
	var req service.CronReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, response.CodeInvalidParam, err.Error())
		return
	}
	j, err := h.svc.Create(req)
	if err != nil {
		response.Error(c, 500, response.CodeInternal, err.Error())
		return
	}
	response.OK(c, j)
}

// Update PUT /cron-jobs/:id
func (h *CronHandler) Update(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.Error(c, 400, response.CodeInvalidParam, "invalid id")
		return
	}
	var req service.CronReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, response.CodeInvalidParam, err.Error())
		return
	}
	j, err := h.svc.Update(uint(id), req)
	if err != nil {
		response.Error(c, 500, response.CodeInternal, err.Error())
		return
	}
	response.OK(c, j)
}

// Delete DELETE /cron-jobs/:id
func (h *CronHandler) Delete(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.Error(c, 400, response.CodeInvalidParam, "invalid id")
		return
	}
	if err := h.svc.Delete(uint(id)); err != nil {
		response.Error(c, 500, response.CodeInternal, err.Error())
		return
	}
	response.OK(c, nil)
}
