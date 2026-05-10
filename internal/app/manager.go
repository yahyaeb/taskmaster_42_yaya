package app

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"os"
	"slices"
	"strings"
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
	// Note: ProcessInstance is owned exclusively by its Watchdog goroutine
	// No mutex needed - all access is serialized through channels
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

// Note: All ProcessInstance methods removed - state is accessed directly
// by the Watchdog goroutine that owns it. External access goes through
// the Manager's event loop which receives updates via the Status channel.

type Manager struct {
	Config   map[string]*config.ConfigSpec
	Process  map[string]*ProcessInstance
	executor engine.ProcessExecutor
	handler  engine.SignalHandler

	// ch holds process channels for communication between components
	ch *ProcessChannels
	// shutdownFunc is called when Shutdown() is invoked
	shutdownFunc func()
	// configPath stores the path used during NewManagerFromConfig() for Reload()
	configPath string

	// commandCh receives commands for the event loop (start, stop, restart, etc.)
	commandCh chan ManagerCommand
	// queryCh receives queries for the event loop (get, list)
	queryCh chan ManagerQuery
	// running signals when the event loop has started
	running chan struct{}
}

func NewManager() *Manager {
	return &Manager{
		Config:    make(map[string]*config.ConfigSpec),
		Process:   make(map[string]*ProcessInstance),
		executor:  engine.NewOsProcessExecutor(),
		handler:   &engine.OSSignalHandler{},
		commandCh: make(chan ManagerCommand, 100),
		queryCh:   make(chan ManagerQuery, 100),
		running:   make(chan struct{}),
	}
}

// SetShutdownFunc sets the function to call when Shutdown is requested.
func (m *Manager) SetShutdownFunc(fn func()) {
	m.shutdownFunc = fn
}

// SetChannels sets the process channels for this manager.
func (m *Manager) SetChannels(ch *ProcessChannels) {
	m.ch = ch
}

// Channels returns the process channels for this manager.
func (m *Manager) Channels() *ProcessChannels {
	return m.ch
}

// Run starts the Manager's event loop. It must be called in a goroutine.
// The event loop owns all Manager state and processes commands/queries synchronously.
func (m *Manager) Run() {
	close(m.running) // Signal that event loop is running
	for {
		select {
		case cmd := <-m.commandCh:
			m.handleCommand(cmd)
		case query := <-m.queryCh:
			m.handleQuery(query)
		case update := <-m.ch.Status:
			m.handleStatusUpdate(update)
		}
	}
}

// WaitForRunning blocks until the event loop has started.
func (m *Manager) WaitForRunning() {
	<-m.running
}

// handleCommand processes a ManagerCommand synchronously.
func (m *Manager) handleCommand(cmd ManagerCommand) {
	switch cmd.Type {
	case "start":
		cmd.Resp <- m.startInternal(cmd.Name)
	case "stop":
		cmd.Resp <- m.stopInternal(cmd.Name)
	case "restart":
		cmd.Resp <- m.restartInternal(cmd.Name)
	case "reload":
		result, err := m.reloadInternal()
		cmd.ReloadResp <- ReloadCommandResult{Result: result, Err: err}
		cmd.Resp <- err
	case "shutdown":
		cmd.Resp <- m.shutdownInternal()
	case "spawn":
		// Spawn is handled internally during init/reload
		cmd.Resp <- nil
	default:
		cmd.Resp <- fmt.Errorf("unknown command: %s", cmd.Type)
	}
}

// handleQuery processes a ManagerQuery synchronously.
func (m *Manager) handleQuery(query ManagerQuery) {
	switch query.Type {
	case "get":
		info, err := m.getProcessInfoInternal(query.Name)
		query.Resp <- QueryResult{Info: info, Err: err}
	case "list":
		infos, err := m.getAllProcessInfoInternal()
		query.Resp <- QueryResult{Infos: infos, Err: err}
	default:
		query.Resp <- QueryResult{Err: fmt.Errorf("unknown query: %s", query.Type)}
	}
}

