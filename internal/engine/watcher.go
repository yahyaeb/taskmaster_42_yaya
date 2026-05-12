package engine

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"taskmaster/internal/bus"
	"taskmaster/internal/config"
	"time"
)

// ProcessWatcher monitors a single process execution.
// It runs the process once, sends status updates, and returns the exit code.
// Retry logic is handled by the caller (Manager).
type ProcessWatcher struct {
	Executor ProcessExecutor
}

// NewProcessWatcher creates a new ProcessWatcher with the given executor.
func NewProcessWatcher(executor ProcessExecutor) *ProcessWatcher {
	return &ProcessWatcher{Executor: executor}
}

// Run starts a process and monitors it until completion or context cancellation.
// Returns the exit code when the process exits, or error if context cancelled.
// Status updates (STARTING, RUNNING) are sent to the updates channel.
func (pw *ProcessWatcher) Run(ctx context.Context, spec config.ConfigSpec, updates chan<- bus.ProcessUpdate) (ExitCode, error) {
	// Start the process
	process, err := pw.Executor.Start(ctx, spec)
	if err != nil {
		updates <- bus.ProcessUpdate{
			Name:   spec.ProcessName,
			Status: bus.FATAL,
		}
		return 0, fmt.Errorf("spawn failed: %w", err)
	}

	// Process started - send STARTING update
	updates <- bus.ProcessUpdate{
		Name:       spec.ProcessName,
		Status:     bus.STARTING,
		Pid:        process.PID,
		Retries:    0,
		HasRetries: true,
		LastStart:  time.Now(),
	}

	// Handle starttime observation window
	starttime := time.Duration(spec.Starttime) * time.Second
	if starttime > 0 {
		timer := time.NewTimer(starttime)
		earlyExit := make(chan struct{})
		var earlyExitOnce sync.Once
		done := make(chan struct{})

		// Poll to check if process dies during starttime
		go func() {
			ticker := time.NewTicker(100 * time.Millisecond)
			defer ticker.Stop()
			for {
				select {
				case <-ticker.C:
					state, err := procState(process.PID)
					if err != nil || state == 'Z' {
						earlyExitOnce.Do(func() { close(earlyExit) })
						return
					}
				case <-done:
					return
				case <-ctx.Done():
					return
				}
			}
		}()

		select {
		case <-earlyExit:
			timer.Stop()
			close(done)
			// Process died during starttime - return without sending RUNNING
			exitCode, _ := pw.Executor.Wait(ctx, process.PID)
			return exitCode, nil

		case <-timer.C:
			close(done)
			// Process survived starttime - now RUNNING
			updates <- bus.ProcessUpdate{
				Name:   spec.ProcessName,
				Status: bus.RUNNING,
				Pid:    process.PID,
			}

		case <-ctx.Done():
			timer.Stop()
			close(done)
			return 0, ctx.Err()
		}
	} else {
		// No starttime required - mark as RUNNING immediately
		updates <- bus.ProcessUpdate{
			Name:   spec.ProcessName,
			Status: bus.RUNNING,
			Pid:    process.PID,
		}
	}

	// Wait for process to exit
	exitCode, err := pw.Executor.Wait(ctx, process.PID)
	if err != nil {
		return exitCode, err
	}

	return exitCode, nil
}

func procState(pid int) (byte, error) {
	statPath := fmt.Sprintf("/proc/%d/stat", pid)
	data, err := os.ReadFile(statPath)
	if err != nil {
		return 0, err
	}
	contents := string(data)
	idx := strings.LastIndex(contents, ") ")
	if idx == -1 || idx+2 >= len(contents) {
		return 0, fmt.Errorf("parse /proc stat for pid %d", pid)
	}
	return contents[idx+2], nil
}
