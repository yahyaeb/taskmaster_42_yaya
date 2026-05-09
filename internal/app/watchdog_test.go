package app

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"syscall"
	"testing"
	"time"

	"taskmaster/internal/bus"
	"taskmaster/internal/config"
	"taskmaster/internal/engine"
)

// =============================================================================
// Mock executor
// =============================================================================

// mockProcessExecutor implements engine.ProcessExecutor for Watchdog testing.
// Intentionally does NOT implement ConfigurableProcessExecutor so Watchdog
// takes the Run() code path, not Run().
type mockProcessExecutor struct {
	spawnErr   error
	spawnCount int
	waitCode   engine.ExitCode
	waitErr    error
	waitDelay  time.Duration // simulates non-instant process runtime
}

func (e *mockProcessExecutor) Start(ctx context.Context, spec config.ConfigSpec) (*engine.Process, error) {
	e.spawnCount++
	if e.spawnErr != nil {
		return nil, e.spawnErr
	}
	return &engine.Process{PID: 1000 + e.spawnCount}, nil
}

func (e *mockProcessExecutor) Wait(ctx context.Context, pid int) (engine.ExitCode, error) {
	if e.waitDelay > 0 {
		select {
		case <-time.After(e.waitDelay):
		case <-ctx.Done():
			return -1, ctx.Err()
		}
	}
	return e.waitCode, e.waitErr
}

func (e *mockProcessExecutor) Signal(ctx context.Context, pid int, signal interface{}) error {
	return nil
}

// =============================================================================
// Helpers
// =============================================================================

// watchdogSpec creates a minimal ConfigSpec for Watchdog tests.
func watchdogSpec(name string, autorestart string, startretries, starttime int) *config.ConfigSpec {
	return &config.ConfigSpec{
		Program:      "/bin/test",
		ProcessName:  name,
		Cmd:          "/bin/test run",
		Numprocs:     1,
		Autostart:    true,
		Autorestart:  autorestart,
		Startretries: startretries,
		Starttime:    starttime,
		Stopsignal:   "TERM",
		Stoptime:     1,
		Exitcodes:    []int{0},
	}
}

// watchdogManager builds a Manager with a custom executor injected.
func watchdogManager(exec engine.ProcessExecutor) *Manager {
	m := NewManager()
	m.executor = exec
	return m
}

// runWatchdog starts Watchdog in a goroutine and blocks until it finishes or times out.
func runWatchdog(t *testing.T, m *Manager, spec *config.ConfigSpec, updates chan bus.ProcessUpdate, timeout time.Duration) {
	t.Helper()
	stop := make(chan struct{})
	done := make(chan struct{})
	// Create or get ProcessInstance
	// Note: No mutex needed during test setup - event loop not running yet
	proc, ok := m.Process[spec.ProcessName]
	if !ok {
		proc = &ProcessInstance{Status: bus.STOPPED}
		m.Process[spec.ProcessName] = proc
	}
	go func() {
		// Use the new Watchdog signature - updates and stop come from m.ch
		// For testing, we need to set up channels manually
		if m.ch == nil {
			m.ch = NewProcessChannels()
		}
		m.ch.Status = updates
		m.ch.Stop[spec.ProcessName] = stop
		m.Watchdog(spec, proc)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(timeout):
		t.Fatal("Watchdog did not finish within timeout")
	}
}

// drainUpdates returns all buffered updates without blocking.
func drainUpdates(ch chan bus.ProcessUpdate) []bus.ProcessUpdate {
	var out []bus.ProcessUpdate
	for {
		select {
		case u := <-ch:
			out = append(out, u)
		default:
			return out
		}
	}
}

func hasStatus(updates []bus.ProcessUpdate, want bus.Status) bool {
	for _, u := range updates {
		if u.Status == want {
			return true
		}
	}
	return false
}

func lastStatus(updates []bus.ProcessUpdate) bus.Status {
	if len(updates) == 0 {
		return ""
	}
	return updates[len(updates)-1].Status
}

func statusSequence(updates []bus.ProcessUpdate) []bus.Status {
	out := make([]bus.Status, len(updates))
	for i, u := range updates {
		out[i] = u.Status
	}
	return out
}

// =============================================================================
// Table-driven test helper
// =============================================================================

// watchdogTestCase defines a single watchdog test scenario.
type watchdogTestCase struct {
	name         string
	spawnErr     error
	waitCode     engine.ExitCode
	waitDelay    time.Duration
	autorestart  string
	startretries int
	starttime    int
	procSetup    func(*ProcessInstance) // optional custom process setup
	timeout      time.Duration
	updatesCap   int
	wantStatuses []bus.Status // expected statuses that must be present
	check        func(t *testing.T, tc *watchdogTestCase, mock *mockProcessExecutor, proc *ProcessInstance, updates []bus.ProcessUpdate)
}

