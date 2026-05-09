package app

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"slices"
	"strings"
	"sync"
	"syscall"
	"time"

	"taskmaster/internal/bus"
	"taskmaster/internal/config"
	"taskmaster/internal/engine"
	"taskmaster/internal/protocol"
)

// ProcessQuerier defines methods for querying process status.
type ProcessQuerier interface {
	GetProcessInfo(name string) (protocol.ProcessInfo, error)
	GetAllProcessInfo() ([]protocol.ProcessInfo, error)
}

// ProcessController defines methods for controlling individual processes.
type ProcessController interface {
	Start(name string) error
	Stop(name string) error
	Restart(name string) error
}

// DaemonController defines methods for daemon-level operations.
type DaemonController interface {
	Reload() (*ReloadResult, error)
	Shutdown() error
}

// ManagerInterface defines the boundary for RPC handlers.
// Handlers use this interface; production uses *Manager which implements it.
// Composed of smaller focused interfaces for better segregation.
type ManagerInterface interface {
	ProcessQuerier
	ProcessController
	DaemonController
}

type ProcessInstance struct {
	Pid        int
	Status     bus.Status
	RetryCount int
	LastStart  time.Time
	Intended   bool
	Strategy   engine.RetryStrategy
	Mu         sync.RWMutex
}

// newProcessInstance creates a new ProcessInstance with the given autostart setting
// and optional retry strategy. This factory ensures consistent initialization.
func newProcessInstance(autostart bool, strategy engine.RetryStrategy) *ProcessInstance {
	return &ProcessInstance{
		Status:     bus.STOPPED,
		Intended:   autostart,
		RetryCount: 0,
		Strategy:   strategy,
	}
}

func (pi *ProcessInstance) GetStatus() bus.Status {
	pi.Mu.RLock()
	defer pi.Mu.RUnlock()
	return pi.Status
}

func (pi *ProcessInstance) SetStatus(s bus.Status) {
	pi.Mu.Lock()
	defer pi.Mu.Unlock()
	pi.Status = s
}

func (pi *ProcessInstance) GetPid() int {
	pi.Mu.RLock()
	defer pi.Mu.RUnlock()
	return pi.Pid
}

func (pi *ProcessInstance) SetPid(p int) {
	pi.Mu.Lock()
	defer pi.Mu.Unlock()
	pi.Pid = p
}

func (pi *ProcessInstance) SetStateOnStart(pid int) {
	pi.Mu.Lock()
	defer pi.Mu.Unlock()
	pi.Pid = pid
	pi.Status = bus.RUNNING
	pi.LastStart = time.Now()
	pi.RetryCount = 0 // Reset retry count on successful start
}

func (pi *ProcessInstance) SetStateOnBackoff(attempt int) {
	pi.Mu.Lock()
	defer pi.Mu.Unlock()
	pi.Status = bus.BACKOFF
	pi.RetryCount = attempt
}

func (pi *ProcessInstance) SetRetryCount(attempt int) {
	pi.Mu.Lock()
	defer pi.Mu.Unlock()
	pi.RetryCount = attempt
}

func (pi *ProcessInstance) State() (int, bus.Status) {
	pi.Mu.RLock()
	defer pi.Mu.RUnlock()
	return pi.Pid, pi.Status
}

type Manager struct {
	Config   map[string]*config.ConfigSpec
	Process  map[string]*ProcessInstance
	executor engine.ProcessExecutor
	handler  engine.SignalHandler
	Mu       sync.RWMutex

	// stops tracks active watchdog stop channels for each process
	stops map[string]chan struct{}
	// updates channel for process status updates
	updates chan bus.ProcessUpdate
	// shutdownFunc is called when Shutdown() is invoked
	shutdownFunc func()
	// configPath stores the path used during NewManagerFromConfig() for Reload()
	configPath string
}

func NewManager() *Manager {
	return &Manager{
		Config:   make(map[string]*config.ConfigSpec),
		Process:  make(map[string]*ProcessInstance),
		stops:    make(map[string]chan struct{}),
		executor: engine.NewOsProcessExecutor(),
		handler:  &engine.OSSignalHandler{},
	}
}

// SetUpdatesChannel sets the channel for receiving process updates.
// Must be called before Start/Reload operations.
func (m *Manager) SetUpdatesChannel(updates chan bus.ProcessUpdate) {
	m.updates = updates
}

