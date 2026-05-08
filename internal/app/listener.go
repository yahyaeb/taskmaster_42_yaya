package app

import (
	"fmt"
	"net"
	"os"
)

func StartSocketListener(path string, m *Manager) error {

	_ = os.Remove(path)

	l, err := net.Listen("unix", path)
	if err != nil {
		return err
	}

	os.Chmod(path, 0666)

	go func() {
		for {
			conn, err := l.Accept()

			if err != nil {
				fmt.Printf("Error accepting connection: %v\n", err)
				continue
			}

			go handleRequest(conn, m)
		}
	}()
	return nil
}
