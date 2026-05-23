package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"taskmaster"
)

func printConfig(configMap map[string]*taskmaster.Config) {
	for _, config := range configMap {
		fmt.Println(config.ProcessName)
	}
}

func startLogger() *taskmaster.Logger {
	logger, err := taskmaster.NewLogger("taskmaster.log")
	if err != nil {
		fmt.Printf("[ERROR] Failed to create logger: %v\n", err)
		return nil
	}
	logger.Start()

	return logger
}

func main() {

	path := "config.yml"
	configMap, err := taskmaster.LoadConfig(path)
	if err != nil {
		fmt.Println(err)
		return
	}

	printConfig(configMap)

	logger := startLogger()
	if logger == nil {
		return
	}

	ctx, shutdown := context.WithCancel(context.Background())

	taskmaster := taskmaster.CreateManager(ctx)
	taskmaster.SetLogger(logger)

	if err := taskmaster.Load(configMap); err != nil {
		fmt.Printf("[ERROR] manager load failed: %v\n", err)
		shutdown()
		return
	}

	svr, err := taskmaster.NewServer("/tmp/taskmaster.sock", taskmaster, path, shutdown)
	if err != nil {
		fmt.Printf("[ERROR] Failed to create server: %v\n", err)
		shutdown()
		return
	}

	go func() {
		if err := svr.Serve(); err != nil {
			fmt.Printf("[ERROR] Server error: %v\n", err)
			shutdown()
		}
	}()

	defer svr.Stop()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM)

	fmt.Println("---- Taskmaster Monitoring (manager) ----")

	for {
		sig := <-sigCh
		switch sig {
		case syscall.SIGHUP:
			fmt.Println("[INFO] Received SIGHUP, reloading configuration...")
			newConfigMap, err := taskmaster.LoadConfig(path)
			if err != nil {
				fmt.Printf("[ERROR] Failed to reload config: %v\n", err)
				continue
			}
			if err := taskmaster.Reload(newConfigMap); err != nil {
				fmt.Printf("[ERROR] Failed to apply new config: %v\n", err)
			}
		case syscall.SIGINT, syscall.SIGTERM:
			fmt.Println("[INFO] Received shutdown signal, exiting...")
			shutdown()
			taskmaster.Shutdown()
			logger.Close()
			return
		}
	}
}
