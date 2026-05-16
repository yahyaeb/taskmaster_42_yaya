package server

import (
	"fmt"
	"net"
	"os"
	"sync"
)

type SocketListener struct {
	listener net.Listener
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
