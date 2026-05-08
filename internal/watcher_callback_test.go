package internal

import (
	"context"
	"testing"
)

func TestProcessWatcher_OnProcessStartedCallback(t *testing.T) {
	var callbackPID int
	callbackCalled := false

	mock := &MockProcessExecutor{
		SpawnFunc: func(ctx context.Context, cmd string, args []string) (*Process, error) {
			return &Process{PID: 12345}, nil
		},
		WaitFunc: func(ctx context.Context, pid int) (ExitCode, error) {
			return 0, nil
		},
	}

	config := RetryConfig{
		MaxRetries:    0,
		RetryDelay:    0,
		ExpectedCodes: map[int]bool{0: true},
	}

	watcher := NewProcessWatcher(mock, config)
	watcher.OnProcessStarted = func(pid int) {
		callbackCalled = true
		callbackPID = pid
	}

	ctx := context.Background()
	exitCode, err := watcher.Run(ctx, "test-cmd", []string{})
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if exitCode != 0 {
		t.Errorf("expected exit code 0, got %d", exitCode)
	}
	if !callbackCalled {
		t.Error("OnProcessStarted callback not called")
	}
	if callbackPID != 12345 {
		t.Errorf("expected callback PID 12345, got %d", callbackPID)
	}
}

func TestProcessWatcher_RunWithConfig(t *testing.T) {
	var spawnedSpec ConfigSpec
	var spawnedPID int

	mock := &MockConfigurableProcessExecutor{
		MockProcessExecutor: &MockProcessExecutor{
			WaitFunc: func(ctx context.Context, pid int) (ExitCode, error) {
				return 0, nil
			},
		},
		SpawnWithConfigFunc: func(ctx context.Context, spec ConfigSpec) (*Process, error) {
			spawnedSpec = spec
			spawnedPID = 999
			return &Process{PID: spawnedPID}, nil
		},
	}

	strategy := &NeverRestart{}
	watcher := NewProcessWatcherWithStrategy(mock, strategy, 0, 0)

	spec := ConfigSpec{
		ProcessName: "test:00",
		Program:     "/bin/echo",
		Cmd:         "hello",
		Env:         map[string]string{"KEY": "VALUE"},
	}

	ctx := context.Background()
	exitCode, err := watcher.RunWithConfig(ctx, spec)
	if err != nil {
		t.Fatalf("RunWithConfig failed: %v", err)
	}
	if exitCode != 0 {
		t.Errorf("expected exit code 0, got %d", exitCode)
	}
	if spawnedSpec.ProcessName != "test:00" {
		t.Errorf("expected process name test:00, got %s", spawnedSpec.ProcessName)
	}
	if spawnedSpec.Program != "/bin/echo" {
		t.Errorf("expected program /bin/echo, got %s", spawnedSpec.Program)
	}
	if val, ok := spawnedSpec.Env["KEY"]; !ok || val != "VALUE" {
		t.Errorf("expected env KEY=VALUE, got %v", spawnedSpec.Env)
	}
	if spawnedPID != 999 {
		t.Errorf("expected PID 999, got %d", spawnedPID)
	}
}

func TestProcessWatcher_RunWithConfig_NonConfigurableExecutor(t *testing.T) {
	mock := &MockProcessExecutor{
		SpawnFunc: func(ctx context.Context, cmd string, args []string) (*Process, error) {
			return &Process{PID: 111}, nil
		},
		WaitFunc: func(ctx context.Context, pid int) (ExitCode, error) {
			return 0, nil
		},
	}

	strategy := &AlwaysRestart{}
	watcher := NewProcessWatcherWithStrategy(mock, strategy, 0, 0)

	spec := ConfigSpec{}
	ctx := context.Background()
	_, err := watcher.RunWithConfig(ctx, spec)
	if err == nil {
		t.Error("expected error for non-configurable executor, got nil")
	}
	expectedErr := "executor does not support SpawnWithConfig"
	if err != nil && err.Error() != expectedErr {
		t.Errorf("expected error %q, got %v", expectedErr, err)
	}
}

func TestProcessWatcher_RunWithConfig_RetryAndCallback(t *testing.T) {
	var callbacks []int
	attempt := 0
	
	mock := &MockConfigurableProcessExecutor{
		MockProcessExecutor: &MockProcessExecutor{
			WaitFunc: func(ctx context.Context, pid int) (ExitCode, error) {
				// First attempt fails with exit code 1, second succeeds with 0
				if attempt == 0 {
					attempt++
					return 1, nil
				}
				return 0, nil
			},
		},
		SpawnWithConfigFunc: func(ctx context.Context, spec ConfigSpec) (*Process, error) {
			return &Process{PID: 1000 + attempt}, nil
		},
	}

	// Strategy: restart on exit code 1 (unexpected), not on 0
	strategy := &UnexpectedOnlyRestart{AllowedCodes: []int{0}}
	watcher := NewProcessWatcherWithStrategy(mock, strategy, 1, 0) // maxRetries=1 (total attempts=2)
	watcher.OnProcessStarted = func(pid int) {
		callbacks = append(callbacks, pid)
	}

	spec := ConfigSpec{ProcessName: "test"}
	ctx := context.Background()
	exitCode, err := watcher.RunWithConfig(ctx, spec)
	if err != nil {
		t.Fatalf("RunWithConfig failed: %v", err)
	}
	if exitCode != 0 {
		t.Errorf("expected exit code 0, got %d", exitCode)
	}
	if len(callbacks) != 2 {
		t.Errorf("expected 2 OnProcessStarted callbacks, got %d", len(callbacks))
	}
	if callbacks[0] != 1000 {
		t.Errorf("first callback PID expected 1000, got %d", callbacks[0])
	}
	if callbacks[1] != 1001 {
		t.Errorf("second callback PID expected 1001, got %d", callbacks[1])
	}
}

func TestProcessWatcher_RunWithConfig_ContextCancellation(t *testing.T) {
	mock := &MockConfigurableProcessExecutor{
		MockProcessExecutor: &MockProcessExecutor{
			WaitFunc: func(ctx context.Context, pid int) (ExitCode, error) {
				// Wait forever until context cancelled
				<-ctx.Done()
				return -1, ctx.Err()
			},
		},
		SpawnWithConfigFunc: func(ctx context.Context, spec ConfigSpec) (*Process, error) {
			return &Process{PID: 999}, nil
		},
	}

	strategy := &NeverRestart{}
	watcher := NewProcessWatcherWithStrategy(mock, strategy, 0, 0)

	spec := ConfigSpec{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately
	
	_, err := watcher.RunWithConfig(ctx, spec)
	if err == nil {
		t.Error("expected error due to cancelled context")
	}
	if err != nil && err.Error() != "wait failed: context canceled" {
		t.Errorf("expected 'wait failed: context canceled', got %v", err)
	}
}
