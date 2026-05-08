package engine

import (
	"context"
	"io"
	"taskmaster/internal/config"
)

// ExitCode represents the exit code of a process.
type ExitCode int

// Process represents a spawned process with its metadata.
type Process struct {
	PID    int
	Stdin  io.WriteCloser
	Stdout io.ReadCloser
	Stderr io.ReadCloser
}

// ProcessExecutor defines the interface for spawning and managing processes.
type ProcessExecutor interface {
	// Spawn starts a new process with the given command and arguments.
	// Returns a Process or an error if the process cannot be started.
	Spawn(ctx context.Context, cmd string, args []string) (*Process, error)

	// Wait blocks until the process with the given PID exits.
	// Returns the exit code of the process.
	Wait(ctx context.Context, pid int) (ExitCode, error)

	// Signal sends a signal to the process with the given PID.
	// The signal parameter should be a valid OS signal.
	Signal(ctx context.Context, pid int, signal interface{}) error
}

// ConfigurableProcessExecutor extends ProcessExecutor with support for full configuration.
type ConfigurableProcessExecutor interface {
	ProcessExecutor
	// SpawnWithConfig starts a new process with full configuration (env, workdir, stdio, etc.).
	// Returns a Process or an error if the process cannot be started.
	SpawnWithConfig(ctx context.Context, spec config.ConfigSpec) (*Process, error)
}
