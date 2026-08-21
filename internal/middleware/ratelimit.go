package middleware

import (
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/opsmini/opsmini/internal/pkg/response"
)

// RateLimiter is an in-memory fixed-window rate limiter.
type RateLimiter struct {
	mu      sync.Mutex
	windows map[string]*window
	limit   int
	window  time.Duration
}

type window struct {
	count int
	reset time.Time
}

// NewRateLimiter creates a rate limiter. limit is the maximum number of requests per window, dur is the window duration.
func NewRateLimiter(limit int, dur time.Duration) *RateLimiter {
	return &RateLimiter{
		windows: make(map[string]*window),
		limit:   limit,
		window:  dur,
	}
}

// Allow determines whether key is allowed and increments the counter.
func (r *RateLimiter) Allow(key string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now()
	w, ok := r.windows[key]
	if !ok || now.After(w.reset) {
		r.windows[key] = &window{count: 1, reset: now.Add(r.window)}
		return true
	}
	if w.count >= r.limit {
		return false
	}
	w.count++
	return true
}

// Limit returns a rate-limiting middleware based on the client IP.
func (r *RateLimiter) Limit() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !r.Allow(c.ClientIP()) {
			response.Error(c, 429, response.CodeForbidden, "too many requests")
			c.Abort()
			return
		}
		c.Next()
	}
}
