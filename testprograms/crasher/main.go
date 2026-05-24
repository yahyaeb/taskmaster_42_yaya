// Crasher exits with a non-zero status—useful to test process manager failure handling
// and supervision of crashing processes.
//
// Usage: crasher [exit-code]
//
//	env CRASH_DELAY_MS controls delay before exit in milliseconds (default: 0).

package main

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

func main() {
	exitCode := 1
	if len(os.Args) > 1 {
		n, err := strconv.Atoi(os.Args[1])
		if err != nil {
			fmt.Fprintf(os.Stderr, "usage: crasher [exit-code], exit-code must be an integer\n")
			os.Exit(2)
		}
		exitCode = n
	}

	delayMs := 0
	if delayStr := os.Getenv("CRASH_DELAY_MS"); delayStr != "" {
		n, err := strconv.Atoi(delayStr)
		if err == nil && n >= 0 {
			delayMs = n
		}
	}

	if delayMs > 0 {
		time.Sleep(time.Duration(delayMs) * time.Millisecond)
	}

	fmt.Printf("crasher: exiting with code %d after %dms\n", exitCode, delayMs)
	os.Exit(exitCode)
}
