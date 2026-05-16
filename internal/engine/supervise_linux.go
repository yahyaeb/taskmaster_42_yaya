//go:build linux

package engine

import (
	"context"
	"fmt"
	"math"
	"os"
	"strings"
	"time"

	"taskmaster/internal/bus"
	"taskmaster/internal/config"
)

type outcome int

const (
	outcomeStopped outcome = iota
	outcomeRetry
	outcomeFatal
)

// StopReport summarizes how process supervision ended for one spec.
type StopReport struct {
	Name    string
	Status  bus.Status
	Retries int
}

func emit(updates chan<- bus.ProcessUpdate, s string, status bus.Status, attempt int) {
	updates <- bus.ProcessUpdate{
		Name:       s,
		Status:     status,
		Retries:    attempt,
		HasRetries: true,
	}
}

func returnMaxRetries(setting config.ConfigSpec) int {
	if setting.Autorestart == "always" {
		return math.MaxInt
	}
	return setting.Startretries
}

func procState(pid int) (byte, error) {
	statPath := fmt.Sprintf("/proc/%d/stat", pid)
	data, err := os.ReadFile(statPath)
	if err != nil {
		return 0, err
	}
	contents := string(data)
	idx := strings.LastIndex(contents, ") ")
	if idx == -1 || idx+2 >= len(contents) {
		return 0, fmt.Errorf("parse /proc stat for pid %d", pid)
	}
	return contents[idx+2], nil
}

func awaitStarttime(ctx context.Context, pid int, starttime int) bool {
	if starttime <= 0 {
		return true
	}
	timer := time.NewTimer(time.Duration(starttime) * time.Second)
	defer timer.Stop()

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			state, err := procState(pid)
			if err != nil || state == 'Z' {
				return false
			}
		case <-timer.C:
			return true
		case <-ctx.Done():
			return false
		}
	}
}

// Run supervises one process according to spec until stopped, fatal, or retries exhausted.
func Run(
	ctx context.Context,
	spec config.ConfigSpec,
	executor ProcessExecutor,
	signals SignalHandler,
	updates chan<- bus.ProcessUpdate,
) StopReport {
	maxRetries := returnMaxRetries(spec)
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if ctx.Err() != nil {
			emit(updates, spec.ProcessName, bus.STOPPED, attempt)
			return StopReport{spec.ProcessName, bus.STOPPED, attempt}
		}

		if attempt > 0 {
			emit(updates, spec.ProcessName, bus.BACKOFF, attempt)
		}

		outcome := runOnce(ctx, spec, executor, signals, updates, attempt)

		switch outcome {
		case outcomeStopped:
			emit(updates, spec.ProcessName, bus.STOPPED, attempt)
			return StopReport{spec.ProcessName, bus.STOPPED, attempt}
		case outcomeFatal:
			emit(updates, spec.ProcessName, bus.FATAL, attempt)
			return StopReport{spec.ProcessName, bus.FATAL, attempt}
		case outcomeRetry:
			// repeat
		}
	}
	emit(updates, spec.ProcessName, bus.FATAL, maxRetries)
	return StopReport{spec.ProcessName, bus.FATAL, maxRetries}
}

func runOnce(
	ctx context.Context,
	spec config.ConfigSpec,
	executor ProcessExecutor,
	signals SignalHandler,
	updates chan<- bus.ProcessUpdate,
	attempt int,
) outcome {

	proc, err := executor.Start(ctx, spec)
	if err != nil {
		return outcomeFatal
	}

	emit(updates, spec.ProcessName, bus.STARTING, attempt)
	pidUpdate(updates, spec.ProcessName, proc.PID)

	if alive := awaitStarttime(ctx, proc.PID, spec.Starttime); !alive {
		executor.Wait(ctx, proc.PID)
		return outcomeRetry
	}

	emit(updates, spec.ProcessName, bus.RUNNING, attempt)

	exitCode, err := awaitExit(ctx, proc.PID, executor.Wait)
	if err != nil {
		return outcomeStopped
	}

	if RestartPolicy(spec.Autorestart, int(exitCode), spec.Exitcodes) {
		return outcomeRetry
	}

	return outcomeStopped

}

func pidUpdate(updates chan<- bus.ProcessUpdate, s string, i int) {
	updates <- bus.ProcessUpdate{
		Name: s,
		Pid:  i,
	}
}

func waitForProcessExit(
	wait func(context.Context, int) (ExitCode, error),
	pid int,
	exited chan struct {
		code ExitCode
		err  error
	},
) {
	code, err := wait(context.Background(), pid)
	exited <- struct {
		code ExitCode
		err  error
	}{code, err}
}

func awaitExit(ctx context.Context, pid int, wait func(context.Context, int) (ExitCode, error)) (ExitCode, error) {

	exited := make(chan struct {
		code ExitCode
		err  error
	}, 1)

	go waitForProcessExit(wait, pid, exited)

	select {
	case result := <-exited:
		return result.code, result.err
	case <-ctx.Done():
		return 0, ctx.Err()
	}
}
