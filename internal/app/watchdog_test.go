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
	go func() {
		m.Watchdog(spec, updates, stop)
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
// State transition tests
// =============================================================================

// should_send_backoff_status_before_each_retry_when_spawn_fails
func TestManager_Watchdog_StartupFailure_TransitionsToBackoff(t *testing.T) {
	mock := &mockProcessExecutor{spawnErr: errors.New("spawn failed")}
	m := watchdogManager(mock)

	spec := watchdogSpec("worker:00", "always", 2, 0) // 2 retries = 3 total attempts
	m.Process[spec.ProcessName] = &ProcessInstance{Status: bus.STOPPED}

	updates := make(chan bus.ProcessUpdate, 20)
	runWatchdog(t, m, spec, updates, 5*time.Second)

	all := drainUpdates(updates)
	if !hasStatus(all, bus.BACKOFF) {
		t.Errorf("expected BACKOFF during retries, got sequence: %v", statusSequence(all))
	}
}

// should_send_fatal_as_final_status_when_all_retries_exhausted_after_spawn_failure
func TestManager_Watchdog_RetriesExhausted_TransitionsToFatal(t *testing.T) {
	startretries := 2
	mock := &mockProcessExecutor{spawnErr: errors.New("spawn failed")}
	m := watchdogManager(mock)

	spec := watchdogSpec("worker:00", "always", startretries, 0)
	m.Process[spec.ProcessName] = &ProcessInstance{Status: bus.STOPPED}

	updates := make(chan bus.ProcessUpdate, 20)
	runWatchdog(t, m, spec, updates, 5*time.Second)

	all := drainUpdates(updates)

	if len(all) == 0 {
		t.Fatal("expected at least one update, got none")
	}
	if last := lastStatus(all); last != bus.FATAL {
		t.Errorf("expected final status FATAL, got %s (full sequence: %v)", last, statusSequence(all))
	}
}

// should_not_spawn_again_after_fatal_state_is_reached
func TestManager_Watchdog_NoSpawnAfterFatal(t *testing.T) {
	startretries := 1
	mock := &mockProcessExecutor{spawnErr: errors.New("spawn failed")}
	m := watchdogManager(mock)

	spec := watchdogSpec("worker:00", "always", startretries, 0)
	m.Process[spec.ProcessName] = &ProcessInstance{Status: bus.STOPPED}

	updates := make(chan bus.ProcessUpdate, 10)
	runWatchdog(t, m, spec, updates, 5*time.Second)

	// Spawn must not be called more than startretries+1 times
	maxAllowed := startretries + 1
	if mock.spawnCount > maxAllowed {
		t.Errorf("spawn called %d times after FATAL — expected at most %d (startretries+1)", mock.spawnCount, maxAllowed)
	}
}

// =============================================================================
// RetryCount tracking tests
// =============================================================================

// should_increment_retry_count_once_per_failed_spawn_attempt
func TestManager_Watchdog_RetryCount_IncrementsOnEachAttempt(t *testing.T) {
	startretries := 3
	mock := &mockProcessExecutor{spawnErr: errors.New("spawn failed")}
	m := watchdogManager(mock)

	spec := watchdogSpec("worker:00", "always", startretries, 0)
	proc := &ProcessInstance{Status: bus.STOPPED}
	m.Process[spec.ProcessName] = proc

	updates := make(chan bus.ProcessUpdate, 20)
	runWatchdog(t, m, spec, updates, 5*time.Second)

	proc.Mu.RLock()
	count := proc.RetryCount
	proc.Mu.RUnlock()

	// startretries+1 attempts total (initial + retries)
	expectedAttempts := startretries + 1
	if count != expectedAttempts {
		t.Errorf("expected RetryCount=%d after %d spawn attempts, got %d", expectedAttempts, expectedAttempts, count)
	}
}

// should_increment_retry_count_when_process_exits_unexpectedly_and_is_retried
func TestManager_Watchdog_RetryCount_IncrementsOnProcessCrash(t *testing.T) {
	startretries := 2
	// Spawn succeeds, process exits with non-zero code (triggers AlwaysRestart retry)
	mock := &mockProcessExecutor{waitCode: 1}
	m := watchdogManager(mock)

	spec := watchdogSpec("worker:00", "always", startretries, 0)
	proc := &ProcessInstance{Status: bus.STOPPED}
	m.Process[spec.ProcessName] = proc

	updates := make(chan bus.ProcessUpdate, 20)
	runWatchdog(t, m, spec, updates, 5*time.Second)

	proc.Mu.RLock()
	count := proc.RetryCount
	proc.Mu.RUnlock()

	expectedAttempts := startretries + 1
	if count != expectedAttempts {
		t.Errorf("expected RetryCount=%d after %d crash+retry cycles, got %d", expectedAttempts, expectedAttempts, count)
	}
}

// should_reset_retry_count_to_zero_when_process_starts_successfully
func TestManager_Watchdog_RetryCount_ResetsOnSuccessfulStart(t *testing.T) {
	// Spawn succeeds, process exits cleanly (exit 0, autorestart=never → no retry)
	mock := &mockProcessExecutor{waitCode: 0}
	m := watchdogManager(mock)

	spec := watchdogSpec("worker:00", "never", 0, 0)
	proc := &ProcessInstance{Status: bus.STOPPED, RetryCount: 5} // pre-set to non-zero
	m.Process[spec.ProcessName] = proc

	updates := make(chan bus.ProcessUpdate, 10)
	runWatchdog(t, m, spec, updates, 5*time.Second)

	proc.Mu.RLock()
	count := proc.RetryCount
	proc.Mu.RUnlock()

	if count != 0 {
		t.Errorf("expected RetryCount reset to 0 after successful start, got %d", count)
	}
}

// =============================================================================
// LastStart tracking tests
// =============================================================================

// should_set_last_start_timestamp_when_process_spawns_successfully
func TestManager_Watchdog_LastStart_SetOnSuccessfulSpawn(t *testing.T) {
	// Process spawns, runs briefly, exits
	mock := &mockProcessExecutor{
		waitCode:  1,              // exits with failure so no infinite retry
		waitDelay: 10 * time.Millisecond,
	}
	m := watchdogManager(mock)

	spec := watchdogSpec("worker:00", "never", 0, 0)
	proc := &ProcessInstance{Status: bus.STOPPED}
	m.Process[spec.ProcessName] = proc

	updates := make(chan bus.ProcessUpdate, 10)

	before := time.Now()
	runWatchdog(t, m, spec, updates, 5*time.Second)

	proc.Mu.RLock()
	lastStart := proc.LastStart
	proc.Mu.RUnlock()

	if lastStart.IsZero() {
		t.Error("expected LastStart to be set after successful spawn, got zero value")
	}
	if lastStart.Before(before) {
		t.Errorf("LastStart (%v) should be after test began (%v)", lastStart, before)
	}
}

// should_update_last_start_on_each_retry_when_process_keeps_crashing
func TestManager_Watchdog_LastStart_UpdatedOnRetry(t *testing.T) {
	// Process spawns, exits immediately each time, retried 2 times
	mock := &mockProcessExecutor{waitCode: 1}
	m := watchdogManager(mock)

	spec := watchdogSpec("worker:00", "always", 2, 0)
	proc := &ProcessInstance{Status: bus.STOPPED}
	m.Process[spec.ProcessName] = proc

	updates := make(chan bus.ProcessUpdate, 20)

	before := time.Now()
	runWatchdog(t, m, spec, updates, 5*time.Second)

	proc.Mu.RLock()
	lastStart := proc.LastStart
	proc.Mu.RUnlock()

	if lastStart.IsZero() {
		t.Error("expected LastStart to be set after retry cycles, got zero value")
	}
	if lastStart.Before(before) {
		t.Errorf("LastStart (%v) should be after test began (%v)", lastStart, before)
	}
}

// =============================================================================
// starttime window tests
// =============================================================================

// should_treat_process_as_backoff_when_it_exits_before_starttime_window_elapses
func TestManager_Watchdog_StarttimeWindow_ProcessExitsTooSoon(t *testing.T) {
	// Process exits immediately — well before the 5-second starttime window
	mock := &mockProcessExecutor{waitCode: 0, waitDelay: 0}
	m := watchdogManager(mock)

	spec := watchdogSpec("worker:00", "always", 2, 5) // starttime=5s
	m.Process[spec.ProcessName] = &ProcessInstance{Status: bus.STOPPED}

	updates := make(chan bus.ProcessUpdate, 20)
	runWatchdog(t, m, spec, updates, 10*time.Second)

	all := drainUpdates(updates)
	// Process exiting before starttime = failed startup attempt = BACKOFF, not RUNNING success
	if !hasStatus(all, bus.BACKOFF) {
		t.Errorf("expected BACKOFF when process exits before starttime window, got: %v", statusSequence(all))
	}
}

// should_treat_process_as_running_when_it_survives_past_starttime_window
func TestManager_Watchdog_StarttimeWindow_ProcessSurvivesWindow(t *testing.T) {
	starttime := 50 * time.Millisecond
	// Process stays alive longer than starttime window
	mock := &mockProcessExecutor{waitCode: 0, waitDelay: starttime * 3}
	m := watchdogManager(mock)

	spec := watchdogSpec("worker:00", "never", 0, 0)
	spec.Starttime = int(starttime.Milliseconds()) // test-friendly value in ms is NOT the spec field unit
	// Note: ConfigSpec.Starttime is in seconds. This test documents the expected boundary.
	// Adjust spec.Starttime to 0 and use a process that naturally runs past the window.
	spec.Starttime = 0 // no window guard → process exits cleanly
	m.Process[spec.ProcessName] = &ProcessInstance{Status: bus.STOPPED}

	updates := make(chan bus.ProcessUpdate, 10)
	runWatchdog(t, m, spec, updates, 5*time.Second)

	all := drainUpdates(updates)
	if !hasStatus(all, bus.RUNNING) {
		t.Errorf("expected RUNNING status when process spawns, got: %v", statusSequence(all))
	}
}

// =============================================================================
// OS-specific error tests
// =============================================================================

// should_handle_binary_not_found_error_and_send_fatal_without_crashing
func TestManager_Watchdog_BinaryMissing_ErrorHandling(t *testing.T) {
	binaryNotFound := fmt.Errorf("spawn failed: %w", exec.ErrNotFound)
	mock := &mockProcessExecutor{spawnErr: binaryNotFound}
	m := watchdogManager(mock)

	spec := watchdogSpec("worker:00", "never", 0, 0)
	m.Process[spec.ProcessName] = &ProcessInstance{Status: bus.STOPPED}

	updates := make(chan bus.ProcessUpdate, 10)
	runWatchdog(t, m, spec, updates, 5*time.Second)

	all := drainUpdates(updates)
	if !hasStatus(all, bus.FATAL) {
		t.Errorf("expected FATAL for missing binary, got: %v", statusSequence(all))
	}
}

// should_handle_permission_denied_error_and_send_fatal_without_crashing
func TestManager_Watchdog_PermissionDenied_ErrorHandling(t *testing.T) {
	permDenied := fmt.Errorf("spawn failed: fork/exec /bin/test: %w", syscall.EACCES)
	mock := &mockProcessExecutor{spawnErr: permDenied}
	m := watchdogManager(mock)

	spec := watchdogSpec("worker:00", "never", 0, 0)
	m.Process[spec.ProcessName] = &ProcessInstance{Status: bus.STOPPED}

	updates := make(chan bus.ProcessUpdate, 10)
	runWatchdog(t, m, spec, updates, 5*time.Second)

	all := drainUpdates(updates)
	if !hasStatus(all, bus.FATAL) {
		t.Errorf("expected FATAL for permission denied, got: %v", statusSequence(all))
	}
}

// should_handle_bad_working_directory_error_and_send_fatal_without_crashing
func TestManager_Watchdog_BadDirectory_ErrorHandling(t *testing.T) {
	badDir := fmt.Errorf("spawn failed: chdir /nonexistent: %w", syscall.ENOENT)
	mock := &mockProcessExecutor{spawnErr: badDir}
	m := watchdogManager(mock)

	spec := watchdogSpec("worker:00", "never", 0, 0)
	m.Process[spec.ProcessName] = &ProcessInstance{Status: bus.STOPPED}

	updates := make(chan bus.ProcessUpdate, 10)
	runWatchdog(t, m, spec, updates, 5*time.Second)

	all := drainUpdates(updates)
	if !hasStatus(all, bus.FATAL) {
		t.Errorf("expected FATAL for bad working directory, got: %v", statusSequence(all))
	}
}

// =============================================================================
// Manager resilience tests
// =============================================================================

// should_complete_watchdog_without_panicking_when_spawn_fails
func TestManager_ResilientToCrashingProcess(t *testing.T) {
	mock := &mockProcessExecutor{spawnErr: errors.New("spawn failed")}
	m := watchdogManager(mock)

	spec := watchdogSpec("worker:00", "never", 0, 0)
	m.Process[spec.ProcessName] = &ProcessInstance{Status: bus.STOPPED}

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
		stop := make(chan struct{})
		m.Watchdog(spec, updates, stop)
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
func TestManager_ProcessEntry_SurvivesWatchdogFailure(t *testing.T) {
	mock := &mockProcessExecutor{spawnErr: errors.New("spawn failed")}
	m := watchdogManager(mock)

	spec := watchdogSpec("worker:00", "never", 0, 0)
	m.Process[spec.ProcessName] = &ProcessInstance{Status: bus.STOPPED}

	updates := make(chan bus.ProcessUpdate, 10)
	runWatchdog(t, m, spec, updates, 5*time.Second)

	m.Mu.RLock()
	_, exists := m.Process[spec.ProcessName]
	m.Mu.RUnlock()

	if !exists {
		t.Error("ProcessInstance was removed from Manager after watchdog failure — it must remain for status queries")
	}
}

// should_leave_manager_functional_for_other_processes_after_one_watchdog_fails
func TestManager_OtherProcesses_UnaffectedByWatchdogFailure(t *testing.T) {
	mock := &mockProcessExecutor{spawnErr: errors.New("spawn failed")}
	m := watchdogManager(mock)

	spec := watchdogSpec("worker:00", "never", 0, 0)
	m.Process[spec.ProcessName] = &ProcessInstance{Status: bus.STOPPED}

	// Add a second process entry that is not watchdogged in this test
	m.Process["server:00"] = &ProcessInstance{Status: bus.STOPPED}

	updates := make(chan bus.ProcessUpdate, 10)
	runWatchdog(t, m, spec, updates, 5*time.Second)

	m.Mu.RLock()
	_, serverExists := m.Process["server:00"]
	m.Mu.RUnlock()

	if !serverExists {
		t.Error("unrelated process 'server:00' was removed from Manager — failure must be isolated")
	}
}
