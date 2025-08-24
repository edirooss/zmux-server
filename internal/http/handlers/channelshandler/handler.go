package channelshandler

import (
	"fmt"

	"github.com/edirooss/zmux-server/services"
	"go.uber.org/zap"
)

type ChannelsHandler struct {
	log *zap.Logger
	svc *services.ChannelService
}

func NewChannelsHandler(log *zap.Logger) (*ChannelsHandler, error) {
	// Service for channel CRUD
	channelService, err := services.NewChannelService(log)
	if err != nil {
		return nil, fmt.Errorf("new channel service: %w", err)
	}

	return &ChannelsHandler{
		log: log.Named("channels"),
		svc: channelService,
	}, nil
}
