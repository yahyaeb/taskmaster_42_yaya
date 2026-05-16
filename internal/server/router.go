package server

import (
	"encoding/json"
	"errors"
	"net"
	"os"
	"strings"
	"time"

	"taskmaster/internal/app"
	"taskmaster/internal/protocol"
)

const readRequestDeadline = 5 * time.Second

type ProcessManager interface {
	GetProcessInfo(name string) (protocol.ProcessInfo, error)
	GetAllProcessInfo() ([]protocol.ProcessInfo, error)
	Start(name string) error
	Stop(name string) error
	Restart(name string) error
	Reload() (*app.ReloadResult, error)
	Shutdown() error
}

func HandleConnection(conn net.Conn, mgr ProcessManager) {
	defer conn.Close()

	if err := conn.SetReadDeadline(time.Now().Add(readRequestDeadline)); err != nil {
		return
	}

	decoder := json.NewDecoder(conn)
	encoder := json.NewEncoder(conn)

	var req protocol.RPCRequest
	if err := decoder.Decode(&req); err != nil {
		if isReadTimeout(err) {
			return
		}
		resp := protocol.NewErrorResponse(protocol.ParseError, "parse error", 0)
		_ = encoder.Encode(resp)
		return
	}

	resp := RouteRequest(&req, mgr)
	_ = encoder.Encode(resp)
}

func isReadTimeout(err error) bool {
	if errors.Is(err, os.ErrDeadlineExceeded) {
		return true
	}
	var ne net.Error
	return errors.As(err, &ne) && ne.Timeout()
}

func RouteRequest(req *protocol.RPCRequest, mgr ProcessManager) *protocol.RPCResponse {
	if req.Method == "" {
		return protocol.NewErrorResponse(protocol.InvalidRequest, "method is required", req.ID)
	}

	parts := strings.Split(req.Method, ".")
	if len(parts) != 2 || parts[0] != protocol.MethodNamespace {
		return protocol.NewErrorResponse(protocol.InvalidRequest, "invalid method format", req.ID)
	}

	action := parts[1]
	info, ok := handlers[action]
	if !ok {
		return protocol.NewErrorResponse(protocol.MethodNotFound, "method not found", req.ID)
	}

	return withRecovery(func() *protocol.RPCResponse {
		params, _ := req.Params.(map[string]any)
		name, _ := params["name"].(string)

		if info.needsName {
			if name == "" {
				return protocol.NewErrorResponse(protocol.InvalidParams, "name is required", req.ID)
			}
			if !info.allowAll && name == "all" {
				return protocol.NewErrorResponse(protocol.InvalidParams, "cannot use 'all' with "+action, req.ID)
			}
		} else if name != "" && !info.allowAll {
			return protocol.NewErrorResponse(protocol.InvalidParams, action+" does not accept a name", req.ID)
		}

		if name != "" && name != "all" && action != "GetStatus" {
			if _, err := mgr.GetProcessInfo(name); err != nil {
				return protocol.NewErrorResponse(protocol.ProcessNotFound, err.Error(), req.ID)
			}
		}

		result, err := info.fn(mgr, params)
		if err != nil {
			return protocol.NewErrorResponse(protocol.OperationFailed, err.Error(), req.ID)
		}

		return protocol.NewSuccessResponse(result, req.ID)
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