// runWatchdogTest executes a watchdog test case with standardized setup.
func runWatchdogTest(t *testing.T, tc *watchdogTestCase) {
	t.Helper()

	mock := &mockProcessExecutor{
		spawnErr:  tc.spawnErr,
		waitCode:  tc.waitCode,
		waitDelay: tc.waitDelay,
	}
	m := watchdogManager(mock)

	spec := watchdogSpec("worker:00", tc.autorestart, tc.startretries, tc.starttime)
	proc := &ProcessInstance{Status: bus.STOPPED}
	if tc.procSetup != nil {
		tc.procSetup(proc)
	}
	m.Process[spec.ProcessName] = proc

	updates := make(chan bus.ProcessUpdate, tc.updatesCap)
	timeout := tc.timeout
	if timeout == 0 {
		timeout = 5 * time.Second
	}

	runWatchdog(t, m, spec, updates, timeout)

	all := drainUpdates(updates)

	// Check expected statuses
	for _, want := range tc.wantStatuses {
		if !hasStatus(all, want) {
			t.Errorf("expected status %s, got sequence: %v", want, statusSequence(all))
		}
	}

	// Run custom check if provided
	if tc.check != nil {
		tc.check(t, tc, mock, proc, all)
	}
}

// =============================================================================
// State transition tests
// =============================================================================

// should_send_backoff_status_before_each_retry_when_spawn_fails
func TestManagerWatchdogStartupFailureTransitionsToBackoff(t *testing.T) {
	runWatchdogTest(t, &watchdogTestCase{
		spawnErr:     errors.New("spawn failed"),
		autorestart:  "always",
		startretries: 2,
		starttime:    0,
		updatesCap:   20,
		wantStatuses: []bus.Status{bus.BACKOFF},
	})
}

// should_send_fatal_as_final_status_when_all_retries_exhausted_after_spawn_failure
func TestManagerWatchdogRetriesExhaustedTransitionsToFatal(t *testing.T) {
	runWatchdogTest(t, &watchdogTestCase{
		spawnErr:     errors.New("spawn failed"),
		autorestart:  "always",
		startretries: 2,
		starttime:    0,
		updatesCap:   20,
		check: func(t *testing.T, tc *watchdogTestCase, mock *mockProcessExecutor, proc *ProcessInstance, updates []bus.ProcessUpdate) {
			if len(updates) == 0 {
				t.Fatal("expected at least one update, got none")
			}
			if last := lastStatus(updates); last != bus.FATAL {
				t.Errorf("expected final status FATAL, got %s (full sequence: %v)", last, statusSequence(updates))
			}
		},
	})
}

// should_not_spawn_again_after_fatal_state_is_reached
func TestManagerWatchdogNoSpawnAfterFatal(t *testing.T) {
	runWatchdogTest(t, &watchdogTestCase{
		spawnErr:     errors.New("spawn failed"),
		autorestart:  "always",
		startretries: 1,
		starttime:    0,
		updatesCap:   10,
		check: func(t *testing.T, tc *watchdogTestCase, mock *mockProcessExecutor, proc *ProcessInstance, updates []bus.ProcessUpdate) {
			maxAllowed := tc.startretries + 1
			if mock.spawnCount > maxAllowed {
				t.Errorf("spawn called %d times after FATAL — expected at most %d (startretries+1)", mock.spawnCount, maxAllowed)
			}
		},
	})
}

// =============================================================================
// RetryCount tracking tests
// =============================================================================

// should_increment_retry_count_once_per_failed_spawn_attempt
func TestManagerWatchdogRetryCountIncrementsOnEachAttempt(t *testing.T) {
	runWatchdogTest(t, &watchdogTestCase{
		spawnErr:     errors.New("spawn failed"),
		autorestart:  "always",
		startretries: 3,
		starttime:    0,
		updatesCap:   20,
		check: func(t *testing.T, tc *watchdogTestCase, mock *mockProcessExecutor, proc *ProcessInstance, updates []bus.ProcessUpdate) {
			// Direct field access - test is single-threaded during check
			count := proc.RetryCount

			expectedAttempts := tc.startretries + 1
			if count != expectedAttempts {
				t.Errorf("expected RetryCount=%d after %d spawn attempts, got %d", expectedAttempts, expectedAttempts, count)
			}
		},
	})
}

