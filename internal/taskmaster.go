package internal

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"slices"
	"time"
)

// DefaultBackoffDelay is the sleep duration used before retrying after a backoff or restart.
const DefaultBackoffDelay = time.Second

type Status string

const (
	STARTING Status = "starting"
	RUNNING  Status = "running"
	STOPPED  Status = "stopped"
	FATAL    Status = "fatal"
	BACKOFF  Status = "backoff"
)

// AutorestartPolicy defines the possible values for the Autorestart configuration field.
type AutorestartPolicy string

const (
	AutorestartAlways     AutorestartPolicy = "always"
	AutorestartNever      AutorestartPolicy = "never"
	AutorestartUnexpected AutorestartPolicy = "unexpected"
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

type processInfo struct {
	ctx         context.Context
	spec        *Config
	tracker     *UpdateTracker
	stopSignal  os.Signal
	stopTimeout time.Duration
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

func stopProcess(cmd *exec.Cmd, stopSignal os.Signal, stopTimeout time.Duration, exitCh <-chan error) int {
	_ = cmd.Process.Signal(stopSignal)

	var err error
	select {
	case err = <-exitCh:
	case <-time.After(stopTimeout):
		_ = cmd.Process.Kill()
		err = <-exitCh
	}
	return getExitCode(err)
}

func restartPolicy(exitCode int, spec *Config) bool {
	policy := AutorestartPolicy(spec.Autorestart)
	switch policy {
	case AutorestartAlways:
		return true
	case AutorestartNever:
		return false
	case AutorestartUnexpected:
		return !slices.Contains(spec.Exitcodes, exitCode)
	}
	return false
}

func launchProcess(spec *Config, tracker *UpdateTracker) (*exec.Cmd, chan error, error) {
	cmd, err := startProcess(spec)
	if err != nil {
		// Process start retry failed
		tracker.Emit(FATAL, 0, -1)
		return nil, nil, err
	}

	exitCh := make(chan error, 1)

	go func(c *exec.Cmd) {
		exitCh <- c.Wait()
	}(cmd)

	return cmd, exitCh, nil
}

func waitForStartup(processInfo *processInfo, name string, logger *Logger) (*exec.Cmd, chan error, int, bool) {
	var cmd *exec.Cmd
	var err error
	var exitCh chan error
	var pid int
	started := false

	for attempt := 0; attempt <= processInfo.spec.Startretries; attempt++ {
		// Emit starting status
		processInfo.tracker.Emit(STARTING, 0, 0)

		// Attempt process start
		cmd, exitCh, err = launchProcess(processInfo.spec, processInfo.tracker)
		if err != nil {
			continue
		}

		pid = cmd.Process.Pid
		if logger != nil {
			logger.LogMessage(LevelInfo, fmt.Sprintf("spawned: '%s' with pid %d", name, pid))
		}

		// Validate process startup within window
		startupWindow := time.Duration(processInfo.spec.Starttime) * time.Second
		// Enforce minimum 1 second startup validation window
		if startupWindow < time.Second {
			startupWindow = time.Second
		}
		timer := time.NewTimer(startupWindow)

		select {
		case <-processInfo.ctx.Done():
			// Context cancelled during startup validation
			stopProcess(cmd, processInfo.stopSignal, processInfo.stopTimeout, exitCh)
			timer.Stop()
			return nil, nil, 0, false

		case err = <-exitCh:
			// Process exited during startup validation
			timer.Stop()
			exitCode := getExitCode(err)
			processInfo.tracker.Emit(FATAL, pid, exitCode)
			if restartPolicy(exitCode, processInfo.spec) {
				processInfo.tracker.Emit(BACKOFF, pid, exitCode)
				time.Sleep(DefaultBackoffDelay)
				continue
			}
			return nil, nil, 0, false

		case <-timer.C:
			// Startup validation passed
			timer.Stop()
			processInfo.tracker.Emit(RUNNING, pid, 0)
			started = true
		}

		if started {
			break
		}
	}

	if !started {
		// All startup retries exhausted
		if logger != nil {
			logger.LogMessage(LevelCritical, fmt.Sprintf("process '%s' failed to start after %d attempts", name, processInfo.spec.Startretries))
		}
		return nil, nil, 0, false
	}

	return cmd, exitCh, pid, true
}

func monitorRuntime(processInfo *processInfo, cmd *exec.Cmd, exitCh chan error, pid int) bool {
	select {
	case <-processInfo.ctx.Done():
		// Context cancelled during runtime
		exitCode := stopProcess(cmd, processInfo.stopSignal, processInfo.stopTimeout, exitCh)
		processInfo.tracker.Emit(STOPPED, pid, exitCode)
		return false

	case err := <-exitCh:
		// Process exited during runtime
		exitCode := getExitCode(err)
		processInfo.tracker.Emit(STOPPED, pid, exitCode)
		if restartPolicy(exitCode, processInfo.spec) {
			processInfo.tracker.Emit(BACKOFF, pid, exitCode)
			return true
		}
		return false
	}
}

func supervise(ctx context.Context, name string, spec *Config, tracker *UpdateTracker, logger *Logger) {

	processInfo := &processInfo{
		ctx:         ctx,
		spec:        spec,
		tracker:     tracker,
		stopSignal:  getStopSignal(spec.Stopsignal),
		stopTimeout: time.Duration(spec.Stoptime) * time.Second,
	}

	for {
		cmd, exitCh, pid, ok := waitForStartup(processInfo, name, logger)
		if !ok {
			return
		}

		shouldRestart := monitorRuntime(processInfo, cmd, exitCh, pid)
		if !shouldRestart {
			return
		}
		time.Sleep(DefaultBackoffDelay)
	}
}
