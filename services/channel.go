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
		return nil, fmt.Errorf("save: %w", err)
	}
	if err := s.commitSystemdService(ch); err != nil {
		// Best effort
		return nil, fmt.Errorf("commit systemd service: %w", err)
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
	chs, err := s.repo.ListFast(ctx)
	if err != nil {
		return nil, fmt.Errorf("list: %w", err)
	}
	return chs, nil
}

func (s *ChannelService) UpdateChannel(ctx context.Context, id int64, req *models.UpdateZmuxChannelReq) (*models.ZmuxChannel, error) {
	// 1) Load current
	ch, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get: %w", err)
	}

	// 2) Apply patch
	req.ApplyTo(ch)

	// 3) Persist
	if err := s.repo.Set(ctx, ch); err != nil {
		return nil, fmt.Errorf("save: %w", err)
	}

	// 4) (Re)apply systemd (idempotent-ish simple approach)
	if err := s.commitSystemdService(ch); err != nil {
		// Best effort
		return nil, fmt.Errorf("commit systemd service: %w", err)
	}

	return ch, nil
}

// DeleteChannel deletes a single channel and disables its systemd unit.
// Order: disable unit first (so we don't orphan a running service), then delete from repo.
func (s *ChannelService) DeleteChannel(ctx context.Context, id int64) error {
	// Load to confirm existence and to build unit name
	ch, err := s.repo.Get(ctx, id)
	if err != nil {
		// propagate typed not-found (e.g., redis.ErrChannelNotFound) for HTTP layer to map to 404
		return fmt.Errorf("get: %w", err)
	}

	unitName := fmt.Sprintf("zmux-channel-%d", ch.ID)

	// Disable the unit; if this fails, return error and keep record so user can retry
	if err := s.systemd.DisableService(unitName); err != nil {
		return fmt.Errorf("disable systemd service: %w", err)
	}

	// Finally delete from repo
	if err := s.repo.Delete(ctx, id); err != nil {
		// Best effort
		return fmt.Errorf("delete: %w", err)
	}
	return nil
}

func (s *ChannelService) commitSystemdService(channel *models.ZmuxChannel) error {
	cfg := SystemdServiceConfig{
		ServiceName: fmt.Sprintf("zmux-channel-%d", channel.ID),
		ExecStart:   BuildRemuxExecStart(channel),
		RestartSec:  strconv.FormatUint(uint64(channel.RestartSec), 10),
	}

	if err := s.systemd.CreateService(cfg); err != nil {
		return fmt.Errorf("create systemd service: %w", err)
	}

	if channel.Enabled {
		if err := s.systemd.EnableService(cfg.ServiceName); err != nil {
			// TODO: Rallback
			return fmt.Errorf("enable systemd service: %w", err)
		}
	} else {
		if err := s.systemd.DisableService(cfg.ServiceName); err != nil {
			// TODO: Rallback
			return fmt.Errorf("disable systemd service: %w", err)
		}
	}

	return nil
}
