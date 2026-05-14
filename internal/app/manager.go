package app

import (
	"context"
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

type stopChan struct {
	once sync.Once
	ch   chan struct{}
}

func (s *stopChan) close() {
	s.once.Do(func() { close(s.ch) })
}

func (s *stopChan) C() chan struct{} {
	return s.ch
}

// ProcessChannels carries supervisor stop coordination and status broadcasts.
// All access to the stop map goes through methods guarded by an internal mutex.
type ProcessChannels struct {
	mu     sync.Mutex
	status chan bus.ProcessUpdate
	stop   map[string]*stopChan
}

func NewProcessChannels() *ProcessChannels {
	return &ProcessChannels{
		status: make(chan bus.ProcessUpdate, 100),
		stop:   make(map[string]*stopChan),
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

// EnsureSupervisorStop returns the stop channel for name, allocating it if missing.
func (pc *ProcessChannels) EnsureSupervisorStop(name string) *stopChan {
	pc.mu.Lock()
	defer pc.mu.Unlock()
	sc := pc.stop[name]
	if sc == nil {
		sc = &stopChan{ch: make(chan struct{})}
		pc.stop[name] = sc
	}
	return sc
}

// HasSupervisorStop reports whether a supervision slot exists for name.
func (pc *ProcessChannels) HasSupervisorStop(name string) bool {
	pc.mu.Lock()
	defer pc.mu.Unlock()
	_, ok := pc.stop[name]
	return ok
}

// CloseSupervisorStop closes stop signaling for name and removes it from the map.
func (pc *ProcessChannels) CloseSupervisorStop(name string) bool {
	pc.mu.Lock()
	defer pc.mu.Unlock()
	sc, ok := pc.stop[name]
	if !ok {
		return false
	}
	sc.close()
	delete(pc.stop, name)
	return true
}

// CloseAllSupervisorStops closes every supervision slot and clears the map.
func (pc *ProcessChannels) CloseAllSupervisorStops() []string {
	pc.mu.Lock()
	defer pc.mu.Unlock()
	names := make([]string, 0, len(pc.stop))
	for name, sc := range pc.stop {
		if sc != nil {
			sc.close()
		}
		names = append(names, name)
	}
	clear(pc.stop)
	return names
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
	Stopped    chan struct{}
}

func newProcessInstance(autostart bool) *ProcessInstance {
	return &ProcessInstance{
		Status:     bus.STOPPED,
		Intended:   autostart,
		RetryCount: 0,
		Stopped:    make(chan struct{}, 1),
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
	statusCtx    context.Context
	statusCancel context.CancelFunc
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
	if m.statusCancel != nil {
		m.statusCancel()
	}
	m.ch = ch
	m.statusCtx, m.statusCancel = context.WithCancel(context.Background())
	go m.runStatusLoop()
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

	if !m.ch.CloseSupervisorStop(name) {
		m.mu.Unlock()
		return nil
	}

	pid := 0
	if proc, ok := m.Process[name]; ok {
		pid = proc.Pid
	}

	m.mu.Unlock()

	if pid > 0 {
		sig, _ := engine.SignalFromString(m.Config[name].Stopsignal)
		if sig == nil {
			sig, _ = engine.SignalFromString("TERM")
		}
		p := &engine.Process{PID: pid}
		if err := engine.StopProcess(m.handler, p, sig); err != nil {
			slog.Error("error stopping program directly", "program", name, "error", err)
		}
	}

	slog.Info("Stop requested", "name", name)

	stoptime := 10
	if spec, ok := m.Config[name]; ok {
		stoptime = spec.Stoptime + 2
	}

	proc, ok := m.Process[name]
	if !ok {
		return nil
	}

	// Drain stale notification from previous stop cycle
	select {
	case <-proc.Stopped:
	default:
	}

	// Check if already stopped
	if proc.Status == bus.STOPPED || proc.Status == bus.FATAL {
		return nil
	}

	// Wait for stop notification with timeout
	select {
	case <-proc.Stopped:
		return nil
	case <-time.After(time.Duration(stoptime) * time.Second):
		return fmt.Errorf("timeout waiting for process to stop")
	}
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
			m.ch.CloseSupervisorStop(name)
		}
		if proc.Pid > 0 {
			sig, _ := engine.SignalFromString("TERM")
			p := &engine.Process{PID: proc.Pid}
			if err := engine.StopProcess(m.handler, p, sig); err != nil {
				slog.Error("error stopping process during shutdown", "process", name, "error", err)
			}
		}
		proc.Status = bus.STOPPED
	}

	if m.statusCancel != nil {
		m.statusCancel()
	}

	shutdownFunc := m.shutdownFunc
	m.mu.Unlock()

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

func (m *Manager) StopAll() {
	m.mu.Lock()

	if m.ch == nil {
		m.mu.Unlock()
		return
	}

	for _, name := range m.ch.CloseAllSupervisorStops() {
		slog.Info("stopped program", "name", name)
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
			go curr.Watchdog(setting, curr.Process[name])
			slog.Info("started new program", "name", setting.ProcessName)
		}
	}

	for _, prevName := range prevKeys {
		if _, exists := curr.Config[prevName]; !exists {
			curr.ch.CloseSupervisorStop(prevName)
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
		// Notify waiters when process reaches terminal state
		if update.Status == bus.STOPPED || update.Status == bus.FATAL {
			select {
			case proc.Stopped <- struct{}{}:
			default:
			}
		}
	}
}

func (m *Manager) isRunning(name string) bool {
	if m.ch == nil {
		return false
	}
	return m.ch.HasSupervisorStop(name)
}