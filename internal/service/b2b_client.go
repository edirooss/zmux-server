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
	"github.com/edirooss/zmux-server/internal/domain/channel"
	"github.com/edirooss/zmux-server/internal/infrastructure/datastore"
	"github.com/edirooss/zmux-server/internal/infrastructure/objectstore"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

const (
	b2bClientKeyPrefix = "zmux:b2b_client:" // zmux:b2b_client:<id> → JSON(B2BClientModel)
)

var ErrConflict = errors.New("conflict")

type B2BClientService struct {
	log     *zap.Logger
	chansvc *ChannelService

	mu          sync.RWMutex
	ds          *datastore.DataStore            // Redis-based persistent store
	objs        *objectstore.ObjectStore        // in-memory object store of B2BClient domain objects
	byToken     map[string]*b2bclient.B2BClient // in-memory token-based index of B2BClient domain objects
	byChannelID map[int64]*b2bclient.B2BClient  // in-memory channel-id based index of B2BClient domain objects
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
		log:         log,
		ds:          ds,
		objs:        objectstore.NewObjectStore(log),
		byToken:     make(map[string]*b2bclient.B2BClient),
		byChannelID: make(map[int64]*b2bclient.B2BClient),
		chansvc:     chansvc,
	}

	if err := s.reconcile(ctx); err != nil {
		return nil, fmt.Errorf("reconcile: %w", err)
	}

	chansvc.WithDeleteHook(func(chnlID int64) error {
		if b2bclnt, ok := s.LookupByChannelID(chnlID); ok {
			return fmt.Errorf("%w: B2B client '%s' holds channel '%d'", ErrConflict, b2bclnt.Name, chnlID)
		}
		return nil
	})

	chansvc.WithUpdateHook(func(chnlID int64, prev *channel.ZmuxChannel, next *channel.ZmuxChannel) error {
		s.mu.Lock()
		defer s.mu.Unlock()

		if b2bclnt, ok := s.byChannelID[chnlID]; ok {
			rollback := false

			// Snapshot originals once
			origChanUsage := b2bclnt.Quotas.EnabledChannels.Usage
			origOutputs := map[string]struct {
				Quota int64
				Usage int64
			}{}

			// Single rollback
			defer func() {
				if rollback {
					b2bclnt.Quotas.EnabledChannels.Usage = origChanUsage
					for ref, rec := range origOutputs {
						b2bclnt.Quotas.EnabledOutputs[ref] = rec
					}
				}
			}()

			// Helper: ensure we snapshot an output once before first mutation
			snapshot := func(ref string) {
				if _, seen := origOutputs[ref]; !seen {
					if q, ok := b2bclnt.Quotas.EnabledOutputs[ref]; ok {
						origOutputs[ref] = q
					}
				}
			}

			// Remove prev state
			if prev.Enabled {
				b2bclnt.Quotas.EnabledChannels.Usage--
			}
			for _, output := range prev.Outputs {
				if q, ok := b2bclnt.Quotas.EnabledOutputs[output.Ref]; ok && output.Enabled {
					snapshot(output.Ref)
					b2bclnt.Quotas.EnabledOutputs[output.Ref] = struct {
						Quota int64
						Usage int64
					}{
						q.Quota,
						q.Usage - 1,
					}
				}
			}

			// Check quotas for next
			if next.Enabled {
				if b2bclnt.Quotas.EnabledChannels.Usage >= b2bclnt.Quotas.EnabledChannels.Quota {
					rollback = true
					return fmt.Errorf("%w: B2B client '%d' enabled channel quota exceeded", ErrConflict, b2bclnt.ID)
				}
			}
			for _, output := range next.Outputs {
				if q, ok := b2bclnt.Quotas.EnabledOutputs[output.Ref]; ok && output.Enabled {
					if q.Usage >= q.Quota {
						rollback = true
						return fmt.Errorf("%w: B2B client '%d' enabled output '%s' quota exceeded", ErrConflict, b2bclnt.ID, output.Ref)
					}
				}
			}

			// Apply next state
			if next.Enabled {
				b2bclnt.Quotas.EnabledChannels.Usage++
			}
			for _, output := range next.Outputs {
				if q, ok := b2bclnt.Quotas.EnabledOutputs[output.Ref]; ok && output.Enabled {
					snapshot(output.Ref)
					b2bclnt.Quotas.EnabledOutputs[output.Ref] = struct {
						Quota int64
						Usage int64
					}{
						q.Quota,
						q.Usage + 1,
					}
				}
			}
		}

		return nil
	})

	return s, nil
}

