// internal/app/channels.go
package app

import (
	"taskmaster/internal/bus"
	"taskmaster/internal/protocol"
)

type ProcessChannels struct {
	// Watchdog → Manager (and eventually to main/socket)
	Status chan bus.ProcessUpdate

	// Manager → Watchdog (stop signal per process)
	Stop map[string]chan struct{}
}

func NewProcessChannels() *ProcessChannels {
	return &ProcessChannels{
		Status: make(chan bus.ProcessUpdate, 100), // buffered!
		Stop:   make(map[string]chan struct{}),
	}
}

// Request is a unified request type for all Manager operations.
type Request struct {
	Type string // "start", "stop", "restart", "reload", "shutdown", "get", "list"
	Name string // process name (for single-target ops)
	Resp chan Response
}

// Response is the unified response type for all Manager operations.
type Response struct {
	Result interface{} // *ReloadResult, []protocol.ProcessInfo, protocol.ProcessInfo, or nil
	Err    error
}

// ReloadResult contains the outcome of a configuration reload.
type ReloadResult struct {
	Added     []string
	Removed   []string
	Restarted []string
}

// ProcessManager defines the interface for process management operations.
type ProcessManager interface {
	GetProcessInfo(name string) (protocol.ProcessInfo, error)
	GetAllProcessInfo() ([]protocol.ProcessInfo, error)
	Start(name string) error
	Stop(name string) error
	Restart(name string) error
	Reload() (*ReloadResult, error)
	Shutdown() error
}
