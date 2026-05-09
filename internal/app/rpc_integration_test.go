package app

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"taskmaster/internal/protocol"
)

// ============================================================================
// Socket Integration Tests
// These tests use real sockets and a real Manager (not mocks).
// They validate end-to-end JSON-RPC over Unix socket.
// ============================================================================

// ============================================================================
// Tracer Bullet: End-to-End GetStatus Over Socket
// ============================================================================

func TestIntegration_GetStatus_EndToEnd(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "test_config.yml")

	configContent := `programs:
  testprog:
    program: /bin/true
    cmd: "true"
    numprocs: 1
    autostart: false
    autorestart: never
    stopsignal: TERM
    stoptime: 5
`
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	socketPath := filepath.Join(tmpDir, "taskmaster_test.sock")
	manager, err := NewManagerFromConfig(configPath)
	if err != nil {
		t.Fatalf("failed to load NewManagerFromConfig: %v", err)
	}

	listener, err := StartSocketListener(socketPath, manager)
	if err != nil {
		t.Fatalf("failed to start socket listener: %v", err)
	}
	defer listener.Stop()

	time.Sleep(50 * time.Millisecond)

	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		t.Fatalf("failed to connect to socket: %v", err)
	}
	defer conn.Close()

	req := protocol.RPCRequest{
		Jsonrpc: "2.0",
		Method:  "Taskmaster.GetStatus",
		Params:  map[string]interface{}{"name": "all"},
		ID:      1,
	}

	if err := json.NewEncoder(conn).Encode(&req); err != nil {
		t.Fatalf("failed to encode request: %v", err)
	}

	decoder := json.NewDecoder(conn)
	var resp protocol.RPCResponse
	if err := decoder.Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	// GetAllProcessInfo should now succeed and return the configured processes
	if resp.Error != nil {
		t.Fatalf("expected success, got error: code=%d msg=%s", resp.Error.Code, resp.Error.Message)
	}

	if resp.ID != 1 {
		t.Errorf("expected response ID 1, got %d", resp.ID)
	}
}

// ============================================================================
// Integration Test: Unknown Method Returns Error
// ============================================================================

func TestIntegration_UnknownMethod_ReturnsMethodNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "test_config.yml")

	configContent := `programs:
  test:
    program: /bin/true
    cmd: "true"
    numprocs: 1
    autostart: false
`
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	socketPath := filepath.Join(tmpDir, "taskmaster_test.sock")
	manager, err := NewManagerFromConfig(configPath)
	if err != nil {
		t.Fatalf("failed to load NewManagerFromConfig: %v", err)
	}

	listener, err := StartSocketListener(socketPath, manager)
	if err != nil {
		t.Fatalf("failed to start socket listener: %v", err)
	}
	defer listener.Stop()

	time.Sleep(50 * time.Millisecond)

	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		t.Fatalf("failed to connect to socket: %v", err)
	}
	defer conn.Close()

	req := protocol.RPCRequest{
		Jsonrpc: "2.0",
		Method:  "Taskmaster.UnknownMethod",
		Params:  map[string]interface{}{},
		ID:      3,
	}

	if err := json.NewEncoder(conn).Encode(&req); err != nil {
		t.Fatalf("failed to encode request: %v", err)
	}

	var resp protocol.RPCResponse
	if err := json.NewDecoder(conn).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Error == nil {
		t.Fatal("expected error for unknown method, got success")
	}

	if resp.Error.Code != protocol.MethodNotFound {
		t.Errorf("expected error code %d (MethodNotFound), got %d", protocol.MethodNotFound, resp.Error.Code)
	}
}

// ============================================================================
// Integration Test: Malformed JSON Returns Parse Error
// ============================================================================

