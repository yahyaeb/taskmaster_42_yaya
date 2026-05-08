package internal

import (
	"taskmaster/internal/engine"
)

// Re-export engine types for backward compatibility with existing tests.
type (
	ProcessExecutor             = engine.ProcessExecutor
	ConfigurableProcessExecutor = engine.ConfigurableProcessExecutor
	Process                     = engine.Process
	ExitCode                    = engine.ExitCode
	RetryStrategy               = engine.RetryStrategy
	RetryConfig                 = engine.RetryConfig
	ProcessWatcher              = engine.ProcessWatcher
	ProcessSpawner              = engine.ProcessSpawner
	AlwaysRestart               = engine.AlwaysRestart
	NeverRestart                = engine.NeverRestart
	UnexpectedOnlyRestart       = engine.UnexpectedOnlyRestart
	SignalHandler               = engine.SignalHandler
	OSSignalHandler             = engine.OSSignalHandler
	ProcessStopper              = engine.ProcessStopper
)

// Re-export constructor functions
var (
	NewOsProcessExecutor          = engine.NewOsProcessExecutor
	NewProcessWatcher             = engine.NewProcessWatcher
	NewProcessWatcherWithStrategy = engine.NewProcessWatcherWithStrategy
	NewProcessStopper             = engine.NewProcessStopper
	RetryStrategyFactory          = engine.RetryStrategyFactory
	RetryStrategyFromExpectedCodes = engine.RetryStrategyFromExpectedCodes
)

// For testing - re-export internal builder type
type commandBuilder = engine.CommandBuilder
