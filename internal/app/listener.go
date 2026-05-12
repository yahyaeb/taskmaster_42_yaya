package app

import (
	"fmt"
	"net"
	"os"
	"sync"
)

// SocketListener wraps a Unix socket listener with graceful shutdown support.
type SocketListener struct {
	listener net.Listener
	stopChan chan struct{}
	wg       sync.WaitGroup
	path     string
}

// NewSocketListener creates and starts a Unix socket listener.
// Returns a SocketListener that can be stopped via Stop() method.
func NewSocketListener(path string, m *Manager) (*SocketListener, error) {
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
	go sl.serve(m)

	return sl, nil
}

// serve runs the accept loop until Stop() is called.
func (sl *SocketListener) serve(m *Manager) {
	defer sl.wg.Done()

	for {
		conn, err := sl.listener.Accept()
		if err != nil {
			// Check if listener was closed via Stop()
			select {
			case <-sl.stopChan:
				return
			default:
			}
			// Log error but continue accepting (unless closed)
			fmt.Printf("Error accepting connection: %v\n", err)
			continue
		}

		sl.wg.Add(1)
		go func() {
			defer sl.wg.Done()
			HandleConnection(conn, m)
		}()
	}
}

// Stop gracefully shuts down the socket listener.
// It stops accepting new connections and waits for existing handlers to complete.
func (sl *SocketListener) Stop() error {
	close(sl.stopChan)
	err := sl.listener.Close()
	sl.wg.Wait()
	return err
}

// Addr returns the listener's network address.
func (sl *SocketListener) Addr() net.Addr {
	return sl.listener.Addr()
}

// StartSocketListener creates a socket listener and returns the handle.
// The caller can shut down the listener by calling Stop() on the returned handle.
func StartSocketListener(path string, m *Manager) (*SocketListener, error) {
	return NewSocketListener(path, m)
}
