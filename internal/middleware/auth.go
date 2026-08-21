// Package middleware provides HTTP middleware.
package middleware

import (
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/opsmini/opsmini/internal/pkg/jwt"
	"github.com/opsmini/opsmini/internal/pkg/response"
)

// Auth validates the Bearer token and injects userID and role into the context.
func Auth(jwtMgr *jwt.Manager) gin.HandlerFunc {
	return func(c *gin.Context) {
		auth := c.GetHeader("Authorization")
		if auth == "" || !strings.HasPrefix(auth, "Bearer ") {
			response.Error(c, 401, response.CodeUnauthorized, "unauthorized")
			c.Abort()
			return
		}
		tokenStr := strings.TrimPrefix(auth, "Bearer ")
		claims, err := jwtMgr.Parse(tokenStr)
		if err != nil {
			response.Error(c, 401, response.CodeUnauthorized, "invalid token")
			c.Abort()
			return
		}
		if jwtMgr.IsRevoked(tokenStr) {
			response.Error(c, 401, response.CodeUnauthorized, "token revoked")
			c.Abort()
			return
		}
		c.Set("userID", claims.UserID)
		c.Set("role", claims.Role)
		c.Set("tokenStr", tokenStr)
		c.Next()
	}
}

// RequireRole validates the role for RBAC.
func RequireRole(roles ...string) gin.HandlerFunc {
	allowed := make(map[string]struct{}, len(roles))
	for _, r := range roles {
		allowed[r] = struct{}{}
	}
	return func(c *gin.Context) {
		role, _ := c.Get("role")
		r, _ := role.(string)
		if _, ok := allowed[r]; ok {
			c.Next()
			return
		}
		response.Error(c, 403, response.CodeForbidden, "forbidden")
		c.Abort()
	}
}
