package bus

// Status represents the runtime status of a process.
type Status string

const (
	STARTING Status = "starting"
	RUNNING  Status = "running"
	STOPPED  Status = "stopped"
	FATAL    Status = "fatal"
	BACKOFF  Status = "backoff"
)

// ProcessUpdate represents a status update from a process.
type ProcessUpdate struct {
	Name   string
	Status Status
	Pid    int
}

// Updates is a channel for process updates.
// Go channel — NOT a socket broadcast
// Engine writes to chan bus.ProcessUpdate. Logger and CTL subscribe via that channel. No external event library needed.
type Updates chan ProcessUpdate
