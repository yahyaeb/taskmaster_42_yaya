package internal

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"slices"
	"time"
)

type Status string

const (
	STARTING Status = "starting"
	RUNNING  Status = "running"
	STOPPED  Status = "stopped"
	FATAL    Status = "fatal"
	BACKOFF  Status = "backoff"
)

type ProcessUpdate struct {
	Name      string
	Status    Status
	Pid       int
	ExitCode  int
	LastStart time.Time
}

type UpdateTracker struct {
	name    string
	updates chan<- ProcessUpdate
}

func NewUpdateTracker(name string, ch chan<- ProcessUpdate) *UpdateTracker {
	return &UpdateTracker{name: name, updates: ch}
}

func (t *UpdateTracker) Emit(status Status, pid int, exitCode int) {
	t.updates <- ProcessUpdate{
		Name:      t.name,
		Status:    status,
		Pid:       pid,
		ExitCode:  exitCode,
		LastStart: time.Now(),
	}
}

func getExitCode(err error) int {
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return exitErr.ExitCode()
		}
		return -1
	}
	return 0
}

func restartPolicy(exitCode int, spec *Config) bool {
	switch spec.Autorestart {
	case "always":
		return true
	case "never":
		return false
	case "unexpected":
		return !slices.Contains(spec.Exitcodes, exitCode)
	}
	return false
}

func supervise(ctx context.Context, name string, spec *Config, tracker *UpdateTracker, logger *Logger) {

	var cmd *exec.Cmd
	var err error
	var exitCh chan error
	var pid int

	stopSignal := getStopSignal(spec.Stopsignal)
	stopTimeout := time.Duration(spec.Stoptime) * time.Second

	for {
		// Startup phase: try to spawn and validate process
		started := false

		for attempt := 0; attempt <= spec.Startretries; attempt++ {
			// Emit starting status
			tracker.Emit(STARTING, 0, 0)

			// Attempt process start
			cmd, err = startProcess(spec)
			if err != nil {
				// Process start retry failed
				tracker.Emit(FATAL, 0, -1)
				continue
			}

			exitCh = make(chan error, 1)
			command := cmd
			go func(c *exec.Cmd) {
				exitCh <- c.Wait()
			}(command)

			pid = cmd.Process.Pid
			if logger != nil {
				logger.LogMessage(LevelInfo, fmt.Sprintf("spawned: '%s' with pid %d", name, pid))
			}

			// Validate process startup within window
			startupWindow := time.Duration(spec.Starttime) * time.Second
			// Enforce minimum 1 second startup validation window
			if startupWindow < time.Second {
				startupWindow = time.Second
			}
			timer := time.NewTimer(startupWindow)

			select {
			case <-ctx.Done():
				// Context cancelled during startup validation
				_ = cmd.Process.Signal(stopSignal)

				select {
				case <-exitCh:
				case <-time.After(stopTimeout):
					_ = cmd.Process.Kill()
					<-exitCh
				}

				tracker.Emit(STOPPED, pid, 0)
				return

			case err = <-exitCh:
				// Process exited during startup validation
				exitCode := getExitCode(err)
				tracker.Emit(FATAL, pid, exitCode)
				if restartPolicy(exitCode, spec) {
					tracker.Emit(BACKOFF, pid, exitCode)
					time.Sleep(time.Duration(1) * time.Second)
					continue
				}
				return

			case <-timer.C:
				// Startup validation passed
				tracker.Emit(RUNNING, pid, 0)
				started = true
			}

			if started {
				break
			}
		}

		if !started {
			// All startup retries exhausted
			if logger != nil {
				logger.LogMessage(LevelCritical, fmt.Sprintf("process '%s' failed to start after %d attempts", name, spec.Startretries))
			}
			return
		}

		// Runtime monitoring phase
		select {
		case <-ctx.Done():
			// Context cancelled during runtime
			_ = cmd.Process.Signal(stopSignal)

			select {
			case <-exitCh:
			case <-time.After(stopTimeout):
				_ = cmd.Process.Kill()
				<-exitCh
			}

			tracker.Emit(STOPPED, pid, 0)
			return

		case err = <-exitCh:
			// Process exited during runtime
			exitCode := getExitCode(err)
			tracker.Emit(STOPPED, pid, exitCode)
			if restartPolicy(exitCode, spec) {
				tracker.Emit(BACKOFF, pid, exitCode)
				time.Sleep(time.Duration(1) * time.Second)
				continue
			}
			return
		}
	}
}
