package app

import (
	"net"
	"os"
)

func StartListener(path string, m *Manager) error {
	_ = os.Remove(path)
	l, err := net.Listen("unix", path)
	if err != nil {
		return err
	}

	os.Chmod(path, 0660)

	go func() {
		for {
			conn, _ := l.Accept()
			go handleRequest(conn, m)
		}
	}()
	return nil
}
