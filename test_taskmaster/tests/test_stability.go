package tests

import (
	"syscall"
	"time"

	"taskmaster/test_taskmaster/config"
	"taskmaster/test_taskmaster/helpers"
)

func RunStability(ctx *helpers.TestContext, r *helpers.Report) {
	r.Section("STABILITY — Daemon Survives Stress Sequence")
	r.Info("Guide: the program must stay up after a variety of operations.")
	r.Info("Runs rapid start/stop/restart/reload/status and verifies the daemon is still healthy.")

	_, _ = helpers.RunCtl(ctx, "start lazy:00")
	_, _ = helpers.WaitForStatus(ctx, 3*time.Second, func(m map[string]helpers.ProcStatus) bool {
		p, ok := m["lazy:00"]
		return ok && p.State == "running"
	})

	_, _ = helpers.RunCtl(ctx, "stop lazy:00")
	_, _ = helpers.RunCtl(ctx, "restart dummy:00")
	_, _ = helpers.RunCtl(ctx, "reload")
	_, _ = helpers.RunCtl(ctx, "status")
	_, _ = helpers.RunCtl(ctx, "status")

	if ctx.Daemon == nil || ctx.Daemon.Process == nil {
		r.Failf("S.1 daemon process handle is missing after stress sequence")
		return
	}
	if err := ctx.Daemon.Process.Signal(syscall.Signal(0)); err != nil {
		r.Failf("S.1 daemon process is not alive after stress sequence: %v", err)
		return
	}
	r.Passf("S.1 daemon process still alive (pid %d)", ctx.Daemon.Process.Pid)

	if err := helpers.WaitForCtlReady(ctx, config.DefaultWaitTimeout); err != nil {
		r.Failf("S.2 ctl lost connectivity after stress sequence: %v", err)
		return
	}
	r.Passf("S.2 ctl still responds after stress sequence")

	stMap, err := helpers.WaitForStatus(ctx, config.DefaultWaitTimeout, func(m map[string]helpers.ProcStatus) bool {
		return len(m) >= config.MinExpectedPrograms
	})
	if err != nil {
		r.Failf("S.3 status query failed after stress sequence: %v", err)
		return
	}
	r.Passf("S.3 status still lists %d supervised programs", len(stMap))
}
