package channelshndlr

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/edirooss/zmux-server/redis"
	"github.com/gin-gonic/gin"
)

func (h *ChannelsHandler) DeleteChannel(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.Error(err)
		c.JSON(http.StatusBadRequest, gin.H{"message": "invalid id"})
		return
	}

	if err := h.svc.DeleteChannel(c.Request.Context(), id); err != nil {
		c.Error(err)
		if errors.Is(err, redis.ErrChannelNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"message": redis.ErrChannelNotFound.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
		return
	}

	// RA-friendly response
	c.JSON(http.StatusOK, gin.H{"id": id})
}
