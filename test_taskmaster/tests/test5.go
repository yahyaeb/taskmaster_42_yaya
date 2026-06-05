package tests

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"taskmaster/test_taskmaster/config"
	"taskmaster/test_taskmaster/helpers"
)

func RunPoint51(ctx *helpers.TestContext, r *helpers.Report) {
	r.Section("TEST 5.1 — cmd")

	p, err := waitForProc(ctx, "dummy:00", config.DefaultWaitTimeout, func(p helpers.ProcStatus) bool {
		return p.State == "running" && p.PID > 0
	})
	if err != nil {
		r.Failf("5.1 could not find dummy:00 running: %v", err)
		return
	}

	cmdline, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(p.PID), "cmdline"))
	if err != nil {
		r.Failf("5.1 could not read /proc for pid %d: %v", p.PID, err)
		return
	}

	line := strings.ReplaceAll(string(cmdline), "\x00", " ")
	if strings.Contains(line, "sleep") && strings.Contains(line, "999") {
		r.Passf("5.1 cmd launches the configured process (%s)", strings.TrimSpace(line))
		return
	}
	r.Failf("5.1 cmdline does not match expectation: %q", line)
}

func RunPoint52(ctx *helpers.TestContext, r *helpers.Report) {
	r.Section("TEST 5.2 — numprocs")

	out, err := helpers.RunCtl(ctx, "status")
	if err != nil {
		r.Failf("5.2 status failed: %v", err)
		return
	}

	if strings.Contains(out, "dummy:00") && strings.Contains(out, "dummy:01") &&
		strings.Contains(out, "dummy:02") && strings.Contains(out, "ticker:00") &&
		strings.Contains(out, "ticker:01") {
		r.Passf("5.2 numprocs expands configured instances for dummy and ticker")
	} else {
		r.Failf("5.2 numprocs expansion missing expected instances: %q", out)
	}
}

func RunPoint53(ctx *helpers.TestContext, r *helpers.Report) {
	r.Section("TEST 5.3 — autostart")

	lazy, err := waitForProc(ctx, "lazy:00", config.DefaultWaitTimeout, func(p helpers.ProcStatus) bool {
		return p.State == "stopped"
	})
	if err != nil {
		r.Failf("5.3 could not observe lazy:00 stopped: %v", err)
		return
	}

	if lazy.State == "stopped" {
		r.Passf("5.3 autostart=false leaves lazy:00 stopped until started manually")
		return
	}
	r.Failf("5.3 lazy:00 unexpected state: %s", lazy.State)
}

func RunPoint54(ctx *helpers.TestContext, r *helpers.Report) {
	r.Section("TEST 5.4 — autorestart / exitcodes")

	cases := []struct {
		label       string
		autorestart string
		exitcodes   []int
		expectation string
	}{
		{label: "5.4a", autorestart: "never", exitcodes: []int{0}, expectation: "stopped"},
		{label: "5.4b", autorestart: "always", exitcodes: []int{0}, expectation: "restarted"},
		{label: "5.4c", autorestart: "unexpected", exitcodes: []int{1}, expectation: "stopped"},
		{label: "5.4d", autorestart: "unexpected", exitcodes: []int{0}, expectation: "restarted"},
	}

	for _, tc := range cases {
		forceStop(ctx, "lazy:00")
		if err := configureLazyCrashSpec(ctx, 2, 1, 0, tc.autorestart, tc.exitcodes); err != nil {
			r.Failf("%s could not update lazy config: %v", tc.label, err)
			continue
		}
		if err := reloadConfig(ctx); err != nil {
			r.Failf("%s reload failed: %v", tc.label, err)
			continue
		}
		if _, err := helpers.RunCtl(ctx, "start lazy:00"); err != nil {
			r.Failf("%s could not start lazy:00: %v", tc.label, err)
			continue
		}

		first, err := waitForProc(ctx, "lazy:00", config.DefaultWaitTimeout, func(p helpers.ProcStatus) bool {
			return p.State == "running" && p.PID > 0
		})
		if err != nil {
			r.Failf("%s lazy:00 never reached RUNNING: %v", tc.label, err)
			continue
		}

		switch tc.expectation {
		case "stopped":
			if _, err := waitForProc(ctx, "lazy:00", 5*time.Second, func(p helpers.ProcStatus) bool {
				return p.State == "stopped"
			}); err != nil {
				r.Failf("%s expected lazy:00 to stay stopped after exit: %v", tc.label, err)
			} else {
				r.Passf("%s %s stops after an expected/allowed exit", tc.label, tc.autorestart)
			}
		case "restarted":
			if next, err := waitForProc(ctx, "lazy:00", 7*time.Second, func(p helpers.ProcStatus) bool {
				return p.State == "running" && p.PID > 0 && p.PID != first.PID
			}); err != nil {
				r.Failf("%s expected lazy:00 to restart after exit: %v", tc.label, err)
			} else {
				r.Passf("%s %s restarts lazy:00 (%d -> %d)", tc.label, tc.autorestart, first.PID, next.PID)
			}
		}
	}
}

