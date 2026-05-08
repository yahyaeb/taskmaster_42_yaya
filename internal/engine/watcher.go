package engine

import (
	"context"
	"fmt"
	"taskmaster/internal/config"
	"time"
)

// RetryConfig defines the retry behavior for process monitoring.
type RetryConfig struct {
	MaxRetries    int
	RetryDelay    time.Duration
	ExpectedCodes map[int]bool // exit codes that should not trigger retry
}

// ProcessWatcher monitors a process and applies retry logic on failure.
type ProcessWatcher struct {
	Executor         ProcessExecutor
	Config           RetryConfig
	Strategy         RetryStrategy // optional, if nil uses config.ExpectedCodes
	OnProcessStarted func(pid int) // optional callback when a process starts
	OnBackoff func(attempt int) 
}

// NewProcessWatcher creates a new ProcessWatcher with the given executor and retry config.
func NewProcessWatcher(executor ProcessExecutor, config RetryConfig) *ProcessWatcher {
	strategy := RetryStrategyFromExpectedCodes(config.ExpectedCodes)
	return &ProcessWatcher{
		Executor:         executor,
		Config:           config,
		Strategy:         strategy,
		OnProcessStarted: nil,
	}
}

// NewProcessWatcherWithStrategy creates a new ProcessWatcher with explicit retry strategy.
func NewProcessWatcherWithStrategy(executor ProcessExecutor, strategy RetryStrategy, maxRetries int, retryDelay time.Duration) *ProcessWatcher {
	return &ProcessWatcher{
		Executor: executor,
		Config: RetryConfig{
			MaxRetries: maxRetries,
			RetryDelay: retryDelay,
		},
		Strategy:         strategy,
		OnProcessStarted: nil,
	}
}

// ProcessSpawner is a function that spawns a process and returns it or an error.
// Used to abstract the spawning mechanism (Spawn vs Start).
type ProcessSpawner func(ctx context.Context) (*Process, error)

// run is the core retry loop shared by Run.
// It accepts a spawner function to abstract the spawn mechanism (Spawn vs Start).
func (pw *ProcessWatcher) run(ctx context.Context, spawner ProcessSpawner) (ExitCode, error) {
	var lastExit ExitCode
	var lastErr error

	for attempt := 0; attempt <= pw.Config.MaxRetries; attempt++ {
		if attempt > 0 {
			if pw.OnBackoff != nil {
				pw.OnBackoff(attempt)
			}
			select {
			case <-time.After(pw.Config.RetryDelay):
			case <-ctx.Done():
				return lastExit, ctx.Err()
			}
		}

		process, err := spawner(ctx)
		if err != nil {
			lastErr = fmt.Errorf("spawn failed: %w", err)
			continue
		}
		if pw.OnProcessStarted != nil {
			pw.OnProcessStarted(process.PID)
		}

		exitCode, err := pw.Executor.Wait(ctx, process.PID)
		lastExit = exitCode
		lastErr = err

		if err != nil {
			lastErr = fmt.Errorf("wait failed: %w", err)
			continue
		}

		// Use RetryStrategy if available, otherwise fall back to ExpectedCodes
		if pw.Strategy != nil {
			if !pw.Strategy.ShouldRestart(int(exitCode), attempt) {
				return exitCode, nil
			}
		} else {
			// Legacy logic using ExpectedCodes
			if pw.Config.ExpectedCodes != nil && pw.Config.ExpectedCodes[int(exitCode)] {
				return exitCode, nil
			}
			if pw.Config.ExpectedCodes == nil && exitCode == 0 {
				return exitCode, nil
			}
		}

		// Retry on failure
		if attempt < pw.Config.MaxRetries {
			continue
		}
	}

	if lastErr != nil {
		return lastExit, lastErr
	}

	return lastExit, fmt.Errorf("process exited with code %d after %d retries", lastExit, pw.Config.MaxRetries)
}


// Run starts monitoring a process using ProcessExecutor with full configuration.
func (pw *ProcessWatcher) Run(ctx context.Context, spec config.ConfigSpec) (ExitCode, error) {	
	return pw.run(ctx, func(ctx context.Context) (*Process, error) {
		return pw.Executor.Start(ctx, spec)
	})
}
