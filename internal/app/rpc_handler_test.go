package app

import (
	"errors"
	"strings"
	"testing"

	"taskmaster/internal/protocol"
)

// ============================================================================
// Fake Manager for Testing
// A minimal implementation of ManagerInterface for testing handlers.
// ============================================================================

type fakeManager struct {
	processes      map[string]protocol.ProcessInfo
	startCalled    []string
	stopCalled     []string
	restartCalled  []string
	reloadCalled   bool
	shutdownCalled bool
	errOnStart     map[string]error
	errOnStop      map[string]error
	errOnRestart   map[string]error
	errOnReload    error
	errOnShutdown  error
	panicOnMethod  string // simulate panics for recovery testing
}

func newFakeManager() *fakeManager {
	return &fakeManager{
		processes:    make(map[string]protocol.ProcessInfo),
		errOnStart:   make(map[string]error),
		errOnStop:    make(map[string]error),
		errOnRestart: make(map[string]error),
	}
}

func (f *fakeManager) GetProcessInfo(name string) (protocol.ProcessInfo, error) {
	if f.panicOnMethod == "GetProcessInfo" {
		panic("simulated panic in GetProcessInfo")
	}
	info, ok := f.processes[name]
	if !ok {
		return protocol.ProcessInfo{}, errors.New("process not found")
	}
	return info, nil
}

func (f *fakeManager) GetAllProcessInfo() ([]protocol.ProcessInfo, error) {
	if f.panicOnMethod == "GetAllProcessInfo" {
		panic("simulated panic in GetAllProcessInfo")
	}
	result := make([]protocol.ProcessInfo, 0, len(f.processes))
	for _, info := range f.processes {
		result = append(result, info)
	}
	return result, nil
}

func (f *fakeManager) Start(name string) error {
	if f.panicOnMethod == "Start" {
		panic("simulated panic in Start")
	}
	f.startCalled = append(f.startCalled, name)
	if err, ok := f.errOnStart[name]; ok {
		return err
	}
	return nil
}

func (f *fakeManager) Stop(name string) error {
	if f.panicOnMethod == "Stop" {
		panic("simulated panic in Stop")
	}
	f.stopCalled = append(f.stopCalled, name)
	if err, ok := f.errOnStop[name]; ok {
		return err
	}
	return nil
}

func (f *fakeManager) Restart(name string) error {
	if f.panicOnMethod == "Restart" {
		panic("simulated panic in Restart")
	}
	f.restartCalled = append(f.restartCalled, name)
	if err, ok := f.errOnRestart[name]; ok {
		return err
	}
	return nil
}

func (f *fakeManager) Reload() (*ReloadResult, error) {
	if f.panicOnMethod == "Reload" {
		panic("simulated panic in Reload")
	}
	f.reloadCalled = true
	if f.errOnReload != nil {
		return nil, f.errOnReload
	}
	return &ReloadResult{Added: []string{}, Removed: []string{}, Restarted: []string{}}, nil
}

func (f *fakeManager) Shutdown() error {
	if f.panicOnMethod == "Shutdown" {
		panic("simulated panic in Shutdown")
	}
	f.shutdownCalled = true
	return f.errOnShutdown
}

// ============================================================================
// Tracer Bullet: GetStatus Success Path
// ============================================================================

func TestGetStatus_SingleProcess_ReturnsProcessInfo(t *testing.T) {
	fake := newFakeManager()
	fake.processes["nginx:01"] = protocol.ProcessInfo{
		Name:    "nginx:01",
		Status:  "running",
		Pid:     1234,
		Uptime:  "1h 20m",
		Retries: 0,
	}

	req := protocol.RPCRequest{
		Jsonrpc: "2.0",
		Method:  "Taskmaster.GetStatus",
		Params:  map[string]interface{}{"name": "nginx:01"},
		ID:      1,
	}

	resp := RouteRequest(&req, fake)

	if resp.Error != nil {
		t.Fatalf("expected no error, got code=%d msg=%s", resp.Error.Code, resp.Error.Message)
	}

	result, ok := resp.Result.(protocol.ProcessInfo)
	if !ok {
		resultMap, isMap := resp.Result.(map[string]interface{})
		if !isMap {
			t.Fatalf("expected ProcessInfo result, got %T", resp.Result)
		}
		if resultMap["name"] != "nginx:01" {
			t.Errorf("expected name=nginx:01, got %v", resultMap["name"])
		}
		if resultMap["status"] != "running" {
			t.Errorf("expected status=running, got %v", resultMap["status"])
		}
	} else {
		if result.Name != "nginx:01" {
			t.Errorf("expected name=nginx:01, got %s", result.Name)
		}
		if result.Status != "running" {
			t.Errorf("expected status=running, got %s", result.Status)
		}
		if result.Pid != 1234 {
			t.Errorf("expected pid=1234, got %d", result.Pid)
		}
	}

	if resp.ID != 1 {
		t.Errorf("expected id=1, got %d", resp.ID)
	}
}

