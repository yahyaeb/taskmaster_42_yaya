package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"taskmaster/internal"
	"time"
)

func startLogger() *internal.Logger {
	logger, err := internal.NewLogger("taskmaster.log")
	if err != nil {
		fmt.Printf("[ERROR] Failed to create logger: %v\n", err)
		return nil
	}
	logger.Start()

	return logger
}

func main() {

	path := "config.yml"
	configMap, err := internal.LoadConfig(path)
	if err != nil {
		fmt.Println(err)
		return
	}

	logger := startLogger()
	if logger == nil {
		return
	}

	ctx, shutdown := context.WithCancel(context.Background())

	mgr := internal.CreateManager(ctx)
	mgr.SetLogger(logger)

	if err := mgr.Load(configMap); err != nil {
		logger.LogMessage(internal.LevelError, fmt.Sprintf("manager load failed: %v", err))
		shutdown()
		return
	}

	svr, err := internal.NewServer("/tmp/taskmaster.sock", mgr, path, shutdown)
	if err != nil {
		logger.LogMessage(internal.LevelError, fmt.Sprintf("failed to create server: %v", err))
		shutdown()
		return
	}

	go func() {
		if err := svr.Serve(); err != nil {
			logger.LogMessage(internal.LevelError, fmt.Sprintf("server error: %v", err))
			shutdown()
		}
	}()

	defer svr.Stop()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	fmt.Println("---- Taskmaster Monitoring (manager) ----")

	for {
		select {
		case sig := <-sigCh:
			switch sig {
			case syscall.SIGINT, syscall.SIGTERM:
				logger.LogMessage(internal.LevelInfo, "received shutdown signal, exiting")
				shutdown()
				mgr.Shutdown()
				time.Sleep(500 * time.Microsecond)
				logger.Close()
				return
			}
		case <-ctx.Done():
			logger.LogMessage(internal.LevelInfo, "received shutdown via RPC, exiting")
			mgr.Shutdown()
			time.Sleep(500 * time.Microsecond)
			logger.Close()
			return
		}
	}
}
