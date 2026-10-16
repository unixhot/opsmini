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
		if !authenticate(c, jwtMgr, strings.TrimPrefix(auth, "Bearer ")) {
			c.Abort()
			return
		}
		c.Next()
	}
}

// AuthQuery behaves like Auth but additionally accepts the token from the query
// string (?<param>=<token>).
//
// This exists for endpoints a browser has to open directly: a new tab, or a plain
// link click, cannot carry an Authorization header. Mount it only on the handful
// of routes that need it (report viewing / download); everything else keeps using
// Auth so the token does not leak into URLs, browser history and access logs more
// often than necessary.
func AuthQuery(jwtMgr *jwt.Manager, param string) gin.HandlerFunc {
	return func(c *gin.Context) {
		tokenStr := ""
		if auth := c.GetHeader("Authorization"); strings.HasPrefix(auth, "Bearer ") {
			tokenStr = strings.TrimPrefix(auth, "Bearer ")
		} else if param != "" {
			tokenStr = c.Query(param)
		}
		if strings.TrimSpace(tokenStr) == "" {
			response.Error(c, 401, response.CodeUnauthorized, "unauthorized")
			c.Abort()
			return
		}
		if !authenticate(c, jwtMgr, tokenStr) {
			c.Abort()
			return
		}
		c.Next()
	}
}

// authenticate validates the token and stores the identity in the context,
// writing the error response itself when the token is unusable.
func authenticate(c *gin.Context, jwtMgr *jwt.Manager, tokenStr string) bool {
	if strings.TrimSpace(tokenStr) == "" {
		response.Error(c, 401, response.CodeUnauthorized, "unauthorized")
		return false
	}
	claims, err := jwtMgr.Parse(tokenStr)
	if err != nil {
		response.Error(c, 401, response.CodeUnauthorized, "invalid token")
		return false
	}
	if jwtMgr.IsRevoked(tokenStr) {
		response.Error(c, 401, response.CodeUnauthorized, "token revoked")
		return false
	}
	c.Set("userID", claims.UserID)
	c.Set("role", claims.Role)
	c.Set("tokenStr", tokenStr)
	return true
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