// ============================================================================
// GetStatus Error Cases
// ============================================================================

func TestGetStatus_MissingProcess_ReturnsError(t *testing.T) {
	fake := newFakeManager()

	req := protocol.RPCRequest{
		Jsonrpc: "2.0",
		Method:  "Taskmaster.GetStatus",
		Params:  map[string]interface{}{"name": "nonexistent:01"},
		ID:      2,
	}

	resp := RouteRequest(&req, fake)

	if resp.Error == nil {
		t.Fatal("expected error response for missing process, got success")
	}

	if resp.Error.Code != protocol.ProcessNotFound {
		t.Errorf("expected error code %d (ProcessNotFound), got %d", protocol.ProcessNotFound, resp.Error.Code)
	}

	if !strings.Contains(resp.Error.Message, "not found") {
		t.Errorf("expected error message to contain 'not found', got: %s", resp.Error.Message)
	}
}

func TestGetStatus_AllKeyword_ReturnsAllProcesses(t *testing.T) {
	fake := newFakeManager()
	fake.processes["nginx:01"] = protocol.ProcessInfo{Name: "nginx:01", Status: "running", Pid: 1234}
	fake.processes["worker:00"] = protocol.ProcessInfo{Name: "worker:00", Status: "stopped", Pid: 0}

	req := protocol.RPCRequest{
		Jsonrpc: "2.0",
		Method:  "Taskmaster.GetStatus",
		Params:  map[string]interface{}{"name": "all"},
		ID:      3,
	}

	resp := RouteRequest(&req, fake)

	if resp.Error != nil {
		t.Fatalf("expected success, got error: %s", resp.Error.Message)
	}

	result, ok := resp.Result.([]protocol.ProcessInfo)
	if !ok {
		resultSlice, isSlice := resp.Result.([]interface{})
		if !isSlice {
			t.Fatalf("expected []ProcessInfo result, got %T", resp.Result)
		}
		if len(resultSlice) != 2 {
			t.Errorf("expected 2 processes, got %d", len(resultSlice))
		}
	} else {
		if len(result) != 2 {
			t.Errorf("expected 2 processes, got %d", len(result))
		}
	}
}

func TestGetStatus_EmptyName_ReturnsAllProcesses(t *testing.T) {
	fake := newFakeManager()
	fake.processes["nginx:01"] = protocol.ProcessInfo{Name: "nginx:01", Status: "running", Pid: 1234}

	req := protocol.RPCRequest{
		Jsonrpc: "2.0",
		Method:  "Taskmaster.GetStatus",
		Params:  map[string]interface{}{"name": ""},
		ID:      4,
	}

	resp := RouteRequest(&req, fake)

	if resp.Error != nil {
		t.Fatalf("expected success for empty name (all), got error: %s", resp.Error.Message)
	}
}

// ============================================================================
// Start Handler Tests
// ============================================================================

func TestStart_SingleProcess_CallsManagerStart(t *testing.T) {
	fake := newFakeManager()
	fake.processes["nginx:01"] = protocol.ProcessInfo{Name: "nginx:01", Status: "stopped", Pid: 0}

	req := protocol.RPCRequest{
		Jsonrpc: "2.0",
		Method:  "Taskmaster.Start",
		Params:  map[string]interface{}{"name": "nginx:01"},
		ID:      5,
	}

	resp := RouteRequest(&req, fake)

	if resp.Error != nil {
		t.Fatalf("expected success, got error: %s", resp.Error.Message)
	}

	if len(fake.startCalled) != 1 {
		t.Fatalf("expected Start called once, called %d times", len(fake.startCalled))
	}
	if fake.startCalled[0] != "nginx:01" {
		t.Errorf("expected Start called with nginx:01, got %s", fake.startCalled[0])
	}

	result, ok := resp.Result.(protocol.ActionResponse)
	if !ok {
		resultMap, isMap := resp.Result.(map[string]interface{})
		if isMap {
			if success, ok := resultMap["success"].(bool); !ok || !success {
				t.Errorf("expected success=true in response")
			}
		}
	} else if !result.Success {
		t.Errorf("expected Success=true, got false")
	}
}