func RunPoint55(ctx *helpers.TestContext, r *helpers.Report) {
	r.Section("TEST 5.5 — starttime")

	forceStop(ctx, "lazy:00")
	if err := configureLazyCrashSpec(ctx, 1, 5, 0, "never", []int{0}); err != nil {
		r.Failf("5.5 could not update lazy config: %v", err)
		return
	}
	if err := reloadConfig(ctx); err != nil {
		r.Failf("5.5 reload failed: %v", err)
		return
	}
	if _, err := helpers.RunCtl(ctx, "start lazy:00"); err != nil {
		r.Failf("5.5 could not start lazy:00: %v", err)
		return
	}

	if p, err := waitForProc(ctx, "lazy:00", 4*time.Second, func(p helpers.ProcStatus) bool {
		return p.State == "fatal"
	}); err == nil {
		r.Passf("5.5 starttime treats early exit as startup failure (state=%s)", strings.ToUpper(p.State))
	} else {
		r.Failf("5.5 expected lazy:00 to fail inside starttime window: %v", err)
	}
}

func RunPoint56(ctx *helpers.TestContext, r *helpers.Report) {
	r.Section("TEST 5.6 — startretries")

	forceStop(ctx, "lazy:00")
	if err := os.WriteFile(ctx.LogPath, []byte{}, 0o644); err != nil {
		r.Failf("5.6 could not clear taskmaster log: %v", err)
		return
	}
	if err := configureLazyCrashSpec(ctx, 1, 5, 2, "always", []int{0}); err != nil {
		r.Failf("5.6 could not update lazy config: %v", err)
		return
	}
	if err := reloadConfig(ctx); err != nil {
		r.Failf("5.6 reload failed: %v", err)
		return
	}
	if _, err := helpers.RunCtl(ctx, "start lazy:00"); err != nil {
		r.Failf("5.6 could not start lazy:00: %v", err)
		return
	}

	if _, err := waitForProc(ctx, "lazy:00", 8*time.Second, func(p helpers.ProcStatus) bool {
		return p.State == "backoff" || p.State == "fatal"
	}); err != nil {
		r.Failf("5.6 lazy:00 never entered a failed-start state: %v", err)
		return
	}

	logText, err := waitForLogContains(ctx.LogPath, "process 'lazy:00' failed to start after 2 attempts", 10*time.Second)
	if err != nil {
		r.Failf("5.6 did not observe the retry-limit log entry: %v", err)
		return
	}

	spawns := strings.Count(logText, "spawned: 'lazy:00'")
	if spawns == 3 {
		r.Passf("5.6 startretries=2 produces three spawn attempts before aborting")
	} else {
		r.Failf("5.6 expected 3 spawn attempts, observed %d", spawns)
	}
}

