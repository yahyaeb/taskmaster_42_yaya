// signal_trap catches SIGTERM (and optionally SIGHUP) and records the received
// signal name in the file pointed to by SIGNAL_LOG env var.
//
// Env vars:
//
//	SIGNAL_LOG     path to write "RECEIVED:<signal>\n"
//	SIGNAL_ACTION  "exit" (default) — exit 0 after logging
//	               "ignore"         — log and keep running (forces SIGKILL from daemon stoptime)
package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	logPath := os.Getenv("SIGNAL_LOG")
	action := os.Getenv("SIGNAL_ACTION")
	if action == "" {
		action = "exit"
	}

	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGTERM, syscall.SIGHUP)

	go func() {
		for sig := range ch {
			writeLog(logPath, fmt.Sprintf("RECEIVED:%s\n", sig))
			if action == "exit" {
				os.Exit(0)
			}
			// action == "ignore": log and continue — daemon must escalate to SIGKILL
		}
	}()

	for {
		time.Sleep(time.Hour)
	}
}

func writeLog(path, msg string) {
	if path == "" {
		return
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()
	f.WriteString(msg)
}
