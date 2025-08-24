package channelshndlr

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/edirooss/zmux-server/redis"
	"github.com/gin-gonic/gin"
)

func (h *ChannelsHandler) GetChannel(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.Error(err)
		c.JSON(http.StatusBadRequest, gin.H{"message": "invalid id"})
		return
	}

	ch, err := h.svc.GetChannel(c.Request.Context(), id)
	if err != nil {
		c.Error(err)
		if errors.Is(err, redis.ErrChannelNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"message": redis.ErrChannelNotFound.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
		return
	}

	c.JSON(http.StatusOK, ch)
}
