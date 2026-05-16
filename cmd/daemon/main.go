package main

import (
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"taskmaster/internal/app"
	"taskmaster/internal/config"
	"taskmaster/internal/engine"
	"taskmaster/internal/server"
	"taskmaster/internal/state"
)

func main() {
	ch := app.NewProcessChannels()

	sighup := make(chan os.Signal, 1)

	const configPath = "config.yml"
	loader := &config.YAMLLoader{}
	specs, err := loader.Load(configPath)
	if err != nil {
		fmt.Printf("Error loading configuration: %v\n", err)
		return
	}

	reg := state.NewRegistry()
	for i := range specs {
		spec := &specs[i]
		reg.Upsert(spec.ProcessName, spec)
	}

	manager := app.NewManager(
		reg,
		engine.NewOsProcessExecutor(),
		&engine.OSSignalHandler{},
		loader,
		slog.Default(),
	)
	manager.SetConfigPath(configPath)

	manager.SetChannels(ch)

	if err := manager.StartAutostartProcesses(); err != nil {
		fmt.Printf("Error autostarting: %v\n", err)
		return
	}

	signal.Notify(sighup, syscall.SIGHUP)

	socketPath := "/tmp/taskmaster.sock"
	_, err = server.NewSocketListener(socketPath, manager)
	if err != nil {
		fmt.Printf("Error starting socket server: %v\n", err)
		return
	}

	for range sighup {
		fmt.Println("Hot-reloading configuration...")

		result, err := manager.Reload()
		if err != nil {
			fmt.Printf("Error reloading configuration: %v\n", err)
			continue
		}

		fmt.Printf("Reload complete: added=%v, removed=%v, restarted=%v\n",
			result.Added, result.Removed, result.Restarted)
	}
}
