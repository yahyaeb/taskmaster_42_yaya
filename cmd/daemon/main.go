package main

import (
	"bufio"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"taskmaster/internal/app"
)

func read(inputChan chan string) {
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		inputChan <- scanner.Text()
	}
}

func main() {
	ch := app.NewProcessChannels()
	sighup := make(chan os.Signal, 1)
	input := make(chan string)

	manager, err := app.NewManagerFromConfig("config.yml")
	if err != nil {
		fmt.Printf("Error loading configuration: %v\n", err)
		return
	}

	manager.SetChannels(ch)

	// Start the Manager's event loop in a goroutine
	go manager.Run()

	manager.Spawn(app.NewManager())
	signal.Notify(sighup, syscall.SIGHUP)

	go read(input)

	socketPath := "/tmp/taskmaster.sock"
	_, err = app.StartSocketListener(socketPath, manager)
	if err != nil {
		fmt.Printf("Error starting socket server: %v\n", err)
		return
	}

	for {
		fmt.Print("> ")

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
			manager = newManager

		case msg := <-ch.Status:
			// Status updates are now handled by the Manager's event loop
			// We just log them here for visibility
			fmt.Printf("Updated status for program %s: %s\n", msg.Name, msg.Status)

		case cmd := <-ch.Commands:
			handleCommand(manager, cmd)

		case in := <-input:
			handleInput(manager, ch, sighup, in)
		}
	}
}

func handleCommand(m *app.Manager, cmd app.Command) {
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

func handleInput(m *app.Manager, ch *app.ProcessChannels, sighup chan os.Signal, input string) {
	switch input {
	case "exit":
		m.StopAll()
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
