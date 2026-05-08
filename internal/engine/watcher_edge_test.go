package engine

import (
	"context"
	"fmt"
	"testing"
	"time"
)

func TestProcessWatcher_Run_SpawnFailure(t *testing.T) {
	spawnCalls := 0
	mock := &MockProcessExecutor{
		SpawnFunc: func(ctx context.Context, cmd string, args []string) (*Process, error) {
			spawnCalls++
			return nil, fmt.Errorf("simulated spawn failure")
		},
		WaitFunc: func(ctx context.Context, pid int) (ExitCode, error) {
			return 0, nil
		},
	}
	config := RetryConfig{
		MaxRetries:    2,
		RetryDelay:    0,
		ExpectedCodes: map[int]bool{0: true},
	}
	watcher := NewProcessWatcher(mock, config)
	ctx := context.Background()
	_, err := watcher.Run(ctx, "test", []string{})
	if err == nil {
		t.Fatal("expected error")
	}
	if spawnCalls != 3 { // initial + 2 retries
		t.Errorf("expected 3 spawn calls, got %d", spawnCalls)
	}
}

func TestProcessWatcher_Run_WaitFailure(t *testing.T) {
	waitCalls := 0
	mock := &MockProcessExecutor{
		SpawnFunc: func(ctx context.Context, cmd string, args []string) (*Process, error) {
			return &Process{PID: 123}, nil
		},
		WaitFunc: func(ctx context.Context, pid int) (ExitCode, error) {
			waitCalls++
			return 0, fmt.Errorf("simulated wait failure")
		},
	}
	config := RetryConfig{
		MaxRetries:    1,
		RetryDelay:    0,
		ExpectedCodes: map[int]bool{0: true},
	}
	watcher := NewProcessWatcher(mock, config)
	ctx := context.Background()
	_, err := watcher.Run(ctx, "test", []string{})
	if err == nil {
		t.Fatal("expected error")
	}
	if waitCalls != 2 { // initial + 1 retry
		t.Errorf("expected 2 wait calls, got %d", waitCalls)
	}
}

func TestProcessWatcher_Run_ContextCancellationDuringRetryDelay(t *testing.T) {
	spawnCalls := 0
	mock := &MockProcessExecutor{
		SpawnFunc: func(ctx context.Context, cmd string, args []string) (*Process, error) {
			spawnCalls++
			return &Process{PID: 100 + spawnCalls}, nil
		},
		WaitFunc: func(ctx context.Context, pid int) (ExitCode, error) {
			// First attempt fails, second will be cancelled
			if spawnCalls == 1 {
				return 1, nil
			}
			return 0, nil
		},
	}
	config := RetryConfig{
		MaxRetries:    2,
		RetryDelay:    time.Minute, // long delay
		ExpectedCodes: map[int]bool{0: true},
	}
	watcher := NewProcessWatcher(mock, config)
	ctx, cancel := context.WithCancel(context.Background())
	// Cancel after first failure, during retry delay
	go func() {
		cancel()
	}()
	_, err := watcher.Run(ctx, "test", []string{})
	if err != context.Canceled {
		t.Errorf("expected context.Canceled, got %v", err)
	}
	if spawnCalls != 1 {
		t.Errorf("expected 1 spawn call before cancellation, got %d", spawnCalls)
	}
}

func TestProcessWatcher_Run_NoCallbackWhenSpawnFails(t *testing.T) {
	callbackCalled := false
	mock := &MockProcessExecutor{
		SpawnFunc: func(ctx context.Context, cmd string, args []string) (*Process, error) {
			return nil, fmt.Errorf("spawn fail")
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
	}
	ctx := context.Background()
	watcher.Run(ctx, "test", []string{})
	if callbackCalled {
		t.Error("OnProcessStarted should not be called when spawn fails")
	}
}

func TestProcessWatcher_Run_NilStrategyUsesExpectedCodes(t *testing.T) {
	mock := &MockConfigurableProcessExecutor{
		MockProcessExecutor: &MockProcessExecutor{
			WaitFunc: func(ctx context.Context, pid int) (ExitCode, error) {
				return 42, nil
			},
		},
		SpawnFunc: func(ctx context.Context, spec ConfigSpec) (*Process, error) {
			return &Process{PID: 777}, nil
		},
	}
	// Manually construct watcher with nil strategy but ExpectedCodes set
	watcher := &ProcessWatcher{
		Executor: mock,
		Config: RetryConfig{
			MaxRetries: 0,
			RetryDelay: 0,
			ExpectedCodes: map[int]bool{42: true},
		},
		Strategy: nil,
	}
	ctx := context.Background()
	exitCode, err := watcher.Run(ctx, ConfigSpec{})
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if exitCode != 42 {
		t.Errorf("expected exit code 42, got %d", exitCode)
	}
}
