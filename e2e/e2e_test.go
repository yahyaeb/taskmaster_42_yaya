package e2e_test

import (
	"fmt"
	"os"
	"strings"
	"syscall"
	"testing"
	"time"
)

// ─── Helpers ────────────────────────────────────────────────────────────────

// sleeperConf returns a config with a long-running /bin/sleep process.
// autostart controls whether it starts automatically.
func sleeperConf(autostart bool) string {
	as := "false"
	if autostart {
		as = "true"
	}
	return fmt.Sprintf(`
programs:
  sleeper:
    cmd: "/bin/sleep 9999"
    numprocs: 1
    autostart: %s
    autorestart: unexpected
    exitcodes: [0]
    startretries: 3
    starttime: 1
    stopsignal: TERM
    stoptime: 5
`, as)
}

// ─── Control Shell: GetStatus ────────────────────────────────────────────────

func Test01_should_return_process_list_when_GetStatus_called(t *testing.T) {
	startDaemon(t, sleeperConf(false))

	resp := rpcCall(t, "Taskmaster.GetStatus", nil)
	assertSuccess(t, resp)

	result, ok := resp["result"].([]any)
	if !ok || len(result) == 0 {
		t.Fatalf("expected non-empty process list, got: %v", resp["result"])
	}
}

func Test02_should_include_instance_name_in_status_result(t *testing.T) {
	startDaemon(t, sleeperConf(false))

	resp := rpcCall(t, "Taskmaster.GetStatus", nil)
	assertSuccess(t, resp)

	list := resp["result"].([]any)
	proc := list[0].(map[string]any)
	// FormatInstanceName("sleeper", 0) → "sleeper:00"
	if proc["name"] != "sleeper:00" {
		t.Fatalf("expected name 'sleeper:00', got %v", proc["name"])
	}
}

// ─── Control Shell: Start ────────────────────────────────────────────────────

func Test03_should_return_ProcessNotFound_when_Start_called_with_unknown_process(t *testing.T) {
	startDaemon(t, sleeperConf(false))

	resp := rpcCall(t, "Taskmaster.Start", map[string]any{"name": "ghost:00"})
	assertErrorCode(t, resp, -32001)
}

func Test04_should_return_success_when_Start_called_with_known_process(t *testing.T) {
	startDaemon(t, sleeperConf(false))

	resp := rpcCall(t, "Taskmaster.Start", map[string]any{"name": "sleeper:00"})
	assertSuccess(t, resp)

	result := resp["result"].(map[string]any)
	if result["success"] != true {
		t.Fatalf("expected success=true, got: %v", result)
	}
}

func Test05_should_reach_running_status_when_process_started(t *testing.T) {
	startDaemon(t, sleeperConf(false))

	rpcCall(t, "Taskmaster.Start", map[string]any{"name": "sleeper:00"})
	status := pollStatus(t, "sleeper:00", "running", 5*time.Second)
	if status != "running" {
		t.Fatalf("expected 'running', got %q", status)
	}
}

// ─── Control Shell: Stop ─────────────────────────────────────────────────────

func Test06_should_return_ProcessNotFound_when_Stop_called_with_unknown_process(t *testing.T) {
	startDaemon(t, sleeperConf(false))

	resp := rpcCall(t, "Taskmaster.Stop", map[string]any{"name": "ghost:00"})
	assertErrorCode(t, resp, -32001)
}

func Test07_should_return_success_when_Stop_called_after_Start(t *testing.T) {
	startDaemon(t, sleeperConf(false))

	rpcCall(t, "Taskmaster.Start", map[string]any{"name": "sleeper:00"})
	status := pollStatus(t, "sleeper:00", "running", 5*time.Second)
	if status != "running" {
		t.Fatalf("expected 'running', got %q", status)
	}

	resp := rpcCall(t, "Taskmaster.Stop", map[string]any{"name": "sleeper:00"})
	assertSuccess(t, resp)

	status = pollStatus(t, "sleeper:00", "stopped", 5*time.Second)
	if status != "stopped" {
		t.Fatalf("expected 'stopped', got %q", status)
	}
}

