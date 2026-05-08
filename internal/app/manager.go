package app

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"taskmaster/internal/bus"
	"taskmaster/internal/config"
	"taskmaster/internal/engine"
)

type ProcessInstance struct {
	Pid        int
	Status     bus.Status
	RetryCount int
	LastStart  time.Time
	Intended   bool
	Strategy   engine.RetryStrategy
	Mu         sync.RWMutex
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
}

func NewManager() *Manager {
	return &Manager{
		Config:   make(map[string]*config.ConfigSpec),
		Process:  make(map[string]*ProcessInstance),
		executor: engine.NewOsProcessExecutor(),
		handler:  &engine.OSSignalHandler{},
	}
}

func (m *Manager) Watchdog(setting *config.ConfigSpec, updates chan bus.ProcessUpdate, stop chan struct{}) {
	parts := strings.Fields(setting.Cmd)
	if len(parts) == 0 {
		fmt.Printf("No command specified for program %s\n", setting.Program)
		updates <- bus.ProcessUpdate{Name: setting.ProcessName, Status: bus.FATAL}
		return
	}

	if !setting.Autostart {
		fmt.Printf("Program %s is set to not autostart, skipping...\n", setting.Program)
		updates <- bus.ProcessUpdate{Name: setting.ProcessName, Status: bus.STOPPED}
		return
	}

	var strategy engine.RetryStrategy
	if proc, ok := m.Process[setting.ProcessName]; ok && proc.Strategy != nil {
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

	watcher.OnProcessStarted = func(pid int) {
		pidMu.Lock()
		currentPID = pid
		pidMu.Unlock()
		updates <- bus.ProcessUpdate{Name: setting.ProcessName, Status: bus.RUNNING, Pid: pid}
		fmt.Printf("Started program %s with PID %d\n", setting.Program, pid)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	result := make(chan engine.ExitCode, 1)
	errCh := make(chan error, 1)

	go func() {
		var exitCode engine.ExitCode
		var err error

		if _, ok := m.executor.(engine.ConfigurableProcessExecutor); ok {
			exitCode, err = watcher.RunWithConfig(ctx, *setting)
		} else {
			exitCode, err = watcher.Run(ctx, parts[0], parts[1:])
		}

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
				fmt.Printf("Error stopping program %s: %v\n", setting.Program, err)
			}
		}

		select {
		case code := <-result:
			sendFinalUpdate(setting, updates, code, true)
		case err := <-errCh:
			fmt.Printf("Program %s exited with error after stop: %v\n", setting.Program, err)
			updates <- bus.ProcessUpdate{Name: setting.ProcessName, Status: bus.FATAL}
		case <-time.After(time.Duration(setting.Stoptime) * time.Second):
			fmt.Printf("Program %s did not exit after stop timeout\n", setting.Program)
			updates <- bus.ProcessUpdate{Name: setting.ProcessName, Status: bus.FATAL}
		}

	case code := <-result:
		sendFinalUpdate(setting, updates, code, false)

	case err := <-errCh:
		fmt.Printf("Program %s exited with error: %v\n", setting.Program, err)
		updates <- bus.ProcessUpdate{Name: setting.ProcessName, Status: bus.FATAL}
	}
}

func sendFinalUpdate(setting *config.ConfigSpec, updates chan bus.ProcessUpdate, code engine.ExitCode, stopped bool) {
	if code == 0 {
		fmt.Printf("Program %s exited successfully\n", setting.Program)
		updates <- bus.ProcessUpdate{Name: setting.ProcessName, Status: bus.STOPPED}
	} else {
		fmt.Printf("Program %s exited with code %d\n", setting.Program, code)
		updates <- bus.ProcessUpdate{Name: setting.ProcessName, Status: bus.FATAL}
	}
}

func Stop(manager *Manager, stops map[string]chan struct{}) {
	for name, ch := range stops {
		if ch != nil {
			closeChannel(ch)
			delete(stops, name)
			fmt.Printf("Stopped program %s\n", name)
		}
	}

	manager.Mu.RLock()
	maxStoptime := 0
	for _, cfg := range manager.Config {
		if cfg.Stoptime > maxStoptime {
			maxStoptime = cfg.Stoptime
		}
	}
	manager.Mu.RUnlock()

	timeout := time.Now().Add(time.Duration(maxStoptime+3) * time.Second)
	for time.Now().Before(timeout) {
		manager.Mu.RLock()
		stopped := true
		for _, proc := range manager.Process {
			pid, status := proc.State()
			if pid > 0 && status == bus.RUNNING {
				stopped = false
				break
			}
		}
		manager.Mu.RUnlock()

		if stopped {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}

	manager.Mu.Lock()
	for _, proc := range manager.Process {
		pid, _ := proc.State()
		if pid > 0 {
			p, _ := os.FindProcess(pid)
			_ = p.Kill()
		}
	}
	manager.Mu.Unlock()
}

func Spawn(prev *Manager, curr *Manager, updates chan bus.ProcessUpdate, stops map[string]chan struct{}) {
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
				curr.Process[name] = &ProcessInstance{
					Status:     bus.STOPPED,
					Intended:   true,
					RetryCount: 0,
					Strategy:   strategy,
				}
			}

			if _, ok := stops[name]; !ok {
				stops[name] = make(chan struct{})
			}

			go curr.Watchdog(setting, updates, stops[name])
			fmt.Printf("Started new program %s\n", setting.ProcessName)
		}
	}

	for _, prevName := range prevKeys {
		if _, exists := curr.Config[prevName]; !exists {
			if ch, ok := stops[prevName]; ok {
				closeChannel(ch)
				delete(stops, prevName)
			}
			fmt.Printf("Stopped removed program %s\n", prevName)
		}
	}
}

func Load(path string) (*Manager, error) {
	loader := &config.YAMLLoader{}
	specs, err := loader.Load(path)
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}

	manager := NewManager()
	manager.Mu.Lock()
	defer manager.Mu.Unlock()

	for i := range specs {
		spec := &specs[i]
		manager.Config[spec.ProcessName] = spec
		if _, ok := manager.Process[spec.ProcessName]; !ok {
			manager.Process[spec.ProcessName] = &ProcessInstance{
				Status:     bus.STOPPED,
				Intended:   spec.Autostart,
				RetryCount: 0,
			}
		}
	}

	return manager, nil
}

func closeChannel(ch chan struct{}) {
	defer func() { recover() }()
	close(ch)
}
