package middleware

import (
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/opsmini/opsmini/internal/service"
)

// Audit records all write operations (POST/PUT/DELETE) to the audit log.
// It must be used after the Auth middleware to obtain userID from the context.
func Audit(audit *service.AuditService) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()

		method := c.Request.Method
		action := map[string]string{
			"POST":   "create",
			"PUT":    "update",
			"DELETE": "delete",
		}[method]
		if action == "" {
			return
		}

		var uid uint
		if v, ok := c.Get("userID"); ok {
			if id, ok := v.(uint); ok {
				uid = id
			}
		}
		audit.Record(uid, "", action, resourceOf(c.FullPath()), c.FullPath(), c.ClientIP())
	}
}

// resourceOf extracts the resource name from a route path, e.g. /api/v1/websites/:id → websites.
func resourceOf(path string) string {
	p := strings.TrimPrefix(path, "/api/v1/")
	if p == "" {
		return "unknown"
	}
	seg := strings.SplitN(p, "/", 2)[0]
	if seg == "" {
		return "unknown"
	}
	return seg
}