// ─── Control Shell: Restart ──────────────────────────────────────────────────

func Test08_should_return_ProcessNotFound_when_Restart_called_with_unknown_process(t *testing.T) {
	startDaemon(t, sleeperConf(false))

	resp := rpcCall(t, "Taskmaster.Restart", map[string]any{"name": "nobody:00"})
	assertErrorCode(t, resp, -32001)
}

func Test09_should_assign_new_pid_when_Restart_called_on_running_process(t *testing.T) {
	startDaemon(t, sleeperConf(false))

	rpcCall(t, "Taskmaster.Start", map[string]any{"name": "sleeper:00"})
	pollStatus(t, "sleeper:00", "running", 5*time.Second)
	pidBefore := pidOf(t, "sleeper:00")

	rpcCall(t, "Taskmaster.Restart", map[string]any{"name": "sleeper:00"})
	pollStatus(t, "sleeper:00", "running", 5*time.Second)
	pidAfter := pidOf(t, "sleeper:00")

	if pidBefore == 0 || pidAfter == 0 || pidBefore == pidAfter {
		t.Fatalf("expected different PIDs before/after restart: %d → %d", pidBefore, pidAfter)
	}
}

// ─── Config: autostart ───────────────────────────────────────────────────────

func Test10_should_start_process_automatically_when_autostart_is_true(t *testing.T) {
	startDaemon(t, sleeperConf(true))

	status := pollStatus(t, "sleeper:00", "running", 5*time.Second)
	if status != "running" {
		t.Fatalf("autostart=true: expected 'running', got %q", status)
	}
}

func Test11_should_not_start_process_when_autostart_is_false(t *testing.T) {
	startDaemon(t, sleeperConf(false))

	time.Sleep(500 * time.Millisecond)
	resp := rpcCall(t, "Taskmaster.GetStatus", nil)
	assertSuccess(t, resp)
	list := resp["result"].([]any)
	proc := list[0].(map[string]any)
	status, _ := proc["status"].(string)
	if status == "running" {
		t.Fatalf("autostart=false: expected not 'running', got %q", status)
	}
}

// ─── Config: numprocs ────────────────────────────────────────────────────────

func Test12_should_create_N_instances_when_numprocs_is_N(t *testing.T) {
	cfg := `
programs:
  worker:
    cmd: "/bin/sleep 9999"
    numprocs: 3
    autostart: false
    autorestart: never
    exitcodes: [0]
    startretries: 0
    starttime: 0
    stopsignal: TERM
    stoptime: 1
`
	startDaemon(t, cfg)

	resp := rpcCall(t, "Taskmaster.GetStatus", nil)
	assertSuccess(t, resp)
	list, _ := resp["result"].([]any)
	if len(list) != 3 {
		t.Fatalf("numprocs=3: expected 3 instances, got %d", len(list))
	}
	// Verify names: worker:00, worker:01, worker:02
	names := make(map[string]bool)
	for _, item := range list {
		p := item.(map[string]any)
		names[p["name"].(string)] = true
	}
	for _, want := range []string{"worker:00", "worker:01", "worker:02"} {
		if !names[want] {
			t.Errorf("numprocs=3: missing instance %q in status", want)
		}
	}
}

// ─── Config: autorestart=always ──────────────────────────────────────────────

