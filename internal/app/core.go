package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
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

type ProcessChannels struct {
	Status chan bus.ProcessUpdate
	Stop   map[string]chan struct{}
}

func NewProcessChannels() *ProcessChannels {
	return &ProcessChannels{
		Status: make(chan bus.ProcessUpdate, 100),
		Stop:   make(map[string]chan struct{}),
	}
}

type ReloadResult struct {
	Added     []string
	Removed   []string
	Restarted []string
}

type ProcessManager interface {
	GetProcessInfo(name string) (protocol.ProcessInfo, error)
	GetAllProcessInfo() ([]protocol.ProcessInfo, error)
	Start(name string) error
	Stop(name string) error
	Restart(name string) error
	Reload() (*ReloadResult, error)
	Shutdown() error
}

var _ ProcessManager = (*Manager)(nil)

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

func NewManager() *Manager {
	return &Manager{
		Config:   make(map[string]*config.ConfigSpec),
		Process:  make(map[string]*ProcessInstance),
		executor: engine.NewOsProcessExecutor(),
		handler:  &engine.OSSignalHandler{},
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
	m.mu.Unlock()

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

	stoptime := 10
	if spec, ok := m.Config[name]; ok {
		stoptime = spec.Stoptime + 2
	}
	timeout := time.Now().Add(time.Duration(stoptime) * time.Second)
	for time.Now().Before(timeout) {
		m.drainChStatus()
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

func (m *Manager) Restart(name string) error {
	if err := m.Stop(name); err != nil {
		return err
	}

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

func (m *Manager) ReloadFromConfig(newConfig map[string]*config.ConfigSpec) (*ReloadResult, error) {
	return m.applyConfigDiff(newConfig)
}

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

func (m *Manager) GetProcessInfo(name string) (protocol.ProcessInfo, error) {
	m.drainChStatus()

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

func (m *Manager) GetAllProcessInfo() ([]protocol.ProcessInfo, error) {
	m.drainChStatus()

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

func slicesEqual(a, b []int) bool {
	if len(a) == 0 && len(b) == 0 {
		return true
	}
	return slices.Equal(a, b)
}

func (m *Manager) drainChStatus() {
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

func (m *Manager) isRunning(name string) bool {
	if m.ch == nil {
		return false
	}
	_, exists := m.ch.Stop[name]
	return exists
}

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

func (m *Manager) Backoff(setting *config.ConfigSpec, stop chan struct{}, updates chan bus.ProcessUpdate, attempt int) error {
	updates <- bus.ProcessUpdate{
		Name:       setting.ProcessName,
		Status:     bus.BACKOFF,
		Retries:    attempt,
		HasRetries: true,
	}
	slog.Info("program backoff", "program", setting.Program, "attempt", attempt)

	select {
	case <-stop:
		updates <- bus.ProcessUpdate{Name: setting.ProcessName, Status: bus.STOPPED}
		return fmt.Errorf("stopped during backoff")
	default:
	}

	updates <- bus.ProcessUpdate{
		Name:   setting.ProcessName,
		Status: bus.STARTING,
	}
	slog.Info("program starting (retry)", "program", setting.Program, "attempt", attempt)
	return nil
}

// validateWatchdog checks channels and config before starting
func (m *Manager) validateWatchdog(setting *config.ConfigSpec, proc *ProcessInstance) error {
	if m.ch == nil {
		slog.Error("process channels not set", "program", setting.Program)
		return errors.New("channels not set")
	}
	if len(strings.Fields(setting.Cmd)) == 0 {
		slog.Error("no command specified", "program", setting.Program)
		m.ch.Status <- bus.ProcessUpdate{Name: setting.ProcessName, Status: bus.FATAL}
		return errors.New("no command")
	}
	if proc == nil {
		slog.Error("ProcessInstance not found", "process", setting.ProcessName)
		m.ch.Status <- bus.ProcessUpdate{Name: setting.ProcessName, Status: bus.FATAL}
		return errors.New("nil process")
	}
	return nil
}

// resolveMaxRetries returns max retries based on autorestart config
func resolveMaxRetries(setting *config.ConfigSpec) int {
	if setting.Autorestart == "always" {
		return math.MaxInt
	}
	return setting.Startretries
}

// isStopped checks if stop signal was received without blocking
func isStopped(stop chan struct{}) bool {
	select {
	case <-stop:
		return true
	default:
		return false
	}
}

// startProcess launches the process in a goroutine
func (m *Manager) startProcess(
	ctx context.Context,
	setting *config.ConfigSpec,
	updates chan bus.ProcessUpdate,
) (chan engine.ExitCode, chan error, chan int) {
	resultCh := make(chan engine.ExitCode, 1)
	errCh := make(chan error, 1)
	watcher := engine.Executor(m.executor)
	pidChan := make(chan int, 1)
	go func() {
		exitCode, err := watcher.Run(ctx, *setting, updates, pidChan)
		if err != nil {
			errCh <- err
		} else {
			resultCh <- exitCode
		}
	}()

	return resultCh, errCh, pidChan
}

// waitForExit waits for process exit or sends SIGKILL after timeout
func (m *Manager) waitForExit(
	setting *config.ConfigSpec,
	p *engine.Process,
	resultCh chan engine.ExitCode,
	errCh chan error,
) {
	select {
	case <-resultCh:
	case <-errCh:
	case <-time.After(time.Duration(setting.Stoptime) * time.Second):
		if err := m.handler.Send(p, syscall.SIGKILL); err == nil {
			slog.Info("sent SIGKILL after timeout", "program", setting.Program, "pid", p.PID)
		}
		select {
		case <-resultCh:
		case <-errCh:
		case <-time.After(2 * time.Second):
			slog.Error("process did not die after SIGKILL", "program", setting.Program, "pid", p.PID)
		}
	}
}

// resolvePid gets PID from channel or process map
func (m *Manager) resolvePid(setting *config.ConfigSpec, pidCh chan int) int {
	select {
	case pid := <-pidCh:
		return pid
	case <-time.After(5 * time.Second):
		if proc, ok := m.Process[setting.ProcessName]; ok && proc.Pid > 0 {
			return proc.Pid
		}
	}
	return 0
}

// stopProcess sends stop signal and waits for process to exit
func (m *Manager) stopProcess(
	setting *config.ConfigSpec,
	pidCh chan int,
	resultCh chan engine.ExitCode,
	errCh chan error,
) {
	pid := m.resolvePid(setting, pidCh)
	if pid == 0 {
		return
	}

	sig, _ := engine.SignalFromString(setting.Stopsignal)
	if sig == nil {
		sig, _ = engine.SignalFromString("TERM")
	}

	p := &engine.Process{PID: pid}
	if err := m.handler.Send(p, sig); err != nil {
		slog.Error("error sending stop signal", "program", setting.Program, "error", err)
	}

	m.waitForExit(setting, p, resultCh, errCh)
}

// evaluateResult decides whether to restart, stop, or fatal based on exit
func (m *Manager) evaluateResult(
	setting *config.ConfigSpec,
	updates chan bus.ProcessUpdate,
	exitCode engine.ExitCode,
	err error,
	attempt int,
) (done bool, shouldReturn bool, code engine.ExitCode) {
	if err != nil {
		slog.Error("program exited with error", "program", setting.Program, "error", err)
		updates <- bus.ProcessUpdate{
			Name: setting.ProcessName, Status: bus.FATAL,
			Retries: attempt, HasRetries: true,
		}
		return true, true, exitCode
	}

	if !engine.ShouldRestart(setting.Autorestart, int(exitCode), setting.Exitcodes) {
		if exitCode == 0 {
			slog.Info("program exited successfully", "program", setting.Program)
		} else {
			slog.Error("program exited with code", "program", setting.Program, "code", exitCode)
		}
		updates <- bus.ProcessUpdate{Name: setting.ProcessName, Status: bus.STOPPED}
		return true, true, exitCode
	}

	return false, false, exitCode
}

// runAttempt runs a single process attempt and returns (done, shouldReturn)
func (m *Manager) runAttempt(
	setting *config.ConfigSpec,
	stop chan struct{},
	updates chan bus.ProcessUpdate,
	attempt int,
) (bool, bool, engine.ExitCode) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	resultCh, errCh, pidCh := m.startProcess(ctx, setting, updates)
	var exitCode engine.ExitCode
	var err error

	select {
	case <-stop:
		m.stopProcess(setting, pidCh, resultCh, errCh)
		updates <- bus.ProcessUpdate{Name: setting.ProcessName, Status: bus.STOPPED}
		return true, true, exitCode

	case exitCode = <-resultCh:
	case err = <-errCh:
	}

	return m.evaluateResult(setting, updates, exitCode, err, attempt)
}

func (m *Manager) Watchdog(setting *config.ConfigSpec, proc *ProcessInstance) {
	if err := m.validateWatchdog(setting, proc); err != nil {
		return
	}

	stop, updates := m.ch.Stop[setting.ProcessName], m.ch.Status

	maxRetries := resolveMaxRetries(setting)

	for attempt := 0; attempt <= maxRetries; attempt++ {
		if isStopped(stop) {
			updates <- bus.ProcessUpdate{Name: setting.ProcessName, Status: bus.STOPPED}
			return
		}

		if err := m.Backoff(setting, stop, updates, attempt); err != nil {
			return
		}

		done, shouldReturn, exitCode := m.runAttempt(setting, stop, updates, attempt)
		if shouldReturn {
			return
		}
		if !done {
			slog.Info("program will restart", "program", setting.Program, "attempt", attempt+1)
		}

		slog.Info("program will restart", "program", setting.Program, "exitCode", exitCode, "attempt", attempt+1)
	}

	slog.Error("max retries exceeded", "program", setting.Program)
	updates <- bus.ProcessUpdate{Name: setting.ProcessName, Status: bus.FATAL}
}