// handleStatusUpdate processes a status update from a Watchdog.
func (m *Manager) handleStatusUpdate(update bus.ProcessUpdate) {
	if proc, ok := m.Process[update.Name]; ok {
		proc.Status = update.Status
		if update.Pid > 0 {
			proc.Pid = update.Pid
		}
	}
}

// isRunning returns true if the watchdog is running for the given process.
func (m *Manager) isRunning(name string) bool {
	if m.ch == nil {
		return false
	}
	_, exists := m.ch.Stop[name]
	return exists
}

func (m *Manager) Watchdog(setting *config.ConfigSpec, proc *ProcessInstance) {
	parts := strings.Fields(setting.Cmd)

	if m.ch == nil {
		slog.Error("process channels not set", "program", setting.Program)
		return
	}

	stop := m.ch.Stop[setting.ProcessName]
	updates := m.ch.Status

	if len(parts) == 0 {
		slog.Error("no command specified for program", "program", setting.Program)
		updates <- bus.ProcessUpdate{Name: setting.ProcessName, Status: bus.FATAL}
		return
	}

	var strategy engine.RetryStrategy
	if proc != nil && proc.Strategy != nil {
		strategy = proc.Strategy
	} else {
		strategy = engine.RetryStrategyFactory(setting.Autorestart, setting.Exitcodes)
	}

	// For autorestart=always, use unlimited retries. For others, use startretries.
	var maxRetries int
	if _, isAlwaysRestart := strategy.(*engine.AlwaysRestart); isAlwaysRestart {
		maxRetries = math.MaxInt  // Restart indefinitely for autorestart=always
	} else {
		maxRetries = setting.Startretries
	}

	retryDelay := time.Duration(0)

	watcher := engine.NewProcessWatcherWithStrategy(m.executor, strategy, maxRetries, retryDelay)
	watcher.StarttimeSec = setting.Starttime

	if proc == nil {
		slog.Error("ProcessInstance not found in manager", "process", setting.ProcessName)
		updates <- bus.ProcessUpdate{Name: setting.ProcessName, Status: bus.FATAL}
		return
	}

	watcher.OnProcessStarted = func(pid int) {
		// Direct field access - Watchdog owns this ProcessInstance
		proc.Pid = pid
		proc.Status = bus.STARTING
		proc.LastStart = time.Now()
		proc.RetryCount = 0

		updates <- bus.ProcessUpdate{Name: setting.ProcessName, Status: bus.STARTING, Pid: pid}
		slog.Info("program starting", "program", setting.Program, "pid", pid)
	}

	watcher.OnProcessRunning = func(pid int) {
		// Direct field access - Watchdog owns this ProcessInstance
		proc.Status = bus.RUNNING
		fmt.Printf("[DEBUG] Sending status update for %s to RUNNING\n", setting.ProcessName)
		updates <- bus.ProcessUpdate{Name: setting.ProcessName, Status: bus.RUNNING, Pid: pid}
		fmt.Printf("[DEBUG] Status update sent for %s\n", setting.ProcessName)
		slog.Info("started program", "program", setting.Program, "pid", pid)
	}

	watcher.OnBackoff = func(attempt int) {
		// Direct field access - Watchdog owns this ProcessInstance
		proc.Status = bus.BACKOFF
		proc.RetryCount = attempt
		// Note: PID is not available during backoff (process hasn't started yet)
		updates <- bus.ProcessUpdate{Name: setting.ProcessName, Status: bus.BACKOFF, Pid: proc.Pid}
		slog.Info("program backoff", "program", setting.Program, "attempt", attempt)
	}

	watcher.OnSpawnFailed = func(attempt int) {
		// Direct field access - Watchdog owns this ProcessInstance
		proc.RetryCount = attempt
	}

	watcher.OnStarting = func() {
		// Direct field access - Watchdog owns this ProcessInstance
		proc.Status = bus.STARTING
		updates <- bus.ProcessUpdate{Name: setting.ProcessName, Status: bus.STARTING}
		slog.Info("program starting (retry)", "program", setting.Program)
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
		// cancel() - do not cancel, let watcher wait for process response

		// Direct field access - Watchdog owns this ProcessInstance
		pid := proc.Pid
		if pid > 0 {
			sig, _ := engine.SignalFromString(setting.Stopsignal)
			if sig == nil {
				sig, _ = engine.SignalFromString("TERM") // Default to TERM
			}
			stopper := engine.NewProcessStopper(m.handler, m.executor, time.Duration(setting.Stoptime)*time.Second, sig)
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
// Note: This is called from outside the event loop, so it needs special handling.
// For now, it sends a command to the event loop.
func (m *Manager) StopAll() {
	if m.ch == nil {
		return
	}

	// Close all stop channels
	for name, stopCh := range m.ch.Stop {
		if stopCh != nil {
			closeChannel(stopCh)
			delete(m.ch.Stop, name)
			slog.Info("stopped program", "name", name)
		}
	}

	// Wait for processes to stop by polling through queries
	maxStoptime := 10 // default
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

	// Force kill remaining processes
	infos, _ := m.GetAllProcessInfo()
	for _, info := range infos {
		if info.Pid > 0 {
			p, _ := os.FindProcess(info.Pid)
			_ = p.Kill()
		}
	}
}

// Spawn starts watchdogs for new processes in the current manager config
// and stops watchdogs for processes that exist in prev but not in current.
// This is typically called during initial load or hot-reload.
// Note: This method is only safe to call before Run() starts or from within the event loop.
func (curr *Manager) Spawn(prev *Manager) {
	if curr.ch == nil {
		slog.Error("process channels not set, cannot spawn watchdogs")
		return
	}

	prevKeys := make([]string, 0, len(prev.Config))
	for name := range prev.Config {
		prevKeys = append(prevKeys, name)
	}

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

// NewManagerFromConfig loads configuration from the specified path and creates
// a Manager with all configured processes. The config path is stored for later Reload().
// Note: The Manager returned is not yet running - call Run() in a goroutine before using.
func NewManagerFromConfig(path string) (*Manager, error) {
	loader := &config.YAMLLoader{}
	specs, err := loader.Load(path)
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}

	manager := NewManager()

	// Store config path for Reload()
	manager.configPath = path

	// Note: No mutex needed - event loop not running yet, single goroutine access
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
// Sends a query to the event loop and waits for response.
func (m *Manager) GetProcessInfo(name string) (protocol.ProcessInfo, error) {
	resp := make(chan QueryResult, 1)
	m.queryCh <- ManagerQuery{Type: "get", Name: name, Resp: resp}
	result := <-resp
	return result.Info, result.Err
}

// getProcessInfoInternal is called by the event loop - no locking needed.
func (m *Manager) getProcessInfoInternal(name string) (protocol.ProcessInfo, error) {
	proc, exists := m.Process[name]
	if !exists {
		return protocol.ProcessInfo{}, fmt.Errorf("process not found: %s", name)
	}

	// Direct field access - event loop owns all state
	info := protocol.ProcessInfo{
		Name:    name,
		Status:  string(proc.Status),
		Pid:     proc.Pid,
		Uptime:  formatUptime(proc.LastStart),
		Retries: proc.RetryCount,
	}
	return info, nil
}

// GetAllProcessInfo returns info for all managed processes.
// Sends a query to the event loop and waits for response.
func (m *Manager) GetAllProcessInfo() ([]protocol.ProcessInfo, error) {
	resp := make(chan QueryResult, 1)
	m.queryCh <- ManagerQuery{Type: "list", Resp: resp}
	result := <-resp
	return result.Infos, result.Err
}

// getAllProcessInfoInternal is called by the event loop - no locking needed.
func (m *Manager) getAllProcessInfoInternal() ([]protocol.ProcessInfo, error) {
	result := make([]protocol.ProcessInfo, 0, len(m.Process))
	for name, proc := range m.Process {
		// Direct field access - event loop owns all state
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
// Sends a command to the event loop and waits for response.
func (m *Manager) Start(name string) error {
	resp := make(chan error, 1)
	m.commandCh <- ManagerCommand{Type: "start", Name: name, Resp: resp}
	return <-resp
}

// startInternal is called by the event loop - no locking needed.
func (m *Manager) startInternal(name string) error {
	if m.ch == nil {
		return fmt.Errorf("process channels not set")
	}

	configSpec, exists := m.Config[name]
	if !exists {
		return fmt.Errorf("process not found: %s", name)
	}

	// Check if watchdog is already running
	if m.isRunning(name) {
		return nil // Idempotent: already running is not an error
	}

	// Create stop channel if not exists
	if _, ok := m.ch.Stop[name]; !ok {
		m.ch.Stop[name] = make(chan struct{})
	}

	// Ensure ProcessInstance exists
	if _, ok := m.Process[name]; !ok {
		strategy := engine.RetryStrategyFactory(configSpec.Autorestart, configSpec.Exitcodes)
		m.Process[name] = newProcessInstance(true, strategy)
	}

	// Set process as starting
	m.Process[name].Status = bus.STARTING

	// Spawn watchdog goroutine
	go m.Watchdog(configSpec, m.Process[name])

	slog.Info("RPC Start requested", "name", name)
	return nil
}

// Stop stops a single process by name.
// Sends a command to the event loop and waits for response.
func (m *Manager) Stop(name string) error {
	resp := make(chan error, 1)
	m.commandCh <- ManagerCommand{Type: "stop", Name: name, Resp: resp}
	return <-resp
}

// stopInternal is called by the event loop - no locking needed.
func (m *Manager) stopInternal(name string) error {
	if m.ch == nil {
		return fmt.Errorf("process channels not set")
	}

	_, exists := m.Config[name]
	if !exists {
		return fmt.Errorf("process not found: %s", name)
	}

	// Check if watchdog is running
	stopChan, running := m.ch.Stop[name]
	if !running {
		return nil // Idempotent: not running is not an error
	}

	// Get the process instance for PID (may not exist if never started)
	pid := 0
	if proc, ok := m.Process[name]; ok {
		pid = proc.Pid
	}

	// Signal watchdog to stop by closing the stop channel
	closeChannel(stopChan)

	// Also try to signal the process directly if we have a PID
	if pid > 0 {
		cfgSpec, ok := m.Config[name]
		sig := os.Signal(syscall.SIGTERM) // Default
		if ok && cfgSpec.Stopsignal != "" {
			if s, err := engine.SignalFromString(cfgSpec.Stopsignal); err == nil {
				sig = s
			}
		}
		p, err := os.FindProcess(pid)
		if err == nil {
			p.Signal(sig)
		}
	}

	// Remove from stops map and update status
	delete(m.ch.Stop, name)
	if proc, ok := m.Process[name]; ok {
		proc.Status = bus.STOPPED
	}

	slog.Info("RPC Stop requested", "name", name)
	return nil
}

// Restart restarts a single process by name.
// Sends a command to the event loop and waits for response.
func (m *Manager) Restart(name string) error {
	resp := make(chan error, 1)
	m.commandCh <- ManagerCommand{Type: "restart", Name: name, Resp: resp}
	return <-resp
}

// restartInternal is called by the event loop - no locking needed.
func (m *Manager) restartInternal(name string) error {
	// Stop the process
	if err := m.stopInternal(name); err != nil {
		return err
	}

	// Brief wait to allow process cleanup
	time.Sleep(100 * time.Millisecond)

	// Start the process
	return m.startInternal(name)
}

// ReloadResult contains the diff of configuration changes.
type ReloadResult struct {
	Added     []string
	Removed   []string
	Restarted []string
}

// Reload reloads the configuration from disk and applies the diff.
// Sends a command to the event loop and waits for response.
func (m *Manager) Reload() (*ReloadResult, error) {
	resp := make(chan error, 1)
	reloadResp := make(chan ReloadCommandResult, 1)
	m.commandCh <- ManagerCommand{Type: "reload", Resp: resp, ReloadResp: reloadResp}
	result := <-reloadResp
	return result.Result, result.Err
}

// ReloadFromConfig applies a new configuration to the manager.
// Note: This method is only safe to call before Run() starts or from within the event loop.
// For runtime reloads, use the Reload() method which sends a command to the event loop.
func (m *Manager) ReloadFromConfig(newConfig map[string]*config.ConfigSpec) (*ReloadResult, error) {
	// Note: No mutex needed - this is called during init before Run() or from event loop
	return m.reloadFromConfigInternal(newConfig)
}

// reloadFromConfigInternal is the internal implementation - no locking needed.
// Called by the event loop or during initialization.
func (m *Manager) reloadFromConfigInternal(newConfig map[string]*config.ConfigSpec) (*ReloadResult, error) {
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
			if newSpec.Autostart {
				if _, ok := m.ch.Stop[name]; !ok {
					m.ch.Stop[name] = make(chan struct{})
				}
				go m.Watchdog(newSpec, m.Process[name])
			}
		} else if configChanged(oldSpec, newSpec) {
			result.Restarted = append(result.Restarted, name)
			// Update config
			m.Config[name] = newSpec
			// Restart if running or if autostart is true
			wasRunning := m.isRunning(name)
			if wasRunning {
				if stopCh, ok := m.ch.Stop[name]; ok {
					closeChannel(stopCh)
					delete(m.ch.Stop, name)
				}
			}
			// Restart immediately if autostart is true
			if newSpec.Autostart {
				if _, ok := m.ch.Stop[name]; !ok {
					m.ch.Stop[name] = make(chan struct{})
				}
				go m.Watchdog(newSpec, m.Process[name])
			}
		}
	}

	// Stop removed processes
	for _, name := range result.Removed {
		if stopCh, ok := m.ch.Stop[name]; ok {
			closeChannel(stopCh)
			delete(m.ch.Stop, name)
		}
		delete(m.Config, name)
		// Don't delete from Process map - keep for status history
		if proc, ok := m.Process[name]; ok {
			proc.Status = bus.STOPPED
		}
	}

	return result, nil
}

// reloadInternal handles the reload command from the event loop.
func (m *Manager) reloadInternal() (*ReloadResult, error) {
	if m.configPath == "" {
		return nil, fmt.Errorf("no config path stored, use NewManagerFromConfig() to initialize Manager")
	}

	// Load new config from disk
	loader := &config.YAMLLoader{}
	specs, err := loader.Load(m.configPath)
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
	result, err := m.reloadFromConfigInternal(newConfig)
	return result, err
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
// Sends a command to the event loop and waits for response.
func (m *Manager) Shutdown() error {
	resp := make(chan error, 1)
	m.commandCh <- ManagerCommand{Type: "shutdown", Resp: resp}
	return <-resp
}

// shutdownInternal is called by the event loop - no locking needed.
func (m *Manager) shutdownInternal() error {
	// Signal all processes to stop, including those without active watchdogs
	for name, proc := range m.Process {
		// Stop watchdog if running
		if m.ch != nil {
			if stopChan, ok := m.ch.Stop[name]; ok {
				closeChannel(stopChan)
				delete(m.ch.Stop, name)
			}
		}

		// Signal process directly if it has a PID
		pid := proc.Pid
		if pid > 0 {
			p, err := os.FindProcess(pid)
			if err == nil {
				p.Signal(syscall.SIGTERM)
			}
		}
		proc.Status = bus.STOPPED
	}

	// Signal daemon to exit
	if m.shutdownFunc != nil {
		go m.shutdownFunc()
	}

	return nil
}
