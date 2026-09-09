// Package v1 defines the v1 HTTP handlers.
package v1

import (
	"errors"

	"github.com/gin-gonic/gin"

	"github.com/opsmini/opsmini/internal/pkg/response"
	"github.com/opsmini/opsmini/internal/service"
)

// AuthHandler authentication handler.
type AuthHandler struct {
	svc   *service.AuthService
	audit *service.AuditService
}

// NewAuthHandler constructor.
func NewAuthHandler(svc *service.AuthService, audit *service.AuditService) *AuthHandler {
	return &AuthHandler{svc: svc, audit: audit}
}

type loginReq struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

// Login POST /auth/login
func (h *AuthHandler) Login(c *gin.Context) {
	var req loginReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, response.CodeInvalidParam, err.Error())
		return
	}
	result, err := h.svc.Login(req.Username, req.Password)
	if err != nil {
		if h.audit != nil {
			h.audit.Record(0, req.Username, "login_fail", "auth", err.Error(), c.ClientIP())
		}
		response.Error(c, 401, response.CodeUnauthorized, err.Error())
		return
	}
	if result.MFARequired {
		if h.audit != nil {
			h.audit.Record(0, req.Username, "login", "auth", "mfa pending", c.ClientIP())
		}
		response.OK(c, result)
		return
	}
	if h.audit != nil {
		h.audit.Record(0, req.Username, "login", "auth", "login success", c.ClientIP())
	}
	response.OK(c, result)
}

type mfaVerifyReq struct {
	MFAToken string `json:"mfa_token" binding:"required"`
	Code     string `json:"code" binding:"required"`
}

// VerifyMFA POST /auth/mfa/verify - issue the formal token after the MFA code is verified.
func (h *AuthHandler) VerifyMFA(c *gin.Context) {
	var req mfaVerifyReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, response.CodeInvalidParam, err.Error())
		return
	}
	pair, err := h.svc.VerifyMFALogin(req.MFAToken, req.Code)
	if err != nil {
		if h.audit != nil {
			h.audit.Record(0, "", "login_fail", "auth", "mfa: "+err.Error(), c.ClientIP())
		}
		if errors.Is(err, service.ErrInvalidMFAToken) {
			response.Error(c, 401, response.CodeUnauthorized, err.Error())
		} else {
			response.Error(c, 401, response.CodeUnauthorized, err.Error())
		}
		return
	}
	if h.audit != nil {
		h.audit.Record(0, "", "login", "auth", "mfa success", c.ClientIP())
	}
	response.OK(c, pair)
}

type mfaCodeReq struct {
	Code string `json:"code" binding:"required"`
}

// MFASetup POST /auth/mfa/setup - generate a TOTP secret and otpauth URI (login required).
func (h *AuthHandler) MFASetup(c *gin.Context) {
	uid, _ := c.Get("userID")
	secret, url, err := h.svc.SetupMFA(toUint(uid))
	if err != nil {
		response.Error(c, 500, response.CodeInternal, err.Error())
		return
	}
	response.OK(c, gin.H{"secret": secret, "otpauth_url": url})
}

// MFAEnable POST /auth/mfa/enable - verify the one-time code then enable MFA (login required).
func (h *AuthHandler) MFAEnable(c *gin.Context) {
	var req mfaCodeReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, response.CodeInvalidParam, err.Error())
		return
	}
	uid, _ := c.Get("userID")
	if err := h.svc.EnableMFA(toUint(uid), req.Code); err != nil {
		response.Error(c, 400, response.CodeInvalidParam, err.Error())
		return
	}
	response.OK(c, nil)
}

// MFADisable POST /auth/mfa/disable - disable MFA (login required).
func (h *AuthHandler) MFADisable(c *gin.Context) {
	uid, _ := c.Get("userID")
	if err := h.svc.DisableMFA(toUint(uid)); err != nil {
		response.Error(c, 500, response.CodeInternal, err.Error())
		return
	}
	response.OK(c, nil)
}

// MFAStatus GET /auth/mfa/status - query the current user's MFA status (login required).
func (h *AuthHandler) MFAStatus(c *gin.Context) {
	uid, _ := c.Get("userID")
	status, err := h.svc.MFAStatus(toUint(uid))
	if err != nil {
		response.Error(c, 500, response.CodeInternal, err.Error())
		return
	}
	response.OK(c, status)
}

type refreshReq struct {
	RefreshToken string `json:"refresh_token" binding:"required"`
}

// Refresh POST /auth/refresh
func (h *AuthHandler) Refresh(c *gin.Context) {
	var req refreshReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, response.CodeInvalidParam, err.Error())
		return
	}
	pair, err := h.svc.Refresh(req.RefreshToken)
	if err != nil {
		response.Error(c, 401, response.CodeUnauthorized, err.Error())
		return
	}
	response.OK(c, pair)
}

// Logout POST /auth/logout - revoke the current token.
func (h *AuthHandler) Logout(c *gin.Context) {
	tokenStr, _ := c.Get("tokenStr")
	if ts, ok := tokenStr.(string); ok && ts != "" {
		h.svc.Logout(ts)
	}
	uid, _ := c.Get("userID")
	if h.audit != nil {
		h.audit.Record(toUint(uid), "", "logout", "auth", "logout", c.ClientIP())
	}
	response.OK(c, nil)
}

// toUint converts a context value into a uint (used to read the JWT userID claim).
func toUint(v interface{}) uint {
	switch n := v.(type) {
	case uint:
		return n
	case int:
		return uint(n)
	case float64:
		return uint(n)
	}
	return 0
}