func Test13_should_restart_when_autorestart_is_always_and_process_exits_cleanly(t *testing.T) {
	cfg := fmt.Sprintf(`
programs:
  exiter:
    cmd: "%s"
    numprocs: 1
    autostart: true
    autorestart: always
    exitcodes: [0]
    startretries: 5
    starttime: 0
    stopsignal: TERM
    stoptime: 1
    env:
      EXIT_CODE: "0"
`, bins.crasher)

	startDaemon(t, cfg)

	// Process exits 0 (expected), but autorestart=always → should keep restarting.
	// After 2s it should have restarted at least once and be in starting/running/backoff.
	time.Sleep(2 * time.Second)
	resp := rpcCall(t, "Taskmaster.GetStatus", nil)
	assertSuccess(t, resp)
	list := resp["result"].([]any)
	proc := list[0].(map[string]any)
	status, _ := proc["status"].(string)
	if status == "stopped" || status == "fatal" {
		t.Fatalf("autorestart=always: expected process to be restarting, got %q", status)
	}
}

// ─── Config: autorestart=never ───────────────────────────────────────────────

func Test14_should_not_restart_when_autorestart_is_never(t *testing.T) {
	cfg := fmt.Sprintf(`
programs:
  exiter:
    cmd: "%s"
    numprocs: 1
    autostart: true
    autorestart: never
    exitcodes: [0]
    startretries: 0
    starttime: 0
    stopsignal: TERM
    stoptime: 1
    env:
      EXIT_CODE: "1"
`, bins.crasher)

	startDaemon(t, cfg)

	// Process exits, autorestart=never → must stay stopped/fatal, never running again.
	time.Sleep(1 * time.Second)
	status := pollStatus(t, "exiter:00", "stopped", 3*time.Second)
	// accept stopped or fatal, but NOT running/starting/backoff
	if status == "running" || status == "starting" || status == "backoff" {
		t.Fatalf("autorestart=never: expected stopped/fatal, got %q", status)
	}
}

// ─── Config: autorestart=unexpected ─────────────────────────────────────────

func Test15_should_restart_when_autorestart_is_unexpected_and_exit_code_not_in_exitcodes(t *testing.T) {
	cfg := fmt.Sprintf(`
programs:
  exiter:
    cmd: "%s"
    numprocs: 1
    autostart: true
    autorestart: unexpected
    exitcodes: [0]
    startretries: 5
    starttime: 0
    stopsignal: TERM
    stoptime: 1
    env:
      EXIT_CODE: "1"
`, bins.crasher)

	startDaemon(t, cfg)

	// exits 1, not in exitcodes [0] → unexpected → should restart (backoff/starting/running)
	time.Sleep(500 * time.Millisecond)
	resp := rpcCall(t, "Taskmaster.GetStatus", nil)
	assertSuccess(t, resp)
	list := resp["result"].([]any)
	proc := list[0].(map[string]any)
	status, _ := proc["status"].(string)
	if status == "stopped" {
		t.Fatalf("autorestart=unexpected with unexpected exit: expected restart activity, got 'stopped'")
	}
}

func Test16_should_not_restart_when_autorestart_is_unexpected_and_exit_code_in_exitcodes(t *testing.T) {
	cfg := fmt.Sprintf(`
programs:
  exiter:
    cmd: "%s"
    numprocs: 1
    autostart: true
    autorestart: unexpected
    exitcodes: [42]
    startretries: 0
    starttime: 0
    stopsignal: TERM
    stoptime: 1
    env:
      EXIT_CODE: "42"
`, bins.crasher)

	startDaemon(t, cfg)

	// exits 42, which IS in exitcodes [42] → expected → should NOT restart
	time.Sleep(1 * time.Second)
	resp := rpcCall(t, "Taskmaster.GetStatus", nil)
	assertSuccess(t, resp)
	list := resp["result"].([]any)
	proc := list[0].(map[string]any)
	status, _ := proc["status"].(string)
	if status == "running" || status == "starting" {
		t.Fatalf("autorestart=unexpected with expected exit: got %q, want stopped", status)
	}
}

// ─── Config: startretries (abort after N failures) ───────────────────────────