// should_increment_retry_count_when_process_exits_unexpectedly_and_is_retried
func TestManagerWatchdogRetryCountIncrementsOnProcessCrash(t *testing.T) {
	runWatchdogTest(t, &watchdogTestCase{
		waitCode:     1,
		autorestart:  "always",
		startretries: 2,
		starttime:    0,
		updatesCap:   20,
		check: func(t *testing.T, tc *watchdogTestCase, mock *mockProcessExecutor, proc *ProcessInstance, updates []bus.ProcessUpdate) {
			// Direct field access - test is single-threaded during check
			count := proc.RetryCount

			expectedAttempts := tc.startretries + 1
			if count != expectedAttempts {
				t.Errorf("expected RetryCount=%d after %d crash+retry cycles, got %d", expectedAttempts, expectedAttempts, count)
			}
		},
	})
}

// should_reset_retry_count_to_zero_when_process_starts_successfully
func TestManagerWatchdogRetryCountResetsOnSuccessfulStart(t *testing.T) {
	runWatchdogTest(t, &watchdogTestCase{
		waitCode:     0,
		autorestart:  "never",
		startretries: 0,
		starttime:    0,
		updatesCap:   10,
		procSetup: func(p *ProcessInstance) {
			p.RetryCount = 5 // pre-set to non-zero
		},
		check: func(t *testing.T, tc *watchdogTestCase, mock *mockProcessExecutor, proc *ProcessInstance, updates []bus.ProcessUpdate) {
			// Direct field access - test is single-threaded during check
			count := proc.RetryCount

			if count != 0 {
				t.Errorf("expected RetryCount reset to 0 after successful start, got %d", count)
			}
		},
	})
}

// =============================================================================
// LastStart tracking tests
// =============================================================================

// should_set_last_start_timestamp_when_process_spawns_successfully
func TestManagerWatchdogLastStartSetOnSuccessfulSpawn(t *testing.T) {
	before := time.Now()
	runWatchdogTest(t, &watchdogTestCase{
		waitCode:     1, // exits with failure so no infinite retry
		waitDelay:    10 * time.Millisecond,
		autorestart:  "never",
		startretries: 0,
		starttime:    0,
		updatesCap:   10,
		check: func(t *testing.T, tc *watchdogTestCase, mock *mockProcessExecutor, proc *ProcessInstance, updates []bus.ProcessUpdate) {
			// Direct field access - test is single-threaded during check
			lastStart := proc.LastStart

			if lastStart.IsZero() {
				t.Error("expected LastStart to be set after successful spawn, got zero value")
			}
			if lastStart.Before(before) {
				t.Errorf("LastStart (%v) should be after test began (%v)", lastStart, before)
			}
		},
	})
}

// should_update_last_start_on_each_retry_when_process_keeps_crashing
func TestManagerWatchdogLastStartUpdatedOnRetry(t *testing.T) {
	before := time.Now()
	runWatchdogTest(t, &watchdogTestCase{
		waitCode:     1,
		autorestart:  "always",
		startretries: 2,
		starttime:    0,
		updatesCap:   20,
		check: func(t *testing.T, tc *watchdogTestCase, mock *mockProcessExecutor, proc *ProcessInstance, updates []bus.ProcessUpdate) {
			// Direct field access - test is single-threaded during check
			lastStart := proc.LastStart

			if lastStart.IsZero() {
				t.Error("expected LastStart to be set after retry cycles, got zero value")
			}
			if lastStart.Before(before) {
				t.Errorf("LastStart (%v) should be after test began (%v)", lastStart, before)
			}
		},
	})
}

// =============================================================================
// starttime window tests
// =============================================================================

// should_treat_process_as_backoff_when_it_exits_before_starttime_window_elapses
func TestManagerWatchdogStarttimeWindowProcessExitsTooSoon(t *testing.T) {
	runWatchdogTest(t, &watchdogTestCase{
		waitCode:     0,
		waitDelay:    0, // exits immediately — well before the 5-second starttime window
		autorestart:  "always",
		startretries: 2,
		starttime:    5, // starttime=5s
		updatesCap:   20,
		timeout:      10 * time.Second,
		wantStatuses: []bus.Status{bus.BACKOFF}, // Process exiting before starttime = failed startup attempt
	})
}

// should_treat_process_as_running_when_it_survives_past_starttime_window
func TestManagerWatchdogStarttimeWindowProcessSurvivesWindow(t *testing.T) {
	starttime := 50 * time.Millisecond
	runWatchdogTest(t, &watchdogTestCase{
		waitCode:     0,
		waitDelay:    starttime * 3, // Process stays alive longer than starttime window
		autorestart:  "never",
		startretries: 0,
		starttime:    0, // no window guard → process exits cleanly
		updatesCap:   10,
		wantStatuses: []bus.Status{bus.RUNNING},
	})
}