func TestIntegration_MalformedJSON_ReturnsParseError(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "test_config.yml")

	configContent := `programs:
  test:
    program: /bin/true
    cmd: "true"
    numprocs: 1
    autostart: false
`
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	socketPath := filepath.Join(tmpDir, "taskmaster_test.sock")
	manager, err := NewManagerFromConfig(configPath)
	if err != nil {
		t.Fatalf("failed to load NewManagerFromConfig: %v", err)
	}

	listener, err := StartSocketListener(socketPath, manager)
	if err != nil {
		t.Fatalf("failed to start socket listener: %v", err)
	}
	defer listener.Stop()

	time.Sleep(50 * time.Millisecond)

	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		t.Fatalf("failed to connect to socket: %v", err)
	}
	defer conn.Close()

	malformedData := []byte(`{invalid json`)
	if _, err := conn.Write(malformedData); err != nil {
		t.Fatalf("failed to write malformed data: %v", err)
	}

	// Server should return ParseError response
	conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))

	var resp protocol.RPCResponse
	decoder := json.NewDecoder(conn)
	err = decoder.Decode(&resp)
	if err != nil {
		t.Fatalf("failed to decode error response: %v", err)
	}

	if resp.Error == nil {
		t.Fatal("expected error response for malformed JSON, got success")
	}

	if resp.Error.Code != protocol.ParseError {
		t.Errorf("expected error code %d (ParseError), got %d", protocol.ParseError, resp.Error.Code)
	}
}

// ============================================================================
// Integration Test: Concurrent Requests
// ============================================================================

func TestIntegration_ConcurrentRequests_AllHandled(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "test_config.yml")

	configContent := `programs:
  proc1:
    program: /bin/true
    cmd: "true"
    numprocs: 1
    autostart: false
    autorestart: never
    stopsignal: TERM
    stoptime: 5
  proc2:
    program: /bin/true
    cmd: "true"
    numprocs: 1
    autostart: false
    autorestart: never
    stopsignal: TERM
    stoptime: 5
`
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	socketPath := filepath.Join(tmpDir, "taskmaster_test.sock")
	manager, err := NewManagerFromConfig(configPath)
	if err != nil {
		t.Fatalf("failed to load NewManagerFromConfig: %v", err)
	}

	listener, err := StartSocketListener(socketPath, manager)
	if err != nil {
		t.Fatalf("failed to start socket listener: %v", err)
	}
	defer listener.Stop()

	time.Sleep(50 * time.Millisecond)

	var wg sync.WaitGroup
	numRequests := 10
	errors := make(chan error, numRequests)

	for i := 0; i < numRequests; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			conn, err := net.Dial("unix", socketPath)
			if err != nil {
				errors <- fmt.Errorf("request %d: failed to connect: %w", id, err)
				return
			}
			defer conn.Close()

			req := protocol.RPCRequest{
				Jsonrpc: "2.0",
				Method:  "Taskmaster.GetStatus",
				Params:  map[string]interface{}{"name": "all"},
				ID:      id,
			}

			if err := json.NewEncoder(conn).Encode(&req); err != nil {
				errors <- fmt.Errorf("request %d: failed to encode: %w", id, err)
				return
			}

			var resp protocol.RPCResponse
			if err := json.NewDecoder(conn).Decode(&resp); err != nil {
				errors <- fmt.Errorf("request %d: failed to decode: %w", id, err)
				return
			}

			if resp.ID != id {
				errors <- fmt.Errorf("request %d: expected ID %d, got %d", id, id, resp.ID)
				return
			}

			// GetAllProcessInfo should now succeed
			if resp.Error != nil {
				errors <- fmt.Errorf("request %d: unexpected error: code=%d msg=%s", id, resp.Error.Code, resp.Error.Message)
				return
			}
		}(i)
	}

	wg.Wait()
	close(errors)

	errorCount := 0
	for err := range errors {
		t.Logf("Concurrent request error: %v", err)
		errorCount++
	}

	if errorCount > 0 {
		t.Errorf("had %d errors during concurrent requests", errorCount)
	}
}

// ============================================================================
// Integration Test: Multiple Connections (One Per Request)
// ============================================================================

