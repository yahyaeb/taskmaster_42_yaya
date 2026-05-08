package internal

import (
	"taskmaster/internal/bus"
)

// Re-export bus types for backward compatibility
type (
	Update = bus.ProcessUpdate
	Status = bus.Status
)

const (
	STARTING = bus.STARTING
	RUNNING  = bus.RUNNING
	STOPPED  = bus.STOPPED
	FATAL    = bus.FATAL
	BACKOFF  = bus.BACKOFF
)
