package app

import (
	"context"

	"taskmaster/internal/config"
	"taskmaster/internal/engine"
)

func (m *Manager) startWatchdog(spec *config.ConfigSpec) {
	name := spec.ProcessName
	ctx, cancel := context.WithCancel(context.Background())
	m.reg.BindWatchdog(name, cancel)
	pub := m.ch.StatusPublisher()
	go func() {
		defer m.reg.NotifyRunReturned(name)
		engine.Run(ctx, *spec, m.executor, m.handler, pub)
	}()
}

func (m *Manager) stopWatchdog(name string) {
	m.reg.ClearWatchdog(name)
}
