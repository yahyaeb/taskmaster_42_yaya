package engine

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"taskmaster/internal/bus"
	"taskmaster/internal/config"
)

func ShouldRestart(autorestart string, exitCode int, exitcodes []int) bool {
	switch autorestart {
	case "always":
		return true
	case "never":
		return false
	case "unexpected":
		for _, expected := range exitcodes {
			if exitCode == expected {
				return false
			}
		}
		return true
	default:
		return false
	}
}

type ProcessWatcher struct {
	Executor ProcessExecutor
}

func NewProcessWatcher(executor ProcessExecutor) *ProcessWatcher {
	return &ProcessWatcher{Executor: executor}
}

func (pw *ProcessWatcher) Run(ctx context.Context, spec config.ConfigSpec, updates chan<- bus.ProcessUpdate) (ExitCode, error) {
	process, err := pw.Executor.Start(ctx, spec)
	if err != nil {
		updates <- bus.ProcessUpdate{
			Name:   spec.ProcessName,
			Status: bus.FATAL,
		}
		return 0, fmt.Errorf("spawn failed: %w", err)
	}

	updates <- bus.ProcessUpdate{
		Name:       spec.ProcessName,
		Status:     bus.STARTING,
		Pid:        process.PID,
		Retries:    0,
		HasRetries: true,
		LastStart:  time.Now(),
	}

	starttime := time.Duration(spec.Starttime) * time.Second
	if starttime > 0 {
		timer := time.NewTimer(starttime)
		earlyExit := make(chan struct{})
		var earlyExitOnce sync.Once
		done := make(chan struct{})

		go func() {
			ticker := time.NewTicker(100 * time.Millisecond)
			defer ticker.Stop()
			for {
				select {
				case <-ticker.C:
					state, err := procState(process.PID)
					if err != nil || state == 'Z' {
						earlyExitOnce.Do(func() { close(earlyExit) })
						return
					}
				case <-done:
					return
				case <-ctx.Done():
					return
				}
			}
		}()

		select {
		case <-earlyExit:
			timer.Stop()
			close(done)
			exitCode, _ := pw.Executor.Wait(ctx, process.PID)
			return exitCode, nil

		case <-timer.C:
			close(done)
			updates <- bus.ProcessUpdate{
				Name:   spec.ProcessName,
				Status: bus.RUNNING,
				Pid:    process.PID,
			}

		case <-ctx.Done():
			timer.Stop()
			close(done)
			return 0, ctx.Err()
		}
	} else {
		updates <- bus.ProcessUpdate{
			Name:   spec.ProcessName,
			Status: bus.RUNNING,
			Pid:    process.PID,
		}
	}

	exitCode, err := pw.Executor.Wait(ctx, process.PID)
	if err != nil {
		return exitCode, err
	}

	return exitCode, nil
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
