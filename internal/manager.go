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
	runtime   *Runtime
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
	}
}

func (m *Manager) Reload(newConfigs map[string]*Config) error {

	type start struct {
		name   string
		config *Config
	}
	var startList []start

	var shutdownList []*Runtime

	m.mu.Lock()
	for name, inst := range m.instances {
		newConfig, exists := newConfigs[name]
		if !exists {
			shutdownList = append(shutdownList, inst.runtime)
			delete(m.instances, name)
			continue
		}

		if isConfigChanged(inst.spec, newConfig) {
			shutdownList = append(shutdownList, inst.runtime)
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
			shutdown.stop()
		}
	}

	for _, s := range startList {
		if err := m.Start(s.name); err != nil {
			if logger := m.logger; logger != nil {
				logger.LogMessage(LevelError, fmt.Sprintf("failed to start process %s during reload: %v", s.name, err))
			}
		}
	}

	return nil
}

func isConfigChanged(prevConfig, newConfig *Config) bool {
	if prevConfig == nil || newConfig == nil {
		return true
	}

	// Only check fields that require process restart
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

	if len(prevConfig.Env) != len(newConfig.Env) {
		return true
	}

	for k, v := range prevConfig.Env {
		if newConfig.Env[k] != v {
			return true
		}
	}

	return false
}

func (m *Manager) Start(name string) error {
	m.mu.Lock()
	inst, exists := m.instances[name]
	m.mu.Unlock()
	if !exists {
		return fmt.Errorf("process %s not found in manager registry", name)
	}

	runtime := newRuntime(m.ctx)
	inst.runtime = runtime
	spec := inst.spec

	m.wait.Add(1)
	tracker := &UpdateTracker{name: name, updates: m.updates}
	logger := m.logger
	go func() {
		supervise(runtime.ctx, name, spec, tracker, logger)
		close(runtime.done)
		m.wait.Done()
	}()

	return nil
}

func (m *Manager) Stop(name string) error {
	m.mu.Lock()
	inst, exists := m.instances[name]
	m.mu.Unlock()
	if !exists {
		return fmt.Errorf("process %s not found in manager registry", name)
	}

	if inst.runtime != nil {
		inst.runtime.stop()
		<-inst.runtime.done
		inst.runtime = nil
	}

	return nil
}

func (m *Manager) Restart(name string) error {
	if err := m.Stop(name); err != nil {
		return err
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
			if logger := m.logger; logger != nil {
				logger.LogMessage(LevelError, fmt.Sprintf("failed to auto-start process %s: %v", name, err))
			}
		}
	}

	return nil
}

func (m *Manager) Shutdown() {
	m.wait.Wait()
	close(m.updates)
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
