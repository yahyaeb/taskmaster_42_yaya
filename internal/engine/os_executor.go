package engine

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"taskmaster/internal/config"
)

// OsProcessExecutor implements ConfigurableProcessExecutor using os/exec.
// It stores a reference to the actual *os.Process object for each started process
// to ensure Wait() waits on the correct process even if the PID is reused by the OS.
type OsProcessExecutor struct {
	// processMap stores PID -> *os.Process to prevent PID-reuse confusion
	processMap sync.Map // map[int]*os.Process
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

	// If context is provided, create a context-aware command while preserving all configuration
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
	oldMask := syscall.Umask(spec.Umask)
	defer func() {
		syscall.Umask(oldMask)
	}()

	err = cmd.Start()
	if err != nil {
		// Close any opened files on error
		e.closeFiles(cmd)
		return nil, fmt.Errorf("start failed: %w", err)
	}

	pid := cmd.Process.Pid

	// Store the actual process object to prevent PID-reuse issues when Wait() is called later
	e.processMap.Store(pid, cmd.Process)

	// Note: Files are now closed by the caller (Watchdog) after Wait() returns
	// No need to track them in a map

	return &Process{PID: pid}, nil
}

// Wait blocks until the process with the given PID exits.
// Uses the stored process object to ensure we wait on the correct process instance,
// even if the OS has reused the PID for a different process.
func (e *OsProcessExecutor) Wait(ctx context.Context, pid int) (ExitCode, error) {
	// Try to find the stored process object for this PID
	var proc *os.Process
	if stored, ok := e.processMap.Load(pid); ok {
		proc = stored.(*os.Process)
	} else {
		// Fallback: if not found in map, try to find by PID
		// This can happen if the process was started before this executor was created
		var err error
		proc, err = os.FindProcess(pid)
		if err != nil {
			return -1, fmt.Errorf("find process failed: %w", err)
		}
	}

	// Check for cancellation before waiting
	select {
	case <-ctx.Done():
		return -1, ctx.Err()
	default:
	}

	// Use a goroutine to wait on the process in parallel with context
	done := make(chan struct {
		exitCode ExitCode
		err      error
	}, 1)

	go func() {
		state, err := proc.Wait()
		// Clean up the process map entry after waiting
		defer e.processMap.Delete(pid)

		if err != nil {
			done <- struct {
				exitCode ExitCode
				err      error
			}{-1, fmt.Errorf("wait failed: %w", err)}
			return
		}

		exitCode := 0
		if !state.Success() {
			if exitStatus, ok := state.Sys().(syscall.WaitStatus); ok {
				exitCode = exitStatus.ExitStatus()
			}
		}
		done <- struct {
			exitCode ExitCode
			err      error
		}{ExitCode(exitCode), nil}
	}()

	// Wait for either the process or context cancellation
	select {
	case result := <-done:
		return result.exitCode, result.err
	case <-ctx.Done():
		return -1, ctx.Err()
	}
}

// Signal sends a signal to the process with the given PID.
func (e *OsProcessExecutor) Signal(ctx context.Context, pid int, signal interface{}) error {
	// Try to find the stored process object first
	var proc *os.Process
	if stored, ok := e.processMap.Load(pid); ok {
		proc = stored.(*os.Process)
	} else {
		// Fallback: find by PID if not in map
		var err error
		proc, err = os.FindProcess(pid)
		if err != nil {
			return fmt.Errorf("find process failed: %w", err)
		}
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
