package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGTERM)

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			fmt.Println("longrunner: still alive")
		case sig := <-sigChan:
			fmt.Printf("longrunner: caught %s, exiting cleanly\n", sig)
			os.Exit(0)
		}
	}
}
