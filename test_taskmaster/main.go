package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"

	"taskmaster/test_taskmaster/helpers"
	"taskmaster/test_taskmaster/tests"
)

func main() {
	os.Exit(run())
}

func run() int {
	root, err := os.Getwd()
	helpers.Must(err)

	fmt.Printf("%s══════════════════════════════════════════════%s\n", helpers.AnsiBold, helpers.AnsiNC)
	fmt.Printf("%s  Taskmaster Evaluation — End-to-End Suite%s\n", helpers.AnsiBold, helpers.AnsiNC)
	fmt.Printf("%s══════════════════════════════════════════════%s\n", helpers.AnsiBold, helpers.AnsiNC)

	ctx := &helpers.TestContext{
		Root:       root,
		ConfigPath: filepath.Join(root, "config.yml"),
		BackupPath: filepath.Join(os.TempDir(), "taskmaster_config_backup.yml"),
		DaemonPath: filepath.Join(root, "taskmasterd"),
		CtlPath:    filepath.Join(root, "taskmasterctl"),
		LogPath:    filepath.Join(root, "taskmaster.log"),
		SocketPath: "/tmp/taskmaster.sock",
	}

	fmt.Printf("\n%s━━━ Building Binaries ━━━%s\n", helpers.AnsiBold, helpers.AnsiNC)
	if err := helpers.RunCmd(ctx.Root, "go", "build", "-o", ctx.DaemonPath, "./cmd/daemon"); err != nil {
		fmt.Printf("Failed to build daemon: %v\n", err)
		return 1
	}
	if err := helpers.RunCmd(ctx.Root, "go", "build", "-o", ctx.CtlPath, "./cmd/ctl"); err != nil {
		fmt.Printf("Failed to build ctl: %v\n", err)
		return 1
	}
	fmt.Printf("  %s✓%s Build succeeded\n", helpers.AnsiGreen, helpers.AnsiNC)

	helpers.Must(helpers.CopyFile(ctx.ConfigPath, ctx.BackupPath))
	defer cleanup(ctx)

	r := &helpers.Report{}

	// Point 0
	runTestBlock(ctx, r, "POINT 0", tests.RunPoint0)

	// Point 1
	runTestBlock(ctx, r, "POINT 1", tests.RunPoint1)

	// Point 2
	runTestBlock(ctx, r, "POINT 2", tests.RunPoint2)

	// Point 3
	runTestBlock(ctx, r, "POINT 3", tests.RunPoint3)

	// Point 4
	runTestBlock(ctx, r, "POINT 4", tests.RunPoint4)

	// Final Results
	fmt.Printf("\n%s══════════════════════════════════════════════%s\n", helpers.AnsiBold, helpers.AnsiNC)
	fmt.Printf("%s  Total Suite Results%s\n", helpers.AnsiBold, helpers.AnsiNC)
	fmt.Printf("%s══════════════════════════════════════════════%s\n", helpers.AnsiBold, helpers.AnsiNC)
	fmt.Printf("  %sPassed:%s %d\n", helpers.AnsiGreen, helpers.AnsiNC, r.Pass)
	fmt.Printf("  %sFailed:%s %d\n", helpers.AnsiRed, helpers.AnsiNC, r.Fail)
	fmt.Printf("  Total:    %d\n\n", r.Total)

	if r.Fail > 0 {
		return 1
	}
	return 0
}

func runTestBlock(ctx *helpers.TestContext, globalReport *helpers.Report, title string, testFunc func(*helpers.TestContext, *helpers.Report)) {
	// Restore config before each point
	_ = helpers.CopyFile(ctx.BackupPath, ctx.ConfigPath)

	// Clean slate
	stopDaemon(ctx)
	_ = os.Remove(ctx.SocketPath)
	_ = os.Remove(ctx.LogPath)

	pointReport := &helpers.Report{}
	if err := helpers.StartDaemon(ctx); err != nil {
		pointReport.Failf("Failed to start daemon for %s: %v", title, err)
		mergeReports(globalReport, pointReport)
		return
	}

	if err := helpers.WaitForCtlReady(ctx, 15*time.Second); err != nil {
		pointReport.Failf("Daemon did not become ready for %s: %v", title, err)
		mergeReports(globalReport, pointReport)
		return
	}

	testFunc(ctx, pointReport)

	pointReport.PrintResults(title)
	mergeReports(globalReport, pointReport)
}

func mergeReports(dest, src *helpers.Report) {
	dest.Pass += src.Pass
	dest.Fail += src.Fail
	dest.Total += src.Total
}

func stopDaemon(ctx *helpers.TestContext) {
	if ctx.Daemon != nil && ctx.Daemon.Process != nil {
		_ = ctx.Daemon.Process.Signal(syscall.SIGTERM)
		_, _ = ctx.Daemon.Process.Wait()
		ctx.Daemon = nil
	}
	// fallback if daemon escaped
	exec.Command("pkill", "-f", "taskmasterd").Run()
}

func cleanup(ctx *helpers.TestContext) {
	stopDaemon(ctx)
	_ = helpers.CopyFile(ctx.BackupPath, ctx.ConfigPath)
	_ = os.Remove(ctx.BackupPath)
	_ = os.Remove(ctx.SocketPath)
}
