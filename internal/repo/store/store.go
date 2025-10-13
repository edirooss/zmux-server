package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

var (
	// ErrNotFound means the string record ID does not exist in the store.
	ErrNotFound = errors.New("record not found")
)

// StringRecord is the durable document stored in Redis.
type StringRecord struct {
	ID    int64  `json:"id"`
	Value string `json:"value"`
}

// StringStore maintains string records in an ordered slice (by monotonic ID)
// with O(1) access via an ID→pointer map and ID→position map.
//
// Space Complexity:
//   - O(n) overall:
//   - Slice of pointers (insertion-ordered list)
//   - byID map (ID → pointer)
//   - position map (ID → index)
//
// Deployment & Operational Model:
//   - Single-process, single-node deployment.
//   - Redis is assumed to be running on localhost.
//   - Design assumes **exclusive process ownership**.
//
// Concurrency Model:
//   - Thread-safe for concurrent use by multiple goroutines within a single process.
//   - Writes are serialized by a write mutex that also encompasses Redis I/O ordering.
//   - Readers use an RWMutex for in-memory access and remain unblocked during Redis I/O.
//   - In-memory mutations are applied under an exclusive state lock after successful persistence.
//
// Reads (GetOne, GetMany, GetList):
//   - Use a read lock (RWMutex RLock) to access the in-memory state.
//   - Return value copies.
//
// Writes (Create, Update, Delete):
//   - A write mutex serializes the entire write path to enforce a total order.
//   - Redis I/O is performed without holding the in-memory state lock, allowing readers to proceed.
//   - After successful persistence, a short critical section under the state lock applies the in-memory mutation.
//   - Guarantees read-after-write visibility for the caller upon return.
//
// Consistency Model:
//   - Redis is the **source of truth** (durable documents keyed by ID).
//   - RAM holds a **materialized, read-optimized state**.
//   - The write path persists to Redis and, upon success, mutates in-memory atomically under the state lock.
//   - Readers never observe partial in-memory mutations.
//
// Write Path:
//  1. Serialize the write with writeMu.
//  2. Persist the document change to Redis (outside state lock).
//  3. On success, briefly lock the state and apply the in-memory mutation.
//  4. Return to the caller.
//
// Read Path:
//   - All reads are served from the in-memory state under an RWMutex read lock.
//   - Redis is bypassed entirely during reads.
//
// Namespace & Multi-Tenancy:
//   - Each instance of StringStore uses a unique `keyPrefix`.
//   - The prefix is **exclusive to the owning process**—no other writers must operate under it.
//   - This is a **hard operational rule/assumption** (not enforced programmatically for simplicity).
//   - Multiple stores may coexist via namespacing.
//
// ID Allocation:
//   - IDs are allocated using Redis INCR on key: `<keyPrefix>id_seq`.
//   - The allocation model is:
//   - **Monotonic**
//   - **Write-once, never recycled**
//   - **Gap-tolerant**: IDs are consumed even if subsequent persistence fails.
//   - Behaves as an **append-only, non-contiguous sequence generator**.
//
// Design Summary:
//   - Combines Redis durability with fast in-memory access.
//   - Provides strong consistency, read concurrency, and efficient reads.
//   - Designed and suitable for single-node deployments with local Redis.
type StringStore struct {
	log       *zap.Logger
	rdb       *redis.Client // Redis used as persistent storage (system of record); documents-only
	keyPrefix string        // Redis key prefix; e.g. <store>:  → JSON(StringRecord) under <prefix><id>

	writeMu sync.Mutex   // serializes write operations (including Redis I/O ordering)
	stateRW sync.RWMutex // protects in-memory state during reads/writes

	byID map[int64]*StringRecord // id -> record
	pos  map[int64]int           // id -> index into ordered list
	list []*StringRecord         // ordered list; sorted by id
}

