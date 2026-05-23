package main

import (
	"fmt"
	"os"
	"time"
)

// Logger listens for process updates on a dedicated channel and logs them to a file.
// This decouples logging from the manager's status update flow, avoiding channel contention.
type Logger struct {
	file    *os.File
	updates chan ProcessUpdate
}

// NewLogger creates a new Logger instance with a dedicated updates channel.
func NewLogger(logFile string) (*Logger, error) {
	f, err := os.OpenFile(logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return nil, fmt.Errorf("open log file %s: %w", logFile, err)
	}
	return &Logger{
		file:    f,
		updates: make(chan ProcessUpdate, 50),
	}, nil
}

// Start begins listening for process updates in a goroutine.
// The logger will continue running until the updates channel is closed.
func (l *Logger) Start() {
	go func() {
		for update := range l.updates {
			timestamp := time.Now().Format(time.RFC3339)
			logEntry := fmt.Sprintf(
				"%s - Process: %s, Status: %s, PID: %d, ExitCode: %d\n",
				timestamp,
				update.Name,
				update.Status,
				update.Pid,
				update.ExitCode,
			)
			if _, err := l.file.WriteString(logEntry); err != nil {
				fmt.Fprintf(os.Stderr, "[logger] write error: %v\n", err)
			}
		}
	}()
}

// Log sends a process update to the logger without blocking.
// If the logger's channel is full, the update is discarded (non-blocking send).
func (l *Logger) Log(update ProcessUpdate) {
	select {
	case l.updates <- update:
	default:
		// Channel full, discard to avoid blocking the manager
	}
}

// Close closes the logger's file and updates channel.
func (l *Logger) Close() error {
	close(l.updates)
	return l.file.Close()
}
