package middleware

import (
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"
)

// CORS allows only same-origin cross-origin requests (echoes the Origin that matches the request Host).
// It no longer uses "*" to prevent pages from arbitrary origins from calling panel APIs cross-origin with user credentials.
func CORS() gin.HandlerFunc {
	return func(c *gin.Context) {
		origin := c.Request.Header.Get("Origin")
		if origin != "" {
			if u, err := url.Parse(origin); err == nil && strings.EqualFold(u.Host, c.Request.Host) {
				c.Header("Access-Control-Allow-Origin", origin)
				c.Header("Vary", "Origin")
				c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
				c.Header("Access-Control-Allow-Headers", "Authorization, Content-Type")
			}
		}
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}
		c.Next()
	}
}
