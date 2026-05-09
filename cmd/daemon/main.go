package main

import (
	"bufio"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"taskmaster/internal/app"
	"taskmaster/internal/bus"
)

func read(inputChan chan string) {
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		inputChan <- scanner.Text()
	}
}

func main() {
	ctl := struct {
		updates chan bus.ProcessUpdate
		input   chan string
		stop    map[string]chan struct{}
		sighup  chan os.Signal
	}{
		updates: make(chan bus.ProcessUpdate),
		input:   make(chan string),
		stop:    make(map[string]chan struct{}),
		sighup:  make(chan os.Signal, 1),
	}

	manager, err := app.Load("config.yml")
	if err != nil {
		fmt.Printf("Error loading configuration: %v\n", err)
		return
	}

	app.Spawn(app.NewManager(), manager, ctl.updates, ctl.stop)
	signal.Notify(ctl.sighup, syscall.SIGHUP)

	go read(ctl.input)

	m := app.NewManager()

	socketPath := "/tmp/taskmaster.sock"
	_, err = app.StartSocketListener(socketPath, m)
	if err != nil {
		fmt.Printf("Error starting socket server: %v\n", err)
		return
	}
	for {
		fmt.Print("> ")

		select {
		case <-ctl.sighup:
			fmt.Println("Hot-reloading configuration...")

			newManager, err := app.Load("config.yml")
			if err != nil {
				fmt.Printf("Error reloading configuration: %v\n", err)
				continue
			}

			app.Spawn(manager, newManager, ctl.updates, ctl.stop)
			manager = newManager

		case msg := <-ctl.updates:
			if proc, ok := manager.Process[msg.Name]; ok {
				proc.SetStatus(msg.Status)
				proc.SetPid(msg.Pid)
				fmt.Printf("Updated status for program %s: %s\n", msg.Name, msg.Status)
			}

		case input := <-ctl.input:
			switch input {
			case "exit":
				app.Stop(manager, ctl.stop)
				return
			case "status":
				manager.Mu.RLock()
				for name, proc := range manager.Process {
					pid, status := proc.State()
					fmt.Printf("Program: %s | Status: %s | PID: %d\n", name, status, pid)
				}
				manager.Mu.RUnlock()
			case "reload":
				fmt.Println("reloading configuration...")
				self, _ := os.FindProcess(os.Getpid())
				self.Signal(syscall.SIGHUP)
			default:
				fmt.Println("Unknown command:", input)
			}
		}
	}
}
