package channelshandler

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/edirooss/zmux-server/internal/dto/channelsdto"
	"github.com/edirooss/zmux-server/redis"
	"github.com/gin-gonic/gin"
)

func (h *ChannelsHandler) ReplaceChannel(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.Error(err)
		c.JSON(http.StatusBadRequest, gin.H{"message": "invalid id"})
		return
	}

	exists, err := h.svc.ChannelExists(c.Request.Context(), id)
	if err != nil {
		c.Error(err)
		c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
		return
	}
	if !exists {
		c.Status(http.StatusNotFound)
		return
	}

	var req channelsdto.ReplaceChannel
	if err := bind(c.Request, &req); err != nil {
		c.Error(err)
		c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
		return
	}

	// Replace obj
	ch, err := req.ToChannel(id)
	if err != nil {
		c.Error(err)
		c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
		return
	}

	if err := ch.Validate(); err != nil {
		c.Error(err)
		c.JSON(http.StatusUnprocessableEntity, gin.H{"message": err.Error()})
		return
	}

	if err := h.svc.UpdateChannel(c.Request.Context(), ch); err != nil {
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
