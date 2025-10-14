package processmgr

import "sync"

// logBuffer is a thread-safe circular buffer for log entries with O(1) append and O(N) read
type logBuffer struct {
	entries [500]string  // Fixed-size circular buffer (no heap allocations)
	head    int          // Next write position (0-499)
	size    int          // Current number of entries (0-500)
	full    bool         // Whether buffer has wrapped around
	mu      sync.RWMutex // Read-write mutex; Protects all fields
}

// Append adds a log entry (overwrites oldest if full)
// Write lock held for ~100ns (single array write + arithmetic)
//
// Complexity: O(1) time, O(1) space
func (b *logBuffer) Append(entry string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	const capN = len(b.entries)

	// Write to circular buffer
	b.entries[b.head] = entry

	// Advance head
	b.head = (b.head + 1) % capN

	// Update size/full state
	if b.full {
		// Size stays at capN, we're overwriting
		return
	}
	b.size++
	if b.size == capN {
		b.full = true
	}
}

// Read returns last N entries (newest → oldest)
// Read lock held for duration of copy operation (~1μs per entry)
// Returns a NEW slice (caller owns memory)
//
// Semantics:
//   - If lines <= 0: returns up to 500 lines (whatever is available), newest → oldest
//   - If lines > 500: clamped to 500
//
// Complexity: O(N) time where N = lines requested
// Space: O(N) for result slice
func (b *logBuffer) Read(lines int) []string {
	b.mu.RLock()
	defer b.mu.RUnlock()

	const capN = len(b.entries)
	if b.size == 0 {
		return nil
	}

	// Clamp request
	if lines <= 0 || lines > capN {
		lines = capN
	}

	// Number of lines we can actually return
	n := b.size
	if n > lines {
		n = lines
	}

	result := make([]string, n)

	// newest index
	var newest int
	if b.full {
		// head points to the oldest (next overwrite); newest is one behind head
		newest = (b.head - 1 + capN) % capN
	} else {
		// valid range is [0, size); newest is size-1
		newest = b.size - 1
	}

	// Fill newest → oldest
	for i := 0; i < n; i++ {
		idx := (newest - i + capN) % capN
		result[i] = b.entries[idx]
	}

	return result
}
