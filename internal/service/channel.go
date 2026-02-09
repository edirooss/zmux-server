package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	b2bclient "github.com/edirooss/zmux-server/internal/domain/b2b-client"
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
	log     *zap.Logger
	logmngr *processmgr.LogManager

	mu         sync.RWMutex
	b2bclntsvc *B2BClientService
	ds         *datastore.DataStore     // Redis-based persistent store
	objs       *objectstore.ObjectStore // in-memory object store
	procmngr   *processmgr.ProcessManager
}

func NewChannelService(ctx context.Context, log *zap.Logger, rdb *redis.Client, b2bclntsvc *B2BClientService, logmngr *processmgr.LogManager) (*ChannelService, error) {
	if log == nil {
		log = zap.NewNop()
	}
	log = log.Named("channel-service")

	ds, err := datastore.NewDataStore(ctx, log, rdb, channelKeyPrefix)
	if err != nil {
		return nil, fmt.Errorf("datastore: %w", err)
	}

	svc := &ChannelService{
		log:     log,
		logmngr: logmngr,

		b2bclntsvc: b2bclntsvc,
		ds:         ds,
		objs:       objectstore.NewObjectStore(log),
		procmngr:   processmgr.NewProcessManager(log, logmngr),
	}

	if err := svc.reconcile(ctx); err != nil {
		return nil, fmt.Errorf("reconcile: %w", err)
	}

	return svc, nil
}