func RunPoint57(ctx *helpers.TestContext, r *helpers.Report) {
	r.Section("TEST 5.7 — stopsignal")

	forceStop(ctx, "slowstopper:00")
	if err := configureSlowstopperStop(ctx, "USR1", 3); err != nil {
		r.Failf("5.7 could not restore slowstopper USR1 config: %v", err)
		return
	}
	if err := reloadConfig(ctx); err != nil {
		r.Failf("5.7 reload failed: %v", err)
		return
	}
	if err := os.WriteFile(config.SlowstopperStdoutLog, []byte{}, 0o644); err != nil && !os.IsNotExist(err) {
		r.Failf("5.7 could not clear slowstopper stdout log: %v", err)
		return
	}
	if _, err := helpers.RunCtl(ctx, "start slowstopper:00"); err != nil {
		r.Failf("5.7 could not start slowstopper:00: %v", err)
		return
	}
	if _, err := waitForProc(ctx, "slowstopper:00", config.DefaultWaitTimeout, func(p helpers.ProcStatus) bool {
		return p.State == "running" && p.PID > 0
	}); err != nil {
		r.Failf("5.7 slowstopper:00 never reached RUNNING: %v", err)
		return
	}

	_, _ = helpers.RunCtl(ctx, "stop slowstopper:00")
	if _, err := waitForProc(ctx, "slowstopper:00", 4*time.Second, func(p helpers.ProcStatus) bool {
		return p.State == "stopped"
	}); err != nil {
		r.Failf("5.7 slowstopper:00 did not stop with USR1: %v", err)
		return
	}
	usr1Log, _ := os.ReadFile(config.SlowstopperStdoutLog)
	if strings.Contains(string(usr1Log), "caught USR1") {
		r.Passf("5.7 stopsignal=USR1 triggers the graceful handler")
	} else {
		r.Failf("5.7 expected graceful USR1 stop, log=%q", string(usr1Log))
	}

	forceStop(ctx, "slowstopper:00")
	if err := configureSlowstopperStop(ctx, "TERM", 3); err != nil {
		r.Failf("5.7 could not update slowstopper TERM config: %v", err)
		return
	}
	if err := reloadConfig(ctx); err != nil {
		r.Failf("5.7 reload with TERM failed: %v", err)
		return
	}
	if err := os.WriteFile(config.SlowstopperStdoutLog, []byte{}, 0o644); err != nil && !os.IsNotExist(err) {
		r.Failf("5.7 could not clear slowstopper stdout log for TERM case: %v", err)
		return
	}
	if _, err := helpers.RunCtl(ctx, "start slowstopper:00"); err != nil {
		r.Failf("5.7 could not restart slowstopper:00 with TERM: %v", err)
		return
	}
	if _, err := waitForProc(ctx, "slowstopper:00", config.DefaultWaitTimeout, func(p helpers.ProcStatus) bool {
		return p.State == "running" && p.PID > 0
	}); err != nil {
		r.Failf("5.7 slowstopper:00 never reached RUNNING after TERM config: %v", err)
		return
	}

	_, _ = helpers.RunCtl(ctx, "stop slowstopper:00")
	if _, err := waitForProc(ctx, "slowstopper:00", 6*time.Second, func(p helpers.ProcStatus) bool {
		return p.State == "stopped"
	}); err != nil {
		r.Failf("5.7 slowstopper:00 did not stop with TERM: %v", err)
		return
	}
	termLog, _ := os.ReadFile(config.SlowstopperStdoutLog)
	if !strings.Contains(string(termLog), "caught USR1") {
		r.Passf("5.7 changing stopsignal from USR1 to TERM changes the stop behavior")
	} else {
		r.Failf("5.7 TERM stop unexpectedly hit the USR1 handler: %q", string(termLog))
	}
}

