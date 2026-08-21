package middleware

import (
	"github.com/gin-gonic/gin"

	"github.com/opsmini/opsmini/internal/pkg/response"
)

// RequirePerm checks whether the current user's role has the specified permission point.
// hasPerm is injected by the caller (e.g. roleService.HasPerm) to avoid the middleware depending directly on the service layer.
func RequirePerm(hasPerm func(role, perm string) bool, perm string) gin.HandlerFunc {
	return func(c *gin.Context) {
		role, _ := c.Get("role")
		r, _ := role.(string)
		if r != "" && hasPerm(r, perm) {
			c.Next()
			return
		}
		response.Error(c, 403, response.CodeForbidden, "forbidden")
		c.Abort()
	}
}
