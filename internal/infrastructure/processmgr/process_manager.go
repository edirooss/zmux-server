//go:build linux

package processmgr

import (
	"os"
	"sync"
	"time"

	"go.uber.org/zap"
)

// ProcessManager owns the entire supervisor model.
//
// Conceptual model:
//
//   - UID = external identity (user-facing service name, etc.)
//   - PID = monotonic internal identity for scheduling + lifecycle ownership
//   - a UID maps to exactly one PID at a time
//   - a PID is the authoritative instance of that unit
//   - PIDs are not reused until explicitly released

//   - specs[pid] describes how to launch the process (argv + restart policy)
//   - ps[pid] is the live running process (if any)
//   - sched is a priority-queue of future launch/restart events keyed by PID
//
// Concurrency model:
//   - All mutable maps + sched are protected by m.mu
//   - Launch, exit-handling, and scheduling always mutate these structures
//     only under the lock.
//   - Event loop wakes on expiration or explicit signals (self-pokes).
type ProcessManager struct {
	log    *zap.Logger
	logmgr *LogManager // per-unit aggregated logs (fan-in sink)
	env    []string    // global environment overlay for all units

	units map[int64]int64    // UID → PID (authoritative mapping)
	specs map[int64]execSpec // PID → execSpec (argv + restart policy)
	ps    map[int64]*process // PID → running process
	gen   *PIDAllocator      // monotonic PID allocator

	sched *scheduler    // priority queue: next processes to launch
	sig   chan struct{} // one-deep wake-up nudge for event loop

	mu sync.Mutex // guards all state transitions
}

// NewProcessManager constructs the supervisor and immediately starts its
// background scheduling loop.
//
// The event loop is intentionally detached: it reacts to timing signals
// and launch/teardown events sent via m.sig.
func NewProcessManager(log *zap.Logger, logmngr *LogManager) *ProcessManager {
	m := &ProcessManager{
		log:    log.Named("process-manager"),
		logmgr: logmngr,

		env: append(os.Environ(), "ENV=prod"), // override-mode overlay

		units: make(map[int64]int64),
		specs: make(map[int64]execSpec),
		ps:    make(map[int64]*process),
		gen:   newPIDAllocator(),

		sched: newScheduler(),
		sig:   make(chan struct{}, 1), // coalescing signal channel
	}

	go m.mainloop() // detached scheduling + lifecycle loop
	return m
}

// Add registers a new unit and schedules its first launch.
//
//   - Safe to call anytime.
//   - Idempotent: re-adding an existing UID is ignored.
//
// Lifecycle notes:
//
//   - A UID becomes a PID → this PID becomes the authoritative identity.
//   - The PID is then inserted into specs[], ps[], sched[], etc.
//   - All future restarts refer strictly to the PID.
//
// This avoids race conditions where a unit is replaced while a restart is pending.
func (m *ProcessManager) Add(uid int64, argv []string, cooldown time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.units[uid]; exists {
		// already known → intentionally ignored
		return
	}

	pid := m.gen.alloc()
	m.units[uid] = pid
	m.specs[pid] = execSpec{
		unitID:          uid,
		argv:            argv,
		restartCooldown: cooldown,
	}

	// schedule first launch immediately
	m.scheduleUnsafe(pid, 0)
}

// Remove unregisters a unit and tears down any running instance.
//
// Removal behavior:
//
//   - If the unit is running → terminate it
//   - Remove UID → PID mapping
//   - Remove PID → spec
//   - Drop from live process table
//   - Scrub scheduler events referencing that PID
//
// Unknown UIDs are ignored safely.
func (m *ProcessManager) Remove(uid int64) {
	m.mu.Lock()
	defer m.mu.Unlock()

	pid, exists := m.units[uid]
	if !exists {
		return // unknown uid
	}

	// if running, ask the process to shut down
	if proc, live := m.ps[pid]; live {
		proc.Close()
	}

	// drop mappings and scheduled restarts
	delete(m.units, uid)
	delete(m.specs, pid)
	delete(m.ps, pid) // if not already removed by exit handler
	m.sched.remove(pid)
}