func RunPoint58(ctx *helpers.TestContext, r *helpers.Report) {
	r.Section("TEST 5.8 — stoptime")

	forceStop(ctx, "slowstopper:00")
	if err := configureSlowstopperStop(ctx, "TERM", 1); err != nil {
		r.Failf("5.8 could not update slowstopper TERM config: %v", err)
		return
	}
	if err := reloadConfig(ctx); err != nil {
		r.Failf("5.8 reload failed: %v", err)
		return
	}
	if _, err := helpers.RunCtl(ctx, "start slowstopper:00"); err != nil {
		r.Failf("5.8 could not start slowstopper:00: %v", err)
		return
	}
	if _, err := waitForProc(ctx, "slowstopper:00", config.DefaultWaitTimeout, func(p helpers.ProcStatus) bool {
		return p.State == "running" && p.PID > 0
	}); err != nil {
		r.Failf("5.8 slowstopper:00 never reached RUNNING: %v", err)
		return
	}

	start := time.Now()
	_, _ = helpers.RunCtl(ctx, "stop slowstopper:00")
	if _, err := waitForProc(ctx, "slowstopper:00", 5*time.Second, func(p helpers.ProcStatus) bool {
		return p.State == "stopped"
	}); err != nil {
		r.Failf("5.8 slowstopper:00 did not stop after TERM/stoptime: %v", err)
		return
	}

	elapsed := time.Since(start)
	if elapsed >= time.Second && elapsed < 4*time.Second {
		r.Passf("5.8 stoptime=1 forces termination after the grace window (%s)", elapsed.Round(100*time.Millisecond))
	} else {
		r.Failf("5.8 expected TERM stop to complete around the configured stoptime, got %s", elapsed.Round(100*time.Millisecond))
	}
}

func RunPoint59(ctx *helpers.TestContext, r *helpers.Report) {
	r.Section("TEST 5.9 — stdout / stderr")

	forceStop(ctx, "hello42:00")
	_ = os.WriteFile(config.HelloStdoutLog, []byte{}, 0o644)
	_ = os.WriteFile(config.HelloStderrLog, []byte{}, 0o644)

	if _, err := helpers.RunCtl(ctx, "start hello42:00"); err != nil {
		r.Failf("5.9 could not restart hello42:00: %v", err)
		return
	}

	time.Sleep(2 * time.Second)
	stdoutText, errOut := os.ReadFile(config.HelloStdoutLog)
	stderrText, errErr := os.ReadFile(config.HelloStderrLog)

	if errOut == nil && strings.Contains(string(stdoutText), "Hello from stdout!") {
		r.Passf("5.9 stdout redirection writes process output to %s", config.HelloStdoutLog)
	} else {
		r.Failf("5.9 stdout redirection failed: %v %q", errOut, string(stdoutText))
	}

	if errErr == nil && strings.Contains(string(stderrText), "Hello from stderr!") {
		r.Passf("5.9 stderr redirection writes process output to %s", config.HelloStderrLog)
	} else {
		r.Failf("5.9 stderr redirection failed: %v %q", errErr, string(stderrText))
	}
}

func RunPoint510(ctx *helpers.TestContext, r *helpers.Report) {
	r.Section("TEST 5.10 — env")

	out, err := triggerEnvReporter(ctx)
	if err != nil {
		r.Failf("5.10 envreporter did not populate stdout: %v", err)
		return
	}

	if strings.Contains(out, "TM_FOO=hello") &&
		strings.Contains(out, "TM_BAR=42") &&
		strings.Contains(out, "TM_GREETING=salut depuis taskmaster") {
		r.Passf("5.10 environment variables are injected into envreporter")
	} else {
		r.Failf("5.10 envreporter output missing configured TM_* variables: %q", out)
	}
}

func RunPoint511(ctx *helpers.TestContext, r *helpers.Report) {
	r.Section("TEST 5.11 — workingdir")

	out, err := triggerEnvReporter(ctx)
	if err != nil {
		r.Failf("5.11 envreporter did not populate stdout: %v", err)
		return
	}

	if strings.Contains(out, "/tmp") {
		r.Passf("5.11 workingdir is applied to envreporter")
	} else {
		r.Failf("5.11 expected envreporter output to include workingdir /tmp, got %q", out)
	}
}

