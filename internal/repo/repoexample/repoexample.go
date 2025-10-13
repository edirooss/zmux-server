package repoexample

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"

	"github.com/edirooss/zmux-server/internal/domain/channel"
	"github.com/edirooss/zmux-server/internal/repo/datastore"
	"github.com/edirooss/zmux-server/internal/repo/objectstore"
	"go.uber.org/zap"
)

var (
	// ErrNotFound means the record ID does not exist in the store.
	ErrNotFound = errors.New("record not found")
)

type ChannelRepository struct {
	log *zap.Logger

	mu   sync.Mutex
	ds   *datastore.DataStore
	objs *objectstore.ObjectStore
}

func NewChannelRepository(ctx context.Context, log *zap.Logger, ds *datastore.DataStore, objs *objectstore.ObjectStore) (*ChannelRepository, error) {
	if ds == nil || objs == nil {
		return nil, errors.New("nil store")
	}
	if log == nil {
		log = zap.NewNop()
	}

	s := &ChannelRepository{
		log:  log,
		ds:   ds,
		objs: objs,
	}

	if err := s.reconcile(ctx); err != nil {
		return nil, fmt.Errorf("reconcile: %w", err)
	}

	return s, nil
}

func (s *ChannelRepository) Create(ctx context.Context, ch *channel.ZmuxChannel) error {
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

	return nil
}

func (s *ChannelRepository) Update(ctx context.Context, ch *channel.ZmuxChannel) error {
	rawCh, err := json.Marshal(ch)
	if err != nil {
		return fmt.Errorf("json marshal: %w", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.ds.Update(ctx, ch.ID, rawCh); err != nil {
		if errors.Is(err, datastore.ErrNotFound) {
			return ErrNotFound
		}
		return fmt.Errorf("update: %w", err)
	}
	s.objs.Upsert(ch.ID, ch)

	return nil
}

func (s *ChannelRepository) Delete(ctx context.Context, id int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.ds.Delete(ctx, id); err != nil {
		return fmt.Errorf("delete: %w", err)
	}
	s.objs.Delete(id)

	return nil
}

func (s *ChannelRepository) GetOne(id int64) (*channel.ZmuxChannel, error) {
	val, ok := s.objs.GetOne(id)
	if !ok {
		return nil, ErrNotFound
	}
	return val.(*channel.ZmuxChannel), nil
}

func (s *ChannelRepository) GetMany(ids []int64) []*channel.ZmuxChannel {
	vals, _ := s.objs.GetMany(ids)
	if len(vals) == 0 {
		return []*channel.ZmuxChannel{}
	}

	chs := make([]*channel.ZmuxChannel, 0, len(vals))
	for _, val := range vals {
		chs = append(chs, val.(*channel.ZmuxChannel))
	}

	return chs
}

func (s *ChannelRepository) GetList() []*channel.ZmuxChannel {
	_, vals := s.objs.GetList()
	if len(vals) == 0 {
		return []*channel.ZmuxChannel{}
	}

	chs := make([]*channel.ZmuxChannel, 0, len(vals))
	for _, val := range vals {
		chs = append(chs, val.(*channel.ZmuxChannel))
	}

	return chs
}

func (s *ChannelRepository) reconcile(ctx context.Context) error {
	ids, chsBytes, err := s.ds.GetList(ctx)
	if err != nil {
		return fmt.Errorf("get list: %w", err)
	}
	for i, id := range ids {
		ch := &channel.ZmuxChannel{}
		if err := json.Unmarshal(chsBytes[i], ch); err != nil {
			s.log.Fatal("bla bla") // should return err instead

		}
		ch.ID = id
		s.objs.Upsert(id, ch)
	}

	return nil
}
