package app

import (
	"fmt"
	"reflect"
	"syscall"
	"time"

	"taskmaster/internal/config"
)

type ReloadResult struct {
	Added     []string
	Removed   []string
	Restarted []string
}

func (m *Manager) Reload() (*ReloadResult, error) {
	m.log.Info("reload requested")
	if m.configPath == "" {
		return nil, fmt.Errorf("no config path stored; set config path before reload")
	}

	specs, err := m.loader.Load(m.configPath)
	if err != nil {
		m.log.Error("reload failed", "phase", "load", "err", err)
		return nil, fmt.Errorf("reload config: %w", err)
	}

	newConfig := make(map[string]*config.ConfigSpec)
	for i := range specs {
		spec := &specs[i]
		newConfig[spec.ProcessName] = spec
	}

	result, err := m.applyConfigDiff(newConfig)
	if err != nil {
		m.log.Error("reload failed", "phase", "apply", "err", err)
		return nil, err
	}
	m.log.Info("reload completed", "added", result.Added, "removed", result.Removed, "restarted", result.Restarted)
	return result, nil
}

func (m *Manager) ReloadFromConfig(newConfig map[string]*config.ConfigSpec) (*ReloadResult, error) {
	return m.applyConfigDiff(newConfig)
}

func (m *Manager) applyConfigDiff(newConfig map[string]*config.ConfigSpec) (*ReloadResult, error) {
	if m.ch == nil {
		return nil, fmt.Errorf("process channels not set")
	}

	result := &ReloadResult{
		Added:     []string{},
		Removed:   []string{},
		Restarted: []string{},
	}

	for _, name := range m.reg.SpecNames() {
		if _, exists := newConfig[name]; !exists {
			result.Removed = append(result.Removed, name)
		}
	}

	for name, newSpec := range newConfig {
		oldSpec, hadOld := m.reg.GetSpec(name)
		if !hadOld {
			func() {
				unlock := m.processLock(name)
				defer unlock()
				result.Added = append(result.Added, name)
				m.reg.Upsert(name, newSpec)
				if newSpec.Autostart {
					m.startWatchdog(newSpec)
				}
			}()
			continue
		}

		if config.SpecsRequireRestart(oldSpec, newSpec) {
			result.Restarted = append(result.Restarted, name)
			wasRunning := m.reg.IsRunning(name)
			unlock := m.processLock(name)
			func() {
				defer unlock()
				if wasRunning {
					m.stopWatchdog(name)
					waitDur := time.Duration(oldSpec.Stoptime)*time.Second + ReloadDiffGraceMillis*time.Millisecond
					if err := m.waitProcessTerminal(name, waitDur); err != nil {
						_, proc, _ := m.reg.Get(name)
						if proc != nil && proc.Pid > 0 {
							_ = syscall.Kill(proc.Pid, syscall.SIGKILL)
						}
						killWait := time.Duration(StopKillVerifySeconds) * time.Second
						_ = m.waitProcessTerminal(name, killWait)
					}
				}
				m.reg.SetSpec(name, newSpec)
				if newSpec.Autostart {
					m.startWatchdog(newSpec)
				}
			}()
			continue
		}

		if !reflect.DeepEqual(oldSpec, newSpec) {
			func() {
				unlock := m.processLock(name)
				defer unlock()
				m.reg.SetSpec(name, newSpec)
			}()
		}
	}

	for _, name := range result.Removed {
		func() {
			unlock := m.processLock(name)
			defer unlock()
			m.stopWatchdog(name)
			m.reg.Delete(name)
		}()
	}

	return result, nil
}
