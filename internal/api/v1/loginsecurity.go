package v1

import (
	"github.com/gin-gonic/gin"

	"github.com/opsmini/opsmini/internal/pkg/response"
	"github.com/opsmini/opsmini/internal/service"
)

// LoginSecurityHandler login security analysis handler.
type LoginSecurityHandler struct {
	svc *service.LoginSecurityService
}

// NewLoginSecurityHandler constructor.
func NewLoginSecurityHandler(svc *service.LoginSecurityService) *LoginSecurityHandler {
	return &LoginSecurityHandler{svc: svc}
}

// Analyze GET /security/login
func (h *LoginSecurityHandler) Analyze(c *gin.Context) {
	report, err := h.svc.Analyze()
	if err != nil {
		response.Error(c, 500, response.CodeInternal, err.Error())
		return
	}
	response.OK(c, report)
}

// SSH GET /security/login/ssh
func (h *LoginSecurityHandler) SSH(c *gin.Context) {
	response.OK(c, h.svc.SSHHardening())
}
