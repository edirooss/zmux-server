package handler

import (
	"net/http"

	"github.com/edirooss/zmux-server/internal/service"
	"github.com/gin-gonic/gin"
)

func Me(authsvc *service.AuthService) gin.HandlerFunc {
	return func(c *gin.Context) {
		p := authsvc.WhoAmI(c)
		if p == nil {
			c.Status(http.StatusUnauthorized)
			return
		}

		c.JSON(http.StatusOK, p)
	}
}
