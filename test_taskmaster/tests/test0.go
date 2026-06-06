package tests

import "taskmaster/test_taskmaster/helpers"

func RunPoint0(ctx *helpers.TestContext, r *helpers.Report) {
	r.Section("TEST 0 — Suite Prerequisites")
	r.Info("Confirms the evaluation harness built successfully and is ready to run.")
	r.Info("Covers evaluation guide sections: control shell, config, logging, hot-reload, config options, scenarios.")
	r.Passf("0.1 Binaries ready: %s + %s", ctx.DaemonPath, ctx.CtlPath)
}