func TestStart_MissingProcess_ReturnsError(t *testing.T) {
	fake := newFakeManager()

	req := protocol.RPCRequest{
		Jsonrpc: "2.0",
		Method:  "Taskmaster.Start",
		Params:  map[string]interface{}{"name": "nonexistent:01"},
		ID:      6,
	}

	resp := RouteRequest(&req, fake)

	if resp.Error == nil {
		t.Fatal("expected error for missing process, got success")
	}

	if resp.Error.Code != protocol.ProcessNotFound {
		t.Errorf("expected error code %d (ProcessNotFound), got %d", protocol.ProcessNotFound, resp.Error.Code)
	}
}

func TestStart_AllKeyword_ReturnsInvalidParams(t *testing.T) {
	fake := newFakeManager()

	req := protocol.RPCRequest{
		Jsonrpc: "2.0",
		Method:  "Taskmaster.Start",
		Params:  map[string]interface{}{"name": "all"},
		ID:      7,
	}

	resp := RouteRequest(&req, fake)

	if resp.Error == nil {
		t.Fatal("expected error for 'all' keyword with Start, got success")
	}

	if resp.Error.Code != protocol.InvalidParams {
		t.Errorf("expected error code %d (InvalidParams), got %d", protocol.InvalidParams, resp.Error.Code)
	}
}

func TestStart_EmptyName_ReturnsInvalidParams(t *testing.T) {
	fake := newFakeManager()

	req := protocol.RPCRequest{
		Jsonrpc: "2.0",
		Method:  "Taskmaster.Start",
		Params:  map[string]interface{}{"name": ""},
		ID:      8,
	}

	resp := RouteRequest(&req, fake)

	if resp.Error == nil {
		t.Fatal("expected error for empty name, got success")
	}

	if resp.Error.Code != protocol.InvalidParams {
		t.Errorf("expected error code %d (InvalidParams), got %d", protocol.InvalidParams, resp.Error.Code)
	}
}

func TestStart_ManagerReturnsError_ReturnsOperationFailed(t *testing.T) {
	fake := newFakeManager()
	fake.processes["nginx:01"] = protocol.ProcessInfo{Name: "nginx:01", Status: "stopped", Pid: 0}
	fake.errOnStart["nginx:01"] = errors.New("failed to start process")

	req := protocol.RPCRequest{
		Jsonrpc: "2.0",
		Method:  "Taskmaster.Start",
		Params:  map[string]interface{}{"name": "nginx:01"},
		ID:      9,
	}

	resp := RouteRequest(&req, fake)

	if resp.Error == nil {
		t.Fatal("expected error when manager.Start fails, got success")
	}

	if resp.Error.Code != protocol.OperationFailed {
		t.Errorf("expected error code %d (OperationFailed), got %d", protocol.OperationFailed, resp.Error.Code)
	}
}

// ============================================================================
// Stop Handler Tests
// ============================================================================

func TestStop_SingleProcess_CallsManagerStop(t *testing.T) {
	fake := newFakeManager()
	fake.processes["nginx:01"] = protocol.ProcessInfo{Name: "nginx:01", Status: "running", Pid: 1234}

	req := protocol.RPCRequest{
		Jsonrpc: "2.0",
		Method:  "Taskmaster.Stop",
		Params:  map[string]interface{}{"name": "nginx:01"},
		ID:      10,
	}

	resp := RouteRequest(&req, fake)

	if resp.Error != nil {
		t.Fatalf("expected success, got error: %s", resp.Error.Message)
	}

	if len(fake.stopCalled) != 1 || fake.stopCalled[0] != "nginx:01" {
		t.Errorf("expected Stop called once with nginx:01, got %v", fake.stopCalled)
	}
}

func TestStop_MissingProcess_ReturnsProcessNotFound(t *testing.T) {
	fake := newFakeManager()

	req := protocol.RPCRequest{
		Jsonrpc: "2.0",
		Method:  "Taskmaster.Stop",
		Params:  map[string]interface{}{"name": "nonexistent:01"},
		ID:      11,
	}

	resp := RouteRequest(&req, fake)

	if resp.Error == nil {
		t.Fatal("expected error for missing process, got success")
	}

	if resp.Error.Code != protocol.ProcessNotFound {
		t.Errorf("expected error code %d (ProcessNotFound), got %d", protocol.ProcessNotFound, resp.Error.Code)
	}
}

