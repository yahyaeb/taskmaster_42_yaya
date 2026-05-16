package state

import (
	"context"
	"sync"
	"time"

	"taskmaster/internal/bus"
	"taskmaster/internal/config"
)

type terminalWait struct {
	ch   chan struct{}
	once sync.Once
}

func newTerminalWait() *terminalWait {
	return &terminalWait{ch: make(chan struct{})}
}

func (t *terminalWait) signal() {
	if t == nil {
		return
	}
	t.once.Do(func() { close(t.ch) })
}

func (t *terminalWait) Chan() <-chan struct{} {
	if t == nil {
		return nil
	}
	return t.ch
}

// ProcessInstance holds runtime fields for one supervised process row.
type ProcessInstance struct {
	Pid        int
	Status     bus.Status
	RetryCount int
	LastStart  time.Time
	Intended   bool
	cancelFn   context.CancelFunc
	term       *terminalWait
}

// NewProcessInstance creates a stopped row with Intended set from autostart.
func NewProcessInstance(autostart bool) *ProcessInstance {
	return &ProcessInstance{
		Status:     bus.STOPPED,
		Intended:   autostart,
		RetryCount: 0,
	}
}

// Registry holds config specs and process runtime rows behind one mutex.
type Registry struct {
	mu      sync.Mutex
	config  map[string]*config.ConfigSpec
	process map[string]*ProcessInstance
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry {
	return &Registry{
		config:  make(map[string]*config.ConfigSpec),
		process: make(map[string]*ProcessInstance),
	}
}

// Upsert sets config and ensures a process slot exists (creates from spec.Autostart if missing).
func (r *Registry) Upsert(name string, spec *config.ConfigSpec) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.config[name] = spec
	if _, ok := r.process[name]; !ok {
		r.process[name] = NewProcessInstance(spec.Autostart)
	}
}

// SetSpec replaces the config entry without creating a process row.
func (r *Registry) SetSpec(name string, spec *config.ConfigSpec) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.config[name] = spec
}

// Delete removes the config entry; if a process row remains, marks it STOPPED.
func (r *Registry) Delete(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.config, name)
	if proc, ok := r.process[name]; ok {
		proc.Status = bus.STOPPED
	}
}

// Get returns config and process for name. ok is false if there is no config entry.
func (r *Registry) Get(name string) (*config.ConfigSpec, *ProcessInstance, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	spec, ok := r.config[name]
	if !ok {
		return nil, nil, false
	}
	return spec, r.process[name], true
}

// GetSpec returns the config entry only.
func (r *Registry) GetSpec(name string) (*config.ConfigSpec, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	spec, ok := r.config[name]
	return spec, ok
}

// GetProcess returns the process row only (e.g. after config was deleted).
func (r *Registry) GetProcess(name string) (*ProcessInstance, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	p, ok := r.process[name]
	return p, ok
}

// All returns a shallow snapshot of every process name → row (for status listing).
func (r *Registry) All() map[string]*ProcessInstance {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make(map[string]*ProcessInstance, len(r.process))
	for n, p := range r.process {
		out[n] = p
	}
	return out
}

// SpecNames returns a copy of current config keys.
func (r *Registry) SpecNames() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]string, 0, len(r.config))
	for name := range r.config {
		out = append(out, name)
	}
	return out
}

// ProcessNames returns every name in the process map (for shutdown).
func (r *Registry) ProcessNames() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]string, 0, len(r.process))
	for name := range r.process {
		out = append(out, name)
	}
	return out
}

// EnsureProcess guarantees a process row exists with Intended from autostart when creating.
func (r *Registry) EnsureProcess(name string, autostart bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.process[name]; !ok {
		r.process[name] = NewProcessInstance(autostart)
	}
}

// ApplyUpdate merges one status update into the process row.
func (r *Registry) ApplyUpdate(update bus.ProcessUpdate) {
	r.mu.Lock()
	defer r.mu.Unlock()
	proc, ok := r.process[update.Name]
	if !ok {
		return
	}
	proc.Status = update.Status
	if update.Pid > 0 {
		proc.Pid = update.Pid
	}
	if update.HasRetries {
		proc.RetryCount = update.Retries
	}
	if !update.LastStart.IsZero() {
		proc.LastStart = update.LastStart
	}
	if update.Status == bus.STOPPED || update.Status == bus.FATAL {
		proc.cancelFn = nil
		proc.term.signal()
	}
}

// TerminalChan returns the channel closed when the supervisor applies STOPPED or FATAL
// for this process (nil if no watchdog cycle has bound a waiter yet).
func (r *Registry) TerminalChan(name string) <-chan struct{} {
	r.mu.Lock()
	defer r.mu.Unlock()
	p, ok := r.process[name]
	if !ok || p.term == nil {
		return nil
	}
	return p.term.Chan()
}

// NotifyRunReturned closes the terminal wait after engine.Run returns (backup if status delivery lags).
func (r *Registry) NotifyRunReturned(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	p, ok := r.process[name]
	if !ok || p.term == nil {
		return
	}
	p.term.signal()
}

// IsRunning reports whether a watchdog cancel function is active.
func (r *Registry) IsRunning(name string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	proc, ok := r.process[name]
	return ok && proc.cancelFn != nil
}

// BindWatchdog attaches a supervisor cancel func and sets STARTING.
func (r *Registry) BindWatchdog(name string, cancel context.CancelFunc) {
	r.mu.Lock()
	defer r.mu.Unlock()
	proc, ok := r.process[name]
	if !ok {
		return
	}
	proc.term = newTerminalWait()
	proc.cancelFn = cancel
	proc.Status = bus.STARTING
}

// ClearWatchdog cancels the supervisor if running.
func (r *Registry) ClearWatchdog(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	proc, ok := r.process[name]
	if !ok || proc.cancelFn == nil {
		return
	}
	proc.cancelFn()
	proc.cancelFn = nil
}
