// Longrunner runs indefinitely until receiving SIGTERM or SIGINT—useful to test
// graceful shutdown, auto-restart-on-kill, and uptime tracking in a process supervisor.
//
// On startup, prints the PID. Then every 2 seconds prints an incrementing tick count.
// On SIGTERM or SIGINT, exits cleanly with code 0.

package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	fmt.Printf("longrunner: started, pid=%d\n", os.Getpid())

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGTERM, syscall.SIGINT)

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	tickCount := 0
	for {
		select {
		case sig := <-sigChan:
			fmt.Printf("longrunner: received %s, exiting cleanly\n", sig)
			os.Exit(0)
		case <-ticker.C:
			tickCount++
			fmt.Printf("longrunner: tick %d\n", tickCount)
		}
	}
}
