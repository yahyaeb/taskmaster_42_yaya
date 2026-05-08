package engine

import (
	"context"
	"fmt"
	"os"
	"time"
)

// ProcessStopper is responsible for gracefully stopping a process,
// escalating to SIGKILL if it doesn't terminate within the timeout.
type ProcessStopper struct {
	handler  SignalHandler
	executor ProcessExecutor
	timeout  time.Duration
}

// NewProcessStopper creates a new ProcessStopper with the given signal handler, executor, and timeout.
func NewProcessStopper(handler SignalHandler, executor ProcessExecutor, timeout time.Duration) *ProcessStopper {
	return &ProcessStopper{
		handler:  handler,
		executor: executor,
		timeout:  timeout,
	}
}

// Stop sends SIGTERM to the process, waits for the timeout, and escalates to SIGKILL if needed.
func (ps *ProcessStopper) Stop(p *Process) error {
	// Send SIGTERM
	if err := ps.handler.Send(p, os.Interrupt); err != nil {
		return fmt.Errorf("failed to send SIGTERM: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), ps.timeout)
	defer cancel()

	// Wait for process to terminate or timeout
	type result struct {
		code ExitCode
		err  error
	}
	done := make(chan result, 1)
	go func() {
		code, err := ps.executor.Wait(ctx, p.PID)
		done <- result{code: code, err: err}
	}()

	select {
	case <-ctx.Done():
		// Timeout reached, escalate to SIGKILL
		if err := ps.handler.Send(p, os.Kill); err != nil {
			return fmt.Errorf("failed to send SIGKILL: %w", err)
		}
		return nil
	case res := <-done:
		// Process terminated or Wait failed
		return res.err
	}
}