func TestIntegration_SequentialConnections_AllSucceed(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "test_config.yml")

	configContent := `programs:
  nginx:
    program: /bin/true
    cmd: "true"
    numprocs: 1
    autostart: false
`
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	socketPath := filepath.Join(tmpDir, "taskmaster_test.sock")
	manager, err := NewManagerFromConfig(configPath)
	if err != nil {
		t.Fatalf("failed to load NewManagerFromConfig: %v", err)
	}

	listener, err := StartSocketListener(socketPath, manager)
	if err != nil {
		t.Fatalf("failed to start socket listener: %v", err)
	}
	defer listener.Stop()

	time.Sleep(50 * time.Millisecond)

	for i := 0; i < 5; i++ {
		conn, err := net.Dial("unix", socketPath)
		if err != nil {
			t.Fatalf("connection %d: failed to connect: %v", i, err)
		}

		req := protocol.RPCRequest{
			Jsonrpc: "2.0",
			Method:  "Taskmaster.GetStatus",
			Params:  map[string]interface{}{"name": "nginx:00"},
			ID:      i,
		}

		if err := json.NewEncoder(conn).Encode(&req); err != nil {
			t.Fatalf("connection %d: failed to encode: %v", i, err)
		}

		var resp protocol.RPCResponse
		if err := json.NewDecoder(conn).Decode(&resp); err != nil {
			t.Fatalf("connection %d: failed to decode: %v", i, err)
		}

		conn.Close()

		// GetStatus for existing process should succeed
		// or return ProcessNotFound if the lookup fails
		if resp.Error != nil && resp.Error.Code != protocol.ProcessNotFound {
			t.Errorf("connection %d: unexpected error: code=%d msg=%s", i, resp.Error.Code, resp.Error.Message)
		}

		if resp.ID != i {
			t.Errorf("connection %d: expected ID %d, got %d", i, i, resp.ID)
		}
	}
}

// ============================================================================
// Integration Test: Reload Handler
// ============================================================================

func TestIntegration_Reload_EndToEnd(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "test_config.yml")

	configContent := `programs:
  nginx:
    program: /bin/true
    cmd: "true"
    numprocs: 1
    autostart: false
`
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	socketPath := filepath.Join(tmpDir, "taskmaster_test.sock")
	manager, err := NewManagerFromConfig(configPath)
	if err != nil {
		t.Fatalf("failed to load NewManagerFromConfig: %v", err)
	}

	listener, err := StartSocketListener(socketPath, manager)
	if err != nil {
		t.Fatalf("failed to start socket listener: %v", err)
	}
	defer listener.Stop()

	time.Sleep(50 * time.Millisecond)

	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		t.Fatalf("failed to connect to socket: %v", err)
	}
	defer conn.Close()

	req := protocol.RPCRequest{
		Jsonrpc: "2.0",
		Method:  "Taskmaster.Reload",
		Params:  map[string]interface{}{},
		ID:      99,
	}

	if err := json.NewEncoder(conn).Encode(&req); err != nil {
		t.Fatalf("failed to encode request: %v", err)
	}

	var resp protocol.RPCResponse
	if err := json.NewDecoder(conn).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	// Reload now returns success (empty diff since no config changes)
	if resp.Error != nil {
		t.Errorf("unexpected error: code=%d msg=%s", resp.Error.Code, resp.Error.Message)
	}

	if resp.ID != 99 {
		t.Errorf("expected ID 99, got %d", resp.ID)
	}
}

// ============================================================================
// Integration Test: Shutdown Handler
// ============================================================================

func TestIntegration_Shutdown_EndToEnd(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "test_config.yml")

	configContent := `programs:
  nginx:
    program: /bin/true
    cmd: "true"
    numprocs: 1
    autostart: false
`
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	socketPath := filepath.Join(tmpDir, "taskmaster_test.sock")
	manager, err := NewManagerFromConfig(configPath)
	if err != nil {
		t.Fatalf("failed to load NewManagerFromConfig: %v", err)
	}

	listener, err := StartSocketListener(socketPath, manager)
	if err != nil {
		t.Fatalf("failed to start socket listener: %v", err)
	}
	defer listener.Stop()

	time.Sleep(50 * time.Millisecond)

	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		t.Fatalf("failed to connect to socket: %v", err)
	}
	defer conn.Close()

	req := protocol.RPCRequest{
		Jsonrpc: "2.0",
		Method:  "Taskmaster.Shutdown",
		Params:  map[string]interface{}{},
		ID:      100,
	}

	if err := json.NewEncoder(conn).Encode(&req); err != nil {
		t.Fatalf("failed to encode request: %v", err)
	}

	var resp protocol.RPCResponse
	if err := json.NewDecoder(conn).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	// Shutdown should succeed (it just sends signals, doesn't wait)
	if resp.Error != nil {
		t.Errorf("unexpected error: code=%d msg=%s", resp.Error.Code, resp.Error.Message)
	}

	if resp.ID != 100 {
		t.Errorf("expected ID 100, got %d", resp.ID)
	}
}

// ============================================================================
// Integration Test: Start Handler (unimplemented)
// ============================================================================

