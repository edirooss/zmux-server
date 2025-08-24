package channelshandler

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

func (h *ChannelsHandler) GetChannelList(c *gin.Context) {
	chs, err := h.svc.ListChannels(c.Request.Context())
	if err != nil {
		c.Error(err)
		c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
		return
	}
	c.Header("X-Total-Count", strconv.Itoa(len(chs))) // RA needs this
	c.JSON(http.StatusOK, chs)
}
