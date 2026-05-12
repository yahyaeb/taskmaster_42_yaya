package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"taskmaster/internal/app"
)

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

	signal.Notify(sighup, syscall.SIGHUP)

	socketPath := "/tmp/taskmaster.sock"
	_, err = app.StartSocketListener(socketPath, manager)
	if err != nil {
		fmt.Printf("Error starting socket server: %v\n", err)
		return
	}

	for range sighup {
		fmt.Println("Hot-reloading configuration...")

		// Just reload config in existing manager
		// This avoids channel competition from manager swap
		result, err := manager.Reload()
		if err != nil {
			fmt.Printf("Error reloading configuration: %v\n", err)
			continue
		}

		fmt.Printf("Reload complete: added=%v, removed=%v, restarted=%v\n",
			result.Added, result.Removed, result.Restarted)
	}
}