func Test17_should_reach_fatal_when_process_exceeds_startretries(t *testing.T) {
	cfg := fmt.Sprintf(`
programs:
  crasher:
    cmd: "%s"
    numprocs: 1
    autostart: true
    autorestart: unexpected
    exitcodes: [0]
    startretries: 2
    starttime: 0
    stopsignal: TERM
    stoptime: 1
    env:
      EXIT_CODE: "1"
`, bins.crasher)

	startDaemon(t, cfg)

	// 2 retries → 3 total attempts → fatal
	status := pollStatus(t, "crasher:00", "fatal", 10*time.Second)
	if status != "fatal" {
		t.Fatalf("startretries=2: expected 'fatal' after exhaustion, got %q", status)
	}
}

// ─── Config: starttime ───────────────────────────────────────────────────────

func Test18_should_be_running_after_starttime_elapses_when_process_survives(t *testing.T) {
	cfg := `
programs:
  sleeper:
    cmd: "/bin/sleep 9999"
    numprocs: 1
    autostart: true
    autorestart: unexpected
    exitcodes: [0]
    startretries: 0
    starttime: 1
    stopsignal: TERM
    stoptime: 1
`
	startDaemon(t, cfg)

	// Should NOT be running immediately (starttime=1 means 1s observation window)
	time.Sleep(300 * time.Millisecond)
	resp := rpcCall(t, "Taskmaster.GetStatus", nil)
	assertSuccess(t, resp)
	list := resp["result"].([]any)
	proc := list[0].(map[string]any)
	early, _ := proc["status"].(string)
	if early == "running" {
		t.Log("note: process already running before starttime elapsed (may be a fast system)")
	}

	// After starttime has elapsed, must be running
	status := pollStatus(t, "sleeper:00", "running", 5*time.Second)
	if status != "running" {
		t.Fatalf("starttime=1: expected 'running' after 1s, got %q", status)
	}
}

// ─── Config: stopsignal ──────────────────────────────────────────────────────

func Test19_should_deliver_configured_stopsignal_when_Stop_called(t *testing.T) {
	logPath := fmt.Sprintf("/tmp/tm_signal_log_%d.txt", time.Now().UnixNano())
	defer os.Remove(logPath)

	cfg := fmt.Sprintf(`
programs:
  trap:
    cmd: "%s"
    numprocs: 1
    autostart: true
    autorestart: never
    exitcodes: [0]
    startretries: 0
    starttime: 1
    stopsignal: TERM
    stoptime: 3
    env:
      SIGNAL_LOG: "%s"
      SIGNAL_ACTION: "exit"
`, bins.signalTrap, logPath)

	startDaemon(t, cfg)
	pollStatus(t, "trap:00", "running", 5*time.Second)

	rpcCall(t, "Taskmaster.Stop", map[string]any{"name": "trap:00"})

	content := readFileWithTimeout(logPath, 5*time.Second)
	if !strings.Contains(content, "RECEIVED:terminated") && !strings.Contains(content, "RECEIVED:hangup") {
		t.Fatalf("stopsignal=TERM: expected SIGTERM in signal log, got: %q", content)
	}
}

// ─── Config: stoptime (escalate to SIGKILL) ──────────────────────────────────

