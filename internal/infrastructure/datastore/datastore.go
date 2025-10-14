package datastore

import (
	"context"
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
	// ErrNotFound means the record ID does not exist in the store.
	ErrNotFound = errors.New("record not found")
)

// DataStore maintains a process-local index of IDs (order + membership)
// while storing raw byte values exclusively in Redis.
//
// Space Complexity:
//   - O(n) for indexes only:
//   - ids slice (ascending by ID)
//   - pos map (ID → index into ids)
//   - No in-memory storage of values.
//
// Deployment & Operational Model:
//   - Single-process, single-node writer for a given keyPrefix (single-writer).
//   - Redis is assumed to be running on localhost.
//   - Design assumes exclusive process ownership for the prefix.
//
// Concurrency Model:
//   - All operations are serialized via a single mutex.
//   - Global serialization removes read↔write TOCTOU within this process.
//     If Redis indicates a missing value for an indexed id, it's treated as an invariant violation.
//   - Suitable for workloads with minimal reads and occasional writes.
//
// Reads (GetOne, GetMany, GetList):
//   - Entire operation executes under the global mutex.
//   - Membership/order is checked from the in-memory index.
//   - Raw bytes are always read from Redis.
//   - Return value copies.
//
// Writes (Create, Update, Delete):
//   - Entire operation executes under the global mutex.
//   - Redis I/O is performed before mutating the local index where applicable.
//   - Guarantees read-after-write visibility upon return.
//
// Consistency Model:
//   - Redis is the source of truth for values keyed by ID and the sequence counter.
//   - RAM holds a materialized index (IDs + positions) but not values.
//   - Readers never observe partial index mutations due to global serialization.
//
// Write Path:
//  1. Lock global mutex.
//  2. Persist the value change to Redis.
//  3. Update the local index as needed.
//  4. Unlock and return.
//
// Read Path:
//   - Lock global mutex.
//   - Validate membership from the local index.
//   - Fetch the raw bytes from Redis.
//   - Unlock and return.
//
// Namespace & Multi-Tenancy:
//   - Each instance of DataStore uses a unique keyPrefix.
//   - The prefix is exclusive to the owning process—no other writers must operate under it.
//   - Multiple stores may coexist via namespacing.
//
// ID Allocation:
//   - IDs are allocated using Redis INCR on key: <keyPrefix>id_seq.
//   - Monotonic, write-once, never recycled; gap-tolerant.
//
// Design Summary:
//   - Combines Redis durability with a simple in-process index for ordering/membership.
//   - Values are never stored in RAM; every value read is served by Redis.
//   - Global serialization simplifies correctness for low-QPS workloads.
type DataStore struct {
	log       *zap.Logger
	rdb       *redis.Client // Redis used as persistent storage (system of record); values-only
	keyPrefix string        // Redis key prefix; e.g. <store>:  → raw bytes under <prefix><id>

	mu  sync.Mutex    // serializes all operations
	pos map[int64]int // id -> index into ordered ids
	ids []int64       // ordered list of ids; sorted by id
}

// NewDataStore constructs a ready-to-use DataStore.
// On initialization, reconciles any existing Redis state under the given keyPrefix
// into the in-memory index. This is a read-only operation against Redis.
func NewDataStore(ctx context.Context, log *zap.Logger, rdb *redis.Client, keyPrefix string) (*DataStore, error) {
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

	s := &DataStore{
		rdb:       rdb,
		keyPrefix: keyPrefix,
		log:       log,
		pos:       make(map[int64]int),
		ids:       make([]int64, 0),
	}

	if err := s.reconcile(ctx); err != nil {
		return nil, fmt.Errorf("reconcile: %w", err)
	}
	return s, nil
}

// Create inserts a new value, assigns a unique increasing ID via Redis INCR,
// and appends it to the ordered index (maintaining ascending ID order). Returns the id.
//
// Time: O(1) avg to update index plus network.
//
// Invariants:
//   - Sequence monotonicity is guaranteed by Redis INCR.
//   - Index update occurs only after Redis persistence succeeds.
func (s *DataStore) Create(ctx context.Context, value []byte) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	id, err := s.rdb.Incr(ctx, sequenceKey(s.keyPrefix)).Result()
	if err != nil {
		return 0, fmt.Errorf("generate id via INCR: %w", err)
	}

	v := bcopy(value)
	if err := s.rdb.Set(ctx, recordKey(s.keyPrefix, id), v, 0).Err(); err != nil {
		return 0, fmt.Errorf("set (key=%s): %w", recordKey(s.keyPrefix, id), err)
	}

	s.indexInsert(id)

	return id, nil
}

// Update overwrites the stored value by the record's id. Returns error.
//
// Time: O(1) for index check plus network.
//
// Invariants:
//   - Index is authoritative for membership.
func (s *DataStore) Update(ctx context.Context, id int64, value []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.pos[id]; !ok {
		return ErrNotFound
	}

	v := bcopy(value)
	if err := s.rdb.Set(ctx, recordKey(s.keyPrefix, id), v, 0).Err(); err != nil {
		return fmt.Errorf("set (key=%s): %w", recordKey(s.keyPrefix, id), err)
	}

	return nil
}

