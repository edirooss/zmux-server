package channelshndlr

import (
	"net/http"
	"strconv"

	"github.com/edirooss/zmux-server/services"
	"github.com/gin-gonic/gin"
)

func GetChannelList(c *gin.Context, channelService *services.ChannelService) {
	chs, err := channelService.ListChannels(c.Request.Context())
	if err != nil {
		c.Error(err)
		c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
		return
	}
	c.Header("X-Total-Count", strconv.Itoa(len(chs))) // RA needs this
	c.JSON(http.StatusOK, chs)
}
