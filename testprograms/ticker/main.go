// Ticker exits after printing a few timestamps—useful to smoke-test startProcess + Wait
// without a long-lived server or external paths.
//
// Usage: ticker [ticks]
//
//	env SHOW_NAME overrides the printed label (YAML can set env for the child).

package main

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

func main() {
	ticks := 4
	if len(os.Args) > 1 {
		n, err := strconv.Atoi(os.Args[1])
		if err != nil || n < 1 {
			fmt.Fprintf(os.Stderr, "usage: ticker [ticks], ticks must be >= 1\n")
			os.Exit(2)
		}
		ticks = n
	}

	name := os.Getenv("SHOW_NAME")
	if name == "" {
		name = "ticker"
	}

	for i := 1; i <= ticks; i++ {
		fmt.Printf("%s [%d/%d]\n", name, i, ticks)
		time.Sleep(300 * time.Millisecond)
	}

}
