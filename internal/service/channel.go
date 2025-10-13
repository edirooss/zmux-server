package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/edirooss/zmux-server/internal/domain/channel"
	"github.com/edirooss/zmux-server/internal/infrastructure/datastore"
	"github.com/edirooss/zmux-server/internal/infrastructure/objectstore"
	"github.com/edirooss/zmux-server/internal/infrastructure/processmgr"
	"github.com/edirooss/zmux-server/pkg/remuxcmd"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

var (
	ErrNotFound = errors.New("not found")
)

const (
	channelKeyPrefix = "zmux:channel:" // → channel document; JSON(ZmuxChannel)
)

type ChannelService struct {
	log *zap.Logger

	mu       sync.RWMutex
	ds       *datastore.DataStore     // Redis-based persistent store
	objs     *objectstore.ObjectStore // in-memory object store
	procmngr *processmgr.ProcessManager

	deleteChannelHook  func(channelID int64) error
	enableChannelHook  func(channelID int64) error
	disableChannelHook func(channelID int64)
}

func NewChannelService(ctx context.Context, log *zap.Logger, rdb *redis.Client) (*ChannelService, error) {
	if log == nil {
		log = zap.NewNop()
	}
	log = log.Named("channel-service")

	ds, err := datastore.NewDataStore(ctx, log, rdb, channelKeyPrefix)
	if err != nil {
		return nil, fmt.Errorf("datastore: %w", err)
	}

	svc := &ChannelService{
		log:      log,
		ds:       ds,
		objs:     objectstore.NewObjectStore(log),
		procmngr: processmgr.NewProcessManager(log),
	}

	if err := svc.reconcile(ctx); err != nil {
		return nil, fmt.Errorf("reconcile: %w", err)
	}

	return svc, nil
}

func (s *ChannelService) Create(ctx context.Context, ch *channel.ZmuxChannel) error {
	rawCh, err := json.Marshal(ch)
	if err != nil {
		return fmt.Errorf("json marshal: %w", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	chID, err := s.ds.Create(ctx, rawCh)
	if err != nil {
		return fmt.Errorf("create: %w", err)
	}
	ch.ID = chID
	s.objs.Upsert(chID, ch)

	if ch.Enabled {
		s.procmngr.Start(ch.ID, remuxcmd.BuildArgv(ch), time.Duration(ch.RestartSec)*time.Second)
	}
	return nil
}

func (s *ChannelService) GetOne(id int64) (*channel.ZmuxChannel, error) {
	val, ok := s.objs.GetOne(id)
	if !ok {
		return nil, ErrNotFound
	}
	return val.(*channel.ZmuxChannel), nil
}

func (s *ChannelService) GetLogs(ctx context.Context, id int64) ([]string, error) {
	logs, ok := s.procmngr.GetLogs(id, 500)
	if !ok {
		return nil, ErrNotFound
	}
	return logs, nil
}

func (s *ChannelService) GetList(ctx context.Context) ([]*channel.ZmuxChannel, error) {
	_, vals := s.objs.GetList()
	if len(vals) == 0 {
		return []*channel.ZmuxChannel{}, nil
	}

	chs := make([]*channel.ZmuxChannel, 0, len(vals))
	for _, val := range vals {
		chs = append(chs, val.(*channel.ZmuxChannel))
	}

	return chs, nil
}

func (s *ChannelService) Exists(id int64) bool {
	_, ok := s.objs.GetOne(id)
	return ok
}

func (s *ChannelService) GetMany(ctx context.Context, ids []int64) ([]*channel.ZmuxChannel, error) {
	vals, _ := s.objs.GetMany(ids)
	if len(vals) == 0 {
		return []*channel.ZmuxChannel{}, nil
	}

	chs := make([]*channel.ZmuxChannel, 0, len(vals))
	for _, val := range vals {
		chs = append(chs, val.(*channel.ZmuxChannel))
	}

	return chs, nil
}

func (s *ChannelService) Update(ctx context.Context, ch *channel.ZmuxChannel) error {
	rawCh, err := json.Marshal(ch)
	if err != nil {
		return fmt.Errorf("json marshal: %w", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	curVal, ok := s.objs.GetOne(ch.ID)
	if !ok {
		return ErrNotFound
	}
	curCh := curVal.(*channel.ZmuxChannel)

	if !curCh.Enabled && ch.Enabled && s.enableChannelHook != nil {
		if err := s.enableChannelHook(ch.ID); err != nil {
			return fmt.Errorf("enable channel hook: %w", err)
		}
	}

	if err := s.ds.Update(ctx, ch.ID, rawCh); err != nil {
		if !curCh.Enabled && ch.Enabled && s.disableChannelHook != nil {
			s.disableChannelHook(ch.ID)
		}
		return fmt.Errorf("update: %w", err)
	}
	s.objs.Upsert(ch.ID, ch)
	if curCh.Enabled && !ch.Enabled && s.disableChannelHook != nil {
		s.disableChannelHook(ch.ID)
	}

	if curCh.Enabled {
		s.procmngr.Stop(ch.ID)
	}
	if ch.Enabled {
		s.procmngr.Start(ch.ID, remuxcmd.BuildArgv(ch), time.Duration(ch.RestartSec)*time.Second)
	}

	return nil
}

func (s *ChannelService) Delete(ctx context.Context, id int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	val, ok := s.objs.GetOne(id)
	if !ok {
		return ErrNotFound
	}
	ch := val.(*channel.ZmuxChannel)

	if s.deleteChannelHook != nil {
		if err := s.deleteChannelHook(id); err != nil {
			return fmt.Errorf("delete hook: %w", err)
		}
	}

	wasEnabled := ch.Enabled
	if err := s.ds.Delete(ctx, id); err != nil {
		return fmt.Errorf("delete: %w", err)
	}
	s.objs.Delete(id)

	if wasEnabled {
		s.procmngr.Stop(ch.ID)
	}

	return nil
}

func (s *ChannelService) reconcile(ctx context.Context) error {
	ids, chsBytes, err := s.ds.GetList(ctx)
	if err != nil {
		return fmt.Errorf("get list: %w", err)
	}

	for i, id := range ids {
		ch := &channel.ZmuxChannel{}
		if err := json.Unmarshal(chsBytes[i], ch); err != nil {
			// Data corruption detected - should never happen in normal operation.
			// Possible causes: manual Redis edits, serialization bugs, bit flips.
			s.log.Error("corrupted chan data detected",
				zap.Int64("id", id),
				zap.String("data_preview", safePreview(chsBytes[i])),
				zap.Error(err))
			return fmt.Errorf("json unmarshal: %w", err)
		}
		ch.ID = id
		s.objs.Upsert(id, ch)
		if ch.Enabled {
			s.procmngr.Start(ch.ID, remuxcmd.BuildArgv(ch), time.Duration(ch.RestartSec)*time.Second)
		}
	}

	return nil
}

func (s *ChannelService) WithDeleteHook(fn func(channelID int64) error) { s.deleteChannelHook = fn }
func (s *ChannelService) WithEnableHook(fn func(channelID int64) error) { s.enableChannelHook = fn }
func (s *ChannelService) WithDisableHook(fn func(channelID int64))      { s.disableChannelHook = fn }
