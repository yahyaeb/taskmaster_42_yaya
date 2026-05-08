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
	// Returns a Process or an error if the process cannot be started.
	Start(ctx context.Context, spec config.ConfigSpec) (*Process, error)

	// Wait blocks until the process with the given PID exits.
	// Returns the exit code of the process.
	Wait(ctx context.Context, pid int) (ExitCode, error)

	// Signal sends a signal to the process with the given PID.
	// The signal parameter should be a valid OS signal.
	Signal(ctx context.Context, pid int, signal interface{}) error
}
