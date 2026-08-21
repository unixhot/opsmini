package middleware

import (
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/opsmini/opsmini/internal/model"
	"github.com/opsmini/opsmini/internal/repository"
)

// AccessLog records each API request to the panel access log (method, path, status code, IP, latency).
func AccessLog(repo *repository.AccessLogRepo) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()

		entry := &model.AccessLog{
			Method:    c.Request.Method,
			Path:      c.FullPath(),
			Status:    c.Writer.Status(),
			IP:        c.ClientIP(),
			LatencyMs: time.Since(start).Milliseconds(),
		}
		if v, ok := c.Get("userID"); ok {
			if id, ok := v.(uint); ok {
				entry.Username = strconv.FormatUint(uint64(id), 10)
			}
		}
		_ = repo.Create(entry)
	}
}
