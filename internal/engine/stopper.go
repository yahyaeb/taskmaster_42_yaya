package engine

import (
	"fmt"
	"os"
	"syscall"
	"time"
)

// SignalFromString converts a signal name string to os.Signal.
// Supports: TERM, HUP, INT, QUIT, KILL, USR1, USR2
func SignalFromString(sig string) (os.Signal, error) {
	switch sig {
	case "TERM":
		return syscall.SIGTERM, nil
	case "HUP":
		return syscall.SIGHUP, nil
	case "INT":
		return os.Interrupt, nil // SIGINT
	case "QUIT":
		return syscall.SIGQUIT, nil
	case "KILL":
		return os.Kill, nil // SIGKILL
	case "USR1":
		return syscall.SIGUSR1, nil
	case "USR2":
		return syscall.SIGUSR2, nil
	default:
		return nil, fmt.Errorf("unsupported signal: %s", sig)
	}
}

// ProcessStopper is responsible for gracefully stopping a process,
// escalating to SIGKILL if it doesn't terminate within the timeout.
type ProcessStopper struct {
	handler     SignalHandler
	executor    ProcessExecutor
	timeout     time.Duration
	stopsignal  os.Signal
}

// NewProcessStopper creates a new ProcessStopper with the given signal handler, executor, timeout, and stop signal.
func NewProcessStopper(handler SignalHandler, executor ProcessExecutor, timeout time.Duration, sig os.Signal) *ProcessStopper {
	return &ProcessStopper{
		handler:    handler,
		executor:   executor,
		timeout:    timeout,
		stopsignal: sig,
	}
}

// Stop sends the configured stop signal to the process.
// The watchdog is responsible for waiting for the process to exit.
// This method returns after sending the signal; the Watchdog will handle the wait and escalation to SIGKILL.
func (ps *ProcessStopper) Stop(p *Process) error {
	// Send the configured stop signal
	if err := ps.handler.Send(p, ps.stopsignal); err != nil {
		return fmt.Errorf("failed to send signal: %w", err)
	}
	return nil
}
