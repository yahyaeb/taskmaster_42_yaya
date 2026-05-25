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

	for {
		fmt.Println("longrunner: still alive")
		time.Sleep(5 * time.Second)
	}
}
