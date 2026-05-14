package app

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"strings"
	"sync"

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

func HandleConnection(conn net.Conn, mgr ProcessManager) {
	defer conn.Close()

	decoder := json.NewDecoder(conn)
	encoder := json.NewEncoder(conn)

	var req protocol.RPCRequest
	if err := decoder.Decode(&req); err != nil {
		resp := protocol.NewErrorResponse(protocol.ParseError, "parse error", 0)
		_ = encoder.Encode(resp)
		return
	}

	resp := RouteRequest(&req, mgr)
	_ = encoder.Encode(resp)
}

func RouteRequest(req *protocol.RPCRequest, mgr ProcessManager) *protocol.RPCResponse {
	if req.Method == "" {
		return protocol.NewErrorResponse(protocol.InvalidRequest, "method is required", req.ID)
	}

	parts := strings.Split(req.Method, ".")
	if len(parts) != 2 || parts[0] != "Taskmaster" {
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

type SocketListener struct {
	listener net.Listener
	stopChan chan struct{}
	wg       sync.WaitGroup
	path     string
}

func NewSocketListener(path string, mgr ProcessManager) (*SocketListener, error) {
	_ = os.Remove(path)

	l, err := net.Listen("unix", path)
	if err != nil {
		return nil, err
	}

	if err := os.Chmod(path, 0666); err != nil {
		l.Close()
		return nil, err
	}

	sl := &SocketListener{
		listener: l,
		stopChan: make(chan struct{}),
		path:     path,
	}

	sl.wg.Add(1)
	go sl.serve(mgr)

	return sl, nil
}

func (sl *SocketListener) serve(mgr ProcessManager) {
	defer sl.wg.Done()

	for {
		conn, err := sl.listener.Accept()
		if err != nil {
			select {
			case <-sl.stopChan:
				return
			default:
			}
			fmt.Printf("Error accepting connection: %v\n", err)
			continue
		}

		sl.wg.Add(1)
		go sl.handleConn(conn, mgr)
	}
}

func (sl *SocketListener) handleConn(conn net.Conn, mgr ProcessManager) {
	defer sl.wg.Done()
	HandleConnection(conn, mgr)
}

func (sl *SocketListener) Stop() error {
	close(sl.stopChan)
	err := sl.listener.Close()
	sl.wg.Wait()
	return err
}

func (sl *SocketListener) Addr() net.Addr {
	return sl.listener.Addr()
}

func StartSocketListener(path string, mgr ProcessManager) (*SocketListener, error) {
	return NewSocketListener(path, mgr)
}
