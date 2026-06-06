package tests

import (
	"syscall"
	"time"

	"taskmaster/test_taskmaster/config"
	"taskmaster/test_taskmaster/helpers"
)

func RunPoint6(ctx *helpers.TestContext, r *helpers.Report) {
	r.Section("TEST 6 — External Kill → Autorestart")
	r.Info("Guide scenario: kill a supervised process and verify it restarts automatically.")
	r.Info("Uses ticker:00 (autorestart: always) — an external SIGKILL must spawn a new instance.")

	proc, err := waitForProc(ctx, "ticker:00", config.DefaultWaitTimeout, func(p helpers.ProcStatus) bool {
		return p.State == "running" && p.PID > 0
	})
	if err != nil {
		r.Failf("6.1 ticker:00 not running before kill test: %v", err)
		return
	}
	r.Passf("6.1 ticker:00 running with PID %d", proc.PID)

	if err := syscall.Kill(proc.PID, syscall.SIGKILL); err != nil {
		r.Failf("6.2 could not SIGKILL ticker:00 (pid %d): %v", proc.PID, err)
		return
	}
	r.Passf("6.2 sent SIGKILL to ticker:00 (pid %d)", proc.PID)

	next, err := waitForProc(ctx, "ticker:00", 8*time.Second, func(p helpers.ProcStatus) bool {
		return p.State == "running" && p.PID > 0 && p.PID != proc.PID
	})
	if err != nil {
		r.Failf("6.3 ticker:00 did not autorestart after SIGKILL: %v", err)
		return
	}
	r.Passf("6.3 ticker:00 autorestarted with new PID %d (was %d)", next.PID, proc.PID)

	logText, err := waitForLogContains(ctx.LogPath, "spawned: 'ticker:00'", 5*time.Second)
	if err != nil {
		r.Failf("6.4 daemon log missing spawn entry after kill: %v", err)
		return
	}
	if containsAny(logText, "exited: 'ticker:00'", "not expected") {
		r.Passf("6.4 daemon log records unexpected exit and respawn for ticker:00")
	} else {
		r.Failf("6.4 expected exit/spawn log lines after SIGKILL, log tail: %q", tailLog(logText, 400))
	}
}
