package main

import (
	"fmt"
	"os"
	"path/filepath"

	"taskmaster/test_taskmaster/config"
	"taskmaster/test_taskmaster/helpers"
	"taskmaster/test_taskmaster/tests"
)

func main() {
	os.Exit(run())
}

func run() int {
	root, err := os.Getwd()
	helpers.Must(err)

	p := helpers.DefaultPrinter
	p.Banner("══════════════════════════════════════════════")
	p.Banner("  Taskmaster Evaluation — End-to-End Suite")
	p.Banner("══════════════════════════════════════════════")

	ctx := &helpers.TestContext{
		Root:       root,
		ConfigPath: filepath.Join(root, config.ConfigFile),
		BackupPath: config.BackupPath(),
		DaemonPath: filepath.Join(root, config.DaemonBin),
		CtlPath:    filepath.Join(root, config.CtlBin),
		LogPath:    filepath.Join(root, config.LogFile),
		SocketPath: config.SocketPath,
	}

	p.Section("Building Binaries")
	if err := helpers.RunCmd(ctx.Root, "go", "build", "-o", ctx.DaemonPath, config.DaemonBuildTarget); err != nil {
		fmt.Printf("Failed to build daemon: %v\n", err)
		return 1
	}
	if err := helpers.RunCmd(ctx.Root, "go", "build", "-o", ctx.CtlPath, config.CtlBuildTarget); err != nil {
		fmt.Printf("Failed to build ctl: %v\n", err)
		return 1
	}
	p.Pass("Build succeeded")

	helpers.Must(helpers.CopyFile(ctx.ConfigPath, ctx.BackupPath))
	defer cleanup(ctx)

	global := helpers.NewReport()

	runTestBlock(ctx, global, "POINT 0", tests.RunPoint0)
	runTestBlock(ctx, global, "POINT 1", tests.RunPoint1)
	runTestBlock(ctx, global, "POINT 2", tests.RunPoint2)
	runTestBlock(ctx, global, "POINT 3", tests.RunPoint3)
	runTestBlock(ctx, global, "POINT 4", tests.RunPoint4)

	p.Banner("══════════════════════════════════════════════")
	p.Banner("  Total Suite Results")
	p.Banner("══════════════════════════════════════════════")
	global.PrintResults("Suite")

	if global.Fail > 0 {
		return 1
	}
	return 0
}

func runTestBlock(ctx *helpers.TestContext, global *helpers.Report, title string, fn func(*helpers.TestContext, *helpers.Report)) {
	_ = helpers.CopyFile(ctx.BackupPath, ctx.ConfigPath)
	helpers.StopDaemon(ctx)
	_ = os.Remove(ctx.SocketPath)
	_ = os.Remove(ctx.LogPath)

	pt := helpers.NewReport()
	if err := helpers.StartDaemon(ctx); err != nil {
		pt.Failf("Failed to start daemon for %s: %v", title, err)
		global.Merge(pt)
		return
	}
	if err := helpers.WaitForCtlReady(ctx, config.DaemonReadyTimeout); err != nil {
		pt.Failf("Daemon did not become ready for %s: %v", title, err)
		global.Merge(pt)
		return
	}

	fn(ctx, pt)
	pt.PrintResults(title)
	global.Merge(pt)
}

func cleanup(ctx *helpers.TestContext) {
	helpers.StopDaemon(ctx)
	_ = helpers.CopyFile(ctx.BackupPath, ctx.ConfigPath)
	_ = os.Remove(ctx.BackupPath)
	_ = os.Remove(ctx.SocketPath)
}