// Delete removes the value with the given ID and compacts the ordered index. Returns error.
//
// Time: O(n) for index compaction plus network.
//
// Invariants:
//   - Idempotent: ensures non-existence in Redis and index; does not error if already absent.
//   - If index claimed presence but Redis deleted 0 keys, a WARN is emitted.
func (s *DataStore) Delete(ctx context.Context, id int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	idx, ok := s.pos[id]

	// Always attempt to delete from Redis; DEL is idempotent:
	//   Key exists → (1, nil)
	//   Key absent → (0, nil)
	n, err := s.rdb.Del(ctx, recordKey(s.keyPrefix, id)).Result()
	if err != nil {
		return fmt.Errorf("del: %w", err)
	}

	// If index thought the id existed but Redis deleted 0 keys, emit invariant WARN.
	if ok && n == 0 {
		s.log.Warn("delete: invariant violation (indexed id missing in Redis)", zap.Int64("id", id))
	}

	// Remove from local index if present.
	if ok {
		s.indexRemoveAt(idx)
	}

	return nil
}

// GetOne returns the value for the given ID as a copy.
//
// Time: O(1) index check + network GET.
//
// Behavior:
//   - If Redis is missing for an indexed id, auto-heal by removing the id from the local index and return ErrNotFound.
func (s *DataStore) GetOne(ctx context.Context, id int64) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	idx, ok := s.pos[id]
	if !ok {
		return nil, ErrNotFound
	}

	val, err := s.rdb.Get(ctx, recordKey(s.keyPrefix, id)).Bytes()
	if err != nil {
		if err == redis.Nil {
			s.log.Warn("get_one: auto-heal (indexed id missing in Redis)", zap.Int64("id", id))
			s.indexRemoveAt(idx)
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("redis get: %w", err)
	}

	return bcopy(val), nil
}

// GetMany returns the values for the provided IDs as copies,
// in the same order as the input.
// For any id not present in the local index, returns nil at that position.
// For any id present in the index but missing in Redis, returns nil and auto-heals.
//
// Time: O(k) to build key set + network MGET.
//
// Invariants:
//   - The result length equals the input length; missing entries yield nil without error.
func (s *DataStore) GetMany(ctx context.Context, ids []int64) ([][]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(ids) == 0 {
		return [][]byte{}, nil
	}

	type present struct {
		id  int64
		key string
	}
	var pres []present
	for _, id := range ids {
		if _, ok := s.pos[id]; ok {
			pres = append(pres, present{id: id, key: recordKey(s.keyPrefix, id)})
		}
	}

	idToVal := make(map[int64][]byte, len(pres))
	if len(pres) > 0 {
		keys := make([]string, len(pres))
		for i := range pres {
			keys[i] = pres[i].key
		}
		raws, err := s.rdb.MGet(ctx, keys...).Result()
		if err != nil {
			return nil, fmt.Errorf("redis mget: %w", err)
		}
		for i, raw := range raws {
			switch v := raw.(type) {
			case nil:
				idToVal[pres[i].id] = nil
			case string:
				idToVal[pres[i].id] = []byte(v)
			case []byte:
				idToVal[pres[i].id] = bcopy(v)
			default:
				return nil, fmt.Errorf("unexpected redis type at index %d", i)
			}
		}
	}

	out := make([][]byte, len(ids))
	for i, id := range ids {
		idx, ok := s.pos[id]
		if !ok {
			out[i] = nil
			continue
		}
		v := idToVal[id]
		if v == nil {
			s.log.Warn("get_many: auto-heal (indexed id missing in Redis)", zap.Int64("id", id))
			s.indexRemoveAt(idx)
			out[i] = nil
		} else {
			out[i] = v
		}
	}

	return out, nil
}

// GetList returns (ids, values) for all IDs present in the local index,
// in ascending ID order. Any id missing in Redis is auto-healed (removed from
// the index) and excluded from the returned slices.
//
// Time: O(n) for index size + network MGET.
//
// Behavior:
//   - Returns only records that exist in Redis; heals any stale index entries.
func (s *DataStore) GetList(ctx context.Context) ([]int64, [][]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(s.ids) == 0 {
		return []int64{}, [][]byte{}, nil
	}

	keys := make([]string, len(s.ids))
	for i, id := range s.ids {
		keys[i] = recordKey(s.keyPrefix, id)
	}

	vals, err := s.rdb.MGet(ctx, keys...).Result()
	if err != nil {
		return nil, nil, fmt.Errorf("redis mget: %w", err)
	}

	idsOut := make([]int64, 0, len(s.ids))
	valsOut := make([][]byte, 0, len(s.ids))
	var toRemove []int

	for i, raw := range vals {
		id := s.ids[i]
		switch v := raw.(type) {
		case nil:
			s.log.Warn("get_list: auto-heal (indexed id missing in Redis)", zap.Int64("id", id))
			toRemove = append(toRemove, i)
		case string:
			idsOut = append(idsOut, id)
			valsOut = append(valsOut, []byte(v))
		case []byte:
			idsOut = append(idsOut, id)
			valsOut = append(valsOut, bcopy(v))
		default:
			return nil, nil, fmt.Errorf("unexpected redis type at index %d", i)
		}
	}

	// Remove missing entries from the index, back-to-front to keep indices valid.
	for i := len(toRemove) - 1; i >= 0; i-- {
		s.indexRemoveAt(toRemove[i])
	}

	return idsOut, valsOut, nil
}

