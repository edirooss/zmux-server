package service

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"sync"

	b2bclient "github.com/edirooss/zmux-server/internal/domain/b2b-client"
	"github.com/edirooss/zmux-server/internal/infrastructure/datastore"
	"github.com/edirooss/zmux-server/internal/infrastructure/objectstore"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

const (
	b2bClientKeyPrefix = "zmux:b2b_client:" // zmux:b2b_client:<id> → JSON(B2BClient)
)

var ErrConflict = errors.New("conflict")

type B2BClientService struct {
	log     *zap.Logger
	chansvc *ChannelService

	mu                       sync.RWMutex
	ds                       *datastore.DataStore            // Redis-based persistent store
	objs                     *objectstore.ObjectStore        // in-memory object store
	byToken                  map[string]*b2bclient.B2BClient // in-memory token-based index of B2BClient(s)
	byChannelID              map[int64]*b2bclient.B2BClient  // in-memory channel-id based index of B2BClient(s)
	enabledChannelsUsageByID map[int64]int64                 // in-memory count of enabled channels per B2B client ID.
}

func NewB2BClientService(ctx context.Context, log *zap.Logger, chansvc *ChannelService, rdb *redis.Client) (*B2BClientService, error) {
	if log == nil {
		log = zap.NewNop()
	}
	log = log.Named("b2b-client-service")

	ds, err := datastore.NewDataStore(ctx, log, rdb, b2bClientKeyPrefix)
	if err != nil {
		return nil, fmt.Errorf("datastore: %w", err)
	}

	s := &B2BClientService{
		log:                      log,
		ds:                       ds,
		objs:                     objectstore.NewObjectStore(log),
		byToken:                  make(map[string]*b2bclient.B2BClient),
		byChannelID:              make(map[int64]*b2bclient.B2BClient),
		enabledChannelsUsageByID: make(map[int64]int64),
	}

	if err := s.reconcile(ctx); err != nil {
		return nil, fmt.Errorf("reconcile: %w", err)
	}

	chansvc.WithDeleteHook(func(chnlID int64) error {
		if b2bclnt, ok := s.LookupByChannelID(chnlID); ok {
			return fmt.Errorf("%w: B2B client '%d' holds channel '%d'", ErrConflict, b2bclnt.ID, chnlID)
		}
		return nil
	})

	chansvc.WithEnableHook(func(chnlID int64) error {
		s.mu.Lock()
		defer s.mu.Unlock()
		b2bclnt, ok := s.byChannelID[chnlID]
		if !ok {
			return nil
		}
		if s.enabledChannelsUsageByID[b2bclnt.ID] >= b2bclnt.EnabledChannelQuota {
			return fmt.Errorf("%w: B2B client '%d' enabled channel quota exceeded", ErrConflict, b2bclnt.ID)
		}
		s.enabledChannelsUsageByID[b2bclnt.ID]++
		return nil
	})

	chansvc.WithDisableHook(func(chnlID int64) {
		s.mu.Lock()
		defer s.mu.Unlock()
		b2bclnt, ok := s.byChannelID[chnlID]
		if ok {
			s.enabledChannelsUsageByID[b2bclnt.ID]--
		}
	})

	return s, nil
}

func (s *B2BClientService) Create(ctx context.Context, c *b2bclient.B2BClient) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, chnlID := range c.ChannelIDs {
		if !s.chansvc.Exists(chnlID) {
			return fmt.Errorf("%w: channel '%d' not found", ErrNotFound, chnlID)
		}

		if b2bclnt, ok := s.byChannelID[chnlID]; ok {
			return fmt.Errorf("%w: B2B client '%d' holds channel '%d'", ErrConflict, b2bclnt.ID, chnlID)
		}
	}

	token, err := generateBearerToken(s.byToken)
	if err != nil {
		return fmt.Errorf("generate token: %w", err)
	}
	c.BearerToken = token

	raw, err := json.Marshal(c)
	if err != nil {
		return fmt.Errorf("json marshal: %w", err)
	}

	b2bclntID, err := s.ds.Create(ctx, raw)
	if err != nil {
		return fmt.Errorf("create: %w", err)
	}
	c.ID = b2bclntID
	s.objs.Upsert(b2bclntID, c)
	s.byToken[c.BearerToken] = c
	for _, chnlID := range c.ChannelIDs {
		s.byChannelID[chnlID] = c
		if ch, _ := s.chansvc.GetOne(chnlID); ch.Enabled {
			s.enabledChannelsUsageByID[b2bclntID]++
		}
	}

	return nil
}

