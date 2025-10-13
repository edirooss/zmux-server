package handler

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"

	b2bclient "github.com/edirooss/zmux-server/internal/domain/b2b-client"
	"github.com/edirooss/zmux-server/internal/domain/channel"
	"github.com/edirooss/zmux-server/internal/service"
	"github.com/gin-gonic/gin"
)

type B2BClientHandler struct {
	b2bclntsvc *service.B2BClientService
	chnlsvc    *service.ChannelService
}

func NewB2BClientHandler(b2bclntsvc *service.B2BClientService, chnlsvc *service.ChannelService) *B2BClientHandler {
	return &B2BClientHandler{b2bclntsvc, chnlsvc}
}

func (h *B2BClientHandler) CreateB2BClient(c *gin.Context) {
	var req struct {
		Name                string `json:"name"`
		EnabledChannelQuota int64  `json:"enabled_channel_quota"`
		EnabledOutputQuotas []struct {
			Ref string `json:"ref"`
			Val int64  `json:"val"`
		} `json:"enabled_output_quotas"`
		OnlineChannelQuota int64   `json:"online_channel_quota"`
		ChannelIDs         []int64 `json:"channel_ids"`
	}
	if err := bind(c.Request, &req); err != nil {
		c.Error(err)
		c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
		return
	}

	b2bClient := &b2bclient.B2BClient{
		Name:                req.Name,
		EnabledChannelQuota: req.EnabledChannelQuota,
		EnabledOutputQuotas: req.EnabledOutputQuotas,
		OnlineChannelQuota:  req.OnlineChannelQuota,
		ChannelIDs:          req.ChannelIDs,
	}

	if err := h.b2bclntsvc.Create(c.Request.Context(), b2bClient); err != nil {
		c.Error(err)

		if errors.Is(err, service.ErrConflict) {
			c.JSON(http.StatusConflict, gin.H{"message": err.Error()})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
		}

		return
	}

	c.Header("Location", fmt.Sprintf("/api/b2b-client/%d", b2bClient.ID))
	c.JSON(http.StatusCreated, b2bClient)
}

func (h *B2BClientHandler) UpdateB2BClient(c *gin.Context) {
	idStr := c.Param("id")
	b2bClientID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.Error(err)
		c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
		return
	}

	var req struct {
		Name                string `json:"name"`
		BearerToken         string `json:"bearer_token"`
		EnabledChannelQuota int64  `json:"enabled_channel_quota"`
		EnabledOutputQuotas []struct {
			Ref string `json:"ref"`
			Val int64  `json:"val"`
		} `json:"enabled_output_quotas"`
		OnlineChannelQuota int64   `json:"online_channel_quota"`
		ChannelIDs         []int64 `json:"channel_ids"`
	}
	if err := bind(c.Request, &req); err != nil {
		c.Error(err)
		c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
		return
	}

	b2bClient := &b2bclient.B2BClient{
		ID:                  b2bClientID,
		Name:                req.Name,
		EnabledChannelQuota: req.EnabledChannelQuota,
		EnabledOutputQuotas: req.EnabledOutputQuotas,
		OnlineChannelQuota:  req.OnlineChannelQuota,
		ChannelIDs:          req.ChannelIDs,
	}

	if err := h.b2bclntsvc.Update(c.Request.Context(), b2bClient); err != nil {
		c.Error(err)

		if errors.Is(err, service.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"message": err.Error()})
		} else if errors.Is(err, service.ErrConflict) {
			c.JSON(http.StatusConflict, gin.H{"message": err.Error()})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
		}

		return
	}

	c.JSON(http.StatusOK, b2bClient)
}

func (h *B2BClientHandler) GetB2BClient(c *gin.Context) {
	idStr := c.Param("id")
	b2bClientID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.Error(err)
		c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
		return
	}

	b2bClient, err := h.b2bclntsvc.GetOne(b2bClientID)
	if err != nil {
		c.Error(err)

		if errors.Is(err, service.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"message": err.Error()})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
		}

		return
	}

	c.JSON(http.StatusOK, b2bClient)
}

func (h *B2BClientHandler) GetAllB2BClients(c *gin.Context) {
	clients, err := h.b2bclntsvc.GetList()
	if err != nil {
		c.Error(err)
		c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
		return
	}

	c.JSON(http.StatusOK, clients)
}

func (h *B2BClientHandler) DeleteB2BClient(c *gin.Context) {
	idStr := c.Param("id")
	b2bClientID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.Error(err)
		c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
		return
	}

	if err := h.b2bclntsvc.Delete(c.Request.Context(), b2bClientID); err != nil {
		c.Error(err)

		if errors.Is(err, service.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"message": err.Error()})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
		}

		return
	}

	c.Status(http.StatusNoContent)
}

//
//
//

func (h *B2BClientHandler) GetChannelsAvailable(c *gin.Context) {
	allChans, err := h.chnlsvc.GetList(c.Request.Context())
	if err != nil {
		c.Error(err)
		c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
		return
	}

	// Pre-allocate with capacity (worst case: all channels)
	avilChans := make([]*channel.ZmuxChannel, 0, len(allChans))

	for _, channel := range allChans {
		// O(1) lookup in map
		if _, ok := h.b2bclntsvc.LookupByChannelID(channel.ID); !ok {
			avilChans = append(avilChans, channel)
		}
	}

	c.JSON(http.StatusOK, avilChans)
}

func (h *B2BClientHandler) GetChannelsSelected(c *gin.Context) {
	idStr := c.Param("id")
	b2bClientID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.Error(err)
		c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
		return
	}

	b2bClient, err := h.b2bclntsvc.GetOne(b2bClientID)
	if err != nil {
		c.Error(err)

		if errors.Is(err, service.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"message": err.Error()})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
		}

		return
	}

	selectedChans, err := h.chnlsvc.GetMany(c.Request.Context(), b2bClient.ChannelIDs)
	if err != nil {
		c.Error(err)
		c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
		return
	}

	c.JSON(http.StatusOK, selectedChans)
}

func (h *B2BClientHandler) GetChannelsAvailableAndSelected(c *gin.Context) {
	idStr := c.Param("id")
	b2bClientID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.Error(err)
		c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
		return
	}

	allChans, err := h.chnlsvc.GetList(c.Request.Context())
	if err != nil {
		c.Error(err)
		c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
		return
	}

	// Pre-allocate with capacity (worst case: all channels)
	chanChoices := make([]*channel.ZmuxChannel, 0, len(allChans))

	for _, channel := range allChans {
		// O(1) lookup in map
		if b2bclnt, ok := h.b2bclntsvc.LookupByChannelID(channel.ID); !ok || b2bClientID == b2bclnt.ID {
			chanChoices = append(chanChoices, channel)
		}
	}

	c.JSON(http.StatusOK, chanChoices)
}

func (h *B2BClientHandler) GetQ(c *gin.Context) {
	idStr := c.Param("id")
	b2bClientID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.Error(err)
		c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
		return
	}

	allChans, err := h.chnlsvc.GetList(c.Request.Context())
	if err != nil {
		c.Error(err)
		c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
		return
	}

	// Pre-allocate with capacity (worst case: all channels)
	chanChoices := make([]*channel.ZmuxChannel, 0, len(allChans))

	for _, channel := range allChans {
		// O(1) lookup in map
		if b2bclnt, ok := h.b2bclntsvc.LookupByChannelID(channel.ID); !ok || b2bClientID == b2bclnt.ID {
			chanChoices = append(chanChoices, channel)
		}
	}

	c.JSON(http.StatusOK, chanChoices)
}
