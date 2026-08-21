// Package response provides the unified API response structure.
package response

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// Body is the unified response body; code=0 means success.
type Body struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// OK sends a success response.
func OK(c *gin.Context, data interface{}) {
	c.JSON(http.StatusOK, Body{Code: 0, Message: "ok", Data: data})
}

// Error sends an error response.
func Error(c *gin.Context, httpStatus, code int, msg string) {
	c.JSON(httpStatus, Body{Code: code, Message: msg})
}

// Common business error codes.
const (
	CodeInvalidParam = 40001
	CodeUnauthorized = 40101
	CodeForbidden    = 40301
	CodeNotFound     = 40401
	CodeInternal     = 50001
)
