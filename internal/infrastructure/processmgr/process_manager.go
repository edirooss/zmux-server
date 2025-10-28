//go:build linux

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

	timer := time.NewTimer(0)
	defer timer.Stop()

	for {
		select {
		case <-proc.ctx.Done():
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			log.Info("supervisor shutdown during restart cooldown",
				zap.String("reason", proc.ctx.Err().Error()))
			return

		case <-timer.C:
			log.Info("spawning process", zap.Duration("restart_cooldown", proc.restartCooldown))

			cmd := exec.Command(proc.argv[0], proc.argv[1:]...)
			cmd.SysProcAttr = &syscall.SysProcAttr{
				Pdeathsig: syscall.SIGKILL, // Linux-only
				Setpgid:   true,            // new process group so we can signal the group
			}
			cmd.Env = mng.env

			stderrPipe, err := cmd.StderrPipe()
			if err != nil {
				log.Error("failed to create stderr pipe", zap.Error(err))
				timer.Reset(proc.restartCooldown)
				continue
			}

			if err := cmd.Start(); err != nil {
				log.Error("failed to spawn process", zap.Error(err), zap.String("command", proc.argv[0]))
				timer.Reset(proc.restartCooldown)
				continue
			}

			pid := cmd.Process.Pid
			log.Info("process started successfully", zap.Int("pid", pid))

			// Drain stderr
			go func(pid int) {
				scanner := bufio.NewScanner(stderrPipe)
				scanner.Buffer(make([]byte, 64*1024), 1024*1024)
				for scanner.Scan() {
					logBuf.Append(scanner.Text())
				}
				if err := scanner.Err(); err != nil {
					logBuf.Append(err.Error())
					log.Error("stderr reader exited abnormally", zap.Int("pid", pid), zap.Error(err))
					return
				}
				log.Info("stderr reader exited normally", zap.Int("pid", pid))
			}(pid)

			// Fresh doneCh per spawn
			doneCh := make(chan error, 1)
			go func() {
				doneCh <- cmd.Wait()
				close(doneCh)
			}()

			select {
			case err := <-doneCh:
				if err != nil {
					if exitErr, ok := err.(*exec.ExitError); ok {
						log.Warn("process exited abnormally",
							zap.Int("pid", pid),
							zap.Int("exit_code", exitErr.ExitCode()),
							zap.Error(err))
					} else {
						log.Warn("process wait failed", zap.Int("pid", pid), zap.Error(err))
					}
				} else {
					log.Info("process exited normally", zap.Int("pid", pid))
				}
				timer.Reset(proc.restartCooldown)
				continue

			case <-proc.ctx.Done():
				log.Info("supervisor shutdown requested, initiating graceful shutdown sequence",
					zap.Int("pid", pid), zap.String("reason", proc.ctx.Err().Error()))

				// Graceful: SIGTERM the *group*
				_ = syscall.Kill(-pid, syscall.SIGTERM)
				log.Info("SIGTERM sent to process group", zap.Int("pgid", pid))

				t := time.NewTimer(3 * time.Second)
				defer t.Stop()

				select {
				case err := <-doneCh:
					if !t.Stop() {
						select {
						case <-t.C:
						default:
						}
					}
					if err != nil {
						log.Info("process terminated after SIGTERM", zap.Int("pid", pid), zap.Error(err))
					} else {
						log.Info("process exited gracefully after SIGTERM", zap.Int("pid", pid))
					}
					return

				case <-t.C:
					log.Warn("graceful shutdown timeout exceeded, sending SIGKILL",
						zap.Int("pid", pid), zap.Duration("timeout", 3*time.Second))
					_ = syscall.Kill(-pid, syscall.SIGKILL)
					err := <-doneCh
					log.Info("process forcefully terminated", zap.Int("pid", pid), zap.Error(err))
					return
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