func TestStop_ManagerReturnsError_ReturnsOperationFailed(t *testing.T) {
	fake := newFakeManager()
	fake.processes["nginx:01"] = protocol.ProcessInfo{Name: "nginx:01", Status: "running", Pid: 1234}
	fake.errOnStop["nginx:01"] = errors.New("failed to stop process")

	req := protocol.RPCRequest{
		Jsonrpc: "2.0",
		Method:  "Taskmaster.Stop",
		Params:  map[string]interface{}{"name": "nginx:01"},
		ID:      12,
	}

	resp := RouteRequest(&req, fake)

	if resp.Error == nil {
		t.Fatal("expected error when manager.Stop fails, got success")
	}

	if resp.Error.Code != protocol.OperationFailed {
		t.Errorf("expected error code %d (OperationFailed), got %d", protocol.OperationFailed, resp.Error.Code)
	}
}

// ============================================================================
// Restart Handler Tests
// ============================================================================

func TestRestart_SingleProcess_CallsManagerRestart(t *testing.T) {
	fake := newFakeManager()
	fake.processes["nginx:01"] = protocol.ProcessInfo{Name: "nginx:01", Status: "running", Pid: 1234}

	req := protocol.RPCRequest{
		Jsonrpc: "2.0",
		Method:  "Taskmaster.Restart",
		Params:  map[string]interface{}{"name": "nginx:01"},
		ID:      13,
	}

	resp := RouteRequest(&req, fake)

	if resp.Error != nil {
		t.Fatalf("expected success, got error: %s", resp.Error.Message)
	}

	if len(fake.restartCalled) != 1 || fake.restartCalled[0] != "nginx:01" {
		t.Errorf("expected Restart called once with nginx:01, got %v", fake.restartCalled)
	}
}

func TestRestart_MissingProcess_ReturnsProcessNotFound(t *testing.T) {
	fake := newFakeManager()

	req := protocol.RPCRequest{
		Jsonrpc: "2.0",
		Method:  "Taskmaster.Restart",
		Params:  map[string]interface{}{"name": "nonexistent:01"},
		ID:      14,
	}

	resp := RouteRequest(&req, fake)

	if resp.Error == nil {
		t.Fatal("expected error for missing process, got success")
	}

	if resp.Error.Code != protocol.ProcessNotFound {
		t.Errorf("expected error code %d (ProcessNotFound), got %d", protocol.ProcessNotFound, resp.Error.Code)
	}
}

func TestRestart_ManagerReturnsError_ReturnsOperationFailed(t *testing.T) {
	fake := newFakeManager()
	fake.processes["nginx:01"] = protocol.ProcessInfo{Name: "nginx:01", Status: "running", Pid: 1234}
	fake.errOnRestart["nginx:01"] = errors.New("failed to restart process")

	req := protocol.RPCRequest{
		Jsonrpc: "2.0",
		Method:  "Taskmaster.Restart",
		Params:  map[string]interface{}{"name": "nginx:01"},
		ID:      15,
	}

	resp := RouteRequest(&req, fake)

	if resp.Error == nil {
		t.Fatal("expected error when manager.Restart fails, got success")
	}

	if resp.Error.Code != protocol.OperationFailed {
		t.Errorf("expected error code %d (OperationFailed), got %d", protocol.OperationFailed, resp.Error.Code)
	}
}

// ============================================================================
// Reload Handler Tests
// ============================================================================

func TestReload_GlobalOperation_CallsManagerReload(t *testing.T) {
	fake := newFakeManager()

	req := protocol.RPCRequest{
		Jsonrpc: "2.0",
		Method:  "Taskmaster.Reload",
		Params:  map[string]interface{}{},
		ID:      16,
	}

	resp := RouteRequest(&req, fake)

	if resp.Error != nil {
		t.Fatalf("expected success, got error: %s", resp.Error.Message)
	}

	if !fake.reloadCalled {
		t.Error("expected Reload to be called on manager")
	}
}

