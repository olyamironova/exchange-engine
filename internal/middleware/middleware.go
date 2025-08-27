package middleware

import (
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

type RateLimiter struct {
	clients map[string]time.Time
	mu      sync.Mutex
	limit   time.Duration
}

func NewRateLimiter(limit time.Duration) *RateLimiter {
	return &RateLimiter{
		clients: make(map[string]time.Time),
		limit:   limit,
	}
}

func (r *RateLimiter) Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		clientID := c.GetHeader("X-Client-ID")
		if clientID == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "X-Client-ID header required"})
			c.Abort()
			return
		}
		r.mu.Lock()
		last, exists := r.clients[clientID]
		if exists && time.Since(last) < r.limit {
			r.mu.Unlock()
			c.JSON(http.StatusTooManyRequests, gin.H{"error": "rate limit exceeded"})
			c.Abort()
			return
		}
		r.clients[clientID] = time.Now()
		r.mu.Unlock()
		c.Next()
	}
}
