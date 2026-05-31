package internal

import (
	"context"
	"fmt"
	"time"
)

type Lifecycle int

const (
	ActionStop Lifecycle = iota
	ActionRestart
)

type LifecycleEvent struct {
	Action   Lifecycle
	Pid      int
	ExitCode int
}

func Exit(logger *Logger, mgr *Manager, shutdownFunc context.CancelFunc, message string) {
	logger.LogMessage(LevelInfo, message)
	shutdownFunc()
	mgr.Shutdown()
	time.Sleep(500 * time.Microsecond)
	logger.Close()
}

func HotWire(
	path string,
	mgr *Manager,
	svr *Server,
	logger *Logger,
) error {
	configMap, memGuardCfg, err := LoadConfig(path)
	if err != nil {
		return fmt.Errorf("failed to reload config: %w", err)
	}

	if err := mgr.Reload(configMap); err != nil {
		return fmt.Errorf("failed to reload manager: %w", err)
	}

	svr.SetMemoryGuardConfig(memGuardCfg)

	logger.LogMessage(LevelInfo, "received reload signal (SIGHUP), reloading config")

	return nil
}
