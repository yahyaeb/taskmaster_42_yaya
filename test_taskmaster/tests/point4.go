package tests

import (
	"fmt"
	"strings"
	"syscall"
	"time"

	"taskmaster/test_taskmaster/helpers"
)

func RunPoint4(ctx *helpers.TestContext, r *helpers.Report) {
	r.Section("POINT 4.1 — Reload Command")

	statusBefore, err := helpers.WaitForStatus(ctx, 10*time.Second, func(m map[string]helpers.ProcStatus) bool {
		d, dok := m["dummy:00"]
		t, tok := m["ticker:00"]
		return dok && tok && d.PID > 0 && d.State == "running" && t.PID > 0 && t.State == "running"
	})
	if err != nil {
		r.Failf("Could not query initial status: %v", err)
		return
	}

	dummyBefore := statusBefore["dummy:00"]
	r.Passf("4.1a dummy:00 initial PID: %d", dummyBefore.PID)

	fmt.Printf("    Modifying config: changing dummy:00 env (forces restart)...\n")
	helpers.Must(helpers.UpdateProgramBlock(ctx.ConfigPath, "dummy", func(block []string) ([]string, error) {
		return helpers.ReplaceBlockLines(block, "env", []string{"    env:", "      POINT4_RELOAD: \"one\""})
	}))

	reloadOut, err := helpers.RunCtl(ctx, "reload")
	if err != nil {
		r.Failf("4.1b reload command failed: %v", err)
	} else if strings.Contains(reloadOut, "ok") {
		r.Passf("4.1b reload command accepted")
	} else {
		r.Failf("4.1b reload command did not confirm success: %q", reloadOut)
	}

	if st, err := helpers.WaitForStatus(ctx, 8*time.Second, func(m map[string]helpers.ProcStatus) bool {
		p, ok := m["dummy:00"]
		return ok && p.PID > 0 && p.PID != dummyBefore.PID
	}); err == nil {
		r.Passf("4.1c dummy:00 got new PID after reload: %d (was %d)", st["dummy:00"].PID, dummyBefore.PID)
	} else {
		r.Failf("4.1c dummy:00 PID unchanged after reload (should have restarted): %v", err)
	}

	r.Section("POINT 4.2 — SIGHUP Reload Signal")
	helpers.Must(helpers.CopyFile(ctx.BackupPath, ctx.ConfigPath))

	fmt.Printf("    Starting lazy:00 manually...\n")
	if _, err := helpers.RunCtl(ctx, "start lazy:00"); err != nil {
		r.Failf("4.2 start lazy:00 failed: %v", err)
	} else {
		_, _ = helpers.WaitForStatus(ctx, 5*time.Second, func(m map[string]helpers.ProcStatus) bool {
			p, ok := m["lazy:00"]
			return ok && p.PID > 0 && p.State == "running"
		})
	}

	out, err := helpers.RunCtl(ctx, "status")
	var lazyBefore helpers.ProcStatus
	if err != nil {
		r.Failf("4.2 status query failed: %v", err)
	} else {
		statusBefore = helpers.ParseStatus(out)
		lazyBefore = statusBefore["lazy:00"]
		if lazyBefore.PID > 0 {
			r.Passf("4.2a lazy:00 running with PID: %d", lazyBefore.PID)
		} else {
			r.Failf("4.2a Could not get lazy:00 running PID")
		}
	}

	fmt.Printf("    Modifying config again (lazy env)...\n")
	helpers.Must(helpers.UpdateProgramBlock(ctx.ConfigPath, "lazy", func(block []string) ([]string, error) {
		return helpers.ReplaceBlockLines(block, "env", []string{"    env:", "      POINT4_HUP: \"two\""})
	}))

	fmt.Printf("    Sending SIGHUP to daemon (PID %d)...\n", ctx.Daemon.Process.Pid)
	helpers.Must(ctx.Daemon.Process.Signal(syscall.SIGHUP))

	if st, err := helpers.WaitForStatus(ctx, 8*time.Second, func(m map[string]helpers.ProcStatus) bool {
		p, ok := m["lazy:00"]
		return ok && p.PID > 0 && p.PID != lazyBefore.PID
	}); err == nil {
		r.Passf("4.2b lazy:00 restarted via SIGHUP: new PID %d (was %d)", st["lazy:00"].PID, lazyBefore.PID)
	} else {
		r.Failf("4.2b lazy:00 did not restart via SIGHUP: %v", err)
	}

	r.Section("POINT 4.3 — Unchanged Programs Survive Reload")
	helpers.Must(helpers.CopyFile(ctx.BackupPath, ctx.ConfigPath))
	out, err = helpers.RunCtl(ctx, "status")
	helpers.Must(err)
	statusBefore = helpers.ParseStatus(out)
	dummyBefore = statusBefore["dummy:00"]
	tickerBefore := statusBefore["ticker:00"]

	fmt.Printf("    Modifying only dummy:00 env...\n")
	helpers.Must(helpers.UpdateProgramBlock(ctx.ConfigPath, "dummy", func(block []string) ([]string, error) {
		return helpers.ReplaceBlockLines(block, "env", []string{"    env:", "      NEW_VAR: \"test\""})
	}))

	if out, err := helpers.RunCtl(ctx, "reload"); err != nil {
		r.Failf("4.3 reload failed: %v", err)
	} else if !strings.Contains(out, "ok") {
		r.Failf("4.3 reload did not confirm success: %q", out)
	}

	if st, err := helpers.WaitForStatus(ctx, 8*time.Second, func(m map[string]helpers.ProcStatus) bool {
		d, dok := m["dummy:00"]
		t, tok := m["ticker:00"]
		return dok && tok && d.PID > 0 && t.PID > 0 && d.PID != dummyBefore.PID && t.PID == tickerBefore.PID
	}); err == nil {
		if st["ticker:00"].PID == tickerBefore.PID {
			r.Passf("4.3a ticker:00 kept same PID (%d) — unchanged program not restarted", tickerBefore.PID)
		} else {
			r.Failf("4.3a ticker:00 restarted unexpectedly (was %d, now %d)", tickerBefore.PID, st["ticker:00"].PID)
		}
		if st["dummy:00"].PID != dummyBefore.PID {
			r.Passf("4.3b dummy:00 got new PID (%d) — modified program restarted properly", st["dummy:00"].PID)
		} else {
			r.Failf("4.3b dummy:00 PID unchanged after config change (expected new PID)")
		}
	} else {
		r.Failf("4.3 reload did not settle into expected PID state: %v", err)
	}
}