func (s *B2BClientService) GetOne(id int64) (*b2bclient.B2BClient, error) {
	val, ok := s.objs.GetOne(id)
	if !ok {
		return nil, ErrNotFound
	}
	return val.(*b2bclient.B2BClient), nil
}

func (s *B2BClientService) GetMany(ids []int64) ([]*b2bclient.B2BClient, error) {
	vals, _ := s.objs.GetMany(ids)
	if len(vals) == 0 {
		return []*b2bclient.B2BClient{}, nil
	}

	bcs := make([]*b2bclient.B2BClient, 0, len(vals))
	for _, val := range vals {
		bcs = append(bcs, val.(*b2bclient.B2BClient))
	}

	return bcs, nil
}

func (s *B2BClientService) GetList() ([]*b2bclient.B2BClient, error) {
	_, vals := s.objs.GetList()
	if len(vals) == 0 {
		return []*b2bclient.B2BClient{}, nil
	}

	bcs := make([]*b2bclient.B2BClient, 0, len(vals))
	for _, val := range vals {
		bcs = append(bcs, val.(*b2bclient.B2BClient))
	}

	return bcs, nil
}

func (s *B2BClientService) Exists(id int64) bool {
	_, ok := s.objs.GetOne(id)
	return ok
}

func (s *B2BClientService) Update(ctx context.Context, c *b2bclient.B2BClient) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	cur, err := s.GetOne(c.ID)
	if err != nil {
		return fmt.Errorf("get one: %w", err)
	}

	chnlsIDsRemoved, chnlsIDsAdded := symmetricDiff(cur.ChannelIDs, c.ChannelIDs)
	for _, chnlID := range chnlsIDsAdded {
		if !s.chansvc.Exists(chnlID) {
			return fmt.Errorf("%w: channel '%d' not found", ErrNotFound, chnlID)
		}

		if b2bclnt, ok := s.byChannelID[chnlID]; ok && b2bclnt.ID != c.ID {
			return fmt.Errorf("%w: B2B client '%d' holds channel '%d'", ErrConflict, b2bclnt.ID, chnlID)
		}
	}

	c.BearerToken = cur.BearerToken // always overwrite with cur token (immutable; validation avoided for body)
	raw, err := json.Marshal(c)
	if err != nil {
		return fmt.Errorf("json marshal: %w", err)
	}
	if err := s.ds.Update(ctx, c.ID, raw); err != nil {
		return fmt.Errorf("update: %w", err)
	}
	s.objs.Upsert(c.ID, c)
	s.byToken[c.BearerToken] = c

	for _, chnlID := range chnlsIDsAdded {
		s.byChannelID[chnlID] = c
		if ch, _ := s.chansvc.GetOne(chnlID); ch.Enabled {
			s.enabledChannelsUsageByID[c.ID]++
		}
	}

	for _, chnlID := range chnlsIDsRemoved {
		delete(s.byChannelID, chnlID)
		if ch, _ := s.chansvc.GetOne(chnlID); ch.Enabled {
			s.enabledChannelsUsageByID[c.ID]--
		}
	}

	return nil
}

