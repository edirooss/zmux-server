package objectstore

import (
	"sort"
	"sync"

	"go.uber.org/zap"
)

// ObjectStore is a concurrent, in-memory KV indexed by int64 IDs.
//
// Data structures:
//   - Mutable state (ids + vals + pos + map) guarded by RWMutex
//
// Iteration is deterministic (ascending IDs).
// Reads use shared (R) locks; writes use exclusive (W) locks.
//
// Concurrency:
//   - Per-store write serialization via exclusive lock
//   - Concurrent reads via shared lock
//
// Typical costs:
//   - Upsert/Delete: O(1) for overwrite/append; O(n) for mid-slice shifts
//   - Reads: O(1)/O(k)/O(n)
//
// Semantics:
//   - Values are stored *as provided*, without deep copying.
//   - Structs are stored by value; pointers are stored by reference.
//   - Callers who mutate pointer-based values after insertion will affect
//     the live in-store representation (same memory reference).
type ObjectStore struct {
	log *zap.Logger

	mu sync.RWMutex // guards st
	st storeState
}

type storeState struct {
	byID map[int64]any
	ids  []int64
	vals []any
	pos  map[int64]int
}

// NewObjectStore constructs a ready-to-use ObjectStore.
//
// Space: O(1) init. No background tasks. Safe for concurrent use post-return.
// Design balances read concurrency with write-heavy workloads.
func NewObjectStore(log *zap.Logger) *ObjectStore {
	if log == nil {
		log = zap.NewNop()
	}
	return &ObjectStore{
		log: log,
		st: storeState{
			byID: make(map[int64]any),
			ids:  make([]int64, 0),
			vals: make([]any, 0),
			pos:  make(map[int64]int),
		},
	}
}

// Upsert inserts or overwrites value at id; no return.
//
// Time:
//   - Overwrite existing id -> O(1)
//   - Append when id is strictly greater than current max -> amortized O(1)
//   - General insert -> O(n) for mid-slice insert/shift
//
// Space: Amortized O(1); mutations in-place under lock.
//
// Strategy:
//   - Overwrite existing id -> update map and vals at position
//   - Append when id is strictly greater than current max -> push back id/val
//   - Otherwise:
//   - sort.Search for insertion point, insert into ids/vals, update pos
//
// Note:
//   - Values are inserted as-is (no deep copy). Pointer inputs will remain
//     live references visible to subsequent readers.
func (s *ObjectStore) Upsert(id int64, value any) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Overwrite existing key.
	if idx, exists := s.st.pos[id]; exists {
		s.st.byID[id] = value
		s.st.vals[idx] = value
		return
	}

	// Append fast path: id is strictly greater than current maximum id.
	if n := len(s.st.ids); n == 0 || id > s.st.ids[n-1] {
		s.st.ids = append(s.st.ids, id)
		s.st.vals = append(s.st.vals, value)
		s.st.byID[id] = value
		s.st.pos[id] = len(s.st.ids) - 1
		return
	}

	// General insert: maintain ascending order via binary search.
	insertIdx := sort.Search(len(s.st.ids), func(i int) bool { return s.st.ids[i] >= id })

	// Insert id.
	s.st.ids = append(s.st.ids, 0)
	copy(s.st.ids[insertIdx+1:], s.st.ids[insertIdx:])
	s.st.ids[insertIdx] = id

	// Insert value aligned with ids.
	s.st.vals = append(s.st.vals, nil)
	copy(s.st.vals[insertIdx+1:], s.st.vals[insertIdx:])
	s.st.vals[insertIdx] = value

	// Update maps.
	s.st.byID[id] = value
	// Recompute pos for shifted tail; leading part stays the same.
	for i := insertIdx; i < len(s.st.ids); i++ {
		s.st.pos[s.st.ids[i]] = i
	}
}

// Delete removes id if present; idempotent (no-op if absent); no return.
//
// Time: O(n) for slice compaction
// Space: Amortized O(1)
//
// Approach:
//   - Mutate in place under lock
//   - remove id from map and ids/vals slices
//   - update positions for shifted tail
func (s *ObjectStore) Delete(id int64) {
	s.mu.Lock()
	defer s.mu.Unlock()

	idx, ok := s.st.pos[id]
	if !ok {
		return
	}

	// Remove from map and pos.
	delete(s.st.byID, id)
	delete(s.st.pos, id)

	// Compact ids slice.
	copy(s.st.ids[idx:], s.st.ids[idx+1:])
	s.st.ids = s.st.ids[:len(s.st.ids)-1]

	// Compact vals slice (aligned).
	copy(s.st.vals[idx:], s.st.vals[idx+1:])
	s.st.vals = s.st.vals[:len(s.st.vals)-1]

	// Update positions for shifted tail.
	for i := idx; i < len(s.st.ids); i++ {
		s.st.pos[s.st.ids[i]] = i
	}
}

// GetOne returns (value, ok). No live pointers are exposed by the store itself,
// but callers may have stored pointers intentionally. Pointer-based inserts
// will yield the same reference object on retrieval.
//
// Time: O(1) hashmap lookup
//
// Read path uses shared lock.
func (s *ObjectStore) GetOne(id int64) (any, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	val, ok := s.st.byID[id]
	return val, ok
}

// GetMany returns values aligned to input ids and an all-present flag.
//
// Time: O(k) where k = len(ids)
// Space: O(k) for output
//
// Missing entries yield zero-values in output; boolean indicates completeness.
// Pointer semantics are preserved as in GetOne.
func (s *ObjectStore) GetMany(ids []int64) ([]any, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make([]any, len(ids))
	all := true
	for i, id := range ids {
		v, ok := s.st.byID[id]
		if !ok {
			all = false
		}
		out[i] = v
	}
	return out, all
}

// GetList returns (ids, values) in ascending order; copies are returned.
//
// Time: O(n) to copy
// Space: O(n) for outputs
//
// Pattern:
//   - copy ID slice
//   - copy value slice (aligned to IDs)
//   - values remain as originally inserted (pointer/reference semantics preserved)
func (s *ObjectStore) GetList() ([]int64, []any) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if len(s.st.ids) == 0 {
		return []int64{}, []any{}
	}
	n := len(s.st.ids)

	idsOut := make([]int64, n)
	copy(idsOut, s.st.ids)

	valsOut := make([]any, n)
	copy(valsOut, s.st.vals)

	return idsOut, valsOut
}
