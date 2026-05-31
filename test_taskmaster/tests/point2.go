package tests

import (
	"strings"
	"time"

	"taskmaster/test_taskmaster/helpers"
)

func RunPoint2(ctx *helpers.TestContext, r *helpers.Report) {
	r.Section("POINT 2 — Configuration File (autostart & numprocs)")

	// Wait briefly for autostart to kick in
	time.Sleep(1 * time.Second)

	// Since config.yml has dummy with numprocs: 3 and autostart: true
	stOut, err := helpers.RunCtl(ctx, "status")
	if err != nil {
		r.Failf("2.1 Failed to get status: %v", err)
	}

	// 1. Check numprocs expansion
	if strings.Contains(stOut, "dummy:00") && strings.Contains(stOut, "dummy:01") && strings.Contains(stOut, "dummy:02") {
		r.Passf("2.1 numprocs expansion works (dummy:00, 01, 02 present)")
	} else {
		r.Failf("2.1 numprocs expansion missing elements in status: %q", stOut)
	}

	// 2. Check autostart works
	stMap, _ := helpers.WaitForStatus(ctx, 2*time.Second, func(m map[string]helpers.ProcStatus) bool {
		p, ok := m["dummy:00"]
		return ok && p.State == "running"
	})

	if p, ok := stMap["dummy:00"]; ok && p.State == "running" {
		r.Passf("2.2 autostart=true program running automatically")
	} else {
		r.Failf("2.2 autostart=true program NOT running automatically")
	}

	// 3. Check autostart=false is respected (e.g. lazy)
	if p, ok := stMap["lazy:00"]; ok {
		if p.State == "stopped" || p.State == "fatal" {
			r.Passf("2.3 autostart=false program is in state: %s", p.State)
		} else {
			r.Failf("2.3 autostart=false program unexpectedly started (state: %s)", p.State)
		}
	} else {
		r.Failf("2.3 lazy:00 process missing from status entirely")
	}
}