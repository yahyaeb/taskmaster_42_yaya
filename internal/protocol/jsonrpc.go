/*
  // Request envelope (JSON-RPC 2.0)
  { "jsonrpc": "2.0", "method": "Taskmaster.Stop", "params": { "name": "nginx:01" }, "id": 1 }

  // Methods: Taskmaster.GetStatus · Taskmaster.Start · Taskmaster.Stop
  //          Taskmaster.Restart · Taskmaster.Reload · Taskmaster.Shutdown

  // "name" = "all" or "" to target everything
  Developer B (CTL) encodes ActionRequest{Name} and decodes ActionResponse or []ProcessInfo.

{
  "jsonrpc": "2.0",
  "method": "Namespace.MethodName",
  "params": {},
  "id": 1
}

 Example JSON-RPC Response:
{
  "jsonrpc": "2.0",
  "result": {},
  "error": null,
  "id": 1
}

 Example Result for "Taskmaster.GetStatus":

[
  {
    "name": "nginx:01",
    "status": "running",
    "pid": 1234,
    "uptime": "1h 20m",
    "retries": 0
  },
  {
    "name": "vogsphere:01",
    "status": "fatal",
    "pid": 0,
    "uptime": "0s",
    "retries": 3
  }
]

Example Params for "Taskmaster.Start, Taskmaster.Stop, Taskmaster.Restart":

{ "name": "nginx:01" }
// Use "all" or an empty string to target everything

Example Result for "Taskmaster.Reload":

{ "added": ["new_worker:01"], "removed": ["old_job:01"], "restarted": [] }

Example Result for "Taskmaster.Shutdown":

{"message": "Daemon shutting down..."}

*/

package protocol

// RPCRequest represents the standard JSON-RPC 2.0 request envelope
type RPCRequest struct {
	Jsonrpc string      `json:"jsonrpc"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params,omitempty"`
	ID      int         `json:"id"`
}

// RPCResponse represents the standard JSON-RPC 2.0 response envelope
type RPCResponse struct {
	Jsonrpc string      `json:"jsonrpc"`
	Result  interface{} `json:"result,omitempty"`
	Error   *RPCError   `json:"error,omitempty"`
	ID      int         `json:"id"`
}

type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// --- Specific Results ---

type ProcessInfo struct {
	Name    string `json:"name"`   // e.g., "nginx:01"
	Status  string `json:"status"` // From internal/bus: STARTING, RUNNING, etc.
	Pid     int    `json:"pid"`
	Uptime  string `json:"uptime"`
	Retries int    `json:"retries"`
}

type ActionRequest struct {
	Name string `json:"name"` // Program name or "all"
}

type ActionResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

type ReloadResponse struct {
	Added     []string `json:"added"`
	Removed   []string `json:"removed"`
	Restarted []string `json:"restarted"`
}

func NewErrorRsponse(code int, message string) *RPCResponse {
	return &RPCResponse{
		Jsonrpc: "2.0",
		Error: &RPCError{
			Code:    code,
			Message: message,
		},
		ID: 1,
	}
}

func NewSuccessResponse(data interface{}) *RPCResponse {
	return &RPCResponse{
		Jsonrpc: "2.0",
		Result:  data,
		Error:   nil,
		ID:      1,
	}
}
