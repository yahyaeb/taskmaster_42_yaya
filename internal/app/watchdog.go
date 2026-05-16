package app

import (
	"context"

	"taskmaster/internal/bus"
	"taskmaster/internal/config"
	"taskmaster/internal/taskmaster"
)

func (m *Manager) startWatchdog(spec *config.ConfigSpec) {
	name := spec.ProcessName
	proc := m.Process[name]
	ctx, cancel := context.WithCancel(context.Background())
	proc.cancelFn = cancel
	proc.Status = bus.STARTING
	go func() {
		taskmaster.Run(ctx, *spec, m.executor, m.handler, m.ch.status)
	}()
}

func (m *Manager) stopWatchdog(name string) {
	proc, ok := m.Process[name]
	if !ok || proc.cancelFn == nil {
		return
	}
	proc.cancelFn()
	proc.cancelFn = nil
}
