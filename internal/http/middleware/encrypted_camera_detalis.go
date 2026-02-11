package middleware

import (
	"net/http"

	"github.com/edirooss/zmux-server/internal/http/handler"
	"github.com/gin-gonic/gin"
)

// RequireValidEncryptedCameraDetailsPost ensures the encrypted camera details ID is valid in request body.
func RequireValidEncryptedCameraDetailsPost() gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			EncryptedCameraDetails string `json:"encrypted_camera_details" binding:"required"`
		}
		if err := c.ShouldBindJSON(&req); err != nil || req.EncryptedCameraDetails == "" {
			c.AbortWithStatus(http.StatusBadRequest)
			return
		}
		decrypted, err := handler.DecryptCameraDetails(req.EncryptedCameraDetails)
		if err != nil {
			c.AbortWithStatus(http.StatusBadRequest)
			return
		}
		c.Set("decrypted_camera_details", decrypted)
		c.Next()
	}
}

// RequireValidEncryptedCameraDetailsGet ensures the path param ":encrypted_camera_details" is valid.
func RequireValidEncryptedCameraDetailsGet() gin.HandlerFunc {
	return func(c *gin.Context) {
		encryptedCameraDetails := c.Query("encrypted_camera_details")
		if encryptedCameraDetails == "" {
			c.AbortWithStatus(http.StatusBadRequest)
			return
		}
		decrypted, err := handler.DecryptCameraDetails(encryptedCameraDetails)
		if err != nil {
			c.AbortWithStatus(http.StatusBadRequest)
			return
		}
		c.Set("decrypted_camera_details", decrypted)
		c.Next()
	}
}
