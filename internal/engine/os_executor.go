package engine

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"syscall"
	"taskmaster/internal/config"
)

// umaskLock prevents races when changing process umask during fork.
// syscall.ForkLock is used by os/exec to serialize fork/exec operations.
var umaskLock = &syscall.ForkLock

// OsProcessExecutor implements ConfigurableProcessExecutor using os/exec.
type OsProcessExecutor struct {
	// Note: No mutex needed - each process's files are managed by its Watchdog goroutine
}

// NewOsProcessExecutor creates a new OsProcessExecutor.
func NewOsProcessExecutor() *OsProcessExecutor {
	return &OsProcessExecutor{}
}

func (e *OsProcessExecutor) Start(ctx context.Context, spec config.ConfigSpec) (*Process, error) {
	builder := &CommandBuilder{}
	cmd, err := builder.BuildCommand(spec)
	if err != nil {
		return nil, fmt.Errorf("build command failed: %w", err)
	}

	// If context is provided, replace with context-aware command while preserving configuration
	if ctx != nil {
		newCmd := exec.CommandContext(ctx, cmd.Path, cmd.Args[1:]...)
		newCmd.Dir = cmd.Dir
		newCmd.Env = cmd.Env
		newCmd.Stdin = cmd.Stdin
		newCmd.Stdout = cmd.Stdout
		newCmd.Stderr = cmd.Stderr
		newCmd.ExtraFiles = cmd.ExtraFiles
		cmd = newCmd
	}

	// Apply umask before cmd.Start (always apply, even if 0)
	umaskLock.Lock()
	oldMask := syscall.Umask(spec.Umask)

	err = cmd.Start()

	// Restore umask immediately after fork/exec
	syscall.Umask(oldMask)
	umaskLock.Unlock()
	if err != nil {
		// Close any opened files on error
		e.closeFiles(cmd)
		return nil, fmt.Errorf("start failed: %w", err)
	}

	pid := cmd.Process.Pid

	// Note: Files are now closed by the caller (Watchdog) after Wait() returns
	// No need to track them in a map

	return &Process{PID: pid}, nil
}

// Wait blocks until the process with the given PID exits.
func (e *OsProcessExecutor) Wait(ctx context.Context, pid int) (ExitCode, error) {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return -1, fmt.Errorf("find process failed: %w", err)
	}
	state, err := proc.Wait()
	if err != nil {
		return -1, fmt.Errorf("wait failed: %w", err)
	}

	exitCode := 0
	if !state.Success() {
		if exitStatus, ok := state.Sys().(syscall.WaitStatus); ok {
			exitCode = exitStatus.ExitStatus()
		}
	}
	return ExitCode(exitCode), nil
}

// Signal sends a signal to the process with the given PID.
func (e *OsProcessExecutor) Signal(ctx context.Context, pid int, signal interface{}) error {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("find process failed: %w", err)
	}
	sig, ok := signal.(os.Signal)
	if !ok {
		return fmt.Errorf("invalid signal type")
	}
	return proc.Signal(sig)
}

// closeFiles closes file handles attached to an exec.Cmd.
func (e *OsProcessExecutor) closeFiles(cmd *exec.Cmd) {
	if f, ok := cmd.Stdout.(io.Closer); ok && f != nil {
		f.Close()
	}
	if f, ok := cmd.Stderr.(io.Closer); ok && f != nil {
		// Avoid closing same file twice if stdout and stderr point to same file
		if sf, ok := cmd.Stdout.(*os.File); ok {
			if tf, ok := f.(*os.File); ok && sf == tf {
				return
			}
		}
		f.Close()
	}
}
