package v1

import (
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/opsmini/opsmini/internal/model"
	"github.com/opsmini/opsmini/internal/pkg/response"
	"github.com/opsmini/opsmini/internal/service"
)

// AlertHandler alert rule handler.
type AlertHandler struct {
	svc *service.AlertService
}

// NewAlertHandler constructor.
func NewAlertHandler(svc *service.AlertService) *AlertHandler {
	return &AlertHandler{svc: svc}
}

type alertReq struct {
	Name      string `json:"name" binding:"required"`
	Metric    string `json:"metric"`
	Condition string `json:"condition"`
	Duration  string `json:"duration"`
	Notify    string `json:"notify"`
	Enabled   *bool  `json:"enabled"`
}

// List GET /alert-rules
func (h *AlertHandler) List(c *gin.Context) {
	list, err := h.svc.List()
	if err != nil {
		response.Error(c, 500, response.CodeInternal, err.Error())
		return
	}
	response.OK(c, list)
}

// Create POST /alert-rules
func (h *AlertHandler) Create(c *gin.Context) {
	var req alertReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, response.CodeInvalidParam, err.Error())
		return
	}
	rule := &model.AlertRule{
		Name: req.Name, Metric: req.Metric, Condition: req.Condition,
		Duration: req.Duration, Notify: req.Notify, Enabled: true,
	}
	if req.Enabled != nil {
		rule.Enabled = *req.Enabled
	}
	if err := h.svc.Create(rule); err != nil {
		response.Error(c, 500, response.CodeInternal, err.Error())
		return
	}
	response.OK(c, rule)
}

// Update PUT /alert-rules/:id
func (h *AlertHandler) Update(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.Error(c, 400, response.CodeInvalidParam, "invalid id")
		return
	}
	var req alertReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, response.CodeInvalidParam, err.Error())
		return
	}
	rule := &model.AlertRule{
		ID: uint(id), Name: req.Name, Metric: req.Metric, Condition: req.Condition,
		Duration: req.Duration, Notify: req.Notify,
	}
	if req.Enabled != nil {
		rule.Enabled = *req.Enabled
	} else {
		rule.Enabled = true
	}
	if err := h.svc.Update(rule); err != nil {
		response.Error(c, 500, response.CodeInternal, err.Error())
		return
	}
	response.OK(c, rule)
}

// Delete DELETE /alert-rules/:id
func (h *AlertHandler) Delete(c *gin.Context) {
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
