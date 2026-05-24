// Slowstopper ignores SIGTERM and only exits cleanly on SIGUSR1—useful to test
// a process supervisor's signal escalation (stopsignal → stoptime → SIGKILL).
//
// Usage: slowstopper
//
//	Will ignore SIGTERM, respond to SIGUSR1 with clean exit.
//	Press Ctrl+C to terminate immediately from terminal.

package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	fmt.Printf("slowstopper: started, pid=%d, will ignore SIGTERM\n", os.Getpid())

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGTERM, syscall.SIGUSR1, syscall.SIGINT)

	go func() {
		for sig := range sigChan {
			switch sig {
			case syscall.SIGTERM:
				fmt.Println("slowstopper: caught SIGTERM, ignoring")
			case syscall.SIGUSR1:
				fmt.Println("slowstopper: caught SIGUSR1, exiting cleanly")
				os.Exit(0)
			case syscall.SIGINT:
				os.Exit(0)
			}
		}
	}()

	for {
		time.Sleep(1 * time.Second)
	}
}
