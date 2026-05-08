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
// It manages file handles for stdout/stderr redirection and ensures they are closed.
type OsProcessExecutor struct {
	mu    sync.Mutex
	files map[int][]io.Closer // PID -> list of open files to close
}

// NewOsProcessExecutor creates a new OsProcessExecutor.
func NewOsProcessExecutor() *OsProcessExecutor {
	return &OsProcessExecutor{
		files: make(map[int][]io.Closer),
	}
}

// Spawn starts a new process with the given command and arguments.
// This simple version does not handle environment, working directory, or file redirection.
// For full configuration, use SpawnWithConfig.
func (e *OsProcessExecutor) Spawn(ctx context.Context, cmd string, args []string) (*Process, error) {
	c := exec.CommandContext(ctx, cmd, args...)
	err := c.Start()
	if err != nil {
		return nil, fmt.Errorf("spawn failed: %w", err)
	}
	return &Process{PID: c.Process.Pid}, nil
}

// SpawnWithConfig starts a new process with full configuration (env, workdir, stdio, etc.).
func (e *OsProcessExecutor) SpawnWithConfig(ctx context.Context, spec config.ConfigSpec) (*Process, error) {
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

	err = cmd.Start()
	if err != nil {
		// Close any opened files on error
		e.closeFiles(cmd)
		return nil, fmt.Errorf("spawn failed: %w", err)
	}

	pid := cmd.Process.Pid

	// Collect file handles that need to be closed after process exits
	var files []io.Closer
	if f, ok := cmd.Stdout.(*os.File); ok && f != nil {
		files = append(files, f)
	}
	if f, ok := cmd.Stderr.(*os.File); ok && f != nil {
		// Check if stderr is the same file as stdout
		if sf, ok := cmd.Stdout.(*os.File); ok && sf == f {
			// same file, skip adding duplicate
		} else {
			files = append(files, f)
		}
	}
	if len(files) > 0 {
		e.mu.Lock()
		e.files[pid] = files
		e.mu.Unlock()
	}

	return &Process{PID: pid}, nil
}

// Wait blocks until the process with the given PID exits.
// Closes any associated file handles after the process exits.
func (e *OsProcessExecutor) Wait(ctx context.Context, pid int) (ExitCode, error) {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return -1, fmt.Errorf("find process failed: %w", err)
	}
	state, err := proc.Wait()
	if err != nil {
		return -1, fmt.Errorf("wait failed: %w", err)
	}

	// Close files associated with this PID
	e.closeFilesForPID(pid)

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

// closeFilesForPID closes and removes file handles for the given PID.
func (e *OsProcessExecutor) closeFilesForPID(pid int) {
	e.mu.Lock()
	files, ok := e.files[pid]
	if ok {
		delete(e.files, pid)
	}
	e.mu.Unlock()

	if ok {
		for _, f := range files {
			f.Close()
		}
	}
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
