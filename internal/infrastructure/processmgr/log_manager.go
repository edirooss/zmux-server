package processmgr

import "sync"

// LogManager manages per-process log buffers.
// - Creates buffers lazily
// - Thread-safe access
type LogManager struct {
	mu   sync.RWMutex         // guards buf map
	bufs map[int64]*logBuffer // PID → log buffer
}

// NewLogManager initializes an empty log-buffer registry.
func NewLogManager() *LogManager {
	return &LogManager{
		bufs: make(map[int64]*logBuffer),
	}
}

// Get returns the log buffer for a PID.
// If missing, a new buffer is created and stored.
func (lm *LogManager) Get(pid int64) *logBuffer {
	lm.mu.Lock()
	defer lm.mu.Unlock()

	if buf, ok := lm.bufs[pid]; ok {
		return buf
	}

	buf := new(logBuffer)
	lm.bufs[pid] = buf
	return buf
}
