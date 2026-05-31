package helpers

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	AnsiRed   = "\033[0;31m"
	AnsiGreen = "\033[0;32m"
	AnsiBold  = "\033[1m"
	AnsiNC    = "\033[0m"
)

type ProcStatus struct {
	Name     string
	State    string
	PID      int
	ExitCode int
}

type Report struct {
	Pass  int
	Fail  int
	Total int
}

func (r *Report) Passf(format string, args ...interface{}) {
	r.Pass++
	r.Total++
	fmt.Printf("  %s✓%s %s\n", AnsiGreen, AnsiNC, fmt.Sprintf(format, args...))
}

func (r *Report) PassMsg(msg string) {
	r.Passf("%s", msg)
}

func (r *Report) Failf(format string, args ...interface{}) {
	r.Fail++
	r.Total++
	fmt.Printf("  %s✗%s %s\n", AnsiRed, AnsiNC, fmt.Sprintf(format, args...))
}

func (r *Report) FailMsg(msg string) {
	r.Failf("%s", msg)
}

func (r *Report) Section(title string) {
	fmt.Printf("\n%s%s━━━ %s ━━━%s\n", AnsiBold, AnsiNC, title, AnsiNC)
}

func (r *Report) PrintResults(pointName string) {
	fmt.Printf("\n%s%s━━━ Results: %s ━━━%s\n\n", AnsiBold, AnsiNC, pointName, AnsiNC)
	fmt.Printf("  %sPassed:%s %d\n", AnsiGreen, AnsiNC, r.Pass)
	fmt.Printf("  %sFailed:%s %d\n", AnsiRed, AnsiNC, r.Fail)
	fmt.Printf("  Total:    %d\n\n", r.Total)
}

type TestContext struct {
	Root       string
	ConfigPath string
	BackupPath string
	DaemonPath string
	CtlPath    string
	LogPath    string
	SocketPath string
	Daemon     *exec.Cmd
}

func StartDaemon(ctx *TestContext) error {
	cmd := exec.Command(ctx.DaemonPath)
	cmd.Dir = ctx.Root
	stdout, err := os.Create(filepath.Join(os.TempDir(), "taskmasterd_eval.stdout"))
	if err != nil {
		return err
	}
	stderr, err := os.Create(filepath.Join(os.TempDir(), "taskmasterd_eval.stderr"))
	if err != nil {
		stdout.Close()
		return err
	}
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	if err := cmd.Start(); err != nil {
		stdout.Close()
		stderr.Close()
		return err
	}
	ctx.Daemon = cmd
	return nil
}

func WaitForCtlReady(ctx *TestContext, timeout time.Duration) error {
	_, err := WaitForStatus(ctx, timeout, func(_ map[string]ProcStatus) bool { return true })
	return err
}

func WaitForStatus(ctx *TestContext, timeout time.Duration, predicate func(map[string]ProcStatus) bool) (map[string]ProcStatus, error) {
	deadline := time.Now().Add(timeout)
	var lastErr error
	for time.Now().Before(deadline) {
		out, err := RunCtl(ctx, "status")
		if err == nil {
			st := ParseStatus(out)
			if predicate(st) {
				return st, nil
			}
		} else {
			lastErr = err
		}
		time.Sleep(200 * time.Millisecond)
	}
	if lastErr == nil {
		lastErr = errors.New("timeout waiting for expected status")
	}
	return nil, lastErr
}

func RunCtl(ctx *TestContext, input string) (string, error) {
	cmd := exec.Command(ctx.CtlPath)
	cmd.Dir = ctx.Root
	cmd.Stdin = strings.NewReader(input + "\n")
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	err := cmd.Run()
	return buf.String(), err
}

func ParseStatus(out string) map[string]ProcStatus {
	result := map[string]ProcStatus{}
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "ok") || strings.HasPrefix(line, "error") || strings.HasPrefix(line, "taskmaster") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		name := fields[0]
		state := strings.ToLower(fields[1])
		result[name] = ProcStatus{
			Name:     name,
			State:    state,
			PID:      extractPID(line),
			ExitCode: extractExitCode(line),
		}
	}
	return result
}

func extractPID(line string) int {
	idx := strings.Index(line, "pid ")
	if idx < 0 {
		return 0
	}
	tail := line[idx+4:]
	end := 0
	for end < len(tail) && tail[end] >= '0' && tail[end] <= '9' {
		end++
	}
	if end == 0 {
		return 0
	}
	pid, _ := strconv.Atoi(tail[:end])
	return pid
}

func extractExitCode(line string) int {
	start := strings.Index(line, "status ")
	if start < 0 {
		return 0
	}
	tail := line[start+7:]
	end := 0
	for end < len(tail) && tail[end] >= '0' && tail[end] <= '9' {
		end++
	}
	if end == 0 {
		return 0
	}
	code, _ := strconv.Atoi(tail[:end])
	return code
}

func CopyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0644)
}

func UpdateProgramBlock(path, program string, mutate func([]string) ([]string, error)) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	content := strings.ReplaceAll(string(data), "\r\n", "\n")
	lines := strings.Split(content, "\n")
	
	start := -1
	for i, line := range lines {
		if line == "  " + program + ":" {
			start = i
			break
		}
	}
	if start < 0 {
		return fmt.Errorf("program %q not found", program)
	}
	
	end := len(lines)
	for i := start + 1; i < len(lines); i++ {
		line := lines[i]
		if strings.HasPrefix(line, "  ") && !strings.HasPrefix(line, "    ") && strings.HasSuffix(strings.TrimSpace(line), ":") {
			end = i
			break
		}
	}
	
	block := append([]string(nil), lines[start+1:end]...)
	block, err = mutate(block)
	if err != nil {
		return err
	}
	
	out := append([]string{}, lines[:start+1]...)
	out = append(out, block...)
	out = append(out, lines[end:]...)
	return os.WriteFile(path, []byte(strings.Join(out, "\n")), 0644)
}

func ReplaceBlockLines(block []string, key string, replacement []string) ([]string, error) {
	prefix := "    " + key + ":"
	for i, line := range block {
		if strings.HasPrefix(line, prefix) {
			out := append([]string{}, block[:i]...)
			out = append(out, replacement...)
			out = append(out, block[i+1:]...)
			return out, nil
		}
	}
	return nil, fmt.Errorf("key %q not found", key)
}

func RunCmd(root string, name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Dir = root
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s %s: %w\n%s", name, strings.Join(args, " "), err, buf.String())
	}
	return nil
}

func Must(err error) {
	if err != nil {
		panic(err)
	}
}
