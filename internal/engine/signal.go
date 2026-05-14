package engine

import (
	"fmt"
	"os"
	"syscall"
)

type SignalHandler interface {
	Send(p *Process, sig os.Signal) error
}

type OSSignalHandler struct{}

func (h *OSSignalHandler) Send(p *Process, sig os.Signal) error {
	proc, err := os.FindProcess(p.PID)
	if err != nil {
		return fmt.Errorf("find process %d: %w", p.PID, err)
	}
	return proc.Signal(sig)
}

func SignalFromString(sig string) (os.Signal, error) {
	switch sig {
	case "TERM":
		return syscall.SIGTERM, nil
	case "HUP":
		return syscall.SIGHUP, nil
	case "INT":
		return os.Interrupt, nil
	case "QUIT":
		return syscall.SIGQUIT, nil
	case "KILL":
		return os.Kill, nil
	case "USR1":
		return syscall.SIGUSR1, nil
	case "USR2":
		return syscall.SIGUSR2, nil
	default:
		return nil, fmt.Errorf("unsupported signal: %s", sig)
	}
}

func StopProcess(handler SignalHandler, p *Process, sig os.Signal) error {
	if err := handler.Send(p, sig); err != nil {
		return fmt.Errorf("failed to send signal: %w", err)
	}
	return nil
}
