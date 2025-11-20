//go:build linux

package processmgr

import (
	"os"
	"sync"
	"time"

	"go.uber.org/zap"
)

// ProcessManager2 is an extended supervisor built on top of PM1’s
// authoritative PID model, but with dual-phase concurrency control:
//
//   - Preflight slots  – limit the number of *booting / warming* processes
//   - Onflight slots   – limit the number of *fully-active / post-ready* processes
//
// Unlike simple rate limits, these slots are *ownership-based*. Each PID
// explicitly acquires and releases its slot, preventing silent leaks.
//
// -----------------------------------------------------------------------------
// Identity model (identical to PM1)
//
//   - UID = external user-facing identity
//   - PID = internal monotonic identity (never reused until released)
//   - A UID always maps to *one* authoritative PID at a time
//   - Restarts always refer to the PID, never the UID
//
// When a UID is removed, its PID eventually becomes non-authoritative and is
// released after process termination.
//
// -----------------------------------------------------------------------------
// Concurrency model
//
//   - All mutable state (units, specs, ps, sched, slot ownership) is protected
//     by a single mutex m.mu
//
//   - Process lifecycle follows PM1 exactly:
//     Start → Ready → Enter → Done → Exit-handler → Restart-or-Release
//
//   - Dual-slot gating is enforced BEFORE launching any process:
//     The scheduler waits until BOTH:
//
//   - preflight capacity is available
//
//   - onflight capacity is available
//
//     This guarantees that *any process we choose to launch* will not become
//     stranded at readiness due to missing onflight capacity.
//
//     This is an intentional design choice producing a “pipeline capacity
//     guarantee”: only as many preflight processes are launched as can be fully
//     promoted into active mode.
//
// -----------------------------------------------------------------------------
// Summary
//
// PM2 behaves exactly like PM1 with respect to lifecycle correctness,
// authoritative PID semantics, and restart logic — but introduces a controlled
// warm-up stage and active stage with enforced concurrency limits.
type ProcessManager2 struct {
	log    *zap.Logger
	logmgr *LogManager
	env    []string

	// Authoritative tables
	units map[int64]int64    // UID → PID
	specs map[int64]execSpec // PID → static exec spec
	ps    map[int64]*process // PID → running process (if any)
	gen   *PIDAllocator

	// Concurrency gates
	preflight *slotPool // warm-up phase
	onflight  *slotPool // active phase

	// Scheduling
	sched *scheduler
	sig   chan struct{}

	mu sync.Mutex
}

// NewProcessManager2 constructs PM2 with explicit warm-up and active caps.
//
// maxPreflight – max warming/booting processes allowed
// maxOnflight  – max active processes allowed
func NewProcessManager2(
	log *zap.Logger,
	logmngr *LogManager,
	maxPreflight, maxOnflight int64,
) *ProcessManager2 {

	m := &ProcessManager2{
		log:    log.Named("process-manager2"),
		logmgr: logmngr,
		env:    append(os.Environ(), "ENV=prod"),

		units: make(map[int64]int64),
		specs: make(map[int64]execSpec),
		ps:    make(map[int64]*process),
		gen:   newPIDAllocator(),

		preflight: newSlotPool(maxPreflight),
		onflight:  newSlotPool(maxOnflight),

		sched: newScheduler(),
		sig:   make(chan struct{}, 1), // coalescing wake-up
	}

	go m.mainloop()
	return m
}

// Add registers a new unit, allocates a PID, stores its spec, and schedules an
// immediate launch.
func (m *ProcessManager2) Add(uid int64, argv []string, cooldown time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.units[uid]; exists {
		return // already registered → ignore
	}

	pid := m.gen.alloc()

	m.units[uid] = pid
	m.specs[pid] = execSpec{
		unitID:          uid,
		argv:            argv,
		restartCooldown: cooldown,
	}

	m.scheduleUnsafe(pid, 0)
}

// Remove deletes a UID and tears down any running instance.
func (m *ProcessManager2) Remove(uid int64) {
	m.mu.Lock()
	defer m.mu.Unlock()

	pid, exists := m.units[uid]
	if !exists {
		return
	}

	// kill running instance
	if proc := m.ps[pid]; proc != nil {
		proc.Close()
	}

	delete(m.units, uid)
	delete(m.specs, pid)
	delete(m.ps, pid)
	m.sched.remove(pid)
}

// UpdateLimits adjusts max preflight/onflight capacity at runtime.
//
// If new limits are smaller than current usage, excess processes are forcibly
// terminated (via Close) to restore invariants.
func (m *ProcessManager2) UpdateLimits(maxPre, maxOn int64) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// --- preflight ---
	oldPre := m.preflight.capacity()
	if oldPre != maxPre {
		m.log.Info("updating preflight limit", zap.Int64("old", oldPre), zap.Int64("new", maxPre))

		m.preflight.updateLimit(maxPre)
		owners := m.preflight.listAcquired()

		if int64(len(owners)) > maxPre {
			excess := int64(len(owners)) - maxPre
			for _, pid := range owners[:excess] {
				if p := m.ps[pid]; p != nil {
					p.Close()
				}
			}
		}
	}

	// --- onflight ---
	oldOn := m.onflight.capacity()
	if oldOn != maxOn {
		m.log.Info("updating onflight limit", zap.Int64("old", oldOn), zap.Int64("new", maxOn))

		m.onflight.updateLimit(maxOn)
		owners := m.onflight.listAcquired()

		if int64(len(owners)) > maxOn {
			excess := int64(len(owners)) - maxOn
			for _, pid := range owners[:excess] {
				if p := m.ps[pid]; p != nil {
					p.Close()
				}
			}
		}
	}

	// poke the scheduler
	select {
	case m.sig <- struct{}{}:
	default:
	}
}

