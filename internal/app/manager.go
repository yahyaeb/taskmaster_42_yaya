package app

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"syscall"
	"time"

	"taskmaster/internal/bus"
	"taskmaster/internal/config"
	"taskmaster/internal/engine"
	"taskmaster/internal/protocol"
)

// ProcessChannels carries supervisor stop coordination and status broadcasts.
// All access to the stop map goes through methods guarded by an internal mutex.
type ProcessChannels struct {
	mu     sync.Mutex
	status chan bus.ProcessUpdate
}

func NewProcessChannels() *ProcessChannels {
	return &ProcessChannels{
		status: make(chan bus.ProcessUpdate, 100),
	}
}

// PublishStatus sends one status update to subscribers (non-blocking on buffer capacity only).
func (pc *ProcessChannels) PublishStatus(u bus.ProcessUpdate) {
	pc.status <- u
}

// StatusPublisher returns the send side for APIs that require a channel (e.g. engine.Run).
func (pc *ProcessChannels) StatusPublisher() chan<- bus.ProcessUpdate {
	return pc.status
}

// StatusUpdates returns the receive side for the manager status loop.
func (pc *ProcessChannels) StatusUpdates() <-chan bus.ProcessUpdate {
	return pc.status
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

type ProcessInstance struct {
	Pid        int
	Status     bus.Status
	RetryCount int
	LastStart  time.Time
	Intended   bool
	cancelFn   context.CancelFunc
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
	ch           *ProcessChannels
	Config       map[string]*config.ConfigSpec
	Process      map[string]*ProcessInstance
	executor     engine.ProcessExecutor
	handler      engine.SignalHandler
	shutdownFunc func()
	configPath   string
	statusCtx    context.Context
	statusCancel context.CancelFunc
	rootCtx      context.Context
	rootCancel   context.CancelFunc
}

func (m *Manager) runUpdateLoop() {
	for {
		select {
		case update := <-m.ch.status:
			m.mu.Lock()
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
				if update.Status == bus.STOPPED || update.Status == bus.FATAL {
					proc.cancelFn = nil
				}
			}
			m.mu.Unlock()
		case <-m.statusCtx.Done():
			return
		}
	}
}

func NewManager() *Manager {
	ctx, cancel := context.WithCancel(context.Background())
	return &Manager{
		Config:       make(map[string]*config.ConfigSpec),
		Process:      make(map[string]*ProcessInstance),
		executor:     engine.NewOsProcessExecutor(),
		handler:      &engine.OSSignalHandler{},
		statusCtx:    context.Background(),
		rootCtx:      ctx,
		rootCancel:   cancel,
		statusCancel: nil,
	}
}

func (m *Manager) SetShutdownFunc(fn func()) {
	m.shutdownFunc = fn
}

func (m *Manager) SetChannels(ch *ProcessChannels) {
	if m.statusCancel != nil {
		m.statusCancel()
	}

	m.ch = ch
	m.statusCtx, m.statusCancel = context.WithCancel(m.rootCtx)

	go m.runUpdateLoop()
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

	spec, exists := m.Config[name]
	if !exists {
		return fmt.Errorf("process not found: %s", name)
	}

	if m.isRunning(name) {
		return nil
	}

	if _, ok := m.Process[name]; !ok {
		m.Process[name] = newProcessInstance(true)
	}

	m.Process[name].Status = bus.STARTING
	m.startWatchdog(spec)

	slog.Info("Start requested", "name", name)
	return nil
}

func (m *Manager) Stop(name string) error {
	m.mu.Lock()
	spec, ok := m.Config[name]
	if !ok {
		m.mu.Unlock()
		return fmt.Errorf("process not found: %s", name)
	}
	proc := m.Process[name]
	if proc == nil {
		m.mu.Unlock()
		return fmt.Errorf("process not found: %s", name)
	}
	pid := proc.Pid
	deadline := time.Now().Add(time.Duration(spec.Stoptime) * time.Second)
	m.mu.Unlock()

	sig, err := engine.SignalFromString(spec.Stopsignal)
	if err != nil || sig == nil {
		sig, _ = engine.SignalFromString("TERM")
	}
	if pid > 0 {
		_ = m.handler.Send(&engine.Process{PID: pid}, sig)
	}

	for time.Now().Before(deadline) {
		m.mu.Lock()
		proc, ok := m.Process[name]
		done := ok && (proc.Status == bus.STOPPED || proc.Status == bus.FATAL)
		m.mu.Unlock()
		if done {
			return nil
		}
		time.Sleep(50 * time.Millisecond)
	}
	if pid > 0 {
		if err := syscall.Kill(pid, syscall.SIGKILL); err != nil && err != syscall.ESRCH {
			return fmt.Errorf("failed to kill process %s: %w", name, err)
		}
	}
	m.stopWatchdog(name)
	for i := 0; i < 20; i++ {
		m.mu.Lock()
		proc, ok := m.Process[name]
		done := ok && (proc.Status == bus.STOPPED || proc.Status == bus.FATAL)
		m.mu.Unlock()
		if done {
			return nil
		}
		time.Sleep(50 * time.Millisecond)
	}
	return fmt.Errorf("timeout stopping process: %s", name)
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
	for name := range m.Process {
		m.stopWatchdog(name)
	}
	shutdownFunc := m.shutdownFunc
	m.mu.Unlock()

	m.rootCancel()
	if shutdownFunc != nil {
		shutdownFunc()
	}

	return nil
}

func (m *Manager) GetProcessInfo(name string) (protocol.ProcessInfo, error) {
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

func (m *Manager) StopAll(names []string) {
	for _, name := range names {
		m.stopWatchdog(name)
	}
}

func (curr *Manager) Spawn(name string) {
	if proc, ok := curr.Process[name]; ok && proc.Intended {
		_ = curr.Start(name)
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

func (m *Manager) runStatusLoop() {
	for {
		select {
		case update := <-m.ch.StatusUpdates():
			m.mu.Lock()
			m.applyUpdate(update)
			m.mu.Unlock()
		case <-m.statusCtx.Done():
			return
		}
	}
}

func (m *Manager) applyUpdate(update bus.ProcessUpdate) {
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
	proc, ok := m.Process[name]
	return ok && proc.cancelFn != nil
}
