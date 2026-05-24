package internal

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"os"
)

type RPCRequest struct {
	ID     int             `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params"`
}

type RPCResponse struct {
	ID     int    `json:"id"`
	Result any    `json:"result,omitempty"`
	Error  string `json:"error,omitempty"`
}

type Server struct {
	listener   net.Listener
	mgr        *Manager
	socketPath string
	configPath string
	exitRoot   context.CancelFunc
}

func NewServer(socketPath string, mgr *Manager, configPath string, exitRoot context.CancelFunc) (*Server, error) {
	_ = os.Remove(socketPath)
	l, err := net.Listen("unix", socketPath)
	if err != nil {
		return nil, err
	}

	return &Server{listener: l, mgr: mgr, socketPath: socketPath, configPath: configPath, exitRoot: exitRoot}, nil
}

func (s *Server) Serve() error {
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return nil
			}
			return err
		}
		go s.handle(conn)
	}
}

func (s *Server) Stop() error {
	_ = s.listener.Close()
	return os.Remove(s.socketPath)
}

func (s *Server) handle(conn net.Conn) {
	defer conn.Close()

	var req RPCRequest
	if err := json.NewDecoder(conn).Decode(&req); err != nil {
		_ = json.NewEncoder(conn).Encode(RPCResponse{Error: err.Error()})
		return
	}

	resp, post := s.dispatch(req)

	_ = json.NewEncoder(conn).Encode(&resp)

	if post != nil {
		post()
	}

}

func (s *Server) dispatch(req RPCRequest) (RPCResponse, func()) {
	var resp RPCResponse
	resp.ID = req.ID

	switch req.Method {
	case "status":
		resp.Result = s.mgr.Status()

	case "start", "stop", "restart":
		var p struct {
			Name string `json:"name"`
		}
		if err := json.Unmarshal(req.Params, &p); err != nil {
			resp.Error = err.Error()
			break
		}
		var err error
		switch req.Method {
		case "start":
			err = s.mgr.Start(p.Name)
		case "stop":
			err = s.mgr.Stop(p.Name)
		case "restart":
			err = s.mgr.Restart(p.Name)
		}
		if err != nil {
			resp.Error = err.Error()
		} else {
			resp.Result = "ok"
		}

	case "reload":
		cfg, err := LoadConfig(s.configPath)
		if err != nil {
			resp.Error = err.Error()
			break
		}
		if err := s.mgr.Reload(cfg); err != nil {
			resp.Error = err.Error()
		} else {
			resp.Result = "ok"
		}

	case "shutdown":
		resp.Result = "ok"
		return resp, func() {
			if s.exitRoot != nil {
				s.exitRoot()
			}
		}

	default:
		resp.Error = "unknown method: " + req.Method
		return resp, nil
	}
	return resp, nil
}
