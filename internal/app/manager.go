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

type ProcessInstance struct {
	Pid        int
	Status     bus.Status
	RetryCount int
	LastStart  time.Time
	Intended   bool
	// Note: ProcessInstance is owned exclusively by its Watchdog goroutine
	// No mutex needed - all access is serialized through channels
}

// newProcessInstance creates a new ProcessInstance with the given autostart setting.
func newProcessInstance(autostart bool) *ProcessInstance {
	return &ProcessInstance{
		Status:     bus.STOPPED,
		Intended:   autostart,
		RetryCount: 0,
	}
}

type Manager struct {
	Config       map[string]*config.ConfigSpec
	Process      map[string]*ProcessInstance
	executor     engine.ProcessExecutor
	handler      engine.SignalHandler
	ch           *ProcessChannels
	shutdownFunc func()
	configPath   string
	reqCh        chan Request
}

func NewManager() *Manager {
	return &Manager{
		Config:   make(map[string]*config.ConfigSpec),
		Process:  make(map[string]*ProcessInstance),
		executor: engine.NewOsProcessExecutor(),
		handler:  &engine.OSSignalHandler{},
		reqCh:    make(chan Request, 100),
	}
}

func (m *Manager) SetShutdownFunc(fn func()) {
	m.shutdownFunc = fn
}

func (m *Manager) SetChannels(ch *ProcessChannels) {
	m.ch = ch
}

func (m *Manager) Channels() *ProcessChannels {
	return m.ch
}

func (m *Manager) EventLoop() {
	for req := range m.reqCh {
		switch req.Type {
		case "start":
			req.Resp <- m.handleStart(req.Name)
		case "stop":
			req.Resp <- m.handleStop(req.Name)
		case "restart":
			req.Resp <- m.handleRestart(req.Name)
		case "reload":
			result, err := m.handleReload()
			req.Resp <- Response{Result: result, Err: err}
		case "shutdown":
			req.Resp <- m.handleShutdown()
		case "get":
			info, err := m.handleGet(req.Name)
			req.Resp <- Response{Result: info, Err: err}
		case "list":
			infos, err := m.handleList()
			req.Resp <- Response{Result: infos, Err: err}
		default:
			req.Resp <- Response{Err: fmt.Errorf("unknown request type: %s", req.Type)}
		}
	}
}

func (m *Manager) handleStart(name string) Response {
	if m.ch == nil {
		return Response{Err: fmt.Errorf("process channels not set")}
	}

	configSpec, exists := m.Config[name]
	if !exists {
		return Response{Err: fmt.Errorf("process not found: %s", name)}
	}

	if m.isRunning(name) {
		return Response{}
	}

	if _, ok := m.ch.Stop[name]; !ok {
		m.ch.Stop[name] = make(chan struct{})
	}

	if _, ok := m.Process[name]; !ok {
		m.Process[name] = newProcessInstance(true)
	}

	m.Process[name].Status = bus.STARTING

	go m.Watchdog(configSpec, m.Process[name])

	slog.Info("RPC Start requested", "name", name)
	return Response{}
}

func (m *Manager) handleStop(name string) Response {
	if m.ch == nil {
		return Response{Err: fmt.Errorf("process channels not set")}
	}

	_, exists := m.Config[name]
	if !exists {
		return Response{Err: fmt.Errorf("process not found: %s", name)}
	}

	stopChan, running := m.ch.Stop[name]
	if !running {
		return Response{}
	}

	pid := 0
	if proc, ok := m.Process[name]; ok {
		pid = proc.Pid
	}

	closeChannel(stopChan)
	delete(m.ch.Stop, name)

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

	slog.Info("RPC Stop requested", "name", name)

	stoptime := 10
	if spec, ok := m.Config[name]; ok {
		stoptime = spec.Stoptime + 2
	}
	timeout := time.Now().Add(time.Duration(stoptime) * time.Second)
	for time.Now().Before(timeout) {
		m.drainStatus()
		if proc, ok := m.Process[name]; ok {
			if proc.Status == bus.STOPPED || proc.Status == bus.FATAL {
				return Response{}
			}
		}
		time.Sleep(50 * time.Millisecond)
	}

	slog.Error("timeout waiting for process to stop", "program", name)
	return Response{Err: fmt.Errorf("timeout waiting for process to stop")}
}

