package v1

import (
	"github.com/gin-gonic/gin"

	"github.com/opsmini/opsmini/internal/pkg/response"
	"github.com/opsmini/opsmini/internal/service"
)

// SettingHandler panel settings handler.
type SettingHandler struct {
	svc *service.SettingService
}

// NewSettingHandler constructor.
func NewSettingHandler(svc *service.SettingService) *SettingHandler {
	return &SettingHandler{svc: svc}
}

// List GET /settings - return all settings.
func (h *SettingHandler) List(c *gin.Context) {
	kv, err := h.svc.List()
	if err != nil {
		response.Error(c, 500, response.CodeInternal, err.Error())
		return
	}
	response.OK(c, kv)
}

// Update PUT /settings - batch update settings (key-value pairs).
func (h *SettingHandler) Update(c *gin.Context) {
	var kv map[string]string
	if err := c.ShouldBindJSON(&kv); err != nil {
		response.Error(c, 400, response.CodeInvalidParam, err.Error())
		return
	}
	if len(kv) == 0 {
		response.Error(c, 400, response.CodeInvalidParam, "empty settings")
		return
	}
	if err := h.svc.Set(kv); err != nil {
		response.Error(c, 500, response.CodeInternal, err.Error())
		return
	}
	response.OK(c, nil)
}