func TestReload_WithProcessName_ReturnsInvalidParams(t *testing.T) {
	fake := newFakeManager()

	req := protocol.RPCRequest{
		Jsonrpc: "2.0",
		Method:  "Taskmaster.Reload",
		Params:  map[string]interface{}{"name": "nginx:01"},
		ID:      17,
	}

	resp := RouteRequest(&req, fake)

	if resp.Error == nil {
		t.Fatal("expected error when providing name to Reload, got success")
	}

	if resp.Error.Code != protocol.InvalidParams {
		t.Errorf("expected error code %d (InvalidParams), got %d", protocol.InvalidParams, resp.Error.Code)
	}
}

func TestReload_ManagerReturnsError_ReturnsOperationFailed(t *testing.T) {
	fake := newFakeManager()
	fake.errOnReload = errors.New("failed to reload configuration")

	req := protocol.RPCRequest{
		Jsonrpc: "2.0",
		Method:  "Taskmaster.Reload",
		Params:  map[string]interface{}{},
		ID:      18,
	}

	resp := RouteRequest(&req, fake)

	if resp.Error == nil {
		t.Fatal("expected error when manager.Reload fails, got success")
	}

	if resp.Error.Code != protocol.OperationFailed {
		t.Errorf("expected error code %d (OperationFailed), got %d", protocol.OperationFailed, resp.Error.Code)
	}
}

// ============================================================================
// Shutdown Handler Tests
// ============================================================================

func TestShutdown_GlobalOperation_CallsManagerShutdown(t *testing.T) {
	fake := newFakeManager()

	req := protocol.RPCRequest{
		Jsonrpc: "2.0",
		Method:  "Taskmaster.Shutdown",
		Params:  map[string]interface{}{},
		ID:      19,
	}

	resp := RouteRequest(&req, fake)

	if resp.Error != nil {
		t.Fatalf("expected success, got error: %s", resp.Error.Message)
	}

	if !fake.shutdownCalled {
		t.Error("expected Shutdown to be called on manager")
	}

	result, ok := resp.Result.(protocol.ActionResponse)
	if !ok {
		resultMap, isMap := resp.Result.(map[string]interface{})
		if isMap {
			if msg, ok := resultMap["message"].(string); !ok || msg == "" {
				t.Error("expected non-empty message in shutdown response")
			}
		}
	} else if result.Message == "" {
		t.Error("expected non-empty message in shutdown response")
	}
}

func TestShutdown_WithProcessName_ReturnsInvalidParams(t *testing.T) {
	fake := newFakeManager()

	req := protocol.RPCRequest{
		Jsonrpc: "2.0",
		Method:  "Taskmaster.Shutdown",
		Params:  map[string]interface{}{"name": "nginx:01"},
		ID:      20,
	}

	resp := RouteRequest(&req, fake)

	if resp.Error == nil {
		t.Fatal("expected error when providing name to Shutdown, got success")
	}

	if resp.Error.Code != protocol.InvalidParams {
		t.Errorf("expected error code %d (InvalidParams), got %d", protocol.InvalidParams, resp.Error.Code)
	}
}

// ============================================================================
// Request Validation Tests
// ============================================================================

func TestRoute_EmptyMethod_ReturnsInvalidRequest(t *testing.T) {
	fake := newFakeManager()

	req := protocol.RPCRequest{
		Jsonrpc: "2.0",
		Method:  "",
		Params:  map[string]interface{}{},
		ID:      21,
	}

	resp := RouteRequest(&req, fake)

	if resp.Error == nil {
		t.Fatal("expected error for empty method, got success")
	}

	if resp.Error.Code != protocol.InvalidRequest {
		t.Errorf("expected error code %d (InvalidRequest), got %d", protocol.InvalidRequest, resp.Error.Code)
	}
}

func TestRoute_InvalidMethodPrefix_ReturnsInvalidRequest(t *testing.T) {
	fake := newFakeManager()

	req := protocol.RPCRequest{
		Jsonrpc: "2.0",
		Method:  "InvalidNamespace.Method",
		Params:  map[string]interface{}{},
		ID:      22,
	}

	resp := RouteRequest(&req, fake)

	if resp.Error == nil {
		t.Fatal("expected error for invalid method prefix, got success")
	}

	if resp.Error.Code != protocol.InvalidRequest {
		t.Errorf("expected error code %d (InvalidRequest), got %d", protocol.InvalidRequest, resp.Error.Code)
	}
}

