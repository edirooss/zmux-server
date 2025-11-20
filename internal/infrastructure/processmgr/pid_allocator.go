package processmgr

import (
	"fmt"
	"sync"
)

// PIDAllocator manages a monotonic, wrap-around PID space.
// Behavior mirrors Linux: increment, wrap, skip in-use.
type PIDAllocator struct {
	mu     sync.Mutex
	next   int64
	inUse  map[int64]struct{}
	pidMax int64
}

// New returns an allocator using a Linux-like PID range [1, 32768].
// Starts at PID 1, mirroring default kernel behavior.
func newPIDAllocator() *PIDAllocator {
	return &PIDAllocator{
		next:   1,
		pidMax: 32768,
		inUse:  make(map[int64]struct{}),
	}
}

// alloc returns the next available PID or panics if the space is exhausted.
// Dev note: linear scan w/ wrap—faithful to how Linux allocates in pidmap.
func (a *PIDAllocator) alloc() int64 {
	a.mu.Lock()
	defer a.mu.Unlock()

	start := a.next

	for {
		p := a.next

		// increment-first semantics (kernel-like)
		a.next++
		if a.next > a.pidMax {
			a.next = 1
		}

		// skip in-use PIDs
		if _, used := a.inUse[p]; used {
			goto cont
		}

		a.inUse[p] = struct{}{}
		return p

	cont:
		// wrapped fully → no available PIDs
		if a.next == start {
			panic(fmt.Sprintf("PIDAllocator exhausted: 1..%d fully allocated", a.pidMax))
		}
	}
}

// release returns a PID to the free pool.
// No-op on invalid or duplicate releases.
func (a *PIDAllocator) release(pid int64) {
	a.mu.Lock()
	defer a.mu.Unlock()

	delete(a.inUse, pid) // map delete is safe on missing keys
}
