package server

import (
	"taskmaster/internal/protocol"
)

type HandlerFunc func(ProcessManager, map[string]any) (any, error)

type handlerInfo struct {
	fn        HandlerFunc
	needsName bool
	allowAll  bool
}

var handlers = map[string]handlerInfo{
	"GetStatus": {fn: handleGetStatus, needsName: false, allowAll: true},
	"Start":     {fn: handleStart, needsName: true, allowAll: false},
	"Stop":      {fn: handleStop, needsName: true, allowAll: false},
	"Restart":   {fn: handleRestart, needsName: true, allowAll: false},
	"Reload":    {fn: handleReload, needsName: false, allowAll: false},
	"Shutdown":  {fn: handleShutdown, needsName: false, allowAll: false},
}

func handleGetStatus(mgr ProcessManager, params map[string]any) (any, error) {
	name, _ := params["name"].(string)
	if name == "" || name == "all" {
		return mgr.GetAllProcessInfo()
	}
	return mgr.GetProcessInfo(name)
}

func handleStart(mgr ProcessManager, params map[string]any) (any, error) {
	name, _ := params["name"].(string)
	if err := mgr.Start(name); err != nil {
		return nil, err
	}
	return protocol.ActionResponse{Success: true, Message: "process starting"}, nil
}

func handleStop(mgr ProcessManager, params map[string]any) (any, error) {
	name, _ := params["name"].(string)
	if err := mgr.Stop(name); err != nil {
		return nil, err
	}
	return protocol.ActionResponse{Success: true, Message: "process stopping"}, nil
}

func handleRestart(mgr ProcessManager, params map[string]any) (any, error) {
	name, _ := params["name"].(string)
	if err := mgr.Restart(name); err != nil {
		return nil, err
	}
	return protocol.ActionResponse{Success: true, Message: "process restarting"}, nil
}

func handleReload(mgr ProcessManager, params map[string]any) (any, error) {
	result, err := mgr.Reload()
	if err != nil {
		return nil, err
	}
	return protocol.ReloadResponse{
		Added:     result.Added,
		Removed:   result.Removed,
		Restarted: result.Restarted,
	}, nil
}

func handleShutdown(mgr ProcessManager, params map[string]any) (any, error) {
	if err := mgr.Shutdown(); err != nil {
		return nil, err
	}
	return protocol.ActionResponse{Success: true, Message: "Daemon shutting down..."}, nil
}