func RunPoint512(ctx *helpers.TestContext, r *helpers.Report) {
	r.Section("TEST 5.12 — umask")

	_, err := triggerEnvReporter(ctx)
	if err != nil {
		r.Failf("5.12 envreporter did not populate stdout: %v", err)
		return
	}

	info, err := waitForFileStat(config.EnvReporterProbePath, 4*time.Second)
	if err != nil {
		r.Failf("5.12 envreporter probe file was not created: %v", err)
		return
	}

	if info.Mode().Perm() == 0o600 {
		r.Passf("5.12 umask=077 produces a 0600 probe file")
	} else {
		r.Failf("5.12 expected umask=077 to create 0600 permissions, got %s", info.Mode().Perm())
	}
}

func RunBonus1(ctx *helpers.TestContext, r *helpers.Report) {
	r.Section("BONUS 1 — uid / gid")

	forceStop(ctx, "server:00")
	_ = os.Remove(config.ServerUIDOut)
	_ = os.Remove(config.ServerGIDOut)
	_ = os.Remove(config.ServerUIDGIDProbe)

	if err := configureBonusIdentityProbe(ctx); err != nil {
		r.Failf("bonus 1 could not update bonus probe config: %v", err)
		return
	}
	if err := reloadConfig(ctx); err != nil {
		r.Failf("bonus 1 reload failed: %v", err)
		return
	}
	if _, err := helpers.RunCtl(ctx, "start server:00"); err != nil {
		r.Failf("bonus 1 could not start server:00: %v", err)
		return
	}

	p, err := waitForProc(ctx, "server:00", config.DefaultWaitTimeout, func(p helpers.ProcStatus) bool {
		return p.State == "running" && p.PID > 0
	})
	if err != nil {
		r.Failf("bonus 1 server:00 never reached RUNNING: %v", err)
		return
	}

	uid, gid, err := readProcIdentity(p.PID)
	if err != nil {
		r.Failf("bonus 1 could not read process identity from /proc: %v", err)
		return
	}
	if uid == 1000 && gid == 1000 {
		r.Passf("bonus 1 process is running with configured uid/gid 1000:1000")
	} else {
		r.Failf("bonus 1 expected uid/gid 1000:1000, got %d:%d", uid, gid)
	}

	uidText, err := waitForFileContains(config.ServerUIDOut, "1000", 4*time.Second)
	if err != nil {
		r.Failf("bonus 1 uid output file was not written: %v", err)
		return
	}
	gidText, err := waitForFileContains(config.ServerGIDOut, "1000", 4*time.Second)
	if err != nil {
		r.Failf("bonus 1 gid output file was not written: %v", err)
		return
	}
	if strings.Contains(uidText, "1000") && strings.Contains(gidText, "1000") {
		r.Passf("bonus 1 child process reports uid/gid 1000 from inside the supervised command")
	} else {
		r.Failf("bonus 1 child reported unexpected uid/gid values: uid=%q gid=%q", uidText, gidText)
	}

	info, err := waitForFileStat(config.ServerUIDGIDProbe, 4*time.Second)
	if err != nil {
		r.Failf("bonus 1 probe file was not created: %v", err)
		return
	}

	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		r.Failf("bonus 1 could not inspect probe ownership")
		return
	}
	if stat.Uid == 1000 && stat.Gid == 1000 {
		r.Passf("bonus 1 files created by the supervised process are owned by uid/gid 1000:1000")
	} else {
		r.Failf("bonus 1 expected probe ownership 1000:1000, got %d:%d", stat.Uid, stat.Gid)
	}
}