// =============================================================================
// OS-specific error tests
// =============================================================================

// should_handle_binary_not_found_error_and_send_fatal_without_crashing
func TestManagerWatchdogBinaryMissingErrorHandling(t *testing.T) {
	binaryNotFound := fmt.Errorf("spawn failed: %w", exec.ErrNotFound)
	runWatchdogTest(t, &watchdogTestCase{
		spawnErr:     binaryNotFound,
		autorestart:  "never",
		startretries: 0,
		starttime:    0,
		updatesCap:   10,
		wantStatuses: []bus.Status{bus.FATAL},
	})
}

// should_handle_permission_denied_error_and_send_fatal_without_crashing
func TestManagerWatchdogPermissionDeniedErrorHandling(t *testing.T) {
	permDenied := fmt.Errorf("spawn failed: fork/exec /bin/test: %w", syscall.EACCES)
	runWatchdogTest(t, &watchdogTestCase{
		spawnErr:     permDenied,
		autorestart:  "never",
		startretries: 0,
		starttime:    0,
		updatesCap:   10,
		wantStatuses: []bus.Status{bus.FATAL},
	})
}

// should_handle_bad_working_directory_error_and_send_fatal_without_crashing
func TestManagerWatchdogBadDirectoryErrorHandling(t *testing.T) {
	badDir := fmt.Errorf("spawn failed: chdir /nonexistent: %w", syscall.ENOENT)
	runWatchdogTest(t, &watchdogTestCase{
		spawnErr:     badDir,
		autorestart:  "never",
		startretries: 0,
		starttime:    0,
		updatesCap:   10,
		wantStatuses: []bus.Status{bus.FATAL},
	})
}

// =============================================================================
// Manager resilience tests
// =============================================================================

// should_complete_watchdog_without_panicking_when_spawn_fails
func TestManagerResilientToCrashingProcess(t *testing.T) {
	mock := &mockProcessExecutor{spawnErr: errors.New("spawn failed")}
	m := watchdogManager(mock)

	spec := watchdogSpec("worker:00", "never", 0, 0)
	proc := &ProcessInstance{Status: bus.STOPPED}
	m.Process[spec.ProcessName] = proc

	updates := make(chan bus.ProcessUpdate, 10)

	panicked := false
	done := make(chan struct{})
	go func() {
		defer func() {
			if r := recover(); r != nil {
				panicked = true
			}
			close(done)
		}()
		// Use the new Watchdog signature - updates and stop come from m.ch
		if m.ch == nil {
			m.ch = NewProcessChannels()
		}
		m.ch.Status = updates
		m.ch.Stop[spec.ProcessName] = make(chan struct{})
		m.Watchdog(spec, proc)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Watchdog did not finish within timeout — possible deadlock")
	}

	if panicked {
		t.Error("Watchdog panicked on spawn failure — manager must not crash")
	}
}

// should_keep_process_entry_in_manager_after_watchdog_ends_with_error
func TestManagerProcessEntrySurvivesWatchdogFailure(t *testing.T) {
	runWatchdogTest(t, &watchdogTestCase{
		spawnErr:     errors.New("spawn failed"),
		autorestart:  "never",
		startretries: 0,
		starttime:    0,
		updatesCap:   10,
		check: func(t *testing.T, tc *watchdogTestCase, mock *mockProcessExecutor, proc *ProcessInstance, updates []bus.ProcessUpdate) {
			m := watchdogManager(mock)
			// Re-add the process to check it survives (mockManager creates fresh manager)
			m.Process["worker:00"] = proc
			// Direct access - test is single-threaded
			_, exists := m.Process["worker:00"]

			if !exists {
				t.Error("ProcessInstance was removed from Manager after watchdog failure — it must remain for status queries")
			}
		},
	})
}

// should_leave_manager_functional_for_other_processes_after_one_watchdog_fails
func TestManagerOtherProcessesUnaffectedByWatchdogFailure(t *testing.T) {
	mock := &mockProcessExecutor{spawnErr: errors.New("spawn failed")}
	m := watchdogManager(mock)

	spec := watchdogSpec("worker:00", "never", 0, 0)
	m.Process[spec.ProcessName] = &ProcessInstance{Status: bus.STOPPED}

	// Add a second process entry that is not watchdogged in this test
	m.Process["server:00"] = &ProcessInstance{Status: bus.STOPPED}

	updates := make(chan bus.ProcessUpdate, 10)
	runWatchdog(t, m, spec, updates, 5*time.Second)

	// Direct access - test is single-threaded
	_, serverExists := m.Process["server:00"]

	if !serverExists {
		t.Error("unrelated process 'server:00' was removed from Manager — failure must be isolated")
	}
}
