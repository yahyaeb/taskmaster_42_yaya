package engine

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
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
	Strategy         RetryStrategyFunc // optional, if nil uses config.ExpectedCodes
	OnProcessStarted func(pid int) // optional callback when a process starts
	OnProcessRunning func(pid int) // optional callback when process completes starttime
	OnBackoff        func(attempt int)
	OnSpawnFailed    func(attempt int)
	OnStarting       func() // optional callback before respawn (for retry flow)
	StarttimeSec     int    // seconds process must stay alive to be considered started
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
func NewProcessWatcherWithStrategy(executor ProcessExecutor, strategy RetryStrategyFunc, maxRetries int, retryDelay time.Duration) *ProcessWatcher {
	return &ProcessWatcher{
		Executor: executor,
		Config: RetryConfig{
			MaxRetries: maxRetries,
			RetryDelay: retryDelay,
		},
		Strategy:         strategy,
		OnProcessStarted: nil,
		OnProcessRunning: nil,
		OnStarting:       nil,
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
			pw.doBackoff(attempt)
			select {
			case <-time.After(pw.Config.RetryDelay):
			case <-ctx.Done():
				return lastExit, ctx.Err()
			}
		}

		// Notify that we're about to start (for retry flow: RUNNING -> BACKOFF -> STARTING -> RUNNING)
		if attempt > 0 && pw.OnStarting != nil {
			pw.OnStarting()
		}

		exitCode, ok, err := pw.trySpawnAndWait(ctx, spawner, attempt)
		if !ok {
			lastExit = exitCode
			if err != nil {
				lastErr = err
			}
			pw.notifySpawnFailed(attempt + 1)
			continue
		}
		return exitCode, nil
	}

	if lastErr != nil {
		return lastExit, lastErr
	}

	return lastExit, fmt.Errorf("process exited with code %d after %d retries", lastExit, pw.Config.MaxRetries)
}

// trySpawnAndWait attempts to spawn and wait for a process.
// Returns (exitCode, success bool, error). Success is true if the process
// completed without needing a retry (either success or terminal failure).
func (pw *ProcessWatcher) trySpawnAndWait(ctx context.Context, spawner ProcessSpawner, attempt int) (ExitCode, bool, error) {
	process, err := spawner(ctx)
	if err != nil {
		return 0, false, fmt.Errorf("spawn failed: %w", err)
	}

	if pw.OnProcessStarted != nil {
		pw.OnProcessStarted(process.PID)
	}

	// Wait for starttime: process must stay alive for this duration to be considered "started"
	starttime := time.Duration(pw.StarttimeSec) * time.Second
	if starttime > 0 {
		// Wait for process to complete startup or die early
		timer := time.NewTimer(starttime)
		earlyExit := make(chan struct{})
		var earlyExitOnce sync.Once
		signalEarlyExit := func() {
			earlyExitOnce.Do(func() {
				close(earlyExit)
			})
		}
		done := make(chan struct{})

		// Poll to check if process is still alive during starttime window
		// Uses /proc/<pid>/stat to detect zombies without reaping the child.
		go func() {
			ticker := time.NewTicker(100 * time.Millisecond)
			defer ticker.Stop()
			for {
				select {
				case <-ticker.C:
					state, err := procState(process.PID)
					if err != nil || state == 'Z' {
						signalEarlyExit()
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
			// Process died during starttime - consider it a failed start
			exitCode, _ := pw.Executor.Wait(ctx, process.PID)
			return exitCode, false, nil
		case <-timer.C:
			close(done)
			// Process survived starttime - it's now "running"
			if pw.OnProcessRunning != nil {
				pw.OnProcessRunning(process.PID)
			}
		case <-ctx.Done():
			timer.Stop()
			close(done)
			return 0, false, ctx.Err()
		}
	} else if pw.OnProcessRunning != nil {
		// No starttime required, mark as running immediately
		pw.OnProcessRunning(process.PID)
	}

	exitCode, err := pw.Executor.Wait(ctx, process.PID)
	if err != nil {
		return exitCode, false, fmt.Errorf("wait failed: %w", err)
	}

	// Check if we should restart based on exit code
	shouldRestart := pw.shouldRestart(exitCode, attempt)
	if shouldRestart {
		return exitCode, false, nil
	}

	return exitCode, true, nil
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

// shouldRestart determines if the process should be restarted based on exit code
func (pw *ProcessWatcher) shouldRestart(exitCode ExitCode, attempt int) bool {
	if pw.Strategy != nil {
		return pw.Strategy(int(exitCode), attempt)
	}
	// Legacy logic using ExpectedCodes
	if pw.Config.ExpectedCodes != nil {
		return !pw.Config.ExpectedCodes[int(exitCode)]
	}
	return exitCode != 0
}

// doBackoff triggers the backoff callback if configured
func (pw *ProcessWatcher) doBackoff(attempt int) {
	if pw.OnBackoff != nil {
		pw.OnBackoff(attempt)
	}
}

// notifySpawnFailed triggers the spawn failed callback if configured
func (pw *ProcessWatcher) notifySpawnFailed(attempt int) {
	if pw.OnSpawnFailed != nil {
		pw.OnSpawnFailed(attempt)
	}
}


// Run starts monitoring a process using ProcessExecutor with full configuration.
func (pw *ProcessWatcher) Run(ctx context.Context, spec config.ConfigSpec) (ExitCode, error) {	
	return pw.run(ctx, func(ctx context.Context) (*Process, error) {
		return pw.Executor.Start(ctx, spec)
	})
}
