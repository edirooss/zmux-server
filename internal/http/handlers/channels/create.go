package channelshndlr

import (
	"fmt"
	"net/http"

	"github.com/edirooss/zmux-server/pkg/models/channelmodel"
	"github.com/gin-gonic/gin"
)

func (h *ChannelsHandler) CreateChannel(c *gin.Context) {
	var req channelmodel.CreateZmuxChannelReq
	if err := bind(c.Request, &req); err != nil {
		c.Error(err)
		c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
		return
	}
	if err := req.Validate(); err != nil {
		c.Error(err)
		c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
		return
	}
	req.ApplyDefaults()

	ch := req.ToChannel(0)
	if err := ch.Validate(); err != nil {
		c.Error(err)
		c.JSON(http.StatusUnprocessableEntity, gin.H{"message": err.Error()})
		return
	}

	if err := h.svc.CreateChannel(c.Request.Context(), ch); err != nil {
		c.Error(err)
		c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
		return
	}

	c.Header("Location", fmt.Sprintf("/api/channels/%d", ch.ID))
	c.JSON(http.StatusCreated, ch)
}
