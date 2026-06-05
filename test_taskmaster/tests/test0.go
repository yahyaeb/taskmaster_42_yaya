package tests

import "taskmaster/test_taskmaster/helpers"

func RunPoint0(ctx *helpers.TestContext, r *helpers.Report) {
	r.Section("TEST 0 — Before starting")
	r.Passf("0.1 Build succeeded")
}
