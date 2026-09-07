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

// readSensitive are settings that must never be returned to the client.
// jwt_secret is a server-side secret (login signing key) that must stay hidden;
// metrics_pass is intentionally readable so the panel can show/copy it for Prometheus basic_auth config.
var readSensitive = map[string]bool{
	"jwt_secret": true, // server-side JWT signing key (generated automatically on first start)
}

// writeProtected are settings the client must never overwrite.
var writeProtected = map[string]bool{
	"jwt_secret": true, // managed by the server (SettingService.EnsureSecret)
}

// List GET /settings - return all non-sensitive settings.
func (h *SettingHandler) List(c *gin.Context) {
	kv, err := h.svc.List()
	if err != nil {
		response.Error(c, 500, response.CodeInternal, err.Error())
		return
	}
	for k := range readSensitive {
		delete(kv, k)
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
	for k := range writeProtected {
		delete(kv, k)
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
