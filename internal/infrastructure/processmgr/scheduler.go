package processmgr

import (
	"container/heap"
	"time"
)

// schedEvent represents a scheduled unit.
// index is required for heap.Fix + O(log n) removals.
type schedEvent struct {
	id    int64
	when  time.Time
	index int
}

type scheduler struct {
	h eventHeap
	// optional small index → event mapping
	// enables selective removal via deschedule(pid).
	entries map[int64]*schedEvent
}

func newScheduler() *scheduler {
	h := eventHeap{}
	heap.Init(&h)
	return &scheduler{
		h:       h,
		entries: make(map[int64]*schedEvent),
	}
}

// push inserts a new event.
func (s *scheduler) push(id int64, when time.Time) {
	// if event already exists, drop old one (fresh boot overrides stale)
	if old, ok := s.entries[id]; ok {
		heap.Remove(&s.h, old.index)
		delete(s.entries, id)
	}

	ev := &schedEvent{id: id, when: when}
	s.entries[id] = ev
	heap.Push(&s.h, ev)
}

// next returns the soonest event but does not remove it.
func (s *scheduler) next() (id int64, when time.Time, ok bool) {
	if len(s.h) == 0 {
		return 0, time.Time{}, false
	}
	ev := s.h[0]
	return ev.id, ev.when, true
}

// pop removes the head event unconditionally.
func (s *scheduler) pop() {
	if len(s.h) == 0 {
		return
	}
	ev := heap.Pop(&s.h).(*schedEvent)
	delete(s.entries, ev.id)
}

// remove delete the event for the given PID (if still pending).
func (s *scheduler) remove(id int64) {
	ev, ok := s.entries[id]
	if !ok {
		return
	}
	heap.Remove(&s.h, ev.index)
	delete(s.entries, id)
}

// --- heap internals ----------------------------------------------------------

// eventHeap is a min-heap ordered by event.when.
type eventHeap []*schedEvent

func (h eventHeap) Len() int { return len(h) }

func (h eventHeap) Less(i, j int) bool {
	return h[i].when.Before(h[j].when)
}

func (h eventHeap) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
	h[i].index = i
	h[j].index = j
}

func (h *eventHeap) Push(x any) {
	ev := x.(*schedEvent)
	ev.index = len(*h)
	*h = append(*h, ev)
}

func (h *eventHeap) Pop() any {
	old := *h
	n := len(old)
	ev := old[n-1]
	ev.index = -1 // mark as removed
	*h = old[:n-1]
	return ev
}
