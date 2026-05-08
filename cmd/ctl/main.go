package main

import (
	"fmt"
	"os"
)

// CLI tool for communicating with the taskmaster daemon.
// Developer B will implement this with socket communication
// to interact with the Manager API from daemon.

func main() {
	if len(os.Args) < 2 {
		printUsage()
		return
	}

	command := os.Args[1]

	// TODO: Implement socket communication to daemon
	// For now, print placeholder messages

	switch command {
	case "status":
		fmt.Println("Getting process status from daemon...")
		// TODO: Connect to daemon socket and request status
	case "start":
		if len(os.Args) < 3 {
			fmt.Println("Usage: ctl start <process_name>")
			return
		}
		procName := os.Args[2]
		fmt.Printf("Starting process: %s\n", procName)
		// TODO: Connect to daemon and request start
	case "stop":
		if len(os.Args) < 3 {
			fmt.Println("Usage: ctl stop <process_name>")
			return
		}
		procName := os.Args[2]
		fmt.Printf("Stopping process: %s\n", procName)
		// TODO: Connect to daemon and request stop
	case "restart":
		if len(os.Args) < 3 {
			fmt.Println("Usage: ctl restart <process_name>")
			return
		}
		procName := os.Args[2]
		fmt.Printf("Restarting process: %s\n", procName)
		// TODO: Connect to daemon and request restart
	case "reload":
		fmt.Println("Reloading configuration...")
		// TODO: Connect to daemon and request reload
	default:
		fmt.Printf("Unknown command: %s\n", command)
		printUsage()
	}
}

func printUsage() {
	fmt.Println(`
Usage: ctl [command] [args]

Commands:
  status              Show status of all processes
  start <name>        Start a process
  stop <name>         Stop a process
  restart <name>      Restart a process
  reload              Reload configuration from file

This tool communicates with the taskmaster daemon via socket.
`)
}