// mainloop drives the scheduling engine.
//
// It repeatedly:
//
//	(1) inspects the next scheduled launch time
//	(2) sleeps until that time OR a wake-up signal arrives
//	(3) launches due processes
//
// This loop runs forever in the background.
// All scheduling operations are serialized by m.mu.
func (m *ProcessManager) mainloop() {
	timer := time.NewTimer(0)

	for {
		m.mu.Lock()
		pid, when, ok := m.sched.next()

		if !ok {
			// no future work; wait until someone pushes new work
			m.mu.Unlock()
			<-m.sig
			continue
		}

		// compute delay until next due event
		delay := time.Until(when)

		// future event → set timer and wait
		if delay > 0 {
			arm(timer, delay)
			m.mu.Unlock()

			select {
			case <-timer.C:
			case <-m.sig: // wake up early due to Add/Remove/reschedule
			}
			continue
		}

		// remove from scheduler → launch
		m.sched.pop()
		m.launchProcessUnsafe(pid)

		m.mu.Unlock()
	}
}

// launchProcessUnsafe launches the process for the given PID.
//
// Preconditions:
//   - caller holds m.mu
//
// Behavior:
//   - constructs process (stdin/stdout/stderr pipes + handlers)
//   - attempts Start()
//   - on success → installs exit handler
//   - on failure → schedules retry
//
// Exit handler semantics:
//   - Deletes the process from live map
//   - If still authoritative (UID still mapped to this PID):
//     schedule a restart
//   - Else (superseded or removed):
//     release PID back to allocator
func (m *ProcessManager) launchProcessUnsafe(pid int64) {
	spec := m.specs[pid] // must exist (manager invariant)

	// pre-scoped logger
	plog := m.log.With(
		zap.Int64("uid", spec.unitID),
		zap.Int64("pid", pid),
	)

	// construct process object (pipes + watchers)
	proc, ok := newProcess(plog, m.logmgr.Get(spec.unitID), m.env, spec.argv)
	if !ok {
		// construction failed → schedule retry
		m.log.Warn("process initialization failed; scheduling retry",
			zap.Int64("uid", spec.unitID), zap.Int64("pid", pid))

		m.scheduleUnsafe(pid, spec.restartCooldown)
		return
	}
	m.ps[pid] = proc // mark as running

	// attempt to start the process
	if !proc.Start() {
		m.log.Warn("process failed to start; scheduling restart",
			zap.Int64("uid", spec.unitID), zap.Int64("pid", pid))

		delete(m.ps, pid)
		m.scheduleUnsafe(pid, spec.restartCooldown)
		return
	}

	// attach background exit handler
	go func(pid int64, uid int64, spec execSpec, proc *process) {
		<-proc.Done() // wait for full shutdown

		m.mu.Lock()
		defer m.mu.Unlock()

		delete(m.ps, pid)

		current, exists := m.units[uid]
		if exists && current == pid {
			// PID is still the authoritative instance for this unit → restart it
			m.log.Info("process exited; scheduling restart",
				zap.Int64("uid", uid),
				zap.Int64("pid", pid),
			)
			m.scheduleUnsafe(pid, spec.restartCooldown)
			return
		}

		// PID no longer authoritative:
		//   • either the unit was removed
		//   • or the unit was removed and later re-added, producing a new PID
		// In both cases this PID must not restart and should be released.
		m.log.Debug("process exited; PID no longer authoritative → releasing",
			zap.Int64("uid", uid),
			zap.Int64("pid", pid),
		)
		m.gen.release(pid)

	}(pid, spec.unitID, spec, proc)
}

// --- sched helper -----------------------------------------------------------

// scheduleUnsafe queues PID for future launch after a given delay.
// Caller must hold m.mu.
//
// Uses non-blocking signal to notify the event loop.
// Multiple wakeups are coalesced naturally.
//
// This ensures no goroutine ever blocks while attempting to wake the scheduler.
func (m *ProcessManager) scheduleUnsafe(pid int64, after time.Duration) {
	m.sched.push(pid, time.Now().Add(after))

	select {
	case m.sig <- struct{}{}:
	default:
		// channel is full → signal already pending → that's fine
	}
}

// --- spec carrier -----------------------------------------------------------

// execSpec is the static configuration for a managed process.
//
// All restarts for a PID share the same spec.
type execSpec struct {
	unitID          int64
	argv            []string
	restartCooldown time.Duration
}

// --- timer helper -----------------------------------------------------------

// arm resets a timer safely, ensuring stale ticks are never left unread.
// Required when reusing timers in select-loops.
func arm(t *time.Timer, d time.Duration) {
	if !t.Stop() {
		select {
		case <-t.C:
		default:
		}
	}
	t.Reset(d)
}
