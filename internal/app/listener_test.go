package app

import (
	"encoding/json"
	"net"
	"os"
	"testing"
	"taskmaster/internal/protocol"
)

func TestDeamonCommunication(t *testing.T) {
	socketPath := "/tmp/test_taskmaster.sock"
	defer os.Remove(socketPath)

	m := NewManager()
	listener, err := StartSocketListener(socketPath, m)
	if err != nil {
		t.Fatalf("Failed to start listener: %v", err)
	}
	defer listener.Stop()

	conn, err := net.Dial("unix", socketPath)

	if err != nil {
		t.Fatalf("Failed to connect to socket: %v", err)
	}

	defer conn.Close()

	req := protocol.RPCRequest{
		Jsonrpc: "2.0",
		Method:  "Taskmaster.GetStatus",
		Params:  map[string]interface{}{"name": "all"},
		ID:      1,
	}

	if err := json.NewEncoder(conn).Encode(req); err != nil {
		t.Fatalf("Failed to encode request: %v", err)
	}

	var res protocol.RPCResponse
	if err := json.NewDecoder(conn).Decode(&res); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if res.Error != nil {
		t.Errorf("Expected success, got error: code=%d msg=%s", res.Error.Code, res.Error.Message)
	}

	if res.ID != 1 {
		t.Errorf("Expected response ID 1, got %v", res.ID)
	}
}
