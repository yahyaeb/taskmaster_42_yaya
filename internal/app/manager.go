package app

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"syscall"
	"time"

	"taskmaster/internal/bus"
	"taskmaster/internal/engine"
	"taskmaster/internal/protocol"
	"taskmaster/internal/state"
)

// ProcessChannels carries supervisor stop coordination and status broadcasts.
// All access to the stop map goes through methods guarded by an internal mutex.
type ProcessChannels struct {
	mu     sync.Mutex
	status chan bus.ProcessUpdate
}

func NewProcessChannels() *ProcessChannels {
	return &ProcessChannels{
		status: make(chan bus.ProcessUpdate, StatusUpdateChanCapacity),
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

type Manager struct {
	reg          *state.Registry
	ch           *ProcessChannels
	executor     engine.ProcessExecutor
	handler      engine.SignalHandler
	loader       ConfigLoader
	log          *slog.Logger
	shutdownFunc func()
	configPath   string
	statusCtx    context.Context
	statusCancel context.CancelFunc
	rootCtx      context.Context
	rootCancel   context.CancelFunc
	plMu         sync.Mutex
	procLocks    map[string]*sync.Mutex
}

// NewManager constructs a manager; reg must be non-nil. logger may be nil (uses slog.Default).
func NewManager(reg *state.Registry, executor engine.ProcessExecutor, handler engine.SignalHandler, loader ConfigLoader, logger *slog.Logger) *Manager {
	ctx, cancel := context.WithCancel(context.Background())
	if logger == nil {
		logger = slog.Default()
	}
	return &Manager{
		reg:          reg,
		executor:     executor,
		handler:      handler,
		loader:       loader,
		log:          logger,
		statusCtx:    context.Background(),
		rootCtx:      ctx,
		rootCancel:   cancel,
		statusCancel: nil,
	}
}

// SetConfigPath stores the path used by Reload.
func (m *Manager) SetConfigPath(path string) {
	m.configPath = path
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

	go m.runStatusLoop()
}

func (m *Manager) Channels() *ProcessChannels {
	return m.ch
}

// StartAutostartProcesses starts every process row marked Intended (daemon bootstrap).
func (m *Manager) StartAutostartProcesses() error {
	for name, proc := range m.reg.All() {
		if proc != nil && proc.Intended {
			if err := m.Start(name); err != nil {
				return err
			}
		}
	}
	return nil
}

func (m *Manager) Start(name string) error {
	unlock := m.processLock(name)
	defer unlock()
	return m.doStart(name)
}

func (m *Manager) doStart(name string) error {
	if m.ch == nil {
		return fmt.Errorf("process channels not set")
	}

	spec, _, ok := m.reg.Get(name)
	if !ok {
		return fmt.Errorf("process not found: %s", name)
	}

	if m.reg.IsRunning(name) {
		return nil
	}

	m.reg.EnsureProcess(name, true)

	m.startWatchdog(spec)

	m.log.Info("Start requested", "name", name)
	return nil
}

func (m *Manager) Stop(name string) error {
	unlock := m.processLock(name)
	defer unlock()
	return m.doStop(name)
}

func (m *Manager) doStop(name string) error {
	spec, proc, ok := m.reg.Get(name)
	if !ok || proc == nil {
		return fmt.Errorf("process not found: %s", name)
	}
	m.log.Info("stop requested", "name", name)
	pid := proc.Pid

	sig, err := engine.SignalFromString(spec.Stopsignal)
	if err != nil || sig == nil {
		sig, _ = engine.SignalFromString("TERM")
	}
	if pid > 0 {
		_ = m.handler.Send(&engine.Process{PID: pid}, sig)
	}

	if spec.Autorestart == engine.RestartAlways || spec.Autorestart == engine.RestartUnexpected {
		m.stopWatchdog(name)
	}

	deadline := time.Now().Add(time.Duration(spec.Stoptime) * time.Second)
	waitDur := time.Until(deadline)
	if waitDur > 0 {
		_ = m.waitProcessTerminal(name, waitDur)
	}

	_, proc2, _ := m.reg.Get(name)
	if proc2 != nil && proc2.Status != bus.STOPPED && proc2.Status != bus.FATAL {
		killPid := pid
		if proc2.Pid > 0 {
			killPid = proc2.Pid
		}
		if killPid > 0 {
			if err := syscall.Kill(killPid, syscall.SIGKILL); err != nil && err != syscall.ESRCH && err != syscall.EPERM {
				return fmt.Errorf("failed to kill process %s: %w", name, err)
			}
		}
		killWait := time.Duration(StopKillVerifySeconds) * time.Second
		if err := m.waitProcessTerminal(name, killWait); err != nil {
			return fmt.Errorf("timeout stopping process: %s", name)
		}
	}

	m.stopWatchdog(name)
	m.log.Info("stop completed", "name", name)
	return nil
}

func (m *Manager) Restart(name string) error {
	unlock := m.processLock(name)
	defer unlock()
	if err := m.doStop(name); err != nil {
		return err
	}
	return m.doStart(name)
}

func (m *Manager) Shutdown() error {
	m.log.Info("shutdown requested")
	for _, name := range m.reg.ProcessNames() {
		m.stopWatchdog(name)
	}
	shutdownFunc := m.shutdownFunc

	m.rootCancel()
	if shutdownFunc != nil {
		shutdownFunc()
	}

	return nil
}

func (m *Manager) GetProcessInfo(name string) (protocol.ProcessInfo, error) {
	proc, ok := m.reg.GetProcess(name)
	if !ok {
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
	snap := m.reg.All()
	result := make([]protocol.ProcessInfo, 0, len(snap))
	for name, proc := range snap {
		if proc == nil {
			continue
		}
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

func (m *Manager) Spawn(name string) {
	proc, ok := m.reg.GetProcess(name)
	if ok && proc.Intended {
		_ = m.Start(name)
	}
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
			m.reg.ApplyUpdate(update)
		case <-m.statusCtx.Done():
			return
		}
	}
}

func (m *Manager) processLock(name string) func() {
	m.plMu.Lock()
	if m.procLocks == nil {
		m.procLocks = make(map[string]*sync.Mutex)
	}
	mu, ok := m.procLocks[name]
	if !ok {
		mu = &sync.Mutex{}
		m.procLocks[name] = mu
	}
	m.plMu.Unlock()
	mu.Lock()
	return mu.Unlock
}

func (m *Manager) waitProcessTerminal(name string, d time.Duration) error {
	return m.waitChan(m.reg.TerminalChan(name), d, m.rootCtx)
}

func (m *Manager) waitChan(ch <-chan struct{}, d time.Duration, ctx context.Context) error {
	if ch == nil {
		return nil
	}
	if d <= 0 {
		select {
		case <-ch:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		default:
			return fmt.Errorf("wait terminal: deadline exceeded")
		}
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ch:
		return nil
	case <-t.C:
		return fmt.Errorf("wait terminal: deadline exceeded")
	case <-ctx.Done():
		return ctx.Err()
	}
}