func (s *ChannelService) Create(ctx context.Context, ch *channel.ZmuxChannel) error {
	rawCh, err := json.Marshal(ch.Model())
	if err != nil {
		return fmt.Errorf("json marshal: %w", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if ch.B2BClientID != nil {
		b2bclntID := *ch.B2BClientID
		b2bclnt, err := s.b2bclntsvc.GetOne(b2bclntID)
		if err != nil {
			return fmt.Errorf("b2b client not found")
		}

		if err := enforceQuotaOnCreate(b2bclnt, ch); err != nil {
			return err
		}
	}
	chID, err := s.ds.Create(ctx, rawCh)
	if err != nil {
		return fmt.Errorf("create: %w", err)
	}
	ch.ID = chID
	s.objs.Upsert(chID, ch)

	//zmux controller comptability start: index input.url → channel.id
	inputURL := ch.Input.URL
	_, err = s.ds.CreateWithIndex(ctx, *inputURL, chID)
	if err != nil {
		return fmt.Errorf("create with index: %w", err)
	}
	//zmux controller comptabilty end

	if ch.B2BClientID != nil {
		s.b2bclntsvc.RegisterChannel(*ch.B2BClientID, ch)
		return nil
	}

	if ch.Enabled {
		s.procmngr.Add(ch.ID, remuxcmd.BuildArgv(ch), time.Duration(ch.RestartSec)*time.Second)
	}
	return nil
}

func (s *ChannelService) Update(ctx context.Context, ch *channel.ZmuxChannel) error {
	rawCh, err := json.Marshal(ch.Model())
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

	if ch.B2BClientID != nil {
		b2bclntID := *ch.B2BClientID
		b2bclnt, err := s.b2bclntsvc.GetOne(b2bclntID)
		if err != nil {
			return fmt.Errorf("b2b client not found")
		}

		if curCh.B2BClientID != nil && *ch.B2BClientID == *curCh.B2BClientID {
			if err := enforceQuotaOnUpdate(b2bclnt, curCh, ch); err != nil {
				return err
			}
		} else {
			if err := enforceQuotaOnCreate(b2bclnt, ch); err != nil {
				return err
			}
		}
	}

	if err := s.ds.Update(ctx, ch.ID, rawCh); err != nil {
		return fmt.Errorf("update: %w", err)
	}

	s.objs.Upsert(ch.ID, ch)

	if curCh.B2BClientID != nil {
		s.b2bclntsvc.UnregisterChannel(*curCh.B2BClientID, curCh)
	} else {
		if curCh.Enabled {
			s.procmngr.Remove(curCh.ID)
		}
	}

	if ch.B2BClientID != nil {
		s.b2bclntsvc.RegisterChannel(*ch.B2BClientID, ch)
		return nil
	}

	if ch.Enabled {
		s.procmngr.Add(ch.ID, remuxcmd.BuildArgv(ch), time.Duration(ch.RestartSec)*time.Second)
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
	if _, ok := s.objs.GetOne(id); !ok {
		return nil, ErrNotFound
	}
	logbuf := s.logmngr.Get(id)
	return logbuf.Read(0), nil
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

func (s *ChannelService) Delete(ctx context.Context, id int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	val, ok := s.objs.GetOne(id)
	if !ok {
		return ErrNotFound
	}
	ch := val.(*channel.ZmuxChannel)

	if err := s.ds.Delete(ctx, id); err != nil {
		return fmt.Errorf("delete: %w", err)
	}
	s.objs.Delete(id)

	if ch.B2BClientID != nil {
		s.b2bclntsvc.UnregisterChannel(*ch.B2BClientID, ch)
	} else {
		if ch.Enabled {
			s.procmngr.Remove(ch.ID)
		}
	}

	return nil
}

func (s *ChannelService) DeleteByInputURL(ctx context.Context, inputURL string) error {
	id, err := s.ds.GetIDByInputURL(ctx, inputURL)
	if err != nil {
		return err
	}
	return s.Delete(ctx, id)
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

		if ch.B2BClientID != nil {
			s.b2bclntsvc.RegisterChannel(*ch.B2BClientID, ch)
			continue
		}

		if ch.Enabled {
			s.procmngr.Add(ch.ID, remuxcmd.BuildArgv(ch), time.Duration(ch.RestartSec)*time.Second)
		}
	}

	return nil
}

func checkEnabledChannelQuota(b2bclnt *b2bclient.B2BClientView, ch *channel.ZmuxChannel) error {
	if !ch.Enabled {
		return nil
	}
	q := b2bclnt.Quotas.EnabledChannels
	if q.Usage+1 > q.Quota {
		return &QuotaExceededError{
			ClientName: b2bclnt.Name,
			ClientID:   b2bclnt.ID,
			Resource:   "enabled channel",
			Usage:      q.Usage + 1,
			Quota:      q.Quota,
		}
	}
	return nil
}

func checkEnabledOutputQuotas(b2bclnt *b2bclient.B2BClientView, ch *channel.ZmuxChannel) error {
	enabledOutputsQuotas := b2bclient.SliceToMapByRef(b2bclnt.Quotas.EnabledOutputs)
	for _, output := range ch.Outputs {
		if !output.Enabled {
			continue
		}
		if q, ok := enabledOutputsQuotas[output.Ref]; ok {
			if q.Usage+1 > q.Quota {
				return &QuotaExceededError{
					ClientName: b2bclnt.Name,
					ClientID:   b2bclnt.ID,
					Resource:   fmt.Sprintf("enabled output '%s'", output.Ref),
					Usage:      q.Usage + 1,
					Quota:      q.Quota,
				}
			}
		}
	}
	return nil
}

func enforceQuotaOnCreate(b2bclnt *b2bclient.B2BClientView, ch *channel.ZmuxChannel) error {
	if err := checkEnabledChannelQuota(b2bclnt, ch); err != nil {
		return err
	}
	if err := checkEnabledOutputQuotas(b2bclnt, ch); err != nil {
		return err
	}
	return nil
}

func enforceQuotaOnUpdate(b2bclnt *b2bclient.B2BClientView, prev, next *channel.ZmuxChannel) error {
	if !prev.Enabled && next.Enabled {
		if err := checkEnabledChannelQuota(b2bclnt, next); err != nil {
			return err
		}
	}

	prevOutputs := prev.OutputsByRef()
	enabledOutputsQuotas := b2bclient.SliceToMapByRef(b2bclnt.Quotas.EnabledOutputs)
	for _, output := range next.Outputs {
		if !output.Enabled {
			continue
		}
		prevOut, ok := prevOutputs[output.Ref]
		if ok && prevOut.Output.Enabled {
			continue
		}
		if q, ok := enabledOutputsQuotas[output.Ref]; ok {
			if q.Usage+1 > q.Quota {
				return &QuotaExceededError{
					ClientName: b2bclnt.Name,
					ClientID:   b2bclnt.ID,
					Resource:   fmt.Sprintf("enabled output '%s'", output.Ref),
					Usage:      q.Usage + 1,
					Quota:      q.Quota,
				}
			}
		}
	}
	return nil
}

var ErrQuotaExceeded = errors.New("quota exceeded")

// QuotaExceededError details a specific quota violation.
type QuotaExceededError struct {
	ClientName string
	ClientID   int64
	Resource   string // e.g., "enabled channel", "enabled output 'ref'"
	Usage      int64
	Quota      int64
}

// Error implements the error interface.
func (e *QuotaExceededError) Error() string {
	return fmt.Sprintf("B2B client (id='%d', name='%s') %s quota exceeded (%d/%d)",
		e.ClientID, e.ClientName, e.Resource, e.Usage, e.Quota)
}

// Unwrap returns the base ErrQuotaExceeded sentinel error for errors.Is() checks.
func (e *QuotaExceededError) Unwrap() error {
	return ErrQuotaExceeded
}
