package main

import (
	"bufio"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"taskmaster/internal/app"
	"taskmaster/internal/protocol"
)

func read(inputChan chan string) {
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		inputChan <- scanner.Text()
	}
}

// ManagerReference holds the current manager and allows atomic updates during reload.
// All operations are forwarded to the current manager.
type ManagerReference struct {
	mu *sync.RWMutex
	m  *app.Manager
}

// NewManagerReference creates a new ManagerReference wrapper.
func NewManagerReference(m *app.Manager) *ManagerReference {
	return &ManagerReference{
		mu: &sync.RWMutex{},
		m:  m,
	}
}

// GetManager returns the current manager (read-only).
func (mr *ManagerReference) GetManager() *app.Manager {
	mr.mu.RLock()
	defer mr.mu.RUnlock()
	return mr.m
}

// SetManager swaps to a new manager. Both old and new manage their respective processes via shared ch.
func (mr *ManagerReference) SetManager(m *app.Manager) {
	mr.mu.Lock()
	defer mr.mu.Unlock()
	mr.m = m
}

// Implement ManagerInterface by forwarding to current manager
func (mr *ManagerReference) GetProcessInfo(name string) (protocol.ProcessInfo, error) {
	return mr.GetManager().GetProcessInfo(name)
}

func (mr *ManagerReference) GetAllProcessInfo() ([]protocol.ProcessInfo, error) {
	return mr.GetManager().GetAllProcessInfo()
}

func (mr *ManagerReference) Start(name string) error {
	return mr.GetManager().Start(name)
}

func (mr *ManagerReference) Stop(name string) error {
	return mr.GetManager().Stop(name)
}

func (mr *ManagerReference) Restart(name string) error {
	return mr.GetManager().Restart(name)
}

func (mr *ManagerReference) Reload() (*app.ReloadResult, error) {
	return mr.GetManager().Reload()
}

func (mr *ManagerReference) Shutdown() error {
	return mr.GetManager().Shutdown()
}

func main() {
	ch := app.NewProcessChannels()

	sighup := make(chan os.Signal, 1)

	manager, err := app.NewManagerFromConfig("config.yml")
	if err != nil {
		fmt.Printf("Error loading configuration: %v\n", err)
		return
	}

	manager.SetChannels(ch)

	// Start the Manager's event loop in a goroutine
	go manager.Run()

	manager.Spawn(app.NewManager())

	// Wrap manager in reference for atomic updates
	managerRef := NewManagerReference(manager)

	signal.Notify(sighup, syscall.SIGHUP)

	socketPath := "/tmp/taskmaster.sock"
	_, err = app.StartSocketListener(socketPath, managerRef)
	if err != nil {
		fmt.Printf("Error starting socket server: %v\n", err)
		return
	}

	for {
		select {
		case <-sighup:
			fmt.Println("Hot-reloading configuration...")

			newManager, err := app.NewManagerFromConfig("config.yml")
			if err != nil {
				fmt.Printf("Error reloading configuration: %v\n", err)
				continue
			}

			newManager.SetChannels(ch)
			// Start the new manager's event loop
			go newManager.Run()
			newManager.Spawn(manager)

			// Atomically swap manager reference
			managerRef.SetManager(newManager)
			manager = newManager

		case msg := <-ch.Status:
			// Status updates are now handled by the Manager's event loop
			// We just log them here for visibility
			fmt.Printf("Updated status for program %s: %s\n", msg.Name, msg.Status)
		}
	}
}
