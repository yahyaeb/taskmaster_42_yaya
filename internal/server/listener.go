package server

import (
	"errors"
	"log/slog"
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

	netLn, err := net.Listen("unix", path)
	if err != nil {
		return nil, err
	}

	if err := os.Chmod(path, 0666); err != nil {
		netLn.Close()
		return nil, err
	}

	l := &SocketListener{
		listener: netLn,
		path:     path,
	}

	l.wg.Add(1)
	go l.serve(mgr)

	return l, nil
}

func (l *SocketListener) serve(mgr ProcessManager) {
	defer l.wg.Done()

	for {
		conn, err := l.listener.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return
			}
			slog.Error("unix socket accept failed", "err", err)
			continue
		}

		l.wg.Add(1)
		go l.handleConn(conn, mgr)
	}
}

func (l *SocketListener) handleConn(conn net.Conn, mgr ProcessManager) {
	defer l.wg.Done()
	HandleConnection(conn, mgr)
}

func (l *SocketListener) Stop() error {
	err := l.listener.Close()
	l.wg.Wait()
	return err
}

func (l *SocketListener) Addr() net.Addr {
	return l.listener.Addr()
}
