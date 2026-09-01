package middleware

import (
	"crypto/subtle"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/opsmini/opsmini/internal/pkg/response"
)

// AgentAuth validates the Agent token (machine authentication for external systems, separate from user JWT).
// The token is resolved at request time via tokenProvider (read from panel settings, managed in the UI).
// When the resolved token is empty, the Agent API is disabled (returns 503).
func AgentAuth(tokenProvider func() string) gin.HandlerFunc {
	return func(c *gin.Context) {
		token := tokenProvider()
		if token == "" {
			response.Error(c, 503, response.CodeForbidden, "agent api not configured")
			c.Abort()
			return
		}
		auth := c.GetHeader("Authorization")
		if !strings.HasPrefix(auth, "Bearer ") {
			response.Error(c, 401, response.CodeUnauthorized, "unauthorized")
			c.Abort()
			return
		}
		got := strings.TrimPrefix(auth, "Bearer ")
		// Constant-time comparison to prevent timing attacks.
		if subtle.ConstantTimeCompare([]byte(got), []byte(token)) != 1 {
			response.Error(c, 401, response.CodeUnauthorized, "invalid agent token")
			c.Abort()
			return
		}
		c.Next()
	}
}