func Test20_should_kill_process_after_stoptime_when_process_ignores_stopsignal(t *testing.T) {
	logPath := fmt.Sprintf("/tmp/tm_signal_log_%d.txt", time.Now().UnixNano())
	defer os.Remove(logPath)

	cfg := fmt.Sprintf(`
programs:
  stubborn:
    cmd: "%s"
    numprocs: 1
    autostart: true
    autorestart: never
    exitcodes: [0]
    startretries: 0
    starttime: 1
    stopsignal: TERM
    stoptime: 1
    env:
      SIGNAL_LOG: "%s"
      SIGNAL_ACTION: "ignore"
`, bins.signalTrap, logPath)

	d := startDaemon(t, cfg)
	_ = d
	pollStatus(t, "stubborn:00", "running", 5*time.Second)
	pid := pidOf(t, "stubborn:00")

	rpcCall(t, "Taskmaster.Stop", map[string]any{"name": "stubborn:00"})

	// Process ignores SIGTERM; after stoptime=1s, daemon must SIGKILL it.
	// Give 3s total: 1s stoptime + 2s buffer.
	deadline := time.Now().Add(3 * time.Second)
	dead := false
	for time.Now().Before(deadline) {
		if err := syscall.Kill(pid, 0); err != nil {
			dead = true
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	if !dead {
		t.Fatalf("stoptime=1: process %d still alive after stoptime+buffer, expected SIGKILL", pid)
	}
}

// ─── Config: stdout redirection ──────────────────────────────────────────────

func Test21_should_redirect_stdout_to_file_when_stdout_configured(t *testing.T) {
	outPath := fmt.Sprintf("/tmp/tm_stdout_%d.txt", time.Now().UnixNano())
	defer os.Remove(outPath)

	cfg := fmt.Sprintf(`
programs:
  printer:
    cmd: "%s"
    numprocs: 1
    autostart: true
    autorestart: never
    exitcodes: [0]
    startretries: 0
    starttime: 1
    stopsignal: TERM
    stoptime: 1
    stdout: "%s"
`, bins.printer, outPath)

	startDaemon(t, cfg)
	pollStatus(t, "printer:00", "running", 5*time.Second)

	content := readFileWithTimeout(outPath, 5*time.Second)
	if content == "" {
		t.Fatal("stdout redirection: expected output file to be non-empty")
	}
	if !strings.Contains(content, "CWD:") {
		t.Fatalf("stdout redirection: expected 'CWD:' line in output, got: %q", content)
	}
}

// ─── Config: env variables ───────────────────────────────────────────────────

func Test22_should_inject_env_variables_into_process_when_env_configured(t *testing.T) {
	outPath := fmt.Sprintf("/tmp/tm_env_%d.txt", time.Now().UnixNano())
	defer os.Remove(outPath)

	cfg := fmt.Sprintf(`
programs:
  printer:
    cmd: "%s"
    numprocs: 1
    autostart: true
    autorestart: never
    exitcodes: [0]
    startretries: 0
    starttime: 1
    stopsignal: TERM
    stoptime: 1
    stdout: "%s"
    env:
      TM_TEST_KEY: "hello_from_taskmaster"
`, bins.printer, outPath)

	startDaemon(t, cfg)
	pollStatus(t, "printer:00", "running", 5*time.Second)

	content := readFileWithTimeout(outPath, 5*time.Second)
	if !strings.Contains(content, "ENV:TM_TEST_KEY=hello_from_taskmaster") {
		t.Fatalf("env: expected 'ENV:TM_TEST_KEY=hello_from_taskmaster' in output, got: %q", content)
	}
}

// ─── Config: workingdir ──────────────────────────────────────────────────────

func Test23_should_set_working_directory_of_process_when_workingdir_configured(t *testing.T) {
	outPath := fmt.Sprintf("/tmp/tm_cwd_%d.txt", time.Now().UnixNano())
	defer os.Remove(outPath)

	cfg := fmt.Sprintf(`
programs:
  printer:
    cmd: "%s"
    numprocs: 1
    autostart: true
    autorestart: never
    exitcodes: [0]
    startretries: 0
    starttime: 1
    stopsignal: TERM
    stoptime: 1
    workingdir: /tmp
    stdout: "%s"
`, bins.printer, outPath)

	startDaemon(t, cfg)
	pollStatus(t, "printer:00", "running", 5*time.Second)

	content := readFileWithTimeout(outPath, 5*time.Second)
	if !strings.Contains(content, "CWD:/tmp") {
		t.Fatalf("workingdir: expected 'CWD:/tmp' in output, got: %q", content)
	}
}

// ─── Config: umask ───────────────────────────────────────────────────────────

func Test24_should_apply_umask_to_process_file_creation_when_umask_configured(t *testing.T) {
	outPath := fmt.Sprintf("/tmp/tm_umask_%d.txt", time.Now().UnixNano())
	defer os.Remove(outPath)

	// umask=0022 → 0666 & ~022 = 0644
	cfg := fmt.Sprintf(`
programs:
  printer:
    cmd: "%s"
    numprocs: 1
    autostart: true
    autorestart: never
    exitcodes: [0]
    startretries: 0
    starttime: 1
    stopsignal: TERM
    stoptime: 1
    workingdir: /tmp
    umask: 18
    stdout: "%s"
`, bins.printer, outPath)
	// umask 0022 decimal = 18

	startDaemon(t, cfg)
	pollStatus(t, "printer:00", "running", 5*time.Second)

	content := readFileWithTimeout(outPath, 5*time.Second)
	if !strings.Contains(content, "UMASK_PERM:0644") {
		t.Fatalf("umask=022: expected 'UMASK_PERM:0644' in output, got: %q", content)
	}
}

// ─── Logging ─────────────────────────────────────────────────────────────────

func Test25_should_log_when_process_starts(t *testing.T) {
	d := startDaemon(t, sleeperConf(false))

	rpcCall(t, "Taskmaster.Start", map[string]any{"name": "sleeper:00"})
	pollStatus(t, "sleeper:00", "running", 5*time.Second)

	log := d.log()
	if !strings.Contains(log, "sleeper") {
		t.Fatalf("logging: expected process name in daemon log after start, got:\n%s", log)
	}
}

func Test26_should_log_when_process_stops(t *testing.T) {
	d := startDaemon(t, sleeperConf(true))
	pollStatus(t, "sleeper:00", "running", 5*time.Second)

	rpcCall(t, "Taskmaster.Stop", map[string]any{"name": "sleeper:00"})
	pollStatus(t, "sleeper:00", "stopped", 5*time.Second)

	log := d.log()
	if !strings.Contains(log, "sleeper") {
		t.Fatalf("logging: expected process name in daemon log after stop, got:\n%s", log)
	}
}

func Test27_should_log_when_process_aborts_after_startretries_exhausted(t *testing.T) {
	cfg := fmt.Sprintf(`
programs:
  crasher:
    cmd: "%s"
    numprocs: 1
    autostart: true
    autorestart: unexpected
    exitcodes: [0]
    startretries: 1
    starttime: 0
    stopsignal: TERM
    stoptime: 1
    env:
      EXIT_CODE: "1"
`, bins.crasher)

	d := startDaemon(t, cfg)
	pollStatus(t, "crasher:00", "fatal", 10*time.Second)

	log := d.log()
	if !strings.Contains(log, "crasher") {
		t.Fatalf("logging: expected process name in daemon log after abort, got:\n%s", log)
	}
}

// ─── Hot-reload: via RPC ─────────────────────────────────────────────────────

func Test28_should_add_new_process_when_Reload_called_after_config_change(t *testing.T) {
	cfgA := `
programs:
  worker_a:
    cmd: "/bin/sleep 9999"
    numprocs: 1
    autostart: true
    autorestart: unexpected
    exitcodes: [0]
    startretries: 0
    starttime: 0
    stopsignal: TERM
    stoptime: 1
`
	cfgB := cfgA + `
  worker_b:
    cmd: "/bin/sleep 9999"
    numprocs: 1
    autostart: true
    autorestart: unexpected
    exitcodes: [0]
    startretries: 0
    starttime: 0
    stopsignal: TERM
    stoptime: 1
`
	d := startDaemon(t, cfgA)

	if err := d.writeConfig(cfgB); err != nil {
		t.Fatalf("writeConfig: %v", err)
	}

	resp := rpcCall(t, "Taskmaster.Reload", nil)
	assertSuccess(t, resp)

	result := resp["result"].(map[string]any)
	added, _ := result["added"].([]any)
	found := false
	for _, a := range added {
		if s, _ := a.(string); strings.Contains(s, "worker_b") {
			found = true
		}
	}
	if !found {
		t.Fatalf("reload: expected worker_b in added list, got result: %v", result)
	}
}

func Test29_should_remove_process_when_Reload_called_after_config_shrinks(t *testing.T) {
	cfgAB := `
programs:
  worker_a:
    cmd: "/bin/sleep 9999"
    numprocs: 1
    autostart: true
    autorestart: unexpected
    exitcodes: [0]
    startretries: 0
    starttime: 0
    stopsignal: TERM
    stoptime: 1
  worker_b:
    cmd: "/bin/sleep 9999"
    numprocs: 1
    autostart: true
    autorestart: unexpected
    exitcodes: [0]
    startretries: 0
    starttime: 0
    stopsignal: TERM
    stoptime: 1
`
	cfgA := `
programs:
  worker_a:
    cmd: "/bin/sleep 9999"
    numprocs: 1
    autostart: true
    autorestart: unexpected
    exitcodes: [0]
    startretries: 0
    starttime: 0
    stopsignal: TERM
    stoptime: 1
`
	d := startDaemon(t, cfgAB)
	pollStatus(t, "worker_a:00", "running", 5*time.Second)

	if err := d.writeConfig(cfgA); err != nil {
		t.Fatalf("writeConfig: %v", err)
	}

	resp := rpcCall(t, "Taskmaster.Reload", nil)
	assertSuccess(t, resp)

	result := resp["result"].(map[string]any)
	removed, _ := result["removed"].([]any)
	found := false
	for _, r := range removed {
		if s, _ := r.(string); strings.Contains(s, "worker_b") {
			found = true
		}
	}
	if !found {
		t.Fatalf("reload: expected worker_b in removed list, got result: %v", result)
	}
}

func Test30_should_not_restart_unaffected_process_when_Reload_called(t *testing.T) {
	cfgAB := `
programs:
  stable:
    cmd: "/bin/sleep 9999"
    numprocs: 1
    autostart: true
    autorestart: unexpected
    exitcodes: [0]
    startretries: 0
    starttime: 0
    stopsignal: TERM
    stoptime: 1
  extra:
    cmd: "/bin/sleep 9999"
    numprocs: 1
    autostart: true
    autorestart: unexpected
    exitcodes: [0]
    startretries: 0
    starttime: 0
    stopsignal: TERM
    stoptime: 1
`
	cfgA := `
programs:
  stable:
    cmd: "/bin/sleep 9999"
    numprocs: 1
    autostart: true
    autorestart: unexpected
    exitcodes: [0]
    startretries: 0
    starttime: 0
    stopsignal: TERM
    stoptime: 1
`
	d := startDaemon(t, cfgAB)
	pollStatus(t, "stable:00", "running", 5*time.Second)
	pidBefore := pidOf(t, "stable:00")

	if err := d.writeConfig(cfgA); err != nil {
		t.Fatalf("writeConfig: %v", err)
	}
	rpcCall(t, "Taskmaster.Reload", nil)
	time.Sleep(500 * time.Millisecond)

	pidAfter := pidOf(t, "stable:00")
	if pidBefore == 0 || pidBefore != pidAfter {
		t.Fatalf("hot-reload: stable process was restarted (pid %d → %d), should not have been", pidBefore, pidAfter)
	}
}

// ─── Hot-reload: via SIGHUP ──────────────────────────────────────────────────

func Test31_should_reload_config_when_SIGHUP_received(t *testing.T) {
	cfgA := `
programs:
  original:
    cmd: "/bin/sleep 9999"
    numprocs: 1
    autostart: true
    autorestart: unexpected
    exitcodes: [0]
    startretries: 0
    starttime: 0
    stopsignal: TERM
    stoptime: 1
`
	cfgB := cfgA + `
  added_by_sighup:
    cmd: "/bin/sleep 9999"
    numprocs: 1
    autostart: true
    autorestart: unexpected
    exitcodes: [0]
    startretries: 0
    starttime: 0
    stopsignal: TERM
    stoptime: 1
`
	d := startDaemon(t, cfgA)
	pollStatus(t, "original:00", "running", 5*time.Second)

	if err := d.writeConfig(cfgB); err != nil {
		t.Fatalf("writeConfig: %v", err)
	}
	if err := d.sendSIGHUP(); err != nil {
		t.Fatalf("sendSIGHUP: %v", err)
	}

	// After SIGHUP the daemon calls NewManagerFromConfig + Spawn — new process should appear.
	status := pollStatus(t, "added_by_sighup:00", "running", 8*time.Second)
	if status != "running" && status != "starting" {
		t.Fatalf("SIGHUP reload: expected added_by_sighup to be running/starting, got %q", status)
	}
}

// ─── Resilience: kill supervised process → auto-restart ─────────────────────

func Test32_should_restart_supervised_process_when_killed_externally(t *testing.T) {
	startDaemon(t, sleeperConf(true))
	pollStatus(t, "sleeper:00", "running", 5*time.Second)
	pid := pidOf(t, "sleeper:00")

	// Kill the process from outside (simulates crash)
	if err := syscall.Kill(pid, syscall.SIGKILL); err != nil {
		t.Fatalf("kill: %v", err)
	}

	// Wait for the daemon to detect the kill: proc status must leave "running".
	// Without this, the first poll may return the stale pre-kill "running" status.
	pollStatusNot(t, "sleeper:00", "running", 3*time.Second)

	// autorestart=unexpected + exitcodes=[0]: killed → non-zero exit → unexpected → restart
	status := pollStatus(t, "sleeper:00", "running", 8*time.Second)
	if status != "running" {
		t.Fatalf("resilience: expected process to restart after external kill, got %q", status)
	}
	newPID := pidOf(t, "sleeper:00")
	if newPID == pid {
		t.Fatalf("resilience: PID did not change after restart (still %d)", pid)
	}
}

// ─── Resilience: always-failing process → fatal after retries ───────────────

func Test33_should_reach_fatal_when_supervised_process_always_fails(t *testing.T) {
	cfg := fmt.Sprintf(`
programs:
  always_fail:
    cmd: "%s"
    numprocs: 1
    autostart: true
    autorestart: unexpected
    exitcodes: [0]
    startretries: 2
    starttime: 0
    stopsignal: TERM
    stoptime: 1
    env:
      EXIT_CODE: "1"
`, bins.crasher)

	startDaemon(t, cfg)

	status := pollStatus(t, "always_fail:00", "fatal", 10*time.Second)
	if status != "fatal" {
		t.Fatalf("resilience: expected 'fatal' after all retries exhausted, got %q", status)
	}
}

// ─── Shutdown ────────────────────────────────────────────────────────────────

func Test34_should_return_success_message_when_Shutdown_called(t *testing.T) {
	startDaemon(t, sleeperConf(false))

	resp := rpcCall(t, "Taskmaster.Shutdown", nil)
	assertSuccess(t, resp)

	result := resp["result"].(map[string]any)
	msg, _ := result["message"].(string)
	if msg == "" {
		t.Fatal("shutdown: expected non-empty message in response")
	}
}

// ─── Protocol: unknown method ────────────────────────────────────────────────

func Test35_should_return_MethodNotFound_when_unknown_method_called(t *testing.T) {
	startDaemon(t, sleeperConf(false))

	resp := rpcCall(t, "Taskmaster.DoNothing", nil)
	assertErrorCode(t, resp, -32601)
}

func Test36_should_return_InvalidRequest_when_malformed_method_called(t *testing.T) {
	startDaemon(t, sleeperConf(false))

	resp := rpcCall(t, "BadNamespace.Something", nil)
	assertErrorCode(t, resp, -32600)
}
