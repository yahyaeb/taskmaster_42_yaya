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

	// RPC → Manager (commands from socket/ctl)
	Commands chan Command

	// Manager internal coordination
	Reload chan struct{}
}

func NewProcessChannels() *ProcessChannels {
	return &ProcessChannels{
		Status:   make(chan bus.ProcessUpdate, 100), // buffered!
		Stop:     make(map[string]chan struct{}),
		Commands: make(chan Command, 10),
		Reload:   make(chan struct{}, 1),
	}
}

// Command types
type Command struct {
	Type   string     // "start", "stop", "restart", "reload"
	Target string     // process name (or "all")
	Resp   chan error // async response
}

// ReloadCommandResult holds the result of a reload command
type ReloadCommandResult struct {
	Result *ReloadResult
	Err    error
}

// ManagerCommand represents a command sent to the Manager's event loop
type ManagerCommand struct {
	Type       string // "start", "stop", "restart", "reload", "shutdown", "spawn"
	Name       string // process name (if applicable)
	Resp       chan error
	ReloadResp chan ReloadCommandResult // for reload command only
}

// ManagerQuery represents a query sent to the Manager's event loop
type ManagerQuery struct {
	Type string // "get", "list"
	Name string // process name for "get"
	Resp chan QueryResult
}

// QueryResult contains the response to a ManagerQuery
type QueryResult struct {
	Info  protocol.ProcessInfo
	Infos []protocol.ProcessInfo
	Err   error
}
