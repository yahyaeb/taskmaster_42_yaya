package internal

type Lifecycle int

const (
	ActionStop Lifecycle = iota
	ActionRestart
)

type LifecycleEvent struct {
	Action   Lifecycle
	Pid      int
	ExitCode int
}
