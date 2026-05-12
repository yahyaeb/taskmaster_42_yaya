package app

import (
	"fmt"
	"log/slog"
	"os"
	"slices"
	"sync"
	"time"

	"taskmaster/internal/bus"
	"taskmaster/internal/config"
	"taskmaster/internal/engine"
	"taskmaster/internal/protocol"
)

// ProcessInstance represents the runtime state of a single process.
// It is owned by its Watchdog goroutine; Manager only accesses it
// through Status channel updates or when holding the mutex.
type ProcessInstance struct {
	Pid        int
	Status     bus.Status
	RetryCount int
	LastStart  time.Time
	Intended   bool
}

func newProcessInstance(autostart bool) *ProcessInstance {
	return &ProcessInstance{
		Status:     bus.STOPPED,
		Intended:   autostart,
		RetryCount: 0,
	}
}

// Manager orchestrates all processes. It is safe for concurrent use.
// All state mutations are protected by mu mutex.
type Manager struct {
	mu           sync.Mutex
	Config       map[string]*config.ConfigSpec
	Process      map[string]*ProcessInstance
	executor     engine.ProcessExecutor
	handler      engine.SignalHandler
	ch           *ProcessChannels
	shutdownFunc func()
	configPath   string
}

// NewManager creates a new Manager with no configuration loaded.
func NewManager() *Manager {
	return &Manager{
		Config:   make(map[string]*config.ConfigSpec),
		Process:  make(map[string]*ProcessInstance),
		executor: engine.NewOsProcessExecutor(),
		handler:  &engine.OSSignalHandler{},
	}
}

// SetShutdownFunc registers a callback to be invoked after all
// processes are stopped during Shutdown().
func (m *Manager) SetShutdownFunc(fn func()) {
	m.shutdownFunc = fn
}

// SetChannels assigns the process communication channels.
func (m *Manager) SetChannels(ch *ProcessChannels) {
	m.ch = ch
}

// Channels returns the assigned process channels.
func (m *Manager) Channels() *ProcessChannels {
	return m.ch
}

// ---------------------------------------------------------------------------
// Public API - these methods are safe to call from any goroutine.
// Each acquires the mutex, performs work, and releases it.
// ---------------------------------------------------------------------------

// Start begins running the named process if it exists and is not already running.
func (m *Manager) Start(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.ch == nil {
		return fmt.Errorf("process channels not set")
	}

	configSpec, exists := m.Config[name]
	if !exists {
		return fmt.Errorf("process not found: %s", name)
	}

	if m.isRunning(name) {
		return nil
	}

	if _, ok := m.ch.Stop[name]; !ok {
		m.ch.Stop[name] = make(chan struct{})
	}

	if _, ok := m.Process[name]; !ok {
		m.Process[name] = newProcessInstance(true)
	}

	m.Process[name].Status = bus.STARTING
	go m.Watchdog(configSpec, m.Process[name])

	slog.Info("Start requested", "name", name)
	return nil
}

// Stop gracefully terminates the named process.
func (m *Manager) Stop(name string) error {
	m.mu.Lock()

	if m.ch == nil {
		m.mu.Unlock()
		return fmt.Errorf("process channels not set")
	}

	_, exists := m.Config[name]
	if !exists {
		m.mu.Unlock()
		return fmt.Errorf("process not found: %s", name)
	}

	stopChan, running := m.ch.Stop[name]
	if !running {
		m.mu.Unlock()
		return nil
	}

	pid := 0
	if proc, ok := m.Process[name]; ok {
		pid = proc.Pid
	}

	closeChannel(stopChan)
	delete(m.ch.Stop, name)
	m.mu.Unlock() // release before blocking on stopper

	if pid > 0 {
		sig, _ := engine.SignalFromString(m.Config[name].Stopsignal)
		if sig == nil {
			sig, _ = engine.SignalFromString("TERM")
		}
		stopper := engine.NewProcessStopper(m.handler, m.executor, time.Duration(m.Config[name].Stoptime)*time.Second, sig)
		p := &engine.Process{PID: pid}
		if err := stopper.Stop(p); err != nil {
			slog.Error("error stopping program directly", "program", name, "error", err)
		}
	}

	slog.Info("Stop requested", "name", name)

	// Wait for status to reflect stop (with timeout)
	stoptime := 10
	if spec, ok := m.Config[name]; ok {
		stoptime = spec.Stoptime + 2
	}
	timeout := time.Now().Add(time.Duration(stoptime) * time.Second)
	for time.Now().Before(timeout) {
		m.drainStatus()
		m.mu.Lock()
		if proc, ok := m.Process[name]; ok {
			if proc.Status == bus.STOPPED || proc.Status == bus.FATAL {
				m.mu.Unlock()
				return nil
			}
		}
		m.mu.Unlock()
		time.Sleep(50 * time.Millisecond)
	}

	return fmt.Errorf("timeout waiting for process to stop")
}

