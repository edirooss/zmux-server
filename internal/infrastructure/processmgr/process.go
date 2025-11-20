//go:build linux

package processmgr

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"go.uber.org/zap"
)

// process encapsulates a supervised external command.
// Features:
//   - race-free pipe setup (stdin/stdout/stderr)
//   - stdout-based readiness signaling
//   - continuous pipe supervision with failure detection
//   - deterministic teardown (SIGTERM → grace → SIGKILL)
//   - idempotent Start / Enter / Close lifecycle
//
// Canonical usage:
//
//	p → Start() → <-Ready() → Enter() → interact → <-Done()
//
// Enter() and Close() are valid only after Start() completes.
type process struct {
	log    *zap.Logger
	logBuf *logBuffer

	cmd    *exec.Cmd
	stdout io.ReadCloser
	stderr io.ReadCloser
	stdin  io.WriteCloser

	// One-shot readiness signal (closed only on real readiness).
	ready     chan struct{}
	readyOnce sync.Once

	// Closed after the process is fully reaped.
	done      chan struct{}
	closeOnce sync.Once
	startOnce sync.Once

	started atomic.Bool
	cmd_pid atomic.Int64

	// Protects mutable state during lifecycle transitions.
	mu sync.Mutex
}

// newProcess constructs a process wrapper around exec.Cmd.
//
// It performs early pipe allocation and applies Linux-specific attributes:
//   - Setpgid: isolates the child into its own process group
//   - Pdeathsig: ensures child receives SIGKILL if the parent dies
//
// Returns (nil, false) on invalid parameters or pipe setup errors.
func newProcess(log *zap.Logger, logBuf *logBuffer, env, argv []string) (*process, bool) {
	if log == nil || logBuf == nil || len(argv) == 0 {
		log.Error("NewProcess: invalid parameters")
		return nil, false
	}

	cmd := exec.Command(argv[0], argv[1:]...)
	stdout, stderr, stdin, err := pipes(cmd)
	if err != nil {
		log.Error("pipe initialization failure", zap.Error(err))
		return nil, false
	}

	cmd.Env = env
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid:   true,
		Pdeathsig: syscall.SIGKILL,
	}

	return &process{
		log:    log,
		logBuf: logBuf,
		cmd:    cmd,
		stdout: stdout,
		stderr: stderr,
		stdin:  stdin,
		ready:  make(chan struct{}),
		done:   make(chan struct{}),
	}, true
}

// Start launches the command exactly once. On success:
//
//   - background supervisors begin consuming stdout/stderr
//   - Ready() may eventually fire
//   - Done() fires when the process is reaped
func (p *process) Start() bool {
	ok := false

	p.startOnce.Do(func() {
		p.mu.Lock()
		defer p.mu.Unlock()

		if err := p.cmd.Start(); err != nil {
			p.log.Error("failed to start command", zap.Error(err))
			return
		}

		pid := p.cmd.Process.Pid

		ok = true
		p.started.Store(true)
		p.cmd_pid.Store(int64(pid))

		p.log.Info("process started", zap.Int("cmd_pid", pid))
		go p.supervise()
	})

	return ok
}

const (
	Stdout string = "stdout"
	Stderr string = "stderr"
)

// supervise orchestrates the complete end-to-end lifecycle:
//
//   - multiplexes stdout/stderr readers
//   - observes early pipe teardown events
//   - differentiates genuine pipe faults from exit-transition races
//   - triggers controlled shutdown when invariants are violated
//   - performs a single Wait() to reap the child
//   - closes stdin post-exit and fires the Done() signal
//
// On Linux, pipe closure frequently precedes actual process exit due to
// user-space teardown ordering. A bounded grace interval is applied to
// avoid misclassifying such events.
func (p *process) supervise() {
	pipeDone := make(chan string, 2)

	// Launch pipe drains.
	go func() {
		p.handleStdout()
		pipeDone <- Stdout
	}()
	go func() {
		p.handleStderr()
		pipeDone <- Stderr
	}()

	// First pipe completes — may indicate a crash, panic, or orderly teardown.
	first := <-pipeDone
	p.log.Debug("first pipe ended", zap.String("pipe", first))

	// Allow a brief window for the second pipe to close as part of normal exit.
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	select {
	case second := <-pipeDone:
		p.log.Debug("second pipe ended", zap.String("pipe", second))

		// Both pipes have terminated. Since pipe closure can precede actual
		// process termination on Linux, spawn a watcher goroutine that waits
		// for natural exit. If the process has not exited within the grace
		// window, we classify the state as unhealthy and force shutdown.
		go func() {
			select {
			case <-p.done:
				// Natural exit occurred within the grace window.
				return

			case <-time.After(250 * time.Millisecond):
				// Pipes closed but process not fully terminated → unhealthy.
				p.Close()
			}
		}()

	case <-ctx.Done():
		// Second pipe failed to close within grace → likely stalled or abnormal.
		p.log.Warn("second pipe did not close in grace interval; issuing shutdown")
		p.Close()

		// Drain second pipe when it eventually terminates.
		second := <-pipeDone
		p.log.Debug("second pipe ended", zap.String("pipe", second))
	}

	// Reap the child once and record exit metadata.
	p.mu.Lock()
	defer p.mu.Unlock()

	if err := p.cmd.Wait(); err != nil {
		var eerr *exec.ExitError
		if errors.As(err, &eerr) {
			status := eerr.ProcessState.Sys().(syscall.WaitStatus)
			p.log.Info("process exited with error status",
				zap.Int("exit_code", status.ExitStatus()),
				zap.Bool("signaled", status.Signaled()),
				zap.String("signal", status.Signal().String()))
		} else {
			p.log.Error("failed to wait for process", zap.Error(err))
		}
	} else {
		p.log.Info("process exited cleanly")
	}

	// Final stdin cleanup.
	if p.stdin != nil {
		_ = p.stdin.Close()
		p.stdin = nil
	}

	close(p.done)
}

