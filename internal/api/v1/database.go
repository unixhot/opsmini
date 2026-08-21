package v1

import (
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/opsmini/opsmini/internal/pkg/response"
	"github.com/opsmini/opsmini/internal/service"
)

// DatabaseHandler database management handler.
type DatabaseHandler struct {
	svc *service.DatabaseService
}

// NewDatabaseHandler constructor.
func NewDatabaseHandler(svc *service.DatabaseService) *DatabaseHandler {
	return &DatabaseHandler{svc: svc}
}

// List GET /databases
func (h *DatabaseHandler) List(c *gin.Context) {
	list, err := h.svc.List()
	if err != nil {
		response.Error(c, 500, response.CodeInternal, err.Error())
		return
	}
	response.OK(c, list)
}

// Create POST /databases
func (h *DatabaseHandler) Create(c *gin.Context) {
	var req service.DatabaseReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, response.CodeInvalidParam, err.Error())
		return
	}
	d, err := h.svc.Create(req)
	if err != nil {
		response.Error(c, 500, response.CodeInternal, err.Error())
		return
	}
	response.OK(c, d)
}

// Update PUT /databases/:id
func (h *DatabaseHandler) Update(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.Error(c, 400, response.CodeInvalidParam, "invalid id")
		return
	}
	var req service.DatabaseReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, response.CodeInvalidParam, err.Error())
		return
	}
	d, err := h.svc.Update(uint(id), req)
	if err != nil {
		response.Error(c, 500, response.CodeInternal, err.Error())
		return
	}
	response.OK(c, d)
}

// Delete DELETE /databases/:id
func (h *DatabaseHandler) Delete(c *gin.Context) {
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