// NewStringStore constructs a ready-to-use StringStore.
// On initialization, reconciles any existing Redis state under the given keyPrefix
// into the in-memory state. This is a read-only operation against Redis.
func NewStringStore(ctx context.Context, log *zap.Logger, rdb *redis.Client, keyPrefix string) (*StringStore, error) {
	if rdb == nil {
		return nil, errors.New("nil redis client")
	}
	if keyPrefix == "" {
		return nil, fmt.Errorf("invalid keyPrefix: must be non-empty")
	}
	if !strings.HasSuffix(keyPrefix, ":") {
		keyPrefix = keyPrefix + ":"
	}
	if log == nil {
		log = zap.NewNop()
	}

	s := &StringStore{
		rdb:       rdb,
		keyPrefix: keyPrefix,
		log:       log,
		byID:      make(map[int64]*StringRecord),
		pos:       make(map[int64]int),
		list:      make([]*StringRecord, 0),
	}

	if err := s.reconcile(ctx); err != nil {
		return nil, fmt.Errorf("reconcile: %w", err)
	}
	return s, nil
}

// Create inserts a new string record, assigns a unique increasing ID via Redis INCR,
// and appends it to the ordered list (maintaining ascending ID order). Returns a value copy.
//
// Time: O(1) avg (append + two map writes) plus network.
func (s *StringStore) Create(ctx context.Context, value string) (StringRecord, error) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	id, err := s.rdb.Incr(ctx, sequenceKey(s.keyPrefix)).Result()
	if err != nil {
		return StringRecord{}, fmt.Errorf("generate id via INCR: %w", err)
	}
	rec := &StringRecord{ID: id, Value: value}

	if err := s.persistRecord(ctx, rec); err != nil {
		return StringRecord{}, fmt.Errorf("persist: %w", err)
	}

	s.stateRW.Lock()
	idx := len(s.list)
	s.list = append(s.list, rec)
	s.byID[id] = rec
	s.pos[id] = idx
	s.stateRW.Unlock()

	return *rec, nil
}

// Update overwrites the stored string record with the provided value by the record's id.
// Returns the stored value copy.
//
// Time: O(1) (single map lookup + struct assignment) plus network.
func (s *StringStore) Update(ctx context.Context, id int64, value string) (StringRecord, error) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	s.stateRW.RLock()
	existing, ok := s.byID[id]
	s.stateRW.RUnlock()
	if !ok || existing == nil {
		return StringRecord{}, ErrNotFound
	}

	newRec := &StringRecord{ID: id, Value: value}
	if err := s.persistRecord(ctx, newRec); err != nil {
		return StringRecord{}, fmt.Errorf("persist: %w", err)
	}

	s.stateRW.Lock()
	s.byID[id] = newRec
	idx := s.pos[id]
	s.list[idx] = newRec
	s.stateRW.Unlock()

	return *newRec, nil
}

// Delete removes the string record with the given ID and compacts the ordered list.
// Preserves order by shifting elements left; updates positions for shifted items.
// Returns the deleted value copy.
//
// Time: O(n) (slice compaction + pos fix-up) plus network.
func (s *StringStore) Delete(ctx context.Context, id int64) (StringRecord, error) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	s.stateRW.RLock()
	rec, ok := s.byID[id]
	var delIdx int
	if ok && rec != nil {
		delIdx, ok = s.pos[id]
	}
	s.stateRW.RUnlock()
	if !ok || rec == nil {
		return StringRecord{}, ErrNotFound
	}

	if err := s.purgeRecord(ctx, rec); err != nil {
		return StringRecord{}, fmt.Errorf("purge: %w", err)
	}

	s.stateRW.Lock()
	last := len(s.list) - 1
	copy(s.list[delIdx:], s.list[delIdx+1:])
	s.list[last] = nil
	s.list = s.list[:last]

	delete(s.byID, id)
	delete(s.pos, id)

	for i := delIdx; i < len(s.list); i++ {
		s.pos[s.list[i].ID] = i
	}
	s.stateRW.Unlock()

	return *rec, nil
}

