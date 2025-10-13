//go:build linux || darwin

package processmgr

import (
	"bufio"
	"context"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"time"

	"go.uber.org/zap"
)

// ProcessManager coordinates multiple supervised processes.
// It is safe for concurrent use.
//
// Process Lifecycle:
//   - Start(id, ...): Spawns a supervisor goroutine for the given ID.
//     Returns immediately if the ID already exists (no-op).
//   - Stop(id): Signals the supervisor to shutdown and removes it from the map.
//     The supervisor goroutine continues running in the background until
//     the process terminates gracefully (or forcefully after timeout).
//
// Restart Semantics:
//
//	Calling Stop(id) followed immediately by Start(id, ...) is supported.
//	The new process will start immediately without waiting for the old
//	process to fully shutdown. Both supervisors run independently with
//	separate contexts and process instances. This enables fast restarts
//	with minimal downtime.
type ProcessManager struct {
	log        *zap.Logger
	env        []string
	processes  map[int64]*managedProcess // Protected by mu
	logBuffers map[int64]*logBuffer      // Per-process log buffers
	mu         sync.RWMutex
}

func NewProcessManager(log *zap.Logger) *ProcessManager {
	return &ProcessManager{
		// log: log.Named("process-manager"),
		log: zap.NewNop(),
		env: append(os.Environ(),
			"ENV=prod",
		), // always prod (overwrite parent ENV=dev)
		processes:  make(map[int64]*managedProcess),
		logBuffers: make(map[int64]*logBuffer),
	}
}

// Start spawns a supervised process
// - id: Unique identifier for this process
// - argv: Command and arguments (argv[0] is executable)
// - restartCooldown: Delay between restart attempts
//
// Idempotent: No-op if ID already exists
// Non-blocking: Returns immediately
func (mng *ProcessManager) Start(id int64, argv []string, restartCooldown time.Duration) {
	mng.mu.Lock()
	_, ok := mng.processes[id]
	if ok /* already exists */ {
		// Process already running - this is idempotent
		mng.mu.Unlock()
		return
	}
	p := newManagedProcess(id, argv, restartCooldown)
	mng.processes[id] = p

	// Get or create log buffer for this process
	// We do not clear buffer on restart; logs for this ID are kept from prev session
	logBuf, exists := mng.logBuffers[p.id]
	if !exists {
		logBuf = new(logBuffer)
		mng.logBuffers[p.id] = logBuf
	}
	mng.mu.Unlock()

	go mng.superviseProcess(p, logBuf)
}

// Stop terminates a supervised process gracefully
// - id: Process identifier to stop
//
// Idempotent: No-op if ID doesn't exist
// Non-blocking: Returns before process fully terminates
// Allows immediate Start() of same ID
func (mng *ProcessManager) Stop(id int64) {
	mng.mu.Lock()
	p, ok := mng.processes[id]
	if !ok /* not exists */ {
		// Process not found - this is idempotent
		mng.mu.Unlock()
		return
	}
	delete(mng.processes, id) // Remove from config immediately
	mng.mu.Unlock()

	// Signal shutdown to supervisor goroutine
	// The goroutine continues running until process terminates
	// Callers can immediately Start() the same ID without waiting
	p.cancel()
}

// GetLogs retrieves the last N log entries for a process
// - id: Process identifier
// - lines: Number of lines to retrieve (0 = all available, max 500)
// Returns: Slice ordered newest → oldest, empty if process doesn't exist
func (mng *ProcessManager) GetLogs(id int64, lines int) ([]string, bool) {
	mng.mu.RLock()
	buffer, exists := mng.logBuffers[id]
	mng.mu.RUnlock()

	if !exists {
		return nil, false
	}

	// Clamp lines to [1..500] with 0 = all available (up to 500)
	if lines <= 0 {
		lines = 500
	}
	if lines > 500 {
		lines = 500
	}

	return buffer.Read(lines), true // newest → oldest
}

