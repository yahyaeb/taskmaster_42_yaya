package app

import (
	"fmt"
	"reflect"

	"taskmaster/internal/bus"
	"taskmaster/internal/config"
)

func (m *Manager) applyConfigDiff(newConfig map[string]*config.ConfigSpec) (*ReloadResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.ch == nil {
		return nil, fmt.Errorf("process channels not set")
	}

	result := &ReloadResult{
		Added:     []string{},
		Removed:   []string{},
		Restarted: []string{},
	}

	for name := range m.Config {
		if _, exists := newConfig[name]; !exists {
			result.Removed = append(result.Removed, name)
		}
	}

	for name, newSpec := range newConfig {
		if oldSpec, exists := m.Config[name]; !exists {
			result.Added = append(result.Added, name)
			m.Config[name] = newSpec
			if _, ok := m.Process[name]; !ok {
				m.Process[name] = newProcessInstance(newSpec.Autostart)
			}
			if newSpec.Autostart {
				m.startWatchdog(newSpec)
			}
		} else if configChanged(oldSpec, newSpec) {
			result.Restarted = append(result.Restarted, name)
			m.Config[name] = newSpec
			wasRunning := m.isRunning(name)
			if wasRunning {
				m.stopWatchdog(name)
			}
			if newSpec.Autostart {
				m.startWatchdog(newSpec)
			}
		}
	}

	for _, name := range result.Removed {
		m.stopWatchdog(name)
		delete(m.Config, name)
		if proc, ok := m.Process[name]; ok {
			proc.Status = bus.STOPPED
		}
	}

	return result, nil
}

func configChanged(a, b *config.ConfigSpec) bool {
	return !reflect.DeepEqual(a, b)
}
