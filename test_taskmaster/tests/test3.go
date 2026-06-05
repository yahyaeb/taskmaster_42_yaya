package tests

import (
	"os"
	"strings"
	"time"

	"taskmaster/test_taskmaster/config"
	"taskmaster/test_taskmaster/helpers"
)

func RunPoint3(ctx *helpers.TestContext, r *helpers.Report) {
	r.Section("TEST 3 — Logging (Stdout / Stderr Redirection)")

	// In config.yml, 'hello' program autostarts, writes to stdout/stderr and sleeps
	// Let's ensure it has had a chance to start and write
	time.Sleep(config.LogFlushWait)

	// Check if taskmaster daemon log was created
	info, err := os.Stat(ctx.LogPath)
	if err == nil && info.Size() > 0 {
		r.Passf("3.1 Daemon creates and writes to its own logfile")
	} else {
		r.Failf("3.1 Daemon logfile missing or empty")
	}

	// Read process logs
	stdoutFile := config.HelloStdoutLog
	stderrFile := config.HelloStderrLog

	// Wait up to some time for logs to be populated if buffered
	outContent, errOut := os.ReadFile(stdoutFile)
	errContent, errErr := os.ReadFile(stderrFile)

	if errOut == nil && strings.Contains(string(outContent), "Hello from stdout!") {
		r.Passf("3.2 Process stdout gets redirected properly")
	} else {
		r.Failf("3.2 Failed stdout redirection. Error: %v, Content: %q", errOut, string(outContent))
	}

	if errErr == nil && strings.Contains(string(errContent), "Hello from stderr!") {
		r.Passf("3.3 Process stderr gets redirected properly")
	} else {
		r.Failf("3.3 Failed stderr redirection. Error: %v, Content: %q", errErr, string(errContent))
	}
}