func (s *B2BClientService) Create(ctx context.Context, r *b2bclient.B2BClientResource) (*b2bclient.B2BClientView, error) {
	b2bclnt := b2bclient.NewB2BClient(r)

	s.mu.Lock()
	defer s.mu.Unlock()

	for _, chnlID := range b2bclnt.ChannelIDs {
		if !s.chansvc.Exists(chnlID) {
			return nil, fmt.Errorf("%w: channel '%d' not found", ErrNotFound, chnlID)
		}
		if b2bclnt2, ok := s.byChannelID[chnlID]; ok {
			return nil, fmt.Errorf("%w: B2B client '%d' holds channel '%d'", ErrConflict, b2bclnt2.ID, chnlID)
		}
	}

	token, err := generateBearerToken(s.byToken)
	if err != nil {
		return nil, fmt.Errorf("generate token: %w", err)
	}
	b2bclnt.BearerToken = token

	raw, err := json.Marshal(b2bclnt.Model())
	if err != nil {
		return nil, fmt.Errorf("json marshal: %w", err)
	}

	b2bclntID, err := s.ds.Create(ctx, raw)
	if err != nil {
		return nil, fmt.Errorf("create: %w", err)
	}

	b2bclnt.ID = b2bclntID
	s.objs.Upsert(b2bclntID, b2bclnt)
	s.byToken[b2bclnt.BearerToken] = b2bclnt

	for _, chnlID := range b2bclnt.ChannelIDs {
		s.byChannelID[chnlID] = b2bclnt
		ch, _ := s.chansvc.GetOne(chnlID)
		if ch.Enabled {
			b2bclnt.Quotas.EnabledChannels.Usage++
		}
		for _, output := range ch.Outputs {
			if q, ok := b2bclnt.Quotas.EnabledOutputs[output.Ref]; ok && output.Enabled {
				b2bclnt.Quotas.EnabledOutputs[output.Ref] = struct {
					Quota int64
					Usage int64
				}{
					q.Quota,
					q.Usage + 1,
				}
			}
		}
	}

	return b2bclnt.View(), nil
}

func (s *B2BClientService) GetOne(id int64) (*b2bclient.B2BClientView, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	val, ok := s.objs.GetOne(id)
	if !ok {
		return nil, ErrNotFound
	}
	return val.(*b2bclient.B2BClient).View(), nil
}

func (s *B2BClientService) GetMany(ids []int64) ([]*b2bclient.B2BClientView, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	vals, _ := s.objs.GetMany(ids)
	if len(vals) == 0 {
		return []*b2bclient.B2BClientView{}, nil
	}

	views := make([]*b2bclient.B2BClientView, 0, len(vals))
	for _, val := range vals {
		views = append(views, val.(*b2bclient.B2BClient).View())
	}
	return views, nil
}

func (s *B2BClientService) GetList() ([]*b2bclient.B2BClientView, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	_, vals := s.objs.GetList()
	if len(vals) == 0 {
		return []*b2bclient.B2BClientView{}, nil
	}

	views := make([]*b2bclient.B2BClientView, 0, len(vals))
	for _, val := range vals {
		views = append(views, val.(*b2bclient.B2BClient).View())
	}
	return views, nil
}

func (s *B2BClientService) Exists(id int64) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	_, ok := s.objs.GetOne(id)
	return ok
}

