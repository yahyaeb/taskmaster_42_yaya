// Package e2e tests the taskmaster daemon as a real subprocess.
// No internal packages are imported — all interaction is over the unix socket
// using JSON-RPC 2.0, exactly as a real user (or ctl binary) would.
package e2e_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

const socketPath = "/tmp/taskmaster.sock"

// bins holds paths to all compiled binaries, set once in TestMain.
var bins struct {
	daemon     string
	crasher    string
	printer    string
	signalTrap string
}

// TestMain compiles all binaries once before any test runs.
func TestMain(m *testing.M) {
	projectRoot, err := filepath.Abs("..")
	if err != nil {
		fmt.Fprintln(os.Stderr, "e2e: resolve project root:", err)
		os.Exit(1)
	}
	tmpDir := os.TempDir()

	builds := []struct {
		dest string
		pkg  string
		into *string
	}{
		{filepath.Join(tmpDir, "tm-daemon-e2e"), "./cmd/daemon", &bins.daemon},
		{filepath.Join(tmpDir, "tm-crasher-e2e"), "./e2e/programs/crasher", &bins.crasher},
		{filepath.Join(tmpDir, "tm-printer-e2e"), "./e2e/programs/printer", &bins.printer},
		{filepath.Join(tmpDir, "tm-signal-trap-e2e"), "./e2e/programs/signal_trap", &bins.signalTrap},
	}

	for _, b := range builds {
		cmd := exec.Command("go", "build", "-o", b.dest, b.pkg)
		cmd.Dir = projectRoot
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "e2e: build %s failed: %v\n", b.pkg, err)
			os.Exit(1)
		}
		*b.into = b.dest
	}

	os.Exit(m.Run())
}

// daemon wraps a running daemon subprocess with helpers for test interaction.
type daemon struct {
	cmd     *exec.Cmd
	cfgDir  string
	logFile *os.File
}

// startDaemon launches the daemon with config written to a temp dir.
// Daemon stderr (slog output) is captured to d.logFile for log assertions.
func startDaemon(t *testing.T, config string) *daemon {
	t.Helper()

	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.yml")
	if err := os.WriteFile(cfgPath, []byte(config), 0644); err != nil {
		t.Fatalf("startDaemon: write config: %v", err)
	}

	logFile, err := os.CreateTemp(t.TempDir(), "daemon-*.log")
	if err != nil {
		t.Fatalf("startDaemon: create log file: %v", err)
	}

	_ = os.Remove(socketPath)

	cmd := exec.Command(bins.daemon)
	cmd.Dir = tmpDir
	cmd.Stdout = logFile // daemon uses fmt.Print + slog to stderr; capture both
	cmd.Stderr = logFile

	if err := cmd.Start(); err != nil {
		t.Fatalf("startDaemon: exec.Start: %v", err)
	}

	if err := waitForSocket(socketPath, 5*time.Second); err != nil {
		_ = cmd.Process.Kill()
		t.Fatalf("startDaemon: %v", err)
	}

	d := &daemon{cmd: cmd, cfgDir: tmpDir, logFile: logFile}
	t.Cleanup(d.stop)
	return d
}

// writeConfig overwrites the daemon's config.yml — used before a reload.
func (d *daemon) writeConfig(config string) error {
	return os.WriteFile(filepath.Join(d.cfgDir, "config.yml"), []byte(config), 0644)
}

// sendSIGHUP sends SIGHUP to the daemon process to trigger a hot-reload.
func (d *daemon) sendSIGHUP() error {
	return d.cmd.Process.Signal(syscall.SIGHUP)
}

// log returns the full captured daemon output (stdout + stderr combined).
func (d *daemon) log() string {
	if _, err := d.logFile.Seek(0, io.SeekStart); err != nil {
		return ""
	}
	data, _ := io.ReadAll(d.logFile)
	return string(data)
}

// stop kills the daemon and cleans up the socket.
func (d *daemon) stop() {
	_ = d.cmd.Process.Kill()
	_ = d.cmd.Wait()
	_ = d.logFile.Close()
	_ = os.Remove(socketPath)
}

// waitForSocket polls the unix socket path until it accepts connections or times out.
func waitForSocket(path string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		conn, err := net.Dial("unix", path)
		if err == nil {
			conn.Close()
			return nil
		}
		time.Sleep(50 * time.Millisecond)
	}
	return fmt.Errorf("socket %s not ready after %s", path, timeout)
}

// rpcCall opens a fresh connection, sends one JSON-RPC 2.0 request, returns the decoded response.
// Each call uses its own connection — matching real ctl behavior (one conn per command).
func rpcCall(t *testing.T, method string, params any) map[string]any {
	t.Helper()

	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		t.Fatalf("rpcCall: dial: %v", err)
	}
	defer conn.Close()

	req := map[string]any{
		"jsonrpc": "2.0",
		"method":  method,
		"params":  params,
		"id":      1,
	}
	if err := json.NewEncoder(conn).Encode(req); err != nil {
		t.Fatalf("rpcCall: encode: %v", err)
	}

	var resp map[string]any
	if err := json.NewDecoder(conn).Decode(&resp); err != nil {
		t.Fatalf("rpcCall: decode: %v", err)
	}
	return resp
}

// assertSuccess fails if the response contains an error field.
func assertSuccess(t *testing.T, resp map[string]any) {
	t.Helper()
	if resp["error"] != nil {
		t.Fatalf("expected success, got error: %v", resp["error"])
	}
}

// assertErrorCode fails if the response does not carry the expected JSON-RPC error code.
func assertErrorCode(t *testing.T, resp map[string]any, want float64) {
	t.Helper()
	errField, ok := resp["error"].(map[string]any)
	if !ok {
		t.Fatalf("expected error response, got: %v", resp)
	}
	code, ok := errField["code"].(float64)
	if !ok {
		t.Fatalf("error.code missing or not a number: %v", errField)
	}
	if code != want {
		t.Fatalf("expected error code %.0f, got %.0f (msg: %v)", want, code, errField["message"])
	}
}

// pollStatus calls GetStatus repeatedly until the named process reaches wantStatus
// or the timeout elapses. Returns the last observed status.
func pollStatus(t *testing.T, processName, wantStatus string, timeout time.Duration) string {
	t.Helper()
	deadline := time.Now().Add(timeout)
	var last string
	for time.Now().Before(deadline) {
		resp := rpcCall(t, "Taskmaster.GetStatus", nil)
		if resp["error"] != nil {
			time.Sleep(100 * time.Millisecond)
			continue
		}
		list, _ := resp["result"].([]any)
		for _, item := range list {
			proc, _ := item.(map[string]any)
			if proc["name"] == processName {
				last, _ = proc["status"].(string)
				if last == wantStatus {
					return last
				}
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	return last
}

// pidOf returns the PID of a named process from GetStatus, or 0 if not found.
func pidOf(t *testing.T, processName string) int {
	t.Helper()
	resp := rpcCall(t, "Taskmaster.GetStatus", nil)
	assertSuccess(t, resp)
	list, _ := resp["result"].([]any)
	for _, item := range list {
		proc, _ := item.(map[string]any)
		if proc["name"] == processName {
			pid, _ := proc["pid"].(float64)
			return int(pid)
		}
	}
	return 0
}

// readFileWithTimeout polls a file until it is non-empty or the timeout elapses.
func readFileWithTimeout(path string, timeout time.Duration) string {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		data, err := os.ReadFile(path)
		if err == nil && len(data) > 0 {
			return string(data)
		}
		time.Sleep(100 * time.Millisecond)
	}
	return ""
}