// Restart stops and then starts the named process.
func (m *Manager) Restart(name string) error {
	if err := m.Stop(name); err != nil {
		return err
	}

	// Wait for process to actually stop
	maxWait := 5 * time.Second
	start := time.Now()
	for time.Since(start) < maxWait {
		if !m.isRunning(name) {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	return m.Start(name)
}

// Reload loads the configuration file and applies the changes:
// - New processes are added and autostarted
// - Removed processes are stopped
// - Changed processes are restarted
func (m *Manager) Reload() (*ReloadResult, error) {
	if m.configPath == "" {
		return nil, fmt.Errorf("no config path stored, use NewManagerFromConfig() to initialize Manager")
	}

	loader := &config.YAMLLoader{}
	specs, err := loader.Load(m.configPath)
	if err != nil {
		return nil, fmt.Errorf("reload config: %w", err)
	}

	newConfig := make(map[string]*config.ConfigSpec)
	for i := range specs {
		spec := &specs[i]
		newConfig[spec.ProcessName] = spec
	}

	return m.applyConfigDiff(newConfig)
}

// ReloadFromConfig applies a new configuration directly.
func (m *Manager) ReloadFromConfig(newConfig map[string]*config.ConfigSpec) (*ReloadResult, error) {
	return m.applyConfigDiff(newConfig)
}

// Shutdown stops all processes and invokes the shutdown callback if set.
func (m *Manager) Shutdown() error {
	m.mu.Lock()

	for name, proc := range m.Process {
		if m.ch != nil {
			if stopCh, ok := m.ch.Stop[name]; ok {
				closeChannel(stopCh)
				delete(m.ch.Stop, name)
			}
		}
		if proc.Pid > 0 {
			sig, _ := engine.SignalFromString("TERM")
			stopper := engine.NewProcessStopper(m.handler, m.executor, 5*time.Second, sig)
			p := &engine.Process{PID: proc.Pid}
			if err := stopper.Stop(p); err != nil {
				slog.Error("error stopping process during shutdown", "process", name, "error", err)
			}
		}
		proc.Status = bus.STOPPED
	}

	shutdownFunc := m.shutdownFunc
	m.mu.Unlock()

	if shutdownFunc != nil {
		shutdownFunc()
	}

	return nil
}

// GetProcessInfo returns information about a specific process.
func (m *Manager) GetProcessInfo(name string) (protocol.ProcessInfo, error) {
	m.drainStatus()

	m.mu.Lock()
	defer m.mu.Unlock()

	proc, exists := m.Process[name]
	if !exists {
		return protocol.ProcessInfo{}, fmt.Errorf("process not found: %s", name)
	}

	return protocol.ProcessInfo{
		Name:    name,
		Status:  string(proc.Status),
		Pid:     proc.Pid,
		Uptime:  formatUptime(proc.LastStart),
		Retries: proc.RetryCount,
	}, nil
}

// GetAllProcessInfo returns information about all processes.
func (m *Manager) GetAllProcessInfo() ([]protocol.ProcessInfo, error) {
	m.drainStatus()

	m.mu.Lock()
	defer m.mu.Unlock()

	result := make([]protocol.ProcessInfo, 0, len(m.Process))
	for name, proc := range m.Process {
		info := protocol.ProcessInfo{
			Name:    name,
			Status:  string(proc.Status),
			Pid:     proc.Pid,
			Uptime:  formatUptime(proc.LastStart),
			Retries: proc.RetryCount,
		}
		result = append(result, info)
	}
	return result, nil
}

// ---------------------------------------------------------------------------
// Internal methods
// ---------------------------------------------------------------------------

// applyConfigDiff applies a new configuration, returning what changed.
func (m *Manager) applyConfigDiff(newConfig map[string]*config.ConfigSpec) (*ReloadResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.ch == nil {
		return nil, fmt.Errorf("process channels not set")
	}

	result := &ReloadResult{
		Added:     []string{},
		Removed:   []string{},
		Restarted: []string{},
	}

	// Find removed processes
	for name := range m.Config {
		if _, exists := newConfig[name]; !exists {
			result.Removed = append(result.Removed, name)
		}
	}

	// Find added or changed processes
	for name, newSpec := range newConfig {
		if oldSpec, exists := m.Config[name]; !exists {
			// New process
			result.Added = append(result.Added, name)
			m.Config[name] = newSpec
			if _, ok := m.Process[name]; !ok {
				m.Process[name] = newProcessInstance(newSpec.Autostart)
			}
			if newSpec.Autostart {
				if _, ok := m.ch.Stop[name]; !ok {
					m.ch.Stop[name] = make(chan struct{})
				}
				go m.Watchdog(newSpec, m.Process[name])
			}
		} else if configChanged(oldSpec, newSpec) {
			// Changed process
			result.Restarted = append(result.Restarted, name)
			m.Config[name] = newSpec
			wasRunning := m.isRunning(name)
			if wasRunning {
				if stopCh, ok := m.ch.Stop[name]; ok {
					closeChannel(stopCh)
					delete(m.ch.Stop, name)
				}
			}
			if newSpec.Autostart {
				if _, ok := m.ch.Stop[name]; !ok {
					m.ch.Stop[name] = make(chan struct{})
				}
				go m.Watchdog(newSpec, m.Process[name])
			}
		}
	}

	// Clean up removed processes
	for _, name := range result.Removed {
		if stopCh, ok := m.ch.Stop[name]; ok {
			closeChannel(stopCh)
			delete(m.ch.Stop, name)
		}
		delete(m.Config, name)
		if proc, ok := m.Process[name]; ok {
			proc.Status = bus.STOPPED
		}
	}

	return result, nil
}

// configChanged reports whether two configs differ in a meaningful way.
func configChanged(a, b *config.ConfigSpec) bool {
	if a.Cmd != b.Cmd ||
		a.Numprocs != b.Numprocs ||
		a.NumprocsStart != b.NumprocsStart ||
		a.Umask != b.Umask ||
		a.Workingdir != b.Workingdir ||
		a.Autostart != b.Autostart ||
		a.Autorestart != b.Autorestart ||
		a.Startretries != b.Startretries ||
		a.Starttime != b.Starttime ||
		a.Stopsignal != b.Stopsignal ||
		a.Stoptime != b.Stoptime ||
		a.Stdout != b.Stdout ||
		a.Stderr != b.Stderr {
		return true
	}
	if !slicesEqual(a.Exitcodes, b.Exitcodes) {
		return true
	}
	if len(a.Env) != len(b.Env) {
		return true
	}
	for k, v := range a.Env {
		if b.Env[k] != v {
			return true
		}
	}
	return false
}

// slicesEqual compares two int slices, treating nil and empty as equal.
func slicesEqual(a, b []int) bool {
	if len(a) == 0 && len(b) == 0 {
		return true
	}
	return slices.Equal(a, b)
}

// drainStatus drains all pending status updates from the channel.
// Must be called without holding the mutex.
func (m *Manager) drainStatus() {
	for {
		select {
		case update := <-m.ch.Status:
			m.handleStatusUpdate(update)
		default:
			return
		}
	}
}

// handleStatusUpdate applies a status update from a Watchdog.
// This is the only way the Manager's view of process state is updated.
func (m *Manager) handleStatusUpdate(update bus.ProcessUpdate) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if proc, ok := m.Process[update.Name]; ok {
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
	}
}

// isRunning reports whether the named process is currently running.
// Caller must hold the mutex.
func (m *Manager) isRunning(name string) bool {
	if m.ch == nil {
		return false
	}
	_, exists := m.ch.Stop[name]
	return exists
}

// StopAll stops all running processes.
func (m *Manager) StopAll() {
	m.mu.Lock()

	if m.ch == nil {
		m.mu.Unlock()
		return
	}

	for name, stopCh := range m.ch.Stop {
		if stopCh != nil {
			closeChannel(stopCh)
			delete(m.ch.Stop, name)
			slog.Info("stopped program", "name", name)
		}
	}
	m.mu.Unlock()

	maxStoptime := 10
	timeout := time.Now().Add(time.Duration(maxStoptime+3) * time.Second)
	for time.Now().Before(timeout) {
		infos, _ := m.GetAllProcessInfo()
		stopped := true
		for _, info := range infos {
			if info.Pid > 0 && info.Status == string(bus.RUNNING) {
				stopped = false
				break
			}
		}
		if stopped {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}

	infos, _ := m.GetAllProcessInfo()
	for _, info := range infos {
		if info.Pid > 0 {
			p, _ := os.FindProcess(info.Pid)
			_ = p.Kill()
		}
	}
}

// Spawn starts Watchdogs for all new processes in current config compared to prev.
// Used during initial startup to start autostart processes.
func (curr *Manager) Spawn(prev *Manager) {
	curr.mu.Lock()
	defer curr.mu.Unlock()

	if curr.ch == nil {
		slog.Error("process channels not set, cannot spawn watchdogs")
		return
	}

	prev.mu.Lock()
	prevKeys := make([]string, 0, len(prev.Config))
	for name := range prev.Config {
		prevKeys = append(prevKeys, name)
	}
	prev.mu.Unlock()

	for name, setting := range curr.Config {
		found := slices.Contains(prevKeys, name)

		if !found {
			if _, ok := curr.ch.Stop[name]; !ok {
				curr.ch.Stop[name] = make(chan struct{})
			}
			go curr.Watchdog(setting, curr.Process[name])
			slog.Info("started new program", "name", setting.ProcessName)
		}
	}

	for _, prevName := range prevKeys {
		if _, exists := curr.Config[prevName]; !exists {
			if stopChan, ok := curr.ch.Stop[prevName]; ok {
				close(stopChan)
				delete(curr.ch.Stop, prevName)
			}
			slog.Info("stopped removed program", "name", prevName)
		}
	}
}

// NewManagerFromConfig creates a Manager with configuration loaded from file.
func NewManagerFromConfig(path string) (*Manager, error) {
	loader := &config.YAMLLoader{}
	specs, err := loader.Load(path)
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}

	manager := NewManager()
	manager.configPath = path

	for i := range specs {
		spec := &specs[i]
		manager.Config[spec.ProcessName] = spec
		if _, ok := manager.Process[spec.ProcessName]; !ok {
			manager.Process[spec.ProcessName] = newProcessInstance(spec.Autostart)
		}
	}

	return manager, nil
}

// closeChannel safely closes a channel, recovering from double-close panics.
func closeChannel(ch chan struct{}) {
	defer func() {
		if r := recover(); r != nil {
			slog.Debug("channel already closed", "reason", r)
		}
	}()
	close(ch)
}

// formatUptime formats a start time as a human-readable uptime string.
func formatUptime(startTime time.Time) string {
	duration := time.Since(startTime)
	if startTime.IsZero() {
		return "0s"
	}
	hours := int(duration.Hours())
	minutes := int(duration.Minutes()) % 60
	seconds := int(duration.Seconds()) % 60

	if hours > 0 {
		return fmt.Sprintf("%dh %dm", hours, minutes)
	}
	if minutes > 0 {
		return fmt.Sprintf("%dm %ds", minutes, seconds)
	}
	return fmt.Sprintf("%ds", seconds)
}
