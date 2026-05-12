package app

import (
	"context"
	"log/slog"
	"math"
	"strings"
	"syscall"
	"time"

	"taskmaster/internal/bus"
	"taskmaster/internal/config"
	"taskmaster/internal/engine"
)

// Watchdog monitors a single process instance, handling its lifecycle
// including startup, restart decisions, and graceful shutdown.
// It runs in its own goroutine for each managed process.
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