// handleStdout streams stdout, detects readiness markers, and appends all
// lines into the shared log buffer. Scanner I/O failures are logged.
func (p *process) handleStdout() {
	sc := bufio.NewScanner(p.stdout)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)

	const readiness = "Press ENTER to continue or Ctrl+C to cancel."

	for sc.Scan() {
		line := strings.TrimRight(sc.Text(), "\r\n")
		if line == readiness {
			p.readyOnce.Do(func() { close(p.ready) })
			p.log.Info("readiness signal received")
			continue
		}
		p.logBuf.Append(line)
	}

	if err := sc.Err(); err != nil {
		p.log.Error("stdout scanner failure", zap.Error(err))
	}
}

// handleStderr streams stderr directly into the shared log buffer and logs
// scanner failures. No readiness semantics are associated with stderr.
func (p *process) handleStderr() {
	sc := bufio.NewScanner(p.stderr)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)

	for sc.Scan() {
		p.logBuf.Append(sc.Text())
	}

	if err := sc.Err(); err != nil {
		p.log.Error("stderr scanner failure", zap.Error(err))
	}
}

// Enter sends a newline to the child's stdin, transitioning it past the
// readiness barrier. Valid only after Start() and before exit completion.
func (p *process) Enter() error {
	if !p.started.Load() {
		return fmt.Errorf("cannot Enter: process not started")
	}

	if !p.mu.TryLock() {
		return fmt.Errorf("cannot Enter: process busy")
	}
	defer p.mu.Unlock()

	select {
	case <-p.done:
		return fmt.Errorf("cannot Enter: process already exited")
	default:
	}

	if p.stdin == nil {
		return fmt.Errorf("stdin not available")
	}

	if _, err := io.WriteString(p.stdin, "\n"); err != nil {
		p.log.Warn("failed to write ENTER", zap.Error(err))
		return err
	}

	p.log.Info("ENTER written to stdin")
	return nil
}

func (p *process) Ready() <-chan struct{} { return p.ready }
func (p *process) Done() <-chan struct{}  { return p.done }

// Close initiates deterministic shutdown:
//
//   - closes stdin to interrupt blocking reads in the child
//   - sends SIGTERM to the process group
//   - escalates to SIGKILL after a fixed timeout if still alive
//
// Close() is idempotent and concurrency-safe.
func (p *process) Close() {
	p.closeOnce.Do(func() {
		go func() {
			if !p.started.Load() {
				p.log.Warn("Close() called before Start(); ignored")
				return
			}

			// If process is already done, abort early.
			select {
			case <-p.done:
				p.log.Debug("Close() called after Done(); ignored")
				return
			default:
			}

			pid := int(p.cmd_pid.Load())
			p.log.Info("sending SIGTERM", zap.Int("cmd_pid", pid))

			if err := syscall.Kill(-pid, syscall.SIGTERM); err != nil {
				p.log.Warn("SIGTERM failed", zap.Error(err), zap.Int("cmd_pid", pid))
			} else {
				p.log.Info("SIGTERM sent successfully", zap.Int("cmd_pid", pid))
			}

			timer := time.NewTimer(3 * time.Second)
			defer timer.Stop()

			select {
			case <-p.done:
				p.log.Info("process exited gracefully", zap.Int("cmd_pid", pid))
				return

			case <-timer.C:
				p.log.Warn("grace timeout expired; sending SIGKILL", zap.Int("cmd_pid", pid))
				if err := syscall.Kill(-pid, syscall.SIGKILL); err != nil {
					p.log.Error("SIGKILL failed", zap.Error(err), zap.Int("cmd_pid", pid))
				} else {
					p.log.Info("SIGKILL sent successfully", zap.Int("cmd_pid", pid))
				}
			}
		}()
	})
}

// pipes prepares stdin, stdout and stderr for exec.Cmd.
//
// Overview for new developers:
//
//   - StdoutPipe(), StderrPipe(), and StdinPipe() each create an os.Pipe().
//   - The end returned (read side for stdout/stderr, write side for stdin)
//     is for the caller; the opposite end is kept by exec.Cmd.
//   - exec.Cmd does NOT own these pipes until Start() succeeds.
//   - If Start() fails, exec.Cmd will close all pipe ends automatically.
//   - Before Start(), caller must close any pipes created during setup errors.
//
// This helper ensures atomicity: if any pipe fails, all previously-created
// pipes are closed and no file descriptors leak.
func pipes(cmd *exec.Cmd) (io.ReadCloser, io.ReadCloser, io.WriteCloser, error) {
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, nil, nil, fmt.Errorf("stdout pipe creation failure: %w", err)
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		_ = stdout.Close()
		return nil, nil, nil, fmt.Errorf("stderr pipe creation failure: %w", err)
	}

	stdin, err := cmd.StdinPipe()
	if err != nil {
		_ = stdout.Close()
		_ = stderr.Close()
		return nil, nil, nil, fmt.Errorf("stdin pipe creation failure: %w", err)
	}

	return stdout, stderr, stdin, nil
}
