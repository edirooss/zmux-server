// internal/http/middleware/request_id.go
package middleware

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const RequestIDKey = "request_id"

// RequestID is a Gin middleware that ensures every request has a unique identifier.
// It checks for an existing X-Request-ID header from the client, and if not present or invalid,
// generates a new UUID. The request ID is then:
// - Added to the response headers as X-Request-ID
// - Stored in the Gin context for use by other middleware/handlers
//
// This is useful for distributed tracing and request correlation across services.
func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Check if request already has an ID
		requestID := c.GetHeader("X-Request-ID")

		// Generate new ID if not present or invalid provided
		l := len(requestID)
		if l < 1 || l > 64 {
			requestID = uuid.New().String()
		}

		// Set in response headers
		c.Header("X-Request-ID", requestID)

		// Store in context for use by other middleware/handlers
		c.Set(RequestIDKey, requestID)

		c.Next()
	}
}

// GetRequestID retrieves the request ID from the Gin context.
// Returns empty string if no request ID is found.
func GetRequestID(c *gin.Context) string {
	if requestID, exists := c.Get(RequestIDKey); exists {
		if id, ok := requestID.(string); ok {
			return id
		}
	}
	return ""
}
