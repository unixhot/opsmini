package middleware

import (
	"crypto/subtle"
	"encoding/base64"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// MetricsAuth validates optional Basic authentication for the /metrics endpoint.
// credsProvider returns (username, password) on each request; an empty username
// disables authentication (allow all).
//
// Authentication scheme: Authorization: Basic base64(user:pass). This is the only
// scheme node_exporter targets support natively, via Prometheus `basic_auth`.
func MetricsAuth(credsProvider func() (string, string)) gin.HandlerFunc {
	return func(c *gin.Context) {
		user, pass := credsProvider()
		if user == "" {
			c.Next()
			return
		}
		auth := c.GetHeader("Authorization")
		if !strings.HasPrefix(auth, "Basic ") {
			c.Header("WWW-Authenticate", `Basic realm="metrics"`)
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}
		got := strings.TrimPrefix(auth, "Basic ")
		decoded, err := base64.StdEncoding.DecodeString(got)
		if err != nil {
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}
		parts := strings.SplitN(string(decoded), ":", 2)
		if len(parts) != 2 {
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}
		userOK := subtle.ConstantTimeCompare([]byte(parts[0]), []byte(user)) == 1
		passOK := subtle.ConstantTimeCompare([]byte(parts[1]), []byte(pass)) == 1
		if !userOK || !passOK {
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}
		c.Next()
	}
}
