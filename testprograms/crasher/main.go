package main

import (
	"os"
	"strconv"
	"time"
)

func main() {
	duration := 0
	if len(os.Args) > 1 {
		duration, _ = strconv.Atoi(os.Args[1])
	}
	time.Sleep(time.Duration(duration) * time.Second)
	os.Exit(1)
}
