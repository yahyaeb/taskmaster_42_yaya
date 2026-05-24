package internal

import (
	"context"
	"fmt"
	"os"
	"sync"
	"syscall"
	"time"
)

func getStopSignal(signalStr string) os.Signal {
	switch signalStr {
	case "HUP":
		return syscall.SIGHUP
	case "INT":
		return syscall.SIGINT
	case "QUIT":
		return syscall.SIGQUIT
	case "KILL":
		return syscall.SIGKILL
	case "USR1":
		return syscall.SIGUSR1
	case "USR2":
		return syscall.SIGUSR2
	case "TERM":
		fallthrough
	default:
		return syscall.SIGTERM // Default to standard graceful termination
	}
}

type Instance struct {
	spec      *Config
	status    Status
	pid       int
	exitCode  int
	cancel    context.CancelFunc
	done      chan struct{}
	lastStart time.Time
}

type Manager struct {
	mu        sync.Mutex
	instances map[string]*Instance
	updates   chan ProcessUpdate
	ctx       context.Context
	wait      sync.WaitGroup
	logger    *Logger
}

type StatusReport struct {
	Name     string
	Status   Status
	Pid      int
	ExitCode int
	Uptime   string
}

func CreateManager(ctx context.Context) *Manager {
	m := &Manager{
		instances: make(map[string]*Instance),
		updates:   make(chan ProcessUpdate, 100),
		ctx:       ctx,
	}
	go m.updateManagerInstances()
	return m
}

// SetLogger configures an optional logger for process updates.
func (m *Manager) SetLogger(logger *Logger) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.logger = logger
}

func (m *Manager) updateManagerInstances() {
	for update := range m.updates {
		m.mu.Lock()
		instance, exists := m.instances[update.Name]
		if exists {
			instance.status = update.Status
			instance.pid = update.Pid
			instance.exitCode = update.ExitCode
			if update.Status == RUNNING && !update.LastStart.IsZero() {
				instance.lastStart = update.LastStart
			}
		}
		logger := m.logger
		m.mu.Unlock()

		// Send to logger if configured
		if logger != nil {
			logger.Log(update)
		}

		switch update.Status {
		case FATAL:
			fmt.Printf("[ALERT] Process %s hit a FATAL startup failure\n", update.Name)
		case STOPPED:
			fmt.Printf("[INFO] Process %s has STOPPED (PID: %d, ExitCode: %d)\n", update.Name, update.Pid, update.ExitCode)
		default:
			fmt.Printf("[EVENT] Process %s is now %s (PID: %d)\n", update.Name, update.Status, update.Pid)
		}
	}
}

func (m *Manager) Reload(newConfigs map[string]*Config) error {

	type start struct {
		name   string
		config *Config
	}
	var startList []start

	var shutdownList []context.CancelFunc

	m.mu.Lock()
	for name, inst := range m.instances {
		newConfig, exists := newConfigs[name]
		if !exists {
			shutdownList = append(shutdownList, inst.cancel)
			delete(m.instances, name)
			continue
		}

		if isConfigChanged(inst.spec, newConfig) {
			shutdownList = append(shutdownList, inst.cancel)
			delete(m.instances, name)
			startList = append(startList, start{name: name, config: newConfig})
			continue
		}

		inst.spec = newConfig

	}

	for name, config := range newConfigs {
		if _, exists := m.instances[name]; exists {
			continue
		}

		m.instances[name] = &Instance{
			spec:   config,
			status: STOPPED,
		}

		startList = append(startList, start{name: name, config: config})
	}

	m.mu.Unlock()

	for _, shutdown := range shutdownList {
		if shutdown != nil {
			shutdown()
		}
	}

	for _, s := range startList {
		if err := m.Start(s.name); err != nil {
			fmt.Printf("[ERROR] Failed to start process %s during reload: %v\n", s.name, err)
		}
	}

	return nil
}

