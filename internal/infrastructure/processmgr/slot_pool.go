package processmgr

import "sync"

// slotPool is a dynamically adjustable semaphore with explicit ownership.
// Each acquisition requires a unique external identifier. This enables
// accountable resource tracking and prevents silent leakage under load.
type slotPool struct {
	mu         sync.Mutex
	cond       *sync.Cond
	maxCap     int64
	usage      int64
	acquiredBy map[int64]struct{} // active ownership table
}

// newSlotPool initializes the pool with a given capacity.
func newSlotPool(max int64) *slotPool {
	s := &slotPool{
		maxCap:     max,
		acquiredBy: make(map[int64]struct{}),
	}
	s.cond = sync.NewCond(&s.mu)
	return s
}

// acquire blocks until usage < maxCap and registers id as the owner.
// Duplicate acquisition by the same id is a protocol violation.
func (s *slotPool) acquire(id int64) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, holds := s.acquiredBy[id]; holds {
		panic("slotPool: id already holds a slot")
	}

	for s.usage >= s.maxCap {
		s.cond.Wait()
	}

	s.usage++
	s.acquiredBy[id] = struct{}{}
}

// tryAcquire attempts a non-blocking acquire.
// On success, id becomes the owner.
func (s *slotPool) tryAcquire(id int64) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, holds := s.acquiredBy[id]; holds {
		panic("slotPool: id already holds a slot")
	}

	if s.usage >= s.maxCap {
		return false
	}

	s.usage++
	s.acquiredBy[id] = struct{}{}
	return true
}

// waitSlot blocks until usage < maxCap without taking a slot.
// This is a readiness check, not an acquisition.
func (s *slotPool) waitSlot() {
	s.mu.Lock()
	for s.usage >= s.maxCap {
		s.cond.Wait()
	}
	s.mu.Unlock()
}

// release frees the slot owned by id.
// Releasing an id that does not own a slot is an invariant violation.
func (s *slotPool) release(id int64) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, holds := s.acquiredBy[id]; !holds {
		panic("slotPool: release for non-owner id")
	}

	delete(s.acquiredBy, id)
	s.usage--
	s.cond.Signal()
}

// listAcquired returns a snapshot of all current owners.
func (s *slotPool) listAcquired() []int64 {
	s.mu.Lock()
	defer s.mu.Unlock()

	out := make([]int64, 0, len(s.acquiredBy))
	for id := range s.acquiredBy {
		out = append(out, id)
	}
	return out
}

// updateLimit adjusts the configured capacity.
// Negative values are clamped to zero since negative semaphore
// cardinality is undefined in standard concurrency models.
func (s *slotPool) updateLimit(newCap int64) {
	// Clamp: negative capacity has no well-defined semantics.
	if newCap < 0 {
		newCap = 0
	}

	s.mu.Lock()
	s.maxCap = newCap
	s.cond.Broadcast()
	s.mu.Unlock()
}

// capacity returns the configured concurrency limit.
func (s *slotPool) capacity() int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.maxCap
}

// current returns the number of active acquired slots.
func (s *slotPool) current() int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.usage
}
