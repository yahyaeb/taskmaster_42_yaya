package app

import (
	"fmt"
	"slices"

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
				if _, ok := m.ch.Stop[name]; !ok {
					m.ch.Stop[name] = make(chan struct{})
				}
				go m.Watchdog(newSpec, m.Process[name])
			}
		} else if configChanged(oldSpec, newSpec) {
			result.Restarted = append(result.Restarted, name)
			m.Config[name] = newSpec
			wasRunning := m.isRunning(name)
			if wasRunning {
				if stopCh, ok := m.ch.Stop[name]; ok {
					closeChannel(stopCh)
					delete(m.ch.Stop, name)
				}
			}
			if newSpec.Autostart {
				if _, ok := m.ch.Stop[name]; !ok {
					m.ch.Stop[name] = make(chan struct{})
				}
				go m.Watchdog(newSpec, m.Process[name])
			}
		}
	}

	for _, name := range result.Removed {
		if stopCh, ok := m.ch.Stop[name]; ok {
			closeChannel(stopCh)
			delete(m.ch.Stop, name)
		}
		delete(m.Config, name)
		if proc, ok := m.Process[name]; ok {
			proc.Status = bus.STOPPED
		}
	}

	return result, nil
}

func configChanged(a, b *config.ConfigSpec) bool {
	if a.Cmd != b.Cmd ||
		a.Numprocs != b.Numprocs ||
		a.NumprocsStart != b.NumprocsStart ||
		a.Umask != b.Umask ||
		a.Workingdir != b.Workingdir ||
		a.Autostart != b.Autostart ||
		a.Autorestart != b.Autorestart ||
		a.Startretries != b.Startretries ||
		a.Starttime != b.Starttime ||
		a.Stopsignal != b.Stopsignal ||
		a.Stoptime != b.Stoptime ||
		a.Stdout != b.Stdout ||
		a.Stderr != b.Stderr {
		return true
	}
	if !slicesEqual(a.Exitcodes, b.Exitcodes) {
		return true
	}
	if len(a.Env) != len(b.Env) {
		return true
	}
	for k, v := range a.Env {
		if b.Env[k] != v {
			return true
		}
	}
	return false
}

func slicesEqual(a, b []int) bool {
	if len(a) == 0 && len(b) == 0 {
		return true
	}
	return slices.Equal(a, b)
}