func (s *B2BClientService) Delete(ctx context.Context, b2bclntID int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	b2bclnt, err := s.GetOne(b2bclntID)
	if err != nil {
		return fmt.Errorf("get one: %w", err)
	}

	if err := s.ds.Delete(ctx, b2bclntID); err != nil {
		return fmt.Errorf("delete: %w", err)
	}
	s.objs.Delete(b2bclntID)
	delete(s.byToken, b2bclnt.BearerToken)
	for _, chnlID := range b2bclnt.ChannelIDs {
		delete(s.byChannelID, chnlID)
		if ch, _ := s.chansvc.GetOne(chnlID); ch.Enabled {
			s.enabledChannelsUsageByID[b2bclntID]--
		}
	}

	return nil
}

func (s *B2BClientService) reconcile(ctx context.Context) error {
	b2bclntIDs, csBytes, err := s.ds.GetList(ctx)
	if err != nil {
		return fmt.Errorf("get list: %w", err)
	}

	for i, b2bclntID := range b2bclntIDs {
		c := &b2bclient.B2BClient{}
		if err := json.Unmarshal(csBytes[i], c); err != nil {
			// Data corruption detected - should never happen in normal operation.
			// Possible causes: manual Redis edits, serialization bugs, bit flips.
			s.log.Error("corrupted data detected",
				zap.Int64("id", b2bclntID),
				zap.String("data_preview", safePreview(csBytes[i])),
				zap.Error(err))
			return fmt.Errorf("json unmarshal: %w", err)
		}
		c.ID = b2bclntID
		s.objs.Upsert(b2bclntID, c)
		s.byToken[c.BearerToken] = c
		for _, chnlID := range c.ChannelIDs {
			s.byChannelID[chnlID] = c
			if ch, _ := s.chansvc.GetOne(chnlID); ch.Enabled {
				s.enabledChannelsUsageByID[b2bclntID]++
			}
		}
	}

	return nil
}

func (s *B2BClientService) LookupByToken(token string) (*b2bclient.B2BClient, bool) {
	s.mu.RLock()
	b2bClient, ok := s.byToken[token]
	s.mu.RUnlock()
	if !ok {
		return nil, false
	}
	return b2bClient, true
}

func (s *B2BClientService) LookupByChannelID(chnlID int64) (*b2bclient.B2BClient, bool) {
	s.mu.RLock()
	b2bClient, ok := s.byChannelID[chnlID]
	s.mu.RUnlock()
	if !ok {
		return nil, false
	}
	return b2bClient, true
}

// generateBearerToken creates a cryptographically secure, URL-safe bearer token.
//
// Format example: b2b_pXGgPptQyC4w2UE1-LyYuwXzle7bnBQ1JmBiY1Ev6xI
func generateBearerToken(index map[string]*b2bclient.B2BClient) (string, error) {
	const tokenBytes = 32
	const prefix = "b2b_"
	for {
		b := make([]byte, tokenBytes)
		if _, err := rand.Read(b); err != nil {
			return "", fmt.Errorf("rand read: %w", err)
		}
		raw := base64.RawURLEncoding.EncodeToString(b)
		token := prefix + raw
		if _, ok := index[token]; ok {
			continue
		}
		return token, nil
	}
}

func safePreview(b []byte) string {
	n := 100
	if len(b) < n {
		n = len(b)
	}
	return string(b[:n])
}

// symmetricDiff returns two slices:
// the first contains elements in a but not in b,
// the second contains elements in b but not in a.
func symmetricDiff(a, b []int64) (onlyInA, onlyInB []int64) {
	setB := make(map[int64]struct{}, len(b))
	for _, v := range b {
		setB[v] = struct{}{}
	}

	for _, v := range a {
		if _, found := setB[v]; !found {
			onlyInA = append(onlyInA, v)
		}
	}

	setA := make(map[int64]struct{}, len(a))
	for _, v := range a {
		setA[v] = struct{}{}
	}

	for _, v := range b {
		if _, found := setA[v]; !found {
			onlyInB = append(onlyInB, v)
		}
	}

	return
}
