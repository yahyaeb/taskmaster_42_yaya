// crasher exits immediately with the code given by EXIT_CODE env (default: 1).
// Used in E2E tests to drive autorestart, exitcodes, and startretries scenarios.
package main

import (
	"os"
	"strconv"
)

func main() {
	code := 1
	if s := os.Getenv("EXIT_CODE"); s != "" {
		if n, err := strconv.Atoi(s); err == nil {
			code = n
		}
	}
	os.Exit(code)
}
