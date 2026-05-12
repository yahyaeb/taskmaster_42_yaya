// internal/app/channels.go
package app

import (
	"taskmaster/internal/bus"
	"taskmaster/internal/protocol"
)

// ProcessChannels holds the channels used for process supervision.
// These are the ONLY channels needed - no command dispatch channel required.
type ProcessChannels struct {
	// Status receives updates from Watchdogs (Watchdog → Manager)
	// Buffered to prevent slow status handling from blocking watchdogs
	Status chan bus.ProcessUpdate

	// Stop signals a Watchdog to stop its process (Manager → Watchdog)
	// Each process has its own stop channel
	Stop map[string]chan struct{}
}

// NewProcessChannels creates a new set of process communication channels.
func NewProcessChannels() *ProcessChannels {
	return &ProcessChannels{
		Status: make(chan bus.ProcessUpdate, 100),
		Stop:   make(map[string]chan struct{}),
	}
}

// ReloadResult contains the outcome of a configuration reload.
type ReloadResult struct {
	Added     []string
	Removed   []string
	Restarted []string
}

// ProcessManager defines the interface for process management operations.
// This is the clean API surface - all methods are synchronous and safe for concurrent use.
type ProcessManager interface {
	GetProcessInfo(name string) (protocol.ProcessInfo, error)
	GetAllProcessInfo() ([]protocol.ProcessInfo, error)
	Start(name string) error
	Stop(name string) error
	Restart(name string) error
	Reload() (*ReloadResult, error)
	Shutdown() error
}

// Compile-time check: Manager must implement ProcessManager
var _ ProcessManager = (*Manager)(nil)
