package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// CapConcurrentRequests returns a Gin middleware that limits the number
// of concurrent HTTP requests being processed. If the number of active
// requests exceeds `maxConcurrent`, new requests are rejected with HTTP 429.
//
// We use it to protect downstream high-latency operations.
//
// Example usage:
//
//	router.Use(CapConcurrentRequests(100)) // allow up to 100 concurrent requests
func CapConcurrentRequests(maxConcurrent int) gin.HandlerFunc {
	semaphore := make(chan struct{}, maxConcurrent)

	return func(c *gin.Context) {
		select {
		case semaphore <- struct{}{}:
			defer func() { <-semaphore }()
			c.Next()
		default:
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"error": "too many concurrent requests",
			})
		}
	}
}
