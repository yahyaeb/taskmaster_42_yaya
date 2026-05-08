package engine

import (
	"fmt"
	"os"
)

// SignalHandler defines the interface for sending signals to processes.
type SignalHandler interface {
	// Send sends a signal to the given process.
	Send(p *Process, sig os.Signal) error
}

// OSSignalHandler is a signal handler implementation for OS signals.
type OSSignalHandler struct{}

// Send sends a signal to the given process using os.FindProcess.
func (h *OSSignalHandler) Send(p *Process, sig os.Signal) error {
	proc, err := os.FindProcess(p.PID)
	if err != nil {
		return fmt.Errorf("find process %d: %w", p.PID, err)
	}
	return proc.Signal(sig)
}
