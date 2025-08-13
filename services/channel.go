package services

import (
	"context"
	"fmt"
	"strconv"

	models "github.com/edirooss/zmux-server/pkg/models/channel"
	"github.com/edirooss/zmux-server/redis"
	"go.uber.org/zap"
)

// ChannelService handles business logic for channels
type ChannelService struct {
	repo    *redis.ChannelRepository
	systemd *SystemdService
}

// NewChannelService creates a new channel service
func NewChannelService(log *zap.Logger) (*ChannelService, error) {
	systemd, err := NewSystemdService(log)
	if err != nil {
		return nil, fmt.Errorf("new systemd service: %w", err)
	}
	return &ChannelService{
		repo:    redis.NewChannelRepository(log),
		systemd: systemd,
	}, nil
}

// CreateChannel creates a new channel
func (s *ChannelService) CreateChannel(ctx context.Context, channelReq *models.CreateZmuxChannelReq) (*models.ZmuxChannel, error) {
	id, err := s.repo.GenerateID(ctx)
	if err != nil {
		return nil, err
	}
	ch := channelReq.ToChannel(id)

	if err := s.repo.Set(ctx, ch); err != nil {
		return nil, fmt.Errorf("set: %w", err)
	}
	if err := s.commitSystemdService(ch); err != nil {
		// What if we fail here? should we delete the channel from Redis?
		return nil, fmt.Errorf("commit systemd service: %w", err)
	}

	if ch.Enabled {
		if err := s.enableChannel(ch.ID); err != nil {
			// What if we fail here? should we delete the channel from Redis?
			return nil, fmt.Errorf("enable channel: %w", err)
		}
	}
	return ch, nil
}

func (s *ChannelService) GetChannel(ctx context.Context, id int64) (*models.ZmuxChannel, error) {
	ch, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get: %w", err)
	}
	return ch, nil
}

func (s *ChannelService) ListChannels(ctx context.Context) ([]*models.ZmuxChannel, error) {
	chs, err := s.repo.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("list: %w", err)
	}
	return chs, nil
}

func (s *ChannelService) UpdateChannel(ctx context.Context, id int64, req *models.UpdateZmuxChannelReq) (*models.ZmuxChannel, error) {
	// Load current
	ch, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get: %w", err)
	}

	// Copy prev enabled
	prevEnabled := ch.Enabled

	// Apply patch
	req.ApplyTo(ch)

	// Persist
	if err := s.repo.Set(ctx, ch); err != nil {
		return nil, fmt.Errorf("set: %w", err)
	}

	// (Re)apply systemd (idempotent-ish simple approach)
	if err := s.commitSystemdService(ch); err != nil {
		// What if we fail here? should we return the channel in Redis to it's prev settings?
		return nil, fmt.Errorf("commit systemd service: %w", err)
	}

	// Re-enableing if channel is enabled (if prev enabled, need to be restart anyway)
	// If prev enabled and now disabled, disable the channel
	if ch.Enabled {
		if err := s.enableChannel(ch.ID); err != nil {
			// What if we fail here? should we return the channel in Redis to it's prev settings?
			return nil, fmt.Errorf("enable channel: %w", err)
		}
	} else if prevEnabled {
		if err := s.disableChannel(ch.ID); err != nil {
			// What if we fail here? should we return the channel in Redis to it's prev settings?
			return nil, fmt.Errorf("disable channel: %w", err)
		}
	}

	return ch, nil
}

// DeleteChannel deletes a single channel and disables its systemd unit.
func (s *ChannelService) DeleteChannel(ctx context.Context, id int64) error {
	// Load to confirm existence and to build unit name
	ch, err := s.repo.Get(ctx, id)
	if err != nil {
		return fmt.Errorf("get: %w", err)
	}

	if ch.Enabled {
		if err := s.disableChannel(ch.ID); err != nil {
			return fmt.Errorf("disable channel: %w", err)
		}
	}

	// Note: Systemd unit file for the channel is still configured on the system.
	// We don't have to delete it and it's one less operation to be errored on so we keep it.

	// Finally delete from repo
	if err := s.repo.Delete(ctx, id); err != nil {
		// What if we fail here? should we re-enable the channel if enabled?
		return fmt.Errorf("delete: %w", err)
	}
	return nil
}

// EnableChannel enables a channel
func (s *ChannelService) EnableChannel(ctx context.Context, id int64) error {
	// Check if exists
	ch, err := s.GetChannel(ctx, id)
	if err != nil {
		return fmt.Errorf("get channel: %w", err)
	}

	// Force enable/ re-enable
	if err := s.enableChannel(ch.ID); err != nil {
		// What if we fail here? should we return the channel in Redis to it's prev settings?
		return fmt.Errorf("enable channel: %w", err)
	}
	return nil
}

// DisableChannel disables a channel
func (s *ChannelService) DisableChannel(ctx context.Context, id int64) error {
	// Check if exists
	ch, err := s.GetChannel(ctx, id)
	if err != nil {
		return fmt.Errorf("get channel: %w", err)
	}

	// Force disable/ re-disable
	if err := s.disableChannel(ch.ID); err != nil {
		return fmt.Errorf("disable channel: %w", err)
	}
	return nil
}

// enableChannel enables a channel
func (s *ChannelService) enableChannel(channelID int64) error {
	serviceName := fmt.Sprintf("zmux-channel-%d", channelID)
	if err := s.systemd.EnableService(serviceName); err != nil {
		return fmt.Errorf("enable systemd service: %w", err)
	}
	return nil
}

// disableChannel disables a channel
func (s *ChannelService) disableChannel(channelID int64) error {
	serviceName := fmt.Sprintf("zmux-channel-%d", channelID)
	if err := s.systemd.DisableService(serviceName); err != nil {
		return fmt.Errorf("disable systemd service: %w", err)
	}
	return nil
}

func (s *ChannelService) commitSystemdService(channel *models.ZmuxChannel) error {
	cfg := SystemdServiceConfig{
		ServiceName: fmt.Sprintf("zmux-channel-%d", channel.ID),
		ExecStart:   BuildRemuxExecStart(channel),
		RestartSec:  strconv.FormatUint(uint64(channel.RestartSec), 10),
	}

	if err := s.systemd.CommitService(cfg); err != nil {
		return fmt.Errorf("commit systemd service: %w", err)
	}

	return nil
}