func recordKey(keyPrefix string, id int64) string { return keyPrefix + strconv.FormatInt(id, 10) }
func sequenceKey(keyPrefix string) string         { return keyPrefix + "id_seq" }

func bcopy(b []byte) []byte {
	out := make([]byte, len(b))
	copy(out, b)
	return out
}

// indexInsert inserts id into the sorted ids slice if absent and rebuilds pos for shifted items.
// Caller must hold the global mutex.
func (s *DataStore) indexInsert(id int64) {
	if _, exists := s.pos[id]; exists {
		return
	}
	i := sort.Search(len(s.ids), func(j int) bool { return s.ids[j] >= id })
	if i == len(s.ids) {
		s.ids = append(s.ids, id)
		s.pos[id] = i
		return
	}
	if s.ids[i] == id {
		s.pos[id] = i
		return
	}
	s.ids = append(s.ids, 0)
	copy(s.ids[i+1:], s.ids[i:])
	s.ids[i] = id
	for k := i; k < len(s.ids); k++ {
		s.pos[s.ids[k]] = k
	}
}

// indexRemoveAt removes the id at index i and fixes positions.
// Caller must hold the global mutex.
func (s *DataStore) indexRemoveAt(i int) {
	id := s.ids[i]
	last := len(s.ids) - 1
	copy(s.ids[i:], s.ids[i+1:])
	s.ids = s.ids[:last]
	delete(s.pos, id)
	for k := i; k < len(s.ids); k++ {
		s.pos[s.ids[k]] = k
	}
}

// reconcile scans Redis for existing IDs under the keyPrefix, reconstructs
// the in-memory index, and publishes it atomically before the store accepts operations.
// This is a read-only pass: no writes or mutations to Redis values are performed,
// except for ensuring the sequence counter is advanced to at least maxID.
//
// Error Policy:
//   - Fatal: Redis connectivity issues.
//   - Recoverable: keyPrefix collision (non-conforming keys under prefix); invalid IDs. These are logged and skipped.
//
// Invariants:
//   - Any non-numeric key under the prefix is treated as a collision (WARN) and skipped.
//   - Sequence is advanced to maxID if regressed (WARN).
func (s *DataStore) reconcile(ctx context.Context) error {
	start := time.Now()
	seqKey := sequenceKey(s.keyPrefix)
	pattern := s.keyPrefix + "*"

	errs := 0
	var ids []int64

	iter := s.rdb.Scan(ctx, 0, pattern, 0).Iterator()
	for iter.Next(ctx) {
		k := iter.Val()
		if k == seqKey {
			continue
		}
		// Validate key: must be strictly numeric suffixes; otherwise treat as collision.
		suffix := strings.TrimPrefix(k, s.keyPrefix)
		id, err := strconv.ParseInt(suffix, 10, 64)
		if err != nil || id <= 0 {
			s.log.Warn("reconcile: keyPrefix collision detected (non-conforming key); skipping")
			errs++
			continue
		}
		ids = append(ids, id)
	}
	if err := iter.Err(); err != nil {
		return fmt.Errorf("redis scan: %w", err)
	}

	// Sort by ascending ID to build ordered index.
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })

	newPos := make(map[int64]int, len(ids))
	for idx, id := range ids {
		newPos[id] = idx
	}

	// Ensure the sequence is advanced to at least maxID to prevent overwrites on next Create.
	maxID := int64(0)
	if len(ids) > 0 {
		maxID = ids[len(ids)-1]
	}
	curSeq, err := s.rdb.IncrBy(ctx, sequenceKey(s.keyPrefix), 0).Result()
	if err != nil {
		return fmt.Errorf("redis incrby(0) seq read: %w", err)
	}
	if curSeq < maxID {
		if err := s.rdb.Set(ctx, sequenceKey(s.keyPrefix), maxID, 0).Err(); err != nil {
			return fmt.Errorf("redis set seq to maxID: %w", err)
		}
		s.log.Warn("reconcile: sequence advanced to maxID to maintain monotonicity",
			zap.Int64("from", curSeq),
			zap.Int64("to", maxID),
			zap.String("prefix", s.keyPrefix),
		)
	}

	// Publish the initial in-memory index atomically.
	s.mu.Lock()
	s.pos = newPos
	s.ids = ids
	s.mu.Unlock()

	s.log.Info("reconcile: complete",
		zap.String("prefix", s.keyPrefix),
		zap.Int("recovered", len(ids)),
		zap.Int("errors", errs),
		zap.Duration("duration", time.Since(start)),
	)

	return nil
}