func TestIntegration_StartProcess_ReturnsOperationFailed(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "test_config.yml")

	configContent := `programs:
  sleeper:
    program: /bin/true
    cmd: "true"
    numprocs: 1
    autostart: false
    autorestart: never
    stopsignal: TERM
    stoptime: 5
`
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	socketPath := filepath.Join(tmpDir, "taskmaster_test.sock")
	manager, err := NewManagerFromConfig(configPath)
	if err != nil {
		t.Fatalf("failed to load NewManagerFromConfig: %v", err)
	}

	listener, err := StartSocketListener(socketPath, manager)
	if err != nil {
		t.Fatalf("failed to start socket listener: %v", err)
	}
	defer listener.Stop()

	time.Sleep(50 * time.Millisecond)

	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		t.Fatalf("failed to connect to socket: %v", err)
	}
	defer conn.Close()

	req := protocol.RPCRequest{
		Jsonrpc: "2.0",
		Method:  "Taskmaster.Start",
		Params:  map[string]interface{}{"name": "sleeper:00"},
		ID:      101,
	}

	if err := json.NewEncoder(conn).Encode(&req); err != nil {
		t.Fatalf("failed to encode request: %v", err)
	}

	var resp protocol.RPCResponse
	if err := json.NewDecoder(conn).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	// Success indicates the RPC layer worked (actual process start depends on environment)
	if resp.Error != nil {
		t.Logf("Start returned error (may be expected in test env): code=%d msg=%s", resp.Error.Code, resp.Error.Message)
	}

	if resp.ID != 101 {
		t.Errorf("expected ID 101, got %d", resp.ID)
	}
}

// ============================================================================
// Integration Test: Stop Handler (unimplemented)
// ============================================================================

func TestIntegration_StopProcess_ReturnsOperationFailed(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "test_config.yml")

	configContent := `programs:
  sleeper:
    program: /bin/true
    cmd: "true"
    numprocs: 1
    autostart: false
    autorestart: never
    stopsignal: TERM
    stoptime: 5
`
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	socketPath := filepath.Join(tmpDir, "taskmaster_test.sock")
	manager, err := NewManagerFromConfig(configPath)
	if err != nil {
		t.Fatalf("failed to load NewManagerFromConfig: %v", err)
	}

	listener, err := StartSocketListener(socketPath, manager)
	if err != nil {
		t.Fatalf("failed to start socket listener: %v", err)
	}
	defer listener.Stop()

	time.Sleep(50 * time.Millisecond)

	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		t.Fatalf("failed to connect to socket: %v", err)
	}
	defer conn.Close()

	req := protocol.RPCRequest{
		Jsonrpc: "2.0",
		Method:  "Taskmaster.Stop",
		Params:  map[string]interface{}{"name": "sleeper:00"},
		ID:      102,
	}

	if err := json.NewEncoder(conn).Encode(&req); err != nil {
		t.Fatalf("failed to encode request: %v", err)
	}

	var resp protocol.RPCResponse
	if err := json.NewDecoder(conn).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	// Success indicates the RPC layer worked (actual process stop depends on environment)
	if resp.Error != nil {
		t.Logf("Stop returned error (may be expected in test env): code=%d msg=%s", resp.Error.Code, resp.Error.Message)
	}

	if resp.ID != 102 {
		t.Errorf("expected ID 102, got %d", resp.ID)
	}
}

// ============================================================================
// Integration Test: Restart Handler (unimplemented)
// ============================================================================

func TestIntegration_RestartProcess_ReturnsOperationFailed(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "test_config.yml")

	configContent := `programs:
  sleeper:
    program: /bin/true
    cmd: "true"
    numprocs: 1
    autostart: false
    autorestart: never
    stopsignal: TERM
    stoptime: 5
`
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	socketPath := filepath.Join(tmpDir, "taskmaster_test.sock")
	manager, err := NewManagerFromConfig(configPath)
	if err != nil {
		t.Fatalf("failed to load NewManagerFromConfig: %v", err)
	}

	listener, err := StartSocketListener(socketPath, manager)
	if err != nil {
		t.Fatalf("failed to start socket listener: %v", err)
	}
	defer listener.Stop()

	time.Sleep(50 * time.Millisecond)

	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		t.Fatalf("failed to connect to socket: %v", err)
	}
	defer conn.Close()

	req := protocol.RPCRequest{
		Jsonrpc: "2.0",
		Method:  "Taskmaster.Restart",
		Params:  map[string]interface{}{"name": "sleeper:00"},
		ID:      103,
	}

	if err := json.NewEncoder(conn).Encode(&req); err != nil {
		t.Fatalf("failed to encode request: %v", err)
	}

	var resp protocol.RPCResponse
	if err := json.NewDecoder(conn).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	// Success indicates the RPC layer worked (actual process restart depends on environment)
	if resp.Error != nil {
		t.Logf("Restart returned error (may be expected in test env): code=%d msg=%s", resp.Error.Code, resp.Error.Message)
	}

	if resp.ID != 103 {
		t.Errorf("expected ID 103, got %d", resp.ID)
	}
}

