package tests

import (
	"os"
	"strings"
	"time"

	"taskmaster/test_taskmaster/helpers"
)

func RunPoint7(ctx *helpers.TestContext, r *helpers.Report) {
	r.Section("TEST 7 — Daemon Lifecycle Logging")
	r.Info("Guide: logging must cover start, stop, restart, and unexpected exits.")
	r.Info("Runs a controlled start → stop → restart cycle on lazy:00 and inspects taskmaster.log.")

	forceStop(ctx, "lazy:00")
	if err := os.WriteFile(ctx.LogPath, []byte{}, 0o644); err != nil {
		r.Failf("7.0 could not clear daemon log: %v", err)
		return
	}

	if _, err := helpers.RunCtl(ctx, "start lazy:00"); err != nil {
		r.Failf("7.1 start lazy:00 failed: %v", err)
		return
	}
	if _, err := waitForProc(ctx, "lazy:00", 5*time.Second, func(p helpers.ProcStatus) bool {
		return p.State == "running" && p.PID > 0
	}); err != nil {
		r.Failf("7.1 lazy:00 did not reach RUNNING: %v", err)
		return
	}
	r.Passf("7.1 start lazy:00 → RUNNING")

	if _, err := helpers.RunCtl(ctx, "stop lazy:00"); err != nil {
		r.Failf("7.2 stop lazy:00 failed: %v", err)
		return
	}
	if _, err := waitForProc(ctx, "lazy:00", 5*time.Second, func(p helpers.ProcStatus) bool {
		return p.State == "stopped"
	}); err != nil {
		r.Failf("7.2 lazy:00 did not reach STOPPED: %v", err)
		return
	}
	r.Passf("7.2 stop lazy:00 → STOPPED")

	if _, err := helpers.RunCtl(ctx, "restart lazy:00"); err != nil {
		r.Failf("7.3 restart lazy:00 failed: %v", err)
		return
	}
	if _, err := waitForProc(ctx, "lazy:00", 5*time.Second, func(p helpers.ProcStatus) bool {
		return p.State == "running" && p.PID > 0
	}); err != nil {
		r.Failf("7.3 lazy:00 did not reach RUNNING after restart: %v", err)
		return
	}
	r.Passf("7.3 restart lazy:00 → RUNNING")

	logText, err := waitForLogContains(ctx.LogPath, "spawned: 'lazy:00'", 3*time.Second)
	if err != nil {
		r.Failf("7.4 log missing spawn entry: %v", err)
		return
	}

	checks := []struct {
		label  string
		needle string
		desc   string
	}{
		{"7.4a", "spawned: 'lazy:00'", "process spawn is logged"},
		{"7.4b", "exited: 'lazy:00'", "graceful stop is logged"},
		{"7.4c", "entered RUNNING state", "running state transition is logged"},
	}
	for _, c := range checks {
		if strings.Contains(logText, c.needle) {
			r.Passf("%s %s", c.label, c.desc)
		} else {
			r.Failf("%s log missing %q (%s)", c.label, c.needle, c.desc)
		}
	}

	spawnCount := strings.Count(logText, "spawned: 'lazy:00'")
	if spawnCount >= 2 {
		r.Passf("7.5 restart produced multiple spawn log entries (%d)", spawnCount)
	} else {
		r.Failf("7.5 expected >=2 spawn entries after start+restart, got %d", spawnCount)
	}
}

func containsAny(text string, needles ...string) bool {
	for _, n := range needles {
		if strings.Contains(text, n) {
			return true
		}
	}
	return false
}

func tailLog(text string, max int) string {
	if len(text) <= max {
		return text
	}
	return text[len(text)-max:]
}