// GetOne returns the string record for the given ID as a value copy.
// Callers receive plain values; no mutable live objects are shared.
//
// Time: O(1).
func (s *StringStore) GetOne(id int64) (StringRecord, error) {
	s.stateRW.RLock()
	rec, ok := s.byID[id]
	s.stateRW.RUnlock()
	if !ok || rec == nil {
		return StringRecord{}, ErrNotFound
	}
	return *rec, nil
}

// GetMany returns the string records for the provided IDs as value copies,
// in the same order as the input. If any ID is missing, returns ErrNotFound.
//
// Time: O(k), where k = len(ids).
func (s *StringStore) GetMany(ids []int64) ([]StringRecord, error) {
	s.stateRW.RLock()
	defer s.stateRW.RUnlock()

	if len(ids) == 0 {
		return []StringRecord{}, nil
	}

	out := make([]StringRecord, len(ids))
	for i, id := range ids {
		rec, ok := s.byID[id]
		if !ok || rec == nil {
			return nil, ErrNotFound
		}
		out[i] = *rec
	}
	return out, nil
}

// GetList returns the string records in ascending ID order as value copies.
// The returned slice contains copies; callers cannot mutate internal state.
//
// Time: O(n).
func (s *StringStore) GetList() []StringRecord {
	s.stateRW.RLock()
	if len(s.list) == 0 {
		s.stateRW.RUnlock()
		return []StringRecord{}
	}
	out := make([]StringRecord, len(s.list))
	for i := range s.list {
		out[i] = *s.list[i]
	}
	s.stateRW.RUnlock()
	return out
}

// persistRecord writes the string document under <keyPrefix><id>.
// Time: O(1) avg (network). Space: O(1).
func (s *StringStore) persistRecord(ctx context.Context, rec *StringRecord) error {
	data, err := json.Marshal(rec)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	if err := s.rdb.Set(ctx, recordKey(s.keyPrefix, rec.ID), data, 0).Err(); err != nil {
		return fmt.Errorf("set: %w", err)
	}
	return nil
}

// purgeRecord deletes the string document from Redis.
// Time: O(1) avg (network). Space: O(1).
func (s *StringStore) purgeRecord(ctx context.Context, rec *StringRecord) error {
	if err := s.rdb.Del(ctx, recordKey(s.keyPrefix, rec.ID)).Err(); err != nil {
		return fmt.Errorf("del: %w", err)
	}
	return nil
}

// --- helpers ---

func recordKey(keyPrefix string, id int64) string { return keyPrefix + strconv.FormatInt(id, 10) }
func sequenceKey(keyPrefix string) string         { return keyPrefix + "id_seq" }

