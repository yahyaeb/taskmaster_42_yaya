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
	var waitRunning chan error

	stopSignal := getStopSignal(spec.Stopsignal)
	stopTimeout := time.Duration(spec.Stoptime) * time.Second

	terminatePolicy := func() int {
		if cmd == nil || cmd.Process == nil {
			return -1
		}
		_ = cmd.Process.Signal(stopSignal)
		var waitErr error
		select {
		case waitErr = <-waitRunning:
			if logger != nil {
				logger.LogMessage(LevelInfo, fmt.Sprintf("process '%s' exited cleanly", name))
			}
		case <-time.After(stopTimeout):
			if logger != nil {
				logger.LogMessage(LevelWarn, fmt.Sprintf("process '%s' timed out after %v; escalating to SIGKILL", name, stopTimeout))
			}
			_ = cmd.Process.Kill()
			// Drain channel
			waitErr = <-waitRunning
		}
		return getExitCode(waitErr)
	}

	for {
		isRunning := false

	StartupLoop:
		for attempt := 0; attempt <= spec.Startretries; attempt++ {

			tracker.Emit(STARTING, 0, 0)

			cmd, err = startProcess(spec)
			if err != nil {
				tracker.Emit(FATAL, 0, -1)
				continue
			}

			waitRunning = make(chan error, 1)

			go func(cmd *exec.Cmd, ch chan<- error) {
				ch <- cmd.Wait()
			}(cmd, waitRunning)

			startupWindow := time.Duration(spec.Starttime) * time.Second

			pid := cmd.Process.Pid
			if logger != nil {
				logger.LogMessage(LevelInfo, fmt.Sprintf("spawned: '%s' with pid %d", name, pid))
			}

			select {
			case <-ctx.Done():
				// User requested shutdown at startup window
				if logger != nil {
					logger.LogMessage(LevelInfo, fmt.Sprintf("context canceled during startup window for '%s'", name))
				}
				exitCode := terminatePolicy()
				tracker.Emit(STOPPED, pid, exitCode)
				return
			case err = <-waitRunning:
				if logger != nil {
					logger.LogMessage(LevelWarn, fmt.Sprintf("process '%s' crashed during startup window (attempt %d/%d)", name, attempt, spec.Startretries))
				}
				continue

			case <-time.After(startupWindow):
				isRunning = true
				tracker.Emit(RUNNING, cmd.Process.Pid, 0)
				break StartupLoop
			}
		}

		if !isRunning {
			if logger != nil {
				logger.LogMessage(LevelCritical, fmt.Sprintf("process '%s' failed to start after %d attempts", name, spec.Startretries))
			}
			tracker.Emit(FATAL, 0, -1)
			return
		}

		// Monitor the process for exit

		select {
		case <-ctx.Done():
			// User request stop at running process
			exitCode := terminatePolicy()
			tracker.Emit(STOPPED, cmd.Process.Pid, exitCode)
			return
		case err = <-waitRunning:
			// Common process died
		}

		exitCode := getExitCode(err)

		if restartPolicy(exitCode, spec) {
			if logger != nil {
				logger.LogMessage(LevelWarn, fmt.Sprintf("process '%s' crashed (exit status %d); restarting", name, exitCode))
			}
			tracker.Emit(BACKOFF, 0, 0)
			continue
		}

		tracker.Emit(STOPPED, cmd.Process.Pid, exitCode)
		return
	}

}