func (m *ProcessManager2) Onflight() int64 {
	return m.onflight.current()
}

// ----------------------------------------------------------------------------
// Event Loop (scheduler) — PM1 semantics extended with dual-slot gating
// ----------------------------------------------------------------------------

func (m *ProcessManager2) mainloop() {
	timer := time.NewTimer(0)

	for {
		// Ensure capacity *before* acquiring lock.
		//
		// We intentionally block until BOTH preflight and onflight capacity
		// exist. This ensures that any process we choose to launch can
		// *guaranteedly* be promoted to active mode once Ready.
		//
		// This enforces pipeline capacity: no stranded preflight processes.
		m.preflight.waitSlot()
		m.onflight.waitSlot()

		m.mu.Lock()
		pid, when, ok := m.sched.next()

		if !ok {
			m.mu.Unlock()
			<-m.sig
			continue
		}

		delay := time.Until(when)

		if delay > 0 {
			arm(timer, delay)
			m.mu.Unlock()

			select {
			case <-timer.C:
			case <-m.sig:
			}
			continue
		}

		// Try acquiring a preflight slot for this PID.
		//
		// Note:
		//   Under normal operation, no other code path acquires preflight slots,
		//   so waitSlot() → tryAcquire() cannot race against another PID.
		//   The only way this can fail is if UpdateLimits() executes between
		//   the waitSlot() unblock and this tryAcquire(), shrinking the capacity
		//   so that the previously-available slot is no longer available.
		if !m.preflight.tryAcquire(pid) {
			m.mu.Unlock()
			continue
		}

		// remove from scheduler & launch
		m.sched.pop()
		m.launchProcessUnsafe(pid)

		m.mu.Unlock()
	}
}

// ----------------------------------------------------------------------------
// Launch / Supervisor Logic
// ----------------------------------------------------------------------------

func (m *ProcessManager2) launchProcessUnsafe(pid int64) {
	spec := m.specs[pid]
	plog := m.log.With(zap.Int64("uid", spec.unitID), zap.Int64("pid", pid))

	// create process wrapper
	proc, ok := newProcess(plog, m.logmgr.Get(spec.unitID), m.env, spec.argv)
	if !ok {
		// construction failed — return its preflight slot and retry later
		m.preflight.release(pid)
		m.scheduleUnsafe(pid, spec.restartCooldown)
		return
	}

	m.ps[pid] = proc

	// start process
	if !proc.Start() {
		delete(m.ps, pid)
		m.preflight.release(pid)
		m.scheduleUnsafe(pid, spec.restartCooldown)
		return
	}

	// delegate lifecycle to supervisor
	go m.superviseInstance(pid, spec, proc)
}

// superviseInstance is PM2’s extended lifecycle:
//
//	preflight slot (acquired)
//	   ↓
//	Ready → acquire onflight → release preflight → Enter()
//	   ↓
//	Done → release onflight
//
// Process may die before Ready: only preflight is released.
//
// Restart/supersession semantics match PM1.
func (m *ProcessManager2) superviseInstance(pid int64, spec execSpec, proc *process) {
	uid := spec.unitID

	// cleanup + authoritative PID logic
	defer m.handleExit(pid, uid)

	// --- Phase 1: warm-up ---
	select {
	case <-proc.Ready():
		// try promoting into active phase
		if !m.onflight.tryAcquire(pid) {
			proc.Close()
			m.preflight.release(pid)
			return
		}
		m.preflight.release(pid)

		// transition to active
		if err := proc.Enter(); err != nil {
			proc.Close()
			m.onflight.release(pid)
			return
		}

	case <-proc.Done():
		// died before ready
		m.preflight.release(pid)
		return
	}

	// --- Phase 2: active ---
	<-proc.Done()
	m.onflight.release(pid)
}

// ----------------------------------------------------------------------------
// Exit handling — identical authoritative-PID logic as PM1
// ----------------------------------------------------------------------------

func (m *ProcessManager2) handleExit(pid, uid int64) {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.ps, pid)

	current, exists := m.units[uid]

	// If still authoritative, reschedule restart
	if exists && current == pid {
		m.scheduleUnsafe(pid, m.specs[pid].restartCooldown)
		return
	}

	// Otherwise:
	//   • UID was removed
	//   • or UID was re-added → now mapped to a new PID
	m.gen.release(pid)
}

// ---- Scheduler helper ------------------------------------------------------

func (m *ProcessManager2) scheduleUnsafe(pid int64, after time.Duration) {
	m.sched.push(pid, time.Now().Add(after))

	select {
	case m.sig <- struct{}{}:
	default:
	}
}
