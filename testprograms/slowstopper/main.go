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
	signal.Notify(sigChan, syscall.SIGUSR1, syscall.SIGTERM)

	for {
		select {
		case sig := <-sigChan:
			switch sig {
			case syscall.SIGUSR1:
				fmt.Println("caught USR1, exiting cleanly")
				os.Exit(0)
			case syscall.SIGTERM:
				time.Sleep(10 * time.Second)
				os.Exit(0)
			}
		default:
			fmt.Println("running...")
			time.Sleep(1 * time.Second)
		}
	}
}