// reconcile scans Redis for existing documents under the keyPrefix, reconstructs
// the in-memory state, and publishes it atomically before the store accepts operations.
// This is a read-only pass: no writes or mutations to Redis are performed.
//
// Error Policy:
//   - Fatal: Redis connectivity issues.
//   - Recoverable: keyPrefix collision (non-conforming keys under prefix); per-record JSON parse errors; invalid/mismatched IDs. These are logged and skipped.
func (s *StringStore) reconcile(ctx context.Context) error {
	start := time.Now()
	seqKey := sequenceKey(s.keyPrefix)
	pattern := s.keyPrefix + "*"

	s.log.Info("reconcile: start",
		zap.String("prefix", s.keyPrefix),
		zap.String("pattern", pattern),
	)

	errs := 0
	var keys []string
	idByKey := make(map[string]int64)
	iter := s.rdb.Scan(ctx, 0, pattern, 0).Iterator()
	for iter.Next(ctx) {
		k := iter.Val()
		if k == seqKey {
			continue
		}
		// Validate key: must be strictly numeric suffixes; otherwise treat as collision.
		suffix := strings.TrimPrefix(k, s.keyPrefix)
		if id, err := strconv.ParseInt(suffix, 10, 64); err != nil || id <= 0 {
			s.log.Warn("reconcile: keyPrefix collision detected (non-conforming key); skipping",
				zap.String("key", k),
				zap.String("prefix", s.keyPrefix),
			)
			errs++
			continue
		} else {
			idByKey[k] = id
		}
		keys = append(keys, k)
	}
	if err := iter.Err(); err != nil {
		return fmt.Errorf("redis scan: %w", err)
	}

	if len(keys) == 0 {
		s.stateRW.Lock()
		s.byID = make(map[int64]*StringRecord)
		s.pos = make(map[int64]int)
		s.list = make([]*StringRecord, 0)
		s.stateRW.Unlock()

		s.log.Info("reconcile: complete",
			zap.String("prefix", s.keyPrefix),
			zap.Int("recovered", 0),
			zap.Int("errors", errs),
			zap.Duration("duration", time.Since(start)),
		)
		return nil
	}

	// Batch fetch documents.
	vals, err := s.rdb.MGet(ctx, keys...).Result()
	if err != nil {
		return fmt.Errorf("redis mget: %w", err)
	}

	recovered := 0

	type pair struct {
		id  int64
		rec *StringRecord
	}
	records := make([]pair, 0, len(vals))

	for i, raw := range vals {
		key := keys[i]
		if raw == nil {
			// Missing value: warn + skip.
			s.log.Warn("reconcile: missing value; skipping",
				zap.String("key", key),
			)
			errs++
			continue
		}

		var b []byte
		switch v := raw.(type) {
		case string:
			b = []byte(v)
		case []byte:
			b = v
		default:
			s.log.Warn("reconcile: unexpected type; skipping",
				zap.String("key", key),
			)
			errs++
			continue
		}

		var rec StringRecord
		if err := json.Unmarshal(b, &rec); err != nil {
			s.log.Warn("reconcile: deserialization failed; skipping",
				zap.String("key", key),
				zap.Error(err),
			)
			errs++
			continue
		}

		if rec.ID <= 0 {
			s.log.Warn("reconcile: invalid id; skipping",
				zap.String("key", key),
				zap.Int64("id", rec.ID),
			)
			errs++
			continue
		}

		expectedID := idByKey[key]
		if rec.ID != expectedID {
			s.log.Warn("reconcile: id mismatch; skipping",
				zap.String("key", key),
				zap.Int64("expected_id", expectedID),
				zap.Int64("doc_id", rec.ID),
			)
			errs++
			continue
		}

		rr := rec
		records = append(records, pair{id: rec.ID, rec: &rr})
		recovered++
	}

	// Sort by ascending ID to build ordered list.
	sort.Slice(records, func(i, j int) bool { return records[i].id < records[j].id })

	newByID := make(map[int64]*StringRecord, len(records))
	newPos := make(map[int64]int, len(records))
	newList := make([]*StringRecord, 0, len(records))

	for idx, p := range records {
		newByID[p.id] = p.rec
		newPos[p.id] = idx
		newList = append(newList, p.rec)
	}

	// Ensure the sequence is advanced to at least maxID to prevent overwrites on next Create.
	maxID := int64(0)
	if len(records) > 0 {
		maxID = records[len(records)-1].id
	}
	curSeq, err := s.rdb.IncrBy(ctx, sequenceKey(s.keyPrefix), 0).Result()
	if err != nil {
		return fmt.Errorf("redis incrby(0) seq read: %w", err)
	}
	if curSeq < maxID {
		if err := s.rdb.Set(ctx, sequenceKey(s.keyPrefix), maxID, 0).Err(); err != nil {
			return fmt.Errorf("redis set seq to maxID: %w", err)
		}
	}

	// Publish the initial in-memory state atomically.
	s.stateRW.Lock()
	s.byID = newByID
	s.pos = newPos
	s.list = newList
	s.stateRW.Unlock()

	s.log.Info("reconcile: complete",
		zap.String("prefix", s.keyPrefix),
		zap.Int("recovered", recovered),
		zap.Int("errors", errs),
		zap.Duration("duration", time.Since(start)),
	)

	return nil
}