func TestRoute_UnknownMethod_ReturnsMethodNotFound(t *testing.T) {
	fake := newFakeManager()

	req := protocol.RPCRequest{
		Jsonrpc: "2.0",
		Method:  "Taskmaster.UnknownMethod",
		Params:  map[string]interface{}{},
		ID:      23,
	}

	resp := RouteRequest(&req, fake)

	if resp.Error == nil {
		t.Fatal("expected error for unknown method, got success")
	}

	if resp.Error.Code != protocol.MethodNotFound {
		t.Errorf("expected error code %d (MethodNotFound), got %d", protocol.MethodNotFound, resp.Error.Code)
	}
}

func TestRoute_MalformedMethodNoDot_ReturnsInvalidRequest(t *testing.T) {
	fake := newFakeManager()

	req := protocol.RPCRequest{
		Jsonrpc: "2.0",
		Method:  "TaskmasterMethod",
		Params:  map[string]interface{}{},
		ID:      24,
	}

	resp := RouteRequest(&req, fake)

	if resp.Error == nil {
		t.Fatal("expected error for malformed method (no dot), got success")
	}

	if resp.Error.Code != protocol.InvalidRequest {
		t.Errorf("expected error code %d (InvalidRequest), got %d", protocol.InvalidRequest, resp.Error.Code)
	}
}

// ============================================================================
// Table-Driven Tests: Method Routing
// ============================================================================

func TestRoute_KnownMethods_Accepted(t *testing.T) {
	tests := []struct {
		name   string
		method string
	}{
		{"GetStatus", "Taskmaster.GetStatus"},
		{"Start", "Taskmaster.Start"},
		{"Stop", "Taskmaster.Stop"},
		{"Restart", "Taskmaster.Restart"},
		{"Reload", "Taskmaster.Reload"},
		{"Shutdown", "Taskmaster.Shutdown"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := newFakeManager()
			fake.processes["test:01"] = protocol.ProcessInfo{Name: "test:01", Status: "running", Pid: 1234}

			var params map[string]interface{}
			if tt.method == "Taskmaster.GetStatus" || tt.method == "Taskmaster.Start" ||
				tt.method == "Taskmaster.Stop" || tt.method == "Taskmaster.Restart" {
				params = map[string]interface{}{"name": "test:01"}
			} else {
				params = map[string]interface{}{}
			}

			req := protocol.RPCRequest{
				Jsonrpc: "2.0",
				Method:  tt.method,
				Params:  params,
				ID:      100,
			}

			resp := RouteRequest(&req, fake)

			// These should NOT return MethodNotFound error
			if resp.Error != nil && resp.Error.Code == protocol.MethodNotFound {
				t.Errorf("method %s should be recognized, got MethodNotFound", tt.method)
			}
		})
	}
}

// ============================================================================
// Panic Recovery Tests
// ============================================================================

func TestRoute_HandlerPanic_RecoversAndReturnsError(t *testing.T) {
	fake := newFakeManager()
	fake.panicOnMethod = "GetProcessInfo"
	fake.processes["nginx:01"] = protocol.ProcessInfo{Name: "nginx:01", Status: "running", Pid: 1234}

	req := protocol.RPCRequest{
		Jsonrpc: "2.0",
		Method:  "Taskmaster.GetStatus",
		Params:  map[string]interface{}{"name": "nginx:01"},
		ID:      25,
	}

	// This should not panic; handler should recover
	resp := RouteRequest(&req, fake)

	// After recovery, should return internal error
	if resp.Error == nil {
		t.Fatal("expected error after panic recovery, got success")
	}

	if resp.Error.Code != protocol.InternalError {
		t.Errorf("expected error code %d (InternalError), got %d", protocol.InternalError, resp.Error.Code)
	}
}

func TestRoute_StartPanic_RecoversAndReturnsError(t *testing.T) {
	fake := newFakeManager()
	fake.panicOnMethod = "Start"
	fake.processes["nginx:01"] = protocol.ProcessInfo{Name: "nginx:01", Status: "stopped", Pid: 0}

	req := protocol.RPCRequest{
		Jsonrpc: "2.0",
		Method:  "Taskmaster.Start",
		Params:  map[string]interface{}{"name": "nginx:01"},
		ID:      26,
	}

	resp := RouteRequest(&req, fake)

	if resp.Error == nil {
		t.Fatal("expected error after panic recovery, got success")
	}

	if resp.Error.Code != protocol.InternalError {
		t.Errorf("expected error code %d (InternalError), got %d", protocol.InternalError, resp.Error.Code)
	}
}