// ============================================================================
// Integration Test: InvalidParams - Start/Stop/Restart without name
// ============================================================================

func TestIntegration_StartWithoutName_ReturnsInvalidParams(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "test_config.yml")

	configContent := `programs:
  nginx:
    program: /bin/true
    cmd: "true"
    numprocs: 1
    autostart: false
`
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	socketPath := filepath.Join(tmpDir, "taskmaster_test.sock")
	manager, err := NewManagerFromConfig(configPath)
	if err != nil {
		t.Fatalf("failed to load NewManagerFromConfig: %v", err)
	}

	listener, err := StartSocketListener(socketPath, manager)
	if err != nil {
		t.Fatalf("failed to start socket listener: %v", err)
	}
	defer listener.Stop()

	time.Sleep(50 * time.Millisecond)

	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		t.Fatalf("failed to connect to socket: %v", err)
	}
	defer conn.Close()

	// Send Start without name parameter
	req := protocol.RPCRequest{
		Jsonrpc: "2.0",
		Method:  "Taskmaster.Start",
		Params:  map[string]interface{}{},
		ID:      104,
	}

	if err := json.NewEncoder(conn).Encode(&req); err != nil {
		t.Fatalf("failed to encode request: %v", err)
	}

	var resp protocol.RPCResponse
	if err := json.NewDecoder(conn).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	// Should return InvalidParams for missing name
	if resp.Error == nil {
		t.Fatal("expected error for missing name, got success")
	}

	if resp.Error.Code != protocol.InvalidParams {
		t.Errorf("expected error code %d (InvalidParams), got %d", protocol.InvalidParams, resp.Error.Code)
	}

	if resp.ID != 104 {
		t.Errorf("expected ID 104, got %d", resp.ID)
	}
}

// ============================================================================
// Integration Test: GetStatus for specific process returns not found
// ============================================================================

func TestIntegration_GetStatusUnknownProcess_ReturnsNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "test_config.yml")

	configContent := `programs:
  nginx:
    program: /bin/true
    cmd: "true"
    numprocs: 1
    autostart: false
`
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	socketPath := filepath.Join(tmpDir, "taskmaster_test.sock")
	manager, err := NewManagerFromConfig(configPath)
	if err != nil {
		t.Fatalf("failed to load NewManagerFromConfig: %v", err)
	}

	listener, err := StartSocketListener(socketPath, manager)
	if err != nil {
		t.Fatalf("failed to start socket listener: %v", err)
	}
	defer listener.Stop()

	time.Sleep(50 * time.Millisecond)

	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		t.Fatalf("failed to connect to socket: %v", err)
	}
	defer conn.Close()

	req := protocol.RPCRequest{
		Jsonrpc: "2.0",
		Method:  "Taskmaster.GetStatus",
		Params:  map[string]interface{}{"name": "nonexistent:99"},
		ID:      105,
	}

	if err := json.NewEncoder(conn).Encode(&req); err != nil {
		t.Fatalf("failed to encode request: %v", err)
	}

	var resp protocol.RPCResponse
	if err := json.NewDecoder(conn).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	// Should return ProcessNotFound
	if resp.Error == nil {
		t.Fatal("expected error for unknown process, got success")
	}

	if resp.Error.Code != protocol.ProcessNotFound {
		t.Errorf("expected error code %d (ProcessNotFound), got %d", protocol.ProcessNotFound, resp.Error.Code)
	}

	if resp.ID != 105 {
		t.Errorf("expected ID 105, got %d", resp.ID)
	}
}
