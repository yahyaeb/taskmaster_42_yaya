package internal

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// ReadMemInfo returns total and available memory in KiB from /proc/meminfo.
func ReadMemInfo() (total, available uint64, err error) {
	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return 0, 0, fmt.Errorf("read /proc/meminfo: %w", err)
	}

	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		switch {
		case strings.HasPrefix(line, "MemTotal:"):
			total, err = parseMemInfoLine(line)
		case strings.HasPrefix(line, "MemAvailable:"):
			available, err = parseMemInfoLine(line)
		}
		if err != nil {
			return 0, 0, err
		}
	}

	if total == 0 {
		return 0, 0, fmt.Errorf("MemTotal not found in /proc/meminfo")
	}
	if available == 0 {
		return 0, 0, fmt.Errorf("MemAvailable not found in /proc/meminfo")
	}
	return total, available, nil
}

// MemoryUsagePercent returns system memory usage as 0.0–100.0.
func MemoryUsagePercent() (float64, error) {
	total, available, err := ReadMemInfo()
	if err != nil {
		return 0, err
	}
	used := float64(total - available)
	return used / float64(total) * 100.0, nil
}

// parseMemInfoLine extracts the numeric value (in KiB) from a /proc/meminfo line.
func parseMemInfoLine(line string) (uint64, error) {
	parts := strings.Fields(line)
	if len(parts) < 2 {
		return 0, fmt.Errorf("unexpected /proc/meminfo line: %q", line)
	}
	val, err := strconv.ParseUint(parts[1], 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parse number from %q: %w", line, err)
	}
	return val, nil
}