func isConfigChanged(prevConfig, newConfig *Config) bool {
	if prevConfig == nil || newConfig == nil {
		return true
	}

	if prevConfig.Cmd == nil && newConfig.Cmd != nil {
		return true
	}

	if prevConfig.Cmd != nil && newConfig.Cmd == nil {
		return true
	}

	if len(prevConfig.Cmd) != len(newConfig.Cmd) {
		return true
	}

	for i := range prevConfig.Cmd {
		if prevConfig.Cmd[i] != newConfig.Cmd[i] {
			return true
		}
	}

	if prevConfig.Workingdir != newConfig.Workingdir {
		return true
	}

	if prevConfig.Umask != newConfig.Umask {
		return true
	}

	if (prevConfig.Uid == nil) != (newConfig.Uid == nil) || (prevConfig.Uid != nil && *prevConfig.Uid != *newConfig.Uid) {
		return true
	}

	if (prevConfig.Gid == nil) != (newConfig.Gid == nil) || (prevConfig.Gid != nil && *prevConfig.Gid != *newConfig.Gid) {
		return true
	}

	if prevConfig.Stdout != newConfig.Stdout {
		return true
	}

	if prevConfig.Stderr != newConfig.Stderr {
		return true
	}

	if len(prevConfig.Env) != len(newConfig.Env) {
		return true
	}

	for k, v := range prevConfig.Env {
		if newConfig.Env[k] != v {
			return true
		}
	}

	if prevConfig.Autorestart != newConfig.Autorestart {
		return true
	}

	if len(prevConfig.Exitcodes) != len(newConfig.Exitcodes) {
		return true
	}

	for i := range prevConfig.Exitcodes {
		if prevConfig.Exitcodes[i] != newConfig.Exitcodes[i] {
			return true
		}
	}

	if prevConfig.Startretries != newConfig.Startretries {
		return true
	}

	if prevConfig.Starttime != newConfig.Starttime {
		return true
	}

	if prevConfig.Stopsignal != newConfig.Stopsignal {
		return true
	}

	if prevConfig.Stoptime != newConfig.Stoptime {
		return true
	}

	if prevConfig.Autostart != newConfig.Autostart {
		return true
	}

	return false
}

func (m *Manager) Start(name string) error {
	m.mu.Lock()
	inst, exists := m.instances[name]
	if !exists {
		m.mu.Unlock()
		return fmt.Errorf("process %s not found in manager registry", name)
	}

	// Dedicate startup routine
	subCtx, cancelFunc := context.WithCancel(m.ctx)
	inst.cancel = cancelFunc
	spec := inst.spec
	m.mu.Unlock()

	inst.done = make(chan struct{})
	m.wait.Add(1)
	tracker := NewUpdateTracker(name, m.updates)
	go func() {
		defer close(inst.done)
		defer m.wait.Done()
		supervise(subCtx, name, spec, tracker)
	}()

	return nil
}

func (m *Manager) Stop(name string) error {
	m.mu.Lock()
	inst, exists := m.instances[name]
	if !exists {
		m.mu.Unlock()
		return fmt.Errorf("process %s not found in manager registry", name)
	}
	cancel := inst.cancel
	m.mu.Unlock()
	if cancel != nil {
		// Triggers terminacePolicy()
		cancel()
	}
	return nil
}

func (m *Manager) Restart(name string) error {
	if err := m.Stop(name); err != nil {
		return err
	}
	m.mu.Lock()
	inst, exists := m.instances[name]
	m.mu.Unlock()

	if !exists {
		return fmt.Errorf("process %s not found in manager registry", name)
	}

	if inst.done != nil {
		// Wait for previous instance to close its done channel
		<-inst.done
	}

	return m.Start(name)
}

func (m *Manager) Status() []StatusReport {
	m.mu.Lock()
	defer m.mu.Unlock()
	reports := make([]StatusReport, 0, len(m.instances))
	for name, inst := range m.instances {
		reports = append(reports, StatusReport{
			Name:     name,
			Status:   inst.status,
			Pid:      inst.pid,
			ExitCode: inst.exitCode,
			Uptime:   formatUptime(inst.lastStart),
		})
	}
	return reports
}

func (m *Manager) Load(specs map[string]*Config) error {

	var autostart []string

	m.mu.Lock()
	for name, spec := range specs {
		m.instances[name] = &Instance{
			spec:   spec,
			status: STOPPED,
		}

		if spec.Autostart {
			autostart = append(autostart, name)
		}
	}
	m.mu.Unlock()

	for _, name := range autostart {
		if err := m.Start(name); err != nil {
			fmt.Printf("[ERROR] Failed to auto-start process %s: %v\n", name, err)
		}
	}

	return nil
}

func (m *Manager) Shutdown() {
	m.wait.Wait()
}

func formatUptime(startTime time.Time) string {
	if startTime.IsZero() {
		return "0s"
	}
	duration := time.Since(startTime)
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
