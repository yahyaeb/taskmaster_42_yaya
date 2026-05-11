package app

import (
	"encoding/json"
	"net"
	"strings"

	"taskmaster/internal/protocol"
)

func HandleConnection(conn net.Conn, manager ProcessManager) {
	defer conn.Close()

	decoder := json.NewDecoder(conn)
	encoder := json.NewEncoder(conn)

	var req protocol.RPCRequest
	if err := decoder.Decode(&req); err != nil {
		resp := protocol.NewErrorResponse(protocol.ParseError, "parse error", 0)
		_ = encoder.Encode(resp)
		return
	}

	resp := RouteRequest(&req, manager)
	_ = encoder.Encode(resp)
}

func RouteRequest(req *protocol.RPCRequest, manager ProcessManager) *protocol.RPCResponse {
	if req.Method == "" {
		return protocol.NewErrorResponse(protocol.InvalidRequest, "method is required", req.ID)
	}

	parts := strings.Split(req.Method, ".")
	if len(parts) != 2 || parts[0] != "Taskmaster" {
		return protocol.NewErrorResponse(protocol.InvalidRequest, "invalid method format", req.ID)
	}

	action := parts[1]

	return withRecovery(func() *protocol.RPCResponse {
		switch action {
		case "GetStatus":
			return handleGetStatus(req, manager)
		case "Start":
			return handleStart(req, manager)
		case "Stop":
			return handleStop(req, manager)
		case "Restart":
			return handleRestart(req, manager)
		case "Reload":
			return handleReload(req, manager)
		case "Shutdown":
			return handleShutdown(req, manager)
		default:
			return protocol.NewErrorResponse(protocol.MethodNotFound, "method not found", req.ID)
		}
	}, req.ID)
}

func withRecovery(fn func() *protocol.RPCResponse, id int) (resp *protocol.RPCResponse) {
	defer func() {
		if r := recover(); r != nil {
			resp = protocol.NewErrorResponse(protocol.InternalError, "internal error", id)
		}
	}()
	return fn()
}

func getNameFromParams(params interface{}) (string, bool) {
	if params == nil {
		return "", false
	}
	m, ok := params.(map[string]interface{})
	if !ok {
		return "", false
	}
	name, ok := m["name"].(string)
	return name, ok
}

func handleGetStatus(req *protocol.RPCRequest, manager ProcessManager) *protocol.RPCResponse {
	name, _ := getNameFromParams(req.Params)

	if name == "" || name == "all" {
		infos, err := manager.GetAllProcessInfo()
		if err != nil {
			return protocol.NewErrorResponse(protocol.OperationFailed, err.Error(), req.ID)
		}
		return protocol.NewSuccessResponse(infos, req.ID)
	}

	info, err := manager.GetProcessInfo(name)
	if err != nil {
		return protocol.NewErrorResponse(protocol.ProcessNotFound, err.Error(), req.ID)
	}
	return protocol.NewSuccessResponse(info, req.ID)
}

func handleStart(req *protocol.RPCRequest, manager ProcessManager) *protocol.RPCResponse {
	name, hasName := getNameFromParams(req.Params)

	if !hasName || name == "" {
		return protocol.NewErrorResponse(protocol.InvalidParams, "name is required", req.ID)
	}
	if name == "all" {
		return protocol.NewErrorResponse(protocol.InvalidParams, "cannot use 'all' with Start", req.ID)
	}

	// Pre-check to return ProcessNotFound (not OperationFailed) for missing process.
	if _, err := manager.GetProcessInfo(name); err != nil {
		return protocol.NewErrorResponse(protocol.ProcessNotFound, err.Error(), req.ID)
	}

	if err := manager.Start(name); err != nil {
		return protocol.NewErrorResponse(protocol.OperationFailed, err.Error(), req.ID)
	}
	return protocol.NewSuccessResponse(protocol.ActionResponse{Success: true, Message: "process starting"}, req.ID)
}

func handleStop(req *protocol.RPCRequest, manager ProcessManager) *protocol.RPCResponse {
	name, hasName := getNameFromParams(req.Params)

	if !hasName || name == "" {
		return protocol.NewErrorResponse(protocol.InvalidParams, "name is required", req.ID)
	}
	if name == "all" {
		return protocol.NewErrorResponse(protocol.InvalidParams, "cannot use 'all' with Stop", req.ID)
	}

	// Pre-check to return ProcessNotFound (not OperationFailed) for missing process.
	if _, err := manager.GetProcessInfo(name); err != nil {
		return protocol.NewErrorResponse(protocol.ProcessNotFound, err.Error(), req.ID)
	}

	if err := manager.Stop(name); err != nil {
		return protocol.NewErrorResponse(protocol.OperationFailed, err.Error(), req.ID)
	}
	return protocol.NewSuccessResponse(protocol.ActionResponse{Success: true, Message: "process stopping"}, req.ID)
}

func handleRestart(req *protocol.RPCRequest, manager ProcessManager) *protocol.RPCResponse {
	name, hasName := getNameFromParams(req.Params)

	if !hasName || name == "" {
		return protocol.NewErrorResponse(protocol.InvalidParams, "name is required", req.ID)
	}
	if name == "all" {
		return protocol.NewErrorResponse(protocol.InvalidParams, "cannot use 'all' with Restart", req.ID)
	}

	// Pre-check to return ProcessNotFound (not OperationFailed) for missing process.
	if _, err := manager.GetProcessInfo(name); err != nil {
		return protocol.NewErrorResponse(protocol.ProcessNotFound, err.Error(), req.ID)
	}

	if err := manager.Restart(name); err != nil {
		return protocol.NewErrorResponse(protocol.OperationFailed, err.Error(), req.ID)
	}
	return protocol.NewSuccessResponse(protocol.ActionResponse{Success: true, Message: "process restarting"}, req.ID)
}

func handleReload(req *protocol.RPCRequest, manager ProcessManager) *protocol.RPCResponse {
	name, hasName := getNameFromParams(req.Params)

	if hasName && name != "" {
		return protocol.NewErrorResponse(protocol.InvalidParams, "Reload does not accept a name", req.ID)
	}

	result, err := manager.Reload()
	if err != nil {
		return protocol.NewErrorResponse(protocol.OperationFailed, err.Error(), req.ID)
	}
	return protocol.NewSuccessResponse(protocol.ReloadResponse{
		Added:     result.Added,
		Removed:   result.Removed,
		Restarted: result.Restarted,
	}, req.ID)
}

func handleShutdown(req *protocol.RPCRequest, manager ProcessManager) *protocol.RPCResponse {
	name, hasName := getNameFromParams(req.Params)

	if hasName && name != "" {
		return protocol.NewErrorResponse(protocol.InvalidParams, "Shutdown does not accept a name", req.ID)
	}

	if err := manager.Shutdown(); err != nil {
		return protocol.NewErrorResponse(protocol.OperationFailed, err.Error(), req.ID)
	}
	return protocol.NewSuccessResponse(protocol.ActionResponse{Success: true, Message: "Daemon shutting down..."}, req.ID)
}
