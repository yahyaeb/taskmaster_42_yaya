package engine

import (
	"context"
	"testing"
)

// MockProcessExecutor is a mock implementation of ProcessExecutor for testing.
type MockProcessExecutor struct {
	SpawnFunc   func(ctx context.Context, cmd string, args []string) (*Process, error)
	WaitFunc    func(ctx context.Context, pid int) (ExitCode, error)
	SignalFunc  func(ctx context.Context, pid int, signal interface{}) error
	SpawnCalls  int
	WaitCalls   int
	SignalCalls int
}

// Spawn calls the mocked Spawn function and tracks calls.
func (m *MockProcessExecutor) Spawn(ctx context.Context, cmd string, args []string) (*Process, error) {
	m.SpawnCalls++
	if m.SpawnFunc != nil {
		return m.SpawnFunc(ctx, cmd, args)
	}
	return nil, nil
}

// Wait calls the mocked Wait function and tracks calls.
func (m *MockProcessExecutor) Wait(ctx context.Context, pid int) (ExitCode, error) {
	m.WaitCalls++
	if m.WaitFunc != nil {
		return m.WaitFunc(ctx, pid)
	}
	return 0, nil
}

// Signal calls the mocked Signal function and tracks calls.
func (m *MockProcessExecutor) Signal(ctx context.Context, pid int, signal interface{}) error {
	m.SignalCalls++
	if m.SignalFunc != nil {
		return m.SignalFunc(ctx, pid, signal)
	}
	return nil
}

// MockConfigurableProcessExecutor is a mock implementation of ConfigurableProcessExecutor.
type MockConfigurableProcessExecutor struct {
	*MockProcessExecutor
	SpawnFunc func(ctx context.Context, spec ConfigSpec) (*Process, error)
}

// Spawn calls the mocked Spawn function.
func (m *MockConfigurableProcessExecutor) Spawn(ctx context.Context, spec ConfigSpec) (*Process, error) {
	if m.SpawnFunc != nil {
		return m.SpawnFunc(ctx, spec)
	}
	return nil, nil
}

// TestProcessWatcherWithMockExecutor validates ProcessWatcher retry behavior with mock executor.
func TestProcessWatcherWithMockExecutor(t *testing.T) {
	tests := []struct {
		name               string
		maxRetries         int
		failureCount       int
		finalExitCode      ExitCode
		expectedCodes      map[int]bool
		shouldSucceed      bool
		expectedSpawnCalls int
	}{
		{
			name:               "succeeds on first attempt",
			maxRetries:         2,
			failureCount:       0,
			finalExitCode:      0,
			expectedCodes:      map[int]bool{0: true},
			shouldSucceed:      true,
			expectedSpawnCalls: 1,
		},
		{
			name:               "succeeds after two retries",
			maxRetries:         2,
			failureCount:       2,
			finalExitCode:      0,
			expectedCodes:      map[int]bool{0: true},
			shouldSucceed:      true,
			expectedSpawnCalls: 3,
		},
		{
			name:               "exhausts retries and fails",
			maxRetries:         2,
			failureCount:       3,
			finalExitCode:      1,
			expectedCodes:      map[int]bool{0: true},
			shouldSucceed:      false,
			expectedSpawnCalls: 3,
		},
		{
			name:               "respects expected exit codes",
			maxRetries:         2,
			failureCount:       0,
			finalExitCode:      42,
			expectedCodes:      map[int]bool{42: true},
			shouldSucceed:      true,
			expectedSpawnCalls: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			attempt := 0
			mock := &MockProcessExecutor{
				SpawnFunc: func(ctx context.Context, cmd string, args []string) (*Process, error) {
					return &Process{PID: 1000 + attempt}, nil
				},
				WaitFunc: func(ctx context.Context, pid int) (ExitCode, error) {
					defer func() { attempt++ }()
					if attempt < tt.failureCount {
						return 1, nil
					}
					return tt.finalExitCode, nil
				},
			}

			config := RetryConfig{
				MaxRetries:    tt.maxRetries,
				RetryDelay:    0,
				ExpectedCodes: tt.expectedCodes,
			}

			watcher := NewProcessWatcher(mock, config)
			ctx := context.Background()

			exitCode, err := watcher.Run(ctx, "test-cmd", []string{})

			if tt.shouldSucceed {
				if err != nil {
					t.Fatalf("expected success, got error: %v", err)
				}
				if exitCode != tt.finalExitCode {
					t.Errorf("expected exit code %d, got %d", tt.finalExitCode, exitCode)
				}
			} else {
				if err == nil {
					t.Fatalf("expected failure, but got success")
				}
			}

			if mock.SpawnCalls != tt.expectedSpawnCalls {
				t.Errorf("expected %d spawn calls, got %d", tt.expectedSpawnCalls, mock.SpawnCalls)
			}
			if mock.WaitCalls != tt.expectedSpawnCalls {
				t.Errorf("expected %d wait calls, got %d", tt.expectedSpawnCalls, mock.WaitCalls)
			}
		})
	}
}