// superviseProcess runs an infinite supervision loop for a managed process.
// It handles:
// - Initial process startup with configurable restart cooldown
// - Automatic restart on process exit/crash
// - Graceful shutdown with SIGTERM (3s timeout) followed by SIGKILL
// - Context cancellation during any phase of the lifecycle
//
// The supervisor continues until the context is cancelled via Stop().
func (mng *ProcessManager) superviseProcess(proc *managedProcess, logBuf *logBuffer) {
	log := mng.log.With(zap.Int64("id", proc.id), zap.Strings("argv", proc.argv))
	log.Info("supervisor started")

	// Timer controls restart scheduling; initialized to fire immediately for first start
	timer := time.NewTimer(0)
	defer timer.Stop()

	// Buffered channel to receive process exit status without blocking the wait goroutine
	// Buffer size of 1 ensures the goroutine can send and exit even if we're not receiving
	// INVARIANT: doneCh is reused but safe because:
	// - Only ONE goroutine writes to it at a time
	// - We ALWAYS consume the value before spawning the next goroutine
	// - All exit paths either consume from doneCh or exit before spawning
	doneCh := make(chan error, 1)
	defer close(doneCh)

	for {
		select {
		// Context cancelled while waiting to restart - supervisor shutdown requested
		case <-proc.ctx.Done():
			// Drain timer channel if it fired before we read ctx.Done()
			// This prevents a stale timer value from being consumed in the next iteration
			if !timer.Stop() {
				<-timer.C
			}
			log.Info("supervisor shutdown during restart cooldown",
				zap.String("reason", proc.ctx.Err().Error()))
			return // Exit supervisor loop

		// Restart cooldown expired or initial start trigger
		case <-timer.C:
			log.Info("spawning process",
				zap.Duration("restart_cooldown", proc.restartCooldown))

			cmd := exec.Command(proc.argv[0], proc.argv[1:]...)

			// Orphan Prevention: Pdeathsig configured before process start.
			// If the supervisor process dies unexpectedly, all child processes will receive SIGKILL automatically.
			cmd.SysProcAttr = &syscall.SysProcAttr{
				Pdeathsig: syscall.SIGKILL, // Kill child when parent dies
			}
			// Attach env
			cmd.Env = mng.env

			// Setup stderr pipe for log collection
			stderrPipe, err := cmd.StderrPipe()
			if err != nil {
				log.Error("failed to create stderr pipe", zap.Error(err))
				timer.Reset(proc.restartCooldown)
				continue
			}

			// Start the process
			if err := cmd.Start(); err != nil {
				log.Error("failed to spawn process",
					zap.Error(err),
					zap.String("command", proc.argv[0]))
				// Schedule retry after cooldown period
				timer.Reset(proc.restartCooldown)
				continue
			}

			pid := cmd.Process.Pid
			log.Info("process started successfully", zap.Int("pid", pid))

			// Launch stderr reader
			go func() {
				scanner := bufio.NewScanner(stderrPipe)
				scanner.Buffer(make([]byte, 64*1024), 1024*1024) // 64KB init, 1MB max

				for scanner.Scan() {
					// Non-blocking write to buffer
					logBuf.Append(scanner.Text())
				}

				// Scanner exited (EOF or error)
				if err := scanner.Err(); err != nil {
					// Log error but don't crash
					logBuf.Append(err.Error())
					log.Error("stderr reader exited abnormally", zap.Int("pid", pid), zap.Error(err))
					return
				}
				log.Info("stderr reader exited normally", zap.Int("pid", pid))
			}()

			// Wait for process exit asynchronously to avoid blocking the supervisor
			// This allows us to handle context cancellation while the process is running
			go func() {
				doneCh <- cmd.Wait()
			}()

			// Wait for either process exit or shutdown signal
			select {
			// Process exited (normally or crashed) - schedule restart
			case err := <-doneCh:
				if err != nil {
					// Process exited with error (non-zero exit code or signal)
					if exitErr, ok := err.(*exec.ExitError); ok {
						log.Warn("process exited abnormally",
							zap.Int("pid", pid),
							zap.Int("exit_code", exitErr.ExitCode()),
							zap.Error(err))
					} else {
						log.Warn("process wait failed",
							zap.Int("pid", pid),
							zap.Error(err))
					}
				} else {
					// Process exited cleanly (exit code 0)
					log.Info("process exited normally",
						zap.Int("pid", pid))
				}

				// Schedule restart after cooldown
				timer.Reset(proc.restartCooldown)
				continue

			// Context cancelled while process is running - initiate graceful shutdown
			case <-proc.ctx.Done():
				log.Info("supervisor shutdown requested, initiating graceful shutdown sequence",
					zap.Int("pid", pid),
					zap.String("reason", proc.ctx.Err().Error()))

				// Step 1: Send SIGTERM for graceful shutdown
				if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
					// Process may have already exited - doneCh will receive the result
					log.Warn("failed to send SIGTERM (process may have already exited)",
						zap.Int("pid", pid),
						zap.Error(err))
				} else {
					log.Info("SIGTERM sent to process", zap.Int("pid", pid))
				}

				// Wait up to 3 seconds for graceful shutdown
				gracefulTimeout := 3 * time.Second
				timer.Reset(gracefulTimeout)

				select {
				// Process exited within grace period
				case err := <-doneCh:
					// Stop and drain timer since we received process exit before timeout
					if !timer.Stop() {
						<-timer.C
					}

					if err != nil {
						log.Info("process terminated after SIGTERM",
							zap.Int("pid", pid),
							zap.Error(err))
					} else {
						log.Info("process exited gracefully after SIGTERM",
							zap.Int("pid", pid))
					}
					return // Exit supervisor loop

				// Grace period expired - force kill
				case <-timer.C:
					log.Warn("graceful shutdown timeout exceeded, sending SIGKILL",
						zap.Int("pid", pid),
						zap.Duration("timeout", gracefulTimeout))

					// Step 2: Send SIGKILL for forceful termination
					if err := cmd.Process.Kill(); err != nil {
						// Process may have exited just before SIGKILL
						log.Warn("failed to send SIGKILL (process may have already exited)",
							zap.Int("pid", pid),
							zap.Error(err))
					} else {
						log.Info("SIGKILL sent to process", zap.Int("pid", pid))
					}

					// Wait for the process to be reaped by the OS
					// This should be nearly instantaneous after SIGKILL
					err := <-doneCh
					log.Info("process forcefully terminated",
						zap.Int("pid", pid),
						zap.Error(err))
					return // Exit supervisor loop
				}
			}
		}
	}
}

// managedProcess encapsulates supervision state for one process
type managedProcess struct {
	id              int64              // Unique identifier; IDs are managed externally (no auto-increment)
	argv            []string           // Command-line arguments
	restartCooldown time.Duration      // Delay between restarts
	ctx             context.Context    // Cancellation signal
	cancel          context.CancelFunc // Trigger shutdown
}

func newManagedProcess(id int64, argv []string, restartCooldown time.Duration) *managedProcess {
	ctx, cancel := context.WithCancel(context.Background())
	return &managedProcess{
		id:              id,
		argv:            argv,
		restartCooldown: restartCooldown,
		ctx:             ctx,
		cancel:          cancel,
	}
}
