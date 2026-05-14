package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"strings"
	"syscall"
	"time"

	"taskmaster/internal/bus"
	"taskmaster/internal/config"
	"taskmaster/internal/engine"
)

func (m *Manager) notifyRetry(setting *config.ConfigSpec, stop chan struct{}, updates chan<- bus.ProcessUpdate, attempt int) error {
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
		m.ch.PublishStatus(bus.ProcessUpdate{Name: setting.ProcessName, Status: bus.FATAL})
		return errors.New("no command")
	}
	if proc == nil {
		slog.Error("ProcessInstance not found", "process", setting.ProcessName)
		m.ch.PublishStatus(bus.ProcessUpdate{Name: setting.ProcessName, Status: bus.FATAL})
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

// spawnRun launches the process in a goroutine
func (m *Manager) spawnRun(
	ctx context.Context,
	setting *config.ConfigSpec,
	updates chan<- bus.ProcessUpdate,
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

// pidAfterStart gets PID from channel or process map
func (m *Manager) pidAfterStart(setting *config.ConfigSpec, pidCh chan int) int {
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

// killProcess sends stop signal and waits for process to exit
func (m *Manager) killProcess(
	setting *config.ConfigSpec,
	pidCh chan int,
	resultCh chan engine.ExitCode,
	errCh chan error,
) {
	pid := m.pidAfterStart(setting, pidCh)
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

// evaluateExit decides whether to restart, stop, or fatal based on exit
func (m *Manager) evaluateExit(
	setting *config.ConfigSpec,
	updates chan<- bus.ProcessUpdate,
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

// launchAndWait runs a single process attempt and returns (done, shouldReturn)
func (m *Manager) launchAndWait(
	setting *config.ConfigSpec,
	stop chan struct{},
	updates chan<- bus.ProcessUpdate,
	attempt int,
) (bool, bool, engine.ExitCode) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	resultCh, errCh, pidCh := m.spawnRun(ctx, setting, updates)
	var exitCode engine.ExitCode
	var err error

	select {
	case <-stop:
		m.killProcess(setting, pidCh, resultCh, errCh)
		updates <- bus.ProcessUpdate{Name: setting.ProcessName, Status: bus.STOPPED}
		return true, true, exitCode

	case exitCode = <-resultCh:
	case err = <-errCh:
	}

	return m.evaluateExit(setting, updates, exitCode, err, attempt)
}

// startWatchdog launches Watchdog in a new goroutine.
func (m *Manager) startWatchdog(spec *config.ConfigSpec, proc *ProcessInstance) {
	go m.Watchdog(spec, proc)
	slog.Info("watchdog started", "name", spec.ProcessName)
}

func (m *Manager) Watchdog(setting *config.ConfigSpec, proc *ProcessInstance) {
	if err := m.validateWatchdog(setting, proc); err != nil {
		return
	}

	stopCh := m.ch.EnsureSupervisorStop(setting.ProcessName)
	stop := stopCh.C()
	updates := m.ch.StatusPublisher()

	maxRetries := resolveMaxRetries(setting)

	for attempt := 0; attempt <= maxRetries; attempt++ {
		if isStopped(stop) {
			updates <- bus.ProcessUpdate{Name: setting.ProcessName, Status: bus.STOPPED}
			return
		}

		if err := m.notifyRetry(setting, stop, updates, attempt); err != nil {
			return
		}

		done, shouldReturn, exitCode := m.launchAndWait(setting, stop, updates, attempt)
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