// SetShutdownFunc sets the function to call when Shutdown is requested.
func (m *Manager) SetShutdownFunc(fn func()) {
	m.shutdownFunc = fn
}

// isRunning returns true if the watchdog is running for the given process.
func (m *Manager) isRunning(name string) bool {
	_, exists := m.stops[name]
	return exists
}

func (m *Manager) Watchdog(setting *config.ConfigSpec, proc *ProcessInstance, updates chan bus.ProcessUpdate, stop chan struct{}) {
	parts := strings.Fields(setting.Cmd)
	if len(parts) == 0 {
		slog.Error("no command specified for program", "program", setting.Program)
		updates <- bus.ProcessUpdate{Name: setting.ProcessName, Status: bus.FATAL}
		return
	}

	if !setting.Autostart {
		slog.Info("program set to not autostart, skipping", "program", setting.Program)
		updates <- bus.ProcessUpdate{Name: setting.ProcessName, Status: bus.STOPPED}
		return
	}

	var strategy engine.RetryStrategy
	if proc != nil && proc.Strategy != nil {
		strategy = proc.Strategy
	} else {
		strategy = engine.RetryStrategyFactory(setting.Autorestart, setting.Exitcodes)
	}

	maxAttempts := setting.Startretries + 1
	if maxAttempts <= 0 {
		maxAttempts = 1
	}
	retryDelay := time.Duration(0)

	watcher := engine.NewProcessWatcherWithStrategy(m.executor, strategy, setting.Startretries, retryDelay)

	var (
		currentPID int
		pidMu      sync.RWMutex
	)

	if proc == nil {
		slog.Error("ProcessInstance not found in manager", "process", setting.ProcessName)
		updates <- bus.ProcessUpdate{Name: setting.ProcessName, Status: bus.FATAL}
		return
	}

	watcher.OnProcessStarted = func(pid int) {
		pidMu.Lock()
		currentPID = pid
		pidMu.Unlock()

		proc.SetStateOnStart(pid)

		updates <- bus.ProcessUpdate{Name: setting.ProcessName, Status: bus.RUNNING, Pid: pid}
		slog.Info("started program", "program", setting.Program, "pid", pid)
	}

	watcher.OnBackoff = func(attempt int) {
		proc.SetStateOnBackoff(attempt)
		pidMu.RLock()
		pid := currentPID
		pidMu.RUnlock()
		updates <- bus.ProcessUpdate{Name: setting.ProcessName, Status: bus.BACKOFF, Pid: pid}
		slog.Info("program backoff", "program", setting.Program, "attempt", attempt)
	}

	watcher.OnSpawnFailed = func(attempt int) {
		proc.SetRetryCount(attempt)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	result := make(chan engine.ExitCode, 1)
	errCh := make(chan error, 1)

	go func() {
		exitCode, err := watcher.Run(ctx, *setting)
		if err != nil {
			errCh <- err
		} else {
			result <- exitCode
		}
	}()

	select {
	case <-stop:
		cancel()

		pidMu.RLock()
		pid := currentPID
		pidMu.RUnlock()

		if pid > 0 {
			stopper := engine.NewProcessStopper(m.handler, m.executor, time.Duration(setting.Stoptime)*time.Second)
			p := &engine.Process{PID: pid}
			if err := stopper.Stop(p); err != nil {
				slog.Error("error stopping program", "program", setting.Program, "error", err)
			}
		}

		select {
		case code := <-result:
			sendFinalUpdate(setting, updates, code, true)
		case err := <-errCh:
			slog.Error("program exited with error after stop", "program", setting.Program, "error", err)
			updates <- bus.ProcessUpdate{Name: setting.ProcessName, Status: bus.FATAL}
		case <-time.After(time.Duration(setting.Stoptime) * time.Second):
			slog.Error("program did not exit after stop timeout", "program", setting.Program)
			updates <- bus.ProcessUpdate{Name: setting.ProcessName, Status: bus.FATAL}
		}

	case code := <-result:
		sendFinalUpdate(setting, updates, code, false)

	case err := <-errCh:
		slog.Error("program exited with error", "program", setting.Program, "error", err)
		updates <- bus.ProcessUpdate{Name: setting.ProcessName, Status: bus.FATAL}
	}
}

func sendFinalUpdate(setting *config.ConfigSpec, updates chan bus.ProcessUpdate, code engine.ExitCode, stopped bool) {
	if stopped {
		if code == 0 {
			slog.Info("program stopped successfully", "program", setting.Program)
		} else {
			slog.Error("program stopped with non-zero exit code", "program", setting.Program, "code", code)
		}
		updates <- bus.ProcessUpdate{Name: setting.ProcessName, Status: bus.STOPPED}
		return
	}

	if code == 0 {
		slog.Info("program exited successfully", "program", setting.Program)
		updates <- bus.ProcessUpdate{Name: setting.ProcessName, Status: bus.STOPPED}
	} else {
		slog.Error("program exited with code", "program", setting.Program, "code", code)
		updates <- bus.ProcessUpdate{Name: setting.ProcessName, Status: bus.FATAL}
	}
}

// StopAll stops all managed processes by signaling their watchdogs and waiting for them to exit.
// It closes all stop channels and force-kills any remaining processes after timeout.
func (m *Manager) StopAll(stops map[string]chan struct{}) {
	for name, ch := range stops {
		if ch != nil {
			closeChannel(ch)
			delete(stops, name)
			slog.Info("stopped program", "name", name)
		}
	}

	m.Mu.RLock()
	maxStoptime := 0
	for _, cfg := range m.Config {
		if cfg.Stoptime > maxStoptime {
			maxStoptime = cfg.Stoptime
		}
	}
	m.Mu.RUnlock()

	timeout := time.Now().Add(time.Duration(maxStoptime+3) * time.Second)
	for time.Now().Before(timeout) {
		m.Mu.RLock()
		stopped := true
		for _, proc := range m.Process {
			pid, status := proc.State()
			if pid > 0 && status == bus.RUNNING {
				stopped = false
				break
			}
		}
		m.Mu.RUnlock()

		if stopped {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}

	m.Mu.Lock()
	for _, proc := range m.Process {
		pid, _ := proc.State()
		if pid > 0 {
			p, _ := os.FindProcess(pid)
			_ = p.Kill()
		}
	}
	m.Mu.Unlock()
}

// Spawn starts watchdogs for new processes in the current manager config
// and stops watchdogs for processes that exist in prev but not in current.
// This is typically called during initial load or hot-reload.
func (curr *Manager) Spawn(prev *Manager, updates chan bus.ProcessUpdate, stops map[string]chan struct{}) {
	curr.Mu.Lock()
	defer curr.Mu.Unlock()

	prev.Mu.RLock()
	prevKeys := make([]string, 0, len(prev.Config))
	for name := range prev.Config {
		prevKeys = append(prevKeys, name)
	}
	prev.Mu.RUnlock()

	for name, setting := range curr.Config {
		found := false
		for _, prevName := range prevKeys {
			if prevName == name {
				found = true
				break
			}
		}

		if !found {
			if _, ok := curr.Process[name]; !ok {
				strategy := engine.RetryStrategyFactory(setting.Autorestart, setting.Exitcodes)
				curr.Process[name] = newProcessInstance(true, strategy)
			}

			if _, ok := stops[name]; !ok {
				stops[name] = make(chan struct{})
			}

			go curr.Watchdog(setting, curr.Process[name], updates, stops[name])
			slog.Info("started new program", "name", setting.ProcessName)
		}
	}

	for _, prevName := range prevKeys {
		if _, exists := curr.Config[prevName]; !exists {
			if ch, ok := stops[prevName]; ok {
				closeChannel(ch)
				delete(stops, prevName)
			}
			slog.Info("stopped removed program", "name", prevName)
		}
	}
}

// NewManagerFromConfig loads configuration from the specified path and creates
// a Manager with all configured processes. The config path is stored for later Reload().
func NewManagerFromConfig(path string) (*Manager, error) {
	loader := &config.YAMLLoader{}
	specs, err := loader.Load(path)
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}

	manager := NewManager()
	manager.Mu.Lock()
	defer manager.Mu.Unlock()

	// Store config path for Reload()
	manager.configPath = path

	for i := range specs {
		spec := &specs[i]
		manager.Config[spec.ProcessName] = spec
		if _, ok := manager.Process[spec.ProcessName]; !ok {
			manager.Process[spec.ProcessName] = newProcessInstance(spec.Autostart, nil)
		}
	}

	return manager, nil
}

// closeChannel closes a channel safely, recovering from double-close panics.
// Logs at debug level if a panic occurs to aid debugging without crashing.
func closeChannel(ch chan struct{}) {
	defer func() {
		if r := recover(); r != nil {
			slog.Debug("channel already closed", "reason", r)
		}
	}()
	close(ch)
}

// ============================================================================
// ManagerInterface Implementation
// ============================================================================

// GetProcessInfo returns info for a single process by name.
func (m *Manager) GetProcessInfo(name string) (protocol.ProcessInfo, error) {
	m.Mu.RLock()
	proc, exists := m.Process[name]
	m.Mu.RUnlock()

	if !exists {
		return protocol.ProcessInfo{}, fmt.Errorf("process not found: %s", name)
	}

	// Read process state under its own lock
	proc.Mu.RLock()
	info := protocol.ProcessInfo{
		Name:    name,
		Status:  string(proc.Status),
		Pid:     proc.Pid,
		Uptime:  formatUptime(proc.LastStart),
		Retries: proc.RetryCount,
	}
	proc.Mu.RUnlock()

	return info, nil
}

// GetAllProcessInfo returns info for all managed processes.
func (m *Manager) GetAllProcessInfo() ([]protocol.ProcessInfo, error) {
	m.Mu.RLock()
	processes := make(map[string]*ProcessInstance, len(m.Process))
	for name, proc := range m.Process {
		processes[name] = proc
	}
	m.Mu.RUnlock()

	result := make([]protocol.ProcessInfo, 0, len(processes))
	for name, proc := range processes {
		// Read each process state under its own lock
		proc.Mu.RLock()
		info := protocol.ProcessInfo{
			Name:    name,
			Status:  string(proc.Status),
			Pid:     proc.Pid,
			Uptime:  formatUptime(proc.LastStart),
			Retries: proc.RetryCount,
		}
		proc.Mu.RUnlock()
		result = append(result, info)
	}
	return result, nil
}

// formatUptime returns a human-readable uptime string.
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

// Start starts a single process by name.
// Returns error if process not found, updates channel not set, or start operation fails.
// If the process watchdog is already running, returns success (idempotent).
func (m *Manager) Start(name string) error {
	m.Mu.Lock()
	defer m.Mu.Unlock()

	if m.updates == nil {
		return fmt.Errorf("updates channel not set, call SetUpdatesChannel first")
	}

	configSpec, exists := m.Config[name]
	if !exists {
		return fmt.Errorf("process not found: %s", name)
	}

	// Check if watchdog is already running
	if m.isRunning(name) {
		return nil // Idempotent: already running is not an error
	}

	// Ensure ProcessInstance exists
	if _, ok := m.Process[name]; !ok {
		strategy := engine.RetryStrategyFactory(configSpec.Autorestart, configSpec.Exitcodes)
		m.Process[name] = newProcessInstance(true, strategy)
	}

	// Create stop channel for this watchdog
	stopChan := make(chan struct{})
	m.stops[name] = stopChan

	// Set process as starting
	m.Process[name].SetStatus(bus.STARTING)

	// Spawn watchdog goroutine
	go m.Watchdog(configSpec, m.Process[name], m.updates, stopChan)

	slog.Info("RPC Start requested", "name", name)
	return nil
}

// Stop stops a single process by name.
// Returns error if process not found or stop operation fails.
// Signals the watchdog to stop the process gracefully.
func (m *Manager) Stop(name string) error {
	m.Mu.Lock()

	_, exists := m.Config[name]
	if !exists {
		m.Mu.Unlock()
		return fmt.Errorf("process not found: %s", name)
	}

	// Check if watchdog is running
	stopChan, running := m.stops[name]
	if !running {
		m.Mu.Unlock()
		return nil // Idempotent: not running is not an error
	}

	// Get the process instance for PID (may not exist if never started)
	pid := 0
	if proc, ok := m.Process[name]; ok {
		pid = proc.GetPid()
	}
	m.Mu.Unlock()

	// Signal watchdog to stop by closing the stop channel
	closeChannel(stopChan)

	// Also try to signal the process directly if we have a PID
	if pid > 0 {
		p, err := os.FindProcess(pid)
		if err == nil {
			p.Signal(syscall.SIGTERM)
		}
	}

	// Remove from stops map and update status
	m.Mu.Lock()
	delete(m.stops, name)
	if proc, ok := m.Process[name]; ok {
		proc.SetStatus(bus.STOPPED)
	}
	m.Mu.Unlock()

	slog.Info("RPC Stop requested", "name", name)
	return nil
}

// Restart restarts a single process by name.
// Returns error if process not found or restart operation fails.
// Waits briefly between stop and start to allow cleanup.
func (m *Manager) Restart(name string) error {
	// Stop the process
	if err := m.Stop(name); err != nil {
		return err
	}

	// Brief wait to allow process cleanup
	time.Sleep(100 * time.Millisecond)

	// Start the process
	return m.Start(name)
}

// ReloadResult contains the diff of configuration changes.
type ReloadResult struct {
	Added     []string
	Removed   []string
	Restarted []string
}

// Reload reloads the configuration from disk and applies the diff.
// Loads config from the path used during NewManagerFromConfig() and applies changes.
// Returns added, removed, and restarted process names.
func (m *Manager) Reload() (*ReloadResult, error) {
	m.Mu.RLock()
	configPath := m.configPath
	m.Mu.RUnlock()

	if configPath == "" {
		return nil, fmt.Errorf("no config path stored, use NewManagerFromConfig() to initialize Manager")
	}

	// Load new config from disk
	loader := &config.YAMLLoader{}
	specs, err := loader.Load(configPath)
	if err != nil {
		return nil, fmt.Errorf("reload config: %w", err)
	}

	// Convert specs to config map
	newConfig := make(map[string]*config.ConfigSpec)
	for i := range specs {
		spec := &specs[i]
		newConfig[spec.ProcessName] = spec
	}

	// Apply the diff
	return m.ReloadFromConfig(newConfig)
}

// ReloadFromConfig applies a new configuration to the manager.
// Stops removed processes, starts added ones, restarts changed ones.
func (m *Manager) ReloadFromConfig(newConfig map[string]*config.ConfigSpec) (*ReloadResult, error) {
	m.Mu.Lock()
	defer m.Mu.Unlock()

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

	// Find added and changed processes
	for name, newSpec := range newConfig {
		if oldSpec, exists := m.Config[name]; !exists {
			result.Added = append(result.Added, name)
			// Add to config and create process instance
			m.Config[name] = newSpec
			if _, ok := m.Process[name]; !ok {
				strategy := engine.RetryStrategyFactory(newSpec.Autorestart, newSpec.Exitcodes)
				m.Process[name] = newProcessInstance(newSpec.Autostart, strategy)
			}
			// If autostart, start the watchdog
			if newSpec.Autostart && m.updates != nil {
				stopChan := make(chan struct{})
				m.stops[name] = stopChan
				go m.Watchdog(newSpec, m.Process[name], m.updates, stopChan)
			}
		} else if configChanged(oldSpec, newSpec) {
			result.Restarted = append(result.Restarted, name)
			// Update config
			m.Config[name] = newSpec
			// Restart if running or if autostart is true
			wasRunning := m.isRunning(name)
			if wasRunning {
				if ch, ok := m.stops[name]; ok {
					closeChannel(ch)
					delete(m.stops, name)
				}
			}
			// Restart immediately if autostart is true
			if newSpec.Autostart && m.updates != nil {
				stopChan := make(chan struct{})
				m.stops[name] = stopChan
				go m.Watchdog(newSpec, m.Process[name], m.updates, stopChan)
			}
		}
	}

	// Stop removed processes
	for _, name := range result.Removed {
		if ch, ok := m.stops[name]; ok {
			closeChannel(ch)
			delete(m.stops, name)
		}
		delete(m.Config, name)
		// Don't delete from Process map - keep for status history
		if proc, ok := m.Process[name]; ok {
			proc.SetStatus(bus.STOPPED)
		}
	}

	return result, nil
}

// configChanged returns true if two config specs are different.
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
	// Compare slices
	if !slices.Equal(a.Exitcodes, b.Exitcodes) {
		return true
	}
	// Compare env maps
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

// Shutdown shuts down the daemon.
// Stops all managed processes gracefully and signals daemon exit.
func (m *Manager) Shutdown() error {
	m.Mu.Lock()

	// Signal all processes to stop, including those without active watchdogs
	for name, proc := range m.Process {
		// Stop watchdog if running
		if stopChan, ok := m.stops[name]; ok {
			closeChannel(stopChan)
			delete(m.stops, name)
		}

		// Signal process directly if it has a PID
		pid := proc.GetPid()
		if pid > 0 {
			p, err := os.FindProcess(pid)
			if err == nil {
				p.Signal(syscall.SIGTERM)
			}
		}
		proc.SetStatus(bus.STOPPED)
	}
	m.Mu.Unlock()

	// Signal daemon to exit
	if m.shutdownFunc != nil {
		go m.shutdownFunc()
	}

	return nil
}
