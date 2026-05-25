package main

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

func main() {
	interval := 1
	if len(os.Args) > 1 {
		interval, _ = strconv.Atoi(os.Args[1])
	}

	ticker := time.NewTicker(time.Duration(interval) * time.Second)
	defer ticker.Stop()

	count := 0
	for range ticker.C {
		count++
		fmt.Printf("[%s] tick %d\n", time.Now().Format("2006-01-02 15:04:05"), count)
	}
}
