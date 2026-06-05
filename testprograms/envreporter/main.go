package main

import (
	"fmt"
	"os"
)

func main() {
	// 1. Print current working directory
	cwd, _ := os.Getwd()
	fmt.Println(cwd)

	// 2. Print TM_* environment variables
	for _, e := range os.Environ() {
		if len(e) >= 3 && e[:3] == "TM_" {
			fmt.Println(e)
		}
	}

	// 3. Create probe file
	err := os.WriteFile("envreporter_probe", []byte("probe"), 0644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create probe file: %v\n", err)
		os.Exit(1)
	}

	// 4. Print file path and permissions
	info, err := os.Stat("envreporter_probe")
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to stat probe file: %v\n", err)
		os.Exit(1)
	}
	fmt.Println(info.Name(), info.Mode())

	os.Exit(0)
}
