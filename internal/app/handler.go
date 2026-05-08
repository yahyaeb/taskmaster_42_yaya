package app

import (
	"encoding/json"
	"net"
	"taskmaster/internal/protocol"
)

func handleRequest(conn net.Conn, m *Manager) {
	defer conn.Close()

	var req protocol.RPCRequest

	if err := json.NewDecoder(conn).Decode(&req); err != nil {
		return
	}

	var res interface{}

	switch req.Method {
	case "Taskmaster.status":
		res = protocol.NewSuccessResponse([]protocol.ProcessInfo{{Name: "test-task", Status: "running", Pid: 1234}}, req.ID)
	default:
		res = protocol.NewErrorRsponse(-32601, "method not found")
	}

	_ = json.NewEncoder(conn).Encode(res)
}