func configureLazyCrashSpec(ctx *helpers.TestContext, delay, starttime, startretries int, autorestart string, exitcodes []int) error {
	return helpers.UpdateProgramBlock(ctx.ConfigPath, "lazy", func(block []string) ([]string, error) {
		var err error
		block, err = helpers.ReplaceBlockValue(block, "cmd", fmt.Sprintf("    cmd: [\"./testprograms/crasher/crasher\", %q]", strconv.Itoa(delay)))
		if err != nil {
			return nil, err
		}
		block, err = helpers.ReplaceBlockValue(block, "autorestart", fmt.Sprintf("    autorestart: %s", autorestart))
		if err != nil {
			return nil, err
		}
		block, err = helpers.ReplaceBlockValue(block, "exitcodes", fmt.Sprintf("    exitcodes: [%s]", joinInts(exitcodes)))
		if err != nil {
			return nil, err
		}
		block, err = helpers.ReplaceBlockValue(block, "startretries", fmt.Sprintf("    startretries: %d", startretries))
		if err != nil {
			return nil, err
		}
		block, err = helpers.ReplaceBlockValue(block, "starttime", fmt.Sprintf("    starttime: %d", starttime))
		if err != nil {
			return nil, err
		}
		block, err = helpers.ReplaceBlockValue(block, "stdout", "    stdout: /tmp/lazy.stdout")
		if err != nil {
			return nil, err
		}
		block, err = helpers.ReplaceBlockValue(block, "stderr", "    stderr: /tmp/lazy.stderr")
		if err != nil {
			return nil, err
		}
		block, err = helpers.ReplaceBlockValue(block, "workingdir", "    workingdir: .")
		if err != nil {
			return nil, err
		}
		return block, nil
	})
}

func configureSlowstopperStop(ctx *helpers.TestContext, signal string, stoptime int) error {
	return helpers.UpdateProgramBlock(ctx.ConfigPath, "slowstopper", func(block []string) ([]string, error) {
		var err error
		block, err = helpers.ReplaceBlockValue(block, "stopsignal", fmt.Sprintf("    stopsignal: %s", signal))
		if err != nil {
			return nil, err
		}
		block, err = helpers.ReplaceBlockValue(block, "stoptime", fmt.Sprintf("    stoptime: %d", stoptime))
		if err != nil {
			return nil, err
		}
		return block, nil
	})
}

func configureBonusIdentityProbe(ctx *helpers.TestContext) error {
	return helpers.UpdateProgramBlock(ctx.ConfigPath, "server", func(block []string) ([]string, error) {
		var err error
		block, err = helpers.ReplaceBlockValue(block, "uid", "    uid: 1000")
		if err != nil {
			return nil, err
		}
		block, err = helpers.ReplaceBlockValue(block, "gid", "    gid: 1000")
		if err != nil {
			return nil, err
		}
		block, err = helpers.ReplaceBlockValue(block, "cmd", "    cmd: [\"sh\", \"-c\", \"id -u > /tmp/server.uid.out; id -g > /tmp/server.gid.out; touch /tmp/server.uidgid_probe; exec sleep 30\"]")
		if err != nil {
			return nil, err
		}
		block, err = helpers.ReplaceBlockValue(block, "workingdir", "    workingdir: /tmp")
		if err != nil {
			return nil, err
		}
		block, err = helpers.ReplaceBlockValue(block, "umask", "    umask: 0")
		if err != nil {
			return nil, err
		}
		block, err = helpers.ReplaceBlockValue(block, "autostart", "    autostart: false")
		if err != nil {
			return nil, err
		}
		block, err = helpers.ReplaceBlockValue(block, "autorestart", "    autorestart: never")
		if err != nil {
			return nil, err
		}
		block, err = helpers.ReplaceBlockValue(block, "starttime", "    starttime: 1")
		if err != nil {
			return nil, err
		}
		block, err = helpers.ReplaceBlockValue(block, "startretries", "    startretries: 0")
		if err != nil {
			return nil, err
		}
		return block, nil
	})
}

func triggerEnvReporter(ctx *helpers.TestContext) (string, error) {
	if err := configureEnvReporterProbe(ctx); err != nil {
		return "", err
	}
	if err := reloadConfig(ctx); err != nil {
		return "", err
	}
	forceStop(ctx, "envreporter:00")
	_ = os.Remove(config.EnvReporterProbePath)
	_ = os.WriteFile(config.EnvReporterStdoutLog, []byte{}, 0o644)
	_ = os.WriteFile(config.EnvReporterStderrLog, []byte{}, 0o644)

	if _, err := helpers.RunCtl(ctx, "start envreporter:00"); err != nil {
		return "", err
	}

	return waitForFileContains(config.EnvReporterStdoutLog, "TM_FOO=hello", 4*time.Second)
}

