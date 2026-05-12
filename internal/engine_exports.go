package internal

import (
	"taskmaster/internal/engine"
)

// Re-export engine types for backward compatibility.
type (
	ProcessExecutor       = engine.ProcessExecutor
	Process               = engine.Process
	ExitCode              = engine.ExitCode
	RetryConfig           = engine.RetryConfig
	ProcessWatcher        = engine.ProcessWatcher
	ProcessSpawner        = engine.ProcessSpawner
	SignalHandler         = engine.SignalHandler
	OSSignalHandler       = engine.OSSignalHandler
	ProcessStopper        = engine.ProcessStopper
)

// Re-export functions
var (
	NewOsProcessExecutor  = engine.NewOsProcessExecutor
	NewProcessWatcher     = engine.NewProcessWatcher
	NewProcessStopper     = engine.NewProcessStopper
	ShouldRestart         = engine.ShouldRestart
)
