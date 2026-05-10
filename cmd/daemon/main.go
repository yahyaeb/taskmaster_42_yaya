package main

import (
	"context"
	"sync"
	"bufio"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"taskmaster/internal/app"
	"taskmaster/internal/protocol"
	"taskmaster/internal/bus"
)

func read(inputChan chan string) {
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		inputChan <- scanner.Text()
	}
}

// ManagerReference is a thread-safe wrapper that holds the manager and forwards status updates
type ManagerReference struct {
	m             *app.Manager
	mu            sync.Mutex
	statusForward chan bus.ProcessUpdate
	done          chan struct{}
	forwardCtx    context.Context
	forwardCancel context.CancelFunc
}

// NewManagerReference creates a new ManagerReference with initial manager
func NewManagerReference() *ManagerReference {
	ctx, cancel := context.WithCancel(context.Background())
	return &ManagerReference{
		statusForward: make(chan bus.ProcessUpdate, 100),
		done:          make(chan struct{}),
		forwardCtx:    ctx,
		forwardCancel: cancel,
	}
}

func (mr *ManagerReference) GetManager() *app.Manager {
	mr.mu.Lock()
	defer mr.mu.Unlock()
	return mr.m
}

// SetManager updates the manager and starts a forwarding goroutine for the new manager's updates
func (mr *ManagerReference) SetManager(m *app.Manager) {
	mr.mu.Lock()
	defer mr.mu.Unlock()

	mr.m = m
	
	// Start forwarding goroutine for the new manager's status updates
	go mr.forwardStatusUpdates()
}

// forwardStatusUpdates reads from the current manager's ch.Status and forwards to statusForward
func (mr *ManagerReference) forwardStatusUpdates() {
	for {
		select {
		case <-mr.done:
			return
		default:
		}
		
		// Get current manager's channel safely
		mr.mu.Lock()
		if mr.m == nil || mr.m.Channels() == nil {
			mr.mu.Unlock()
			return
		}
		statusCh := mr.m.Channels()
		mr.mu.Unlock()

		select {
		case update := <-statusCh.Status:
			// Forward the update downstream
			select {
			case mr.statusForward <- update:
			case <-mr.forwardCtx.Done():
				return
			}
		case <-mr.forwardCtx.Done():
			return
		case <-mr.done:
			return
		}
	}
}

// StatusForward returns the channel for consuming forwarded status updates
func (mr *ManagerReference) StatusForward() <-chan bus.ProcessUpdate {
	return mr.statusForward
}

// StopForwarding stops the forwarding goroutine
func (mr *ManagerReference) StopForwarding() {
	close(mr.done)
	mr.forwardCancel()
}

// Implement ManagerInterface methods
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
	
	managerRef := NewManagerReference()
	defer managerRef.StopForwarding()

	manager, err := app.NewManagerFromConfig("config.yml")
	if err != nil {
		fmt.Printf("Error loading configuration: %v\n", err)
		return
	}

	manager.SetChannels(ch)
	managerRef.SetManager(manager)

	// Start the Manager's event loop in a goroutine
	go manager.Run()

	manager.Spawn(app.NewManager())
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
			
			// Update manager reference and start forwarding from new manager
			managerRef.SetManager(newManager)
			manager = newManager

		case msg := <-managerRef.StatusForward():
			fmt.Printf("[DEBUG] Received status update: %s -> %s\n", msg.Name, msg.Status)
			// Status updates are now handled by the Manager's event loop
			// We just log them here for visibility
			fmt.Printf("Updated status for program %s: %s\n", msg.Name, msg.Status)

		case cmd := <-ch.Commands:
			handleCommand(managerRef, cmd)
		}
	}
}

func handleCommand(m app.ManagerInterface, cmd app.Command) {
	switch cmd.Type {
	case "start":
		cmd.Resp <- m.Start(cmd.Target)
	case "stop":
		cmd.Resp <- m.Stop(cmd.Target)
	case "restart":
		cmd.Resp <- m.Restart(cmd.Target)
	case "reload":
		_, err := m.Reload()
		cmd.Resp <- err
	case "shutdown":
		cmd.Resp <- m.Shutdown()
	}
}

func handleInput(m app.ManagerInterface, ch *app.ProcessChannels, sighup chan os.Signal, input string) {
	switch input {
	case "exit":
		// TODO: implement stop all
		os.Exit(0)
	case "status":
		// Query process info through the event loop
		infos, err := m.GetAllProcessInfo()
		if err != nil {
			fmt.Printf("Error getting process info: %v\n", err)
			return
		}
		for _, info := range infos {
			fmt.Printf("Program: %s | Status: %s | PID: %d\n", info.Name, info.Status, info.Pid)
		}
	case "reload":
		fmt.Println("reloading configuration...")
		self, _ := os.FindProcess(os.Getpid())
		self.Signal(syscall.SIGHUP)
	default:
		fmt.Println("Unknown command:", input)
	}
}