func configureEnvReporterProbe(ctx *helpers.TestContext) error {
	absCmd := filepath.Join(ctx.Root, "testprograms", "envreporter", "envreporter")
	return helpers.UpdateProgramBlock(ctx.ConfigPath, "envreporter", func(block []string) ([]string, error) {
		var err error
		block, err = helpers.ReplaceBlockValue(block, "cmd", fmt.Sprintf("    cmd: [%q]", absCmd))
		if err != nil {
			return nil, err
		}
		return block, nil
	})
}

func reloadConfig(ctx *helpers.TestContext) error {
	out, err := helpers.RunCtl(ctx, "reload")
	if err != nil {
		return err
	}
	if !strings.Contains(out, "ok") {
		return fmt.Errorf("reload did not confirm success: %q", out)
	}
	return nil
}

func forceStop(ctx *helpers.TestContext, name string) {
	_, _ = helpers.RunCtl(ctx, "stop "+name)
	_, _ = helpers.WaitForStatus(ctx, 5*time.Second, func(m map[string]helpers.ProcStatus) bool {
		p, ok := m[name]
		if !ok {
			return false
		}
		return p.State != "running" && p.State != "starting" && p.State != "backoff"
	})
}

func waitForProc(ctx *helpers.TestContext, name string, timeout time.Duration, predicate func(helpers.ProcStatus) bool) (helpers.ProcStatus, error) {
	st, err := helpers.WaitForStatus(ctx, timeout, func(m map[string]helpers.ProcStatus) bool {
		p, ok := m[name]
		return ok && predicate(p)
	})
	if err != nil {
		return helpers.ProcStatus{}, err
	}
	return st[name], nil
}

func waitForLogContains(path, needle string, timeout time.Duration) (string, error) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		data, err := os.ReadFile(path)
		if err == nil {
			text := string(data)
			if strings.Contains(text, needle) {
				return text, nil
			}
		}
		time.Sleep(200 * time.Millisecond)
	}
	return "", fmt.Errorf("log %s never contained %q", path, needle)
}

func waitForFileContains(path, needle string, timeout time.Duration) (string, error) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		data, err := os.ReadFile(path)
		if err == nil {
			text := string(data)
			if strings.Contains(text, needle) {
				return text, nil
			}
		}
		time.Sleep(200 * time.Millisecond)
	}
	return "", fmt.Errorf("file %s never contained %q", path, needle)
}

func waitForFileStat(path string, timeout time.Duration) (os.FileInfo, error) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		info, err := os.Stat(path)
		if err == nil {
			return info, nil
		}
		time.Sleep(200 * time.Millisecond)
	}
	return nil, fmt.Errorf("file %s was not created in time", path)
}

func readProcIdentity(pid int) (uint32, uint32, error) {
	data, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "status"))
	if err != nil {
		return 0, 0, err
	}

	var uid uint32
	var gid uint32
	var haveUID bool
	var haveGID bool

	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "Uid:") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				v, err := strconv.ParseUint(fields[1], 10, 32)
				if err != nil {
					return 0, 0, err
				}
				uid = uint32(v)
				haveUID = true
			}
		}
		if strings.HasPrefix(line, "Gid:") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				v, err := strconv.ParseUint(fields[1], 10, 32)
				if err != nil {
					return 0, 0, err
				}
				gid = uint32(v)
				haveGID = true
			}
		}
	}

	if !haveUID || !haveGID {
		return 0, 0, fmt.Errorf("could not parse uid/gid from /proc/%d/status", pid)
	}

	return uid, gid, nil
}

func joinInts(values []int) string {
	out := make([]string, 0, len(values))
	for _, v := range values {
		out = append(out, strconv.Itoa(v))
	}
	return strings.Join(out, ", ")
}
