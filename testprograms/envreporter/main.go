// Envreporter verifies environment variables, working directory, and umask settings
// applied by a process manager. Writes a report to stdout and creates a test file
// to check the umask was correctly applied.
//
// Usage: envreporter
//
//	Reports cwd, TM_* environment variables, and file mode of a created file.

package main

import (
	"fmt"
	"os"
	"sort"
)

func main() {
	// Print header
	fmt.Println("=== env-reporter report ===")

	// Print working directory
	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to get working directory: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("cwd=%s\n", cwd)

	// Collect and sort TM_* environment variables
	var tmEnvs []string
	for _, env := range os.Environ() {
		// Check if the variable name starts with TM_
		for i := 0; i < len(env); i++ {
			if env[i] == '=' {
				if i >= 3 && env[0:3] == "TM_" {
					tmEnvs = append(tmEnvs, env)
				}
				break
			}
		}
	}
	sort.Strings(tmEnvs)

	// Print TM_* environment variables
	for _, env := range tmEnvs {
		fmt.Printf("env: %s\n", env)
	}

	// Create test file with mode 0666
	testFile := "./env-reporter-touched.file"
	file, err := os.OpenFile(testFile, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0666)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create test file: %v\n", err)
		os.Exit(1)
	}

	// Write content
	_, err = file.WriteString("created by env-reporter")
	if err != nil {
		file.Close()
		fmt.Fprintf(os.Stderr, "failed to write to test file: %v\n", err)
		os.Exit(1)
	}
	file.Close()

	// Stat the file and print its mode
	fi, err := os.Stat(testFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to stat test file: %v\n", err)
		os.Exit(1)
	}

	mode := fi.Mode().Perm()
	fmt.Printf("file_mode=%#o\n", mode)

	// Print footer
	fmt.Println("=== end report ===")

	os.Exit(0)
}
