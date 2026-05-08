package internal

import (
	"context"
	"os"
	"testing"
	"time"
)

// mockSignalHandler records signals sent to processes.
type mockSignalHandler struct {
	signals []os.Signal
}

func (m *mockSignalHandler) Send(p *Process, sig os.Signal) error {
	m.signals = append(m.signals, sig)
	return nil
}

// mockProcessExecutor simulates process execution with control over Wait behavior.
type mockProcessExecutor struct {
	waitDelay time.Duration // How long to delay before returning from Wait
	waitChan  chan ExitCode // Channel to control when Wait returns
}

func (m *mockProcessExecutor) Spawn(ctx context.Context, cmd string, args []string) (*Process, error) {
	return nil, nil
}

func (m *mockProcessExecutor) Wait(ctx context.Context, pid int) (ExitCode, error) {
	if m.waitChan != nil {
		select {
		case code := <-m.waitChan:
			return code, nil
		case <-ctx.Done():
			return 0, ctx.Err()
		}
	}

	if m.waitDelay > 0 {
		select {
		case <-time.After(m.waitDelay):
			return 0, nil
		case <-ctx.Done():
			return 0, ctx.Err()
		}
	}

	return 0, nil
}

func (m *mockProcessExecutor) Signal(ctx context.Context, pid int, signal interface{}) error {
	return nil
}

// TestProcessStopperTimeout verifies the SIGTERM → timeout → SIGKILL escalation.
func TestProcessStopperTimeout(t *testing.T) {
	handler := &mockSignalHandler{}
	// Use waitChan set to nil so Wait blocks until context timeout
	executor := &mockProcessExecutor{
		waitDelay: 5 * time.Second, // Long delay ensures timeout is reached
	}

	stopper := NewProcessStopper(handler, executor, 50*time.Millisecond)
	process := &Process{PID: 1234}

	// Execute Stop, which should timeout and escalate to SIGKILL
	err := stopper.Stop(process)

	// Verify SIGTERM was sent first
	if len(handler.signals) < 1 {
		t.Fatalf("expected at least 1 signal, got %d", len(handler.signals))
	}
	if handler.signals[0] != os.Interrupt {
		t.Errorf("first signal should be SIGTERM (os.Interrupt), got %v", handler.signals[0])
	}

	// Verify SIGKILL was sent after timeout
	if len(handler.signals) < 2 {
		t.Fatalf("expected at least 2 signals (SIGTERM + SIGKILL), got %d", len(handler.signals))
	}
	if handler.signals[1] != os.Kill {
		t.Errorf("second signal should be SIGKILL (os.Kill), got %v", handler.signals[1])
	}

	// Verify no error is returned from Stop (SIGKILL send succeeded)
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
}

// TestProcessStopperCleanExit verifies graceful exit without SIGKILL.
func TestProcessStopperCleanExit(t *testing.T) {
	handler := &mockSignalHandler{}
	executor := &mockProcessExecutor{
		waitDelay: 10 * time.Millisecond, // Process exits quickly
	}

	stopper := NewProcessStopper(handler, executor, 500*time.Millisecond)
	process := &Process{PID: 1234}

	err := stopper.Stop(process)

	// Verify SIGTERM was sent
	if len(handler.signals) < 1 {
		t.Fatalf("expected at least 1 signal, got %d", len(handler.signals))
	}
	if handler.signals[0] != os.Interrupt {
		t.Errorf("first signal should be SIGTERM, got %v", handler.signals[0])
	}

	// Verify SIGKILL was NOT sent (process exited gracefully)
	if len(handler.signals) > 1 {
		t.Errorf("expected only SIGTERM, got %d signals", len(handler.signals))
	}

	// Verify no error is returned
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
}
