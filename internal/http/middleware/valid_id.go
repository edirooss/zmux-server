package middleware

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

// RequireValidChannelID ensures the path param ":id" is a valid int > 0.
func RequireValidChannelID() gin.HandlerFunc {
	return func(c *gin.Context) {
		idStr := c.Param("id")
		id, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil || id <= 0 {
			c.AbortWithStatus(http.StatusBadRequest)
			return
		}
		c.Next()
	}
}