func (m *Manager) handleRestart(name string) Response {
	if resp := m.handleStop(name); resp.Err != nil {
		return resp
	}

	maxWait := 5 * time.Second
	start := time.Now()
	for time.Since(start) < maxWait {
		if !m.isRunning(name) {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	return m.handleStart(name)
}

func (m *Manager) handleReload() (*ReloadResult, error) {
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

	return m.handleReloadFromConfig(newConfig)
}

func (m *Manager) handleReloadFromConfig(newConfig map[string]*config.ConfigSpec) (*ReloadResult, error) {
	if m.ch == nil {
		return nil, fmt.Errorf("process channels not set")
	}

	result := &ReloadResult{
		Added:     []string{},
		Removed:   []string{},
		Restarted: []string{},
	}

	for name := range m.Config {
		if _, exists := newConfig[name]; !exists {
			result.Removed = append(result.Removed, name)
		}
	}

	for name, newSpec := range newConfig {
		if oldSpec, exists := m.Config[name]; !exists {
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
	// Handle nil vs empty slice comparison
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

// slicesEqual compares two int slices, treating nil and empty as equal
func slicesEqual(a, b []int) bool {
	// Treat nil and empty slice as equal
	if len(a) == 0 && len(b) == 0 {
		return true
	}
	return slices.Equal(a, b)
}

func (m *Manager) handleShutdown() Response {
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

	if m.shutdownFunc != nil {
		m.shutdownFunc()
	}

	return Response{}
}

func (m *Manager) handleGet(name string) (protocol.ProcessInfo, error) {
	m.drainStatus()

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

func (m *Manager) handleList() ([]protocol.ProcessInfo, error) {
	m.drainStatus()

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

func (m *Manager) handleStatusUpdate(update bus.ProcessUpdate) {
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

func (m *Manager) isRunning(name string) bool {
	if m.ch == nil {
		return false
	}
	_, exists := m.ch.Stop[name]
	return exists
}

func (m *Manager) Watchdog(setting *config.ConfigSpec, proc *ProcessInstance) {
	if m.ch == nil {
		slog.Error("process channels not set", "program", setting.Program)
		return
	}

	stop := m.ch.Stop[setting.ProcessName]
	updates := m.ch.Status

	if len(strings.Fields(setting.Cmd)) == 0 {
		slog.Error("no command specified for program", "program", setting.Program)
		updates <- bus.ProcessUpdate{Name: setting.ProcessName, Status: bus.FATAL}
		return
	}

	if proc == nil {
		slog.Error("ProcessInstance not found in manager", "process", setting.ProcessName)
		updates <- bus.ProcessUpdate{Name: setting.ProcessName, Status: bus.FATAL}
		return
	}

	// Determine max retries
	var maxRetries int
	if setting.Autorestart == "always" {
		maxRetries = math.MaxInt
	} else {
		maxRetries = setting.Startretries
	}

	watcher := engine.NewProcessWatcher(m.executor)

	// Main retry loop - Manager decides when to restart
	for attempt := 0; attempt <= maxRetries; attempt++ {
		// Check if we were asked to stop before starting
		select {
		case <-stop:
			updates <- bus.ProcessUpdate{Name: setting.ProcessName, Status: bus.STOPPED}
			return
		default:
		}

		// Backoff before retry (not on first attempt)
		if attempt > 0 {
			updates <- bus.ProcessUpdate{
				Name:       setting.ProcessName,
				Status:     bus.BACKOFF,
				Retries:    attempt,
				HasRetries: true,
			}
			slog.Info("program backoff", "program", setting.Program, "attempt", attempt)

			// Wait for backoff period or stop signal
			select {
			case <-stop:
				updates <- bus.ProcessUpdate{Name: setting.ProcessName, Status: bus.STOPPED}
				return
			case <-time.After(0): // No delay for now, could add Config.RetryDelay
			}

			updates <- bus.ProcessUpdate{
				Name:   setting.ProcessName,
				Status: bus.STARTING,
			}
			slog.Info("program starting (retry)", "program", setting.Program, "attempt", attempt)
		}

		// Create fresh context for this run
		ctx, cancel := context.WithCancel(context.Background())

		// Run the watcher in a goroutine
		result := make(chan engine.ExitCode, 1)
		errCh := make(chan error, 1)
		go func() {
			exitCode, err := watcher.Run(ctx, *setting, updates)
			if err != nil {
				errCh <- err
			} else {
				result <- exitCode
			}
		}()

		// Wait for watcher to report PID via status updates (processed by EventLoop)
		// Poll m.Process directly since EventLoop updates it
		pidReady := make(chan int, 1)
		go func() {
			ticker := time.NewTicker(50 * time.Millisecond)
			defer ticker.Stop()
			for range ticker.C {
				if proc, ok := m.Process[setting.ProcessName]; ok && proc.Pid > 0 {
					pidReady <- proc.Pid
					return
				}
			}
		}()

		// Wait for watcher to complete OR stop signal
		var exitCode engine.ExitCode
		var err error
		var pid int
		stopped := false
		select {
		case <-stop:
			// We were asked to stop - get PID and send signal
			stopped = true
			select {
			case p := <-pidReady:
				pid = p
			case <-time.After(5 * time.Second):
				// PID never became ready, still try to stop what we can
				if proc, ok := m.Process[setting.ProcessName]; ok && proc.Pid > 0 {
					pid = proc.Pid
				}
			}

			if pid > 0 {
				sig, _ := engine.SignalFromString(setting.Stopsignal)
				if sig == nil {
					sig, _ = engine.SignalFromString("TERM")
				}

				// Send stop signal
				p := &engine.Process{PID: pid}
				if stopErr := m.handler.Send(p, sig); stopErr != nil {
					slog.Error("error sending stop signal", "program", setting.Program, "error", stopErr)
				}

				// Wait for watcher to finish (process will exit after receiving signal)
				select {
				case exitCode = <-result:
				case err = <-errCh:
				case <-time.After(time.Duration(setting.Stoptime) * time.Second):
					// Timeout - escalate to SIGKILL
					if killErr := m.handler.Send(p, syscall.SIGKILL); killErr == nil {
						slog.Info("sent SIGKILL after timeout", "program", setting.Program, "pid", pid)
					}
					// Wait for process to die after SIGKILL
					select {
					case exitCode = <-result:
					case err = <-errCh:
					case <-time.After(2 * time.Second):
						slog.Error("process did not die after SIGKILL", "program", setting.Program, "pid", pid)
					}
				}
			}
			cancel()

		case exitCode = <-result:
			// Watcher completed normally
			cancel()

		case err = <-errCh:
			// Watcher had an error
			cancel()
		}

		// Check if we were stopped intentionally
		if stopped {
			updates <- bus.ProcessUpdate{Name: setting.ProcessName, Status: bus.STOPPED}
			return
		}

		// Check if run had an error
		if err != nil {
			slog.Error("program exited with error", "program", setting.Program, "error", err, "attempt", attempt)
			updates <- bus.ProcessUpdate{
				Name:       setting.ProcessName,
				Status:     bus.FATAL,
				Retries:    attempt,
				HasRetries: true,
			}
			// Don't retry on error - treat as fatal
			return
		}

		// Process exited normally - check if we should restart
		if !engine.ShouldRestart(setting.Autorestart, int(exitCode), setting.Exitcodes) {
			// No restart needed - send final status and exit
			if exitCode == 0 {
				slog.Info("program exited successfully", "program", setting.Program)
			} else {
				slog.Error("program exited with code", "program", setting.Program, "code", exitCode)
			}
			updates <- bus.ProcessUpdate{Name: setting.ProcessName, Status: bus.STOPPED}
			return
		}

		// Should restart - loop will continue
		slog.Info("program will restart", "program", setting.Program, "exitCode", exitCode, "attempt", attempt+1)
	}

	// Max retries exceeded
	slog.Error("max retries exceeded", "program", setting.Program)
	updates <- bus.ProcessUpdate{Name: setting.ProcessName, Status: bus.FATAL}
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

func (m *Manager) StopAll() {
	if m.ch == nil {
		return
	}

	for name, stopCh := range m.ch.Stop {
		if stopCh != nil {
			closeChannel(stopCh)
			delete(m.ch.Stop, name)
			slog.Info("stopped program", "name", name)
		}
	}

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

func closeChannel(ch chan struct{}) {
	defer func() {
		if r := recover(); r != nil {
			slog.Debug("channel already closed", "reason", r)
		}
	}()
	close(ch)
}

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

func (m *Manager) GetProcessInfo(name string) (protocol.ProcessInfo, error) {
	resp := make(chan Response, 1)
	m.reqCh <- Request{Type: "get", Name: name, Resp: resp}
	result := <-resp
	if result.Err != nil {
		return protocol.ProcessInfo{}, result.Err
	}
	return result.Result.(protocol.ProcessInfo), nil
}

func (m *Manager) GetAllProcessInfo() ([]protocol.ProcessInfo, error) {
	resp := make(chan Response, 1)
	m.reqCh <- Request{Type: "list", Resp: resp}
	result := <-resp
	if result.Err != nil {
		return nil, result.Err
	}
	return result.Result.([]protocol.ProcessInfo), nil
}

func (m *Manager) Start(name string) error {
	resp := make(chan Response, 1)
	m.reqCh <- Request{Type: "start", Name: name, Resp: resp}
	return (<-resp).Err
}

func (m *Manager) Stop(name string) error {
	resp := make(chan Response, 1)
	m.reqCh <- Request{Type: "stop", Name: name, Resp: resp}
	return (<-resp).Err
}

func (m *Manager) Restart(name string) error {
	resp := make(chan Response, 1)
	m.reqCh <- Request{Type: "restart", Name: name, Resp: resp}
	return (<-resp).Err
}

func (m *Manager) Reload() (*ReloadResult, error) {
	resp := make(chan Response, 1)
	m.reqCh <- Request{Type: "reload", Resp: resp}
	result := <-resp
	if result.Err != nil {
		return nil, result.Err
	}
	return result.Result.(*ReloadResult), nil
}

func (m *Manager) ReloadFromConfig(newConfig map[string]*config.ConfigSpec) (*ReloadResult, error) {
	return m.handleReloadFromConfig(newConfig)
}

func (m *Manager) Shutdown() error {
	resp := make(chan Response, 1)
	m.reqCh <- Request{Type: "shutdown", Resp: resp}
	return (<-resp).Err
}