func (s *B2BClientService) Update(ctx context.Context, id int64, r *b2bclient.B2BClientResource) (*b2bclient.B2BClientView, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	val, ok := s.objs.GetOne(id)
	if !ok {
		return nil, ErrNotFound
	}
	b2bclnt := val.(*b2bclient.B2BClient)

	chnlsIDsRemoved, chnlsIDsAdded := symmetricDiff(b2bclnt.ChannelIDs, r.ChannelIDs)

	for _, chnlID := range chnlsIDsAdded {
		if !s.chansvc.Exists(chnlID) {
			return nil, fmt.Errorf("%w: channel '%d' not found", ErrNotFound, chnlID)
		}
		if b2bclnt2, ok := s.byChannelID[chnlID]; ok && b2bclnt2.ID != id {
			return nil, fmt.Errorf("%w: B2B client '%d' holds channel '%d'", ErrConflict, b2bclnt2.ID, chnlID)
		}
	}

	next := b2bclient.NewB2BClient(r)
	next.BearerToken = b2bclnt.BearerToken
	raw, err := json.Marshal(next.Model())
	if err != nil {
		return nil, fmt.Errorf("json marshal: %w", err)
	}
	if err := s.ds.Update(ctx, id, raw); err != nil {
		return nil, fmt.Errorf("update: %w", err)
	}
	b2bclnt.Update(r)

	// remove prev from map
	for _, chnlID := range chnlsIDsRemoved {
		delete(s.byChannelID, chnlID)
	}

	// Recompute enabled channel usage from channel service.
	for _, chnlID := range b2bclnt.ChannelIDs {
		s.byChannelID[chnlID] = b2bclnt
		ch, _ := s.chansvc.GetOne(chnlID)
		if ch.Enabled {
			b2bclnt.Quotas.EnabledChannels.Usage++
		}
		for _, output := range ch.Outputs {
			if q, ok := b2bclnt.Quotas.EnabledOutputs[output.Ref]; ok && output.Enabled {
				b2bclnt.Quotas.EnabledOutputs[output.Ref] = struct {
					Quota int64
					Usage int64
				}{
					q.Quota,
					q.Usage + 1,
				}
			}
		}
	}

	return b2bclnt.View(), nil
}

// Delete removes a B2B client by ID and updates in-memory indices.
func (s *B2BClientService) Delete(ctx context.Context, b2bclntID int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	val, ok := s.objs.GetOne(b2bclntID)
	if !ok {
		return ErrNotFound
	}
	b2bclnt := val.(*b2bclient.B2BClient)

	if err := s.ds.Delete(ctx, b2bclntID); err != nil {
		return fmt.Errorf("delete: %w", err)
	}
	s.objs.Delete(b2bclntID)
	delete(s.byToken, b2bclnt.BearerToken)

	for _, chnlID := range b2bclnt.ChannelIDs {
		delete(s.byChannelID, chnlID)
	}

	return nil
}

// reconcile loads persisted records into domain objects and rebuilds in-memory indices.
//
//   - DB (Persistence Layer) → Domain (in-memory)
func (s *B2BClientService) reconcile(ctx context.Context) error {
	b2bclntIDs, csBytes, err := s.ds.GetList(ctx)
	if err != nil {
		return fmt.Errorf("get list: %w", err)
	}

	for i, b2bclntID := range b2bclntIDs {
		var m b2bclient.B2BClientModel
		if err := json.Unmarshal(csBytes[i], &m); err != nil {
			// Data corruption detected - should never happen in normal operation.
			// Possible causes: manual Redis edits, serialization bugs, bit flips.
			s.log.Error("corrupted data detected",
				zap.Int64("id", b2bclntID),
				zap.String("data_preview", safePreview(csBytes[i])),
				zap.Error(err))
			return fmt.Errorf("json unmarshal: %w", err)
		}

		c := b2bclient.LoadB2BClient(&m)
		c.ID = b2bclntID
		s.objs.Upsert(b2bclntID, c)
		s.byToken[c.BearerToken] = c

		// Recompute enabled channel usage from channel service.
		for _, chnlID := range c.ChannelIDs {
			s.byChannelID[chnlID] = c
			ch, _ := s.chansvc.GetOne(chnlID)
			if ch.Enabled {
				c.Quotas.EnabledChannels.Usage++
			}
			for _, output := range ch.Outputs {
				if q, ok := c.Quotas.EnabledOutputs[output.Ref]; ok && output.Enabled {
					c.Quotas.EnabledOutputs[output.Ref] = struct {
						Quota int64
						Usage int64
					}{
						q.Quota,
						q.Usage + 1,
					}
				}
			}
		}
	}

	return nil
}

// LookupByToken returns a domain object by bearer token from the in-memory index.
func (s *B2BClientService) LookupByToken(token string) (*b2bclient.B2BClient, bool) {
	s.mu.RLock()
	b2bClient, ok := s.byToken[token]
	s.mu.RUnlock()
	if !ok {
		return nil, false
	}
	return b2bClient, true
}

// LookupByChannelID returns a domain object by channel ID from the in-memory index.
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
