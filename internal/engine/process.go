package engine

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"syscall"

	"taskmaster/internal/config"
)

type ExitCode int

type Process struct {
	PID    int
	Stdin  io.WriteCloser
	Stdout io.ReadCloser
	Stderr io.ReadCloser
}

type ProcessExecutor interface {
	Start(ctx context.Context, spec config.ConfigSpec) (*Process, error)
	Wait(ctx context.Context, pid int) (ExitCode, error)
	Signal(ctx context.Context, pid int, signal interface{}) error
}

type OsProcessExecutor struct{}

func NewOsProcessExecutor() *OsProcessExecutor {
	return &OsProcessExecutor{}
}

func (e *OsProcessExecutor) Start(ctx context.Context, spec config.ConfigSpec) (*Process, error) {
	builder := &CommandBuilder{}
	cmd, err := builder.BuildCommand(ctx, spec)
	if err != nil {
		return nil, fmt.Errorf("build command failed: %w", err)
	}

	oldMask := syscall.Umask(spec.Umask)
	defer func() {
		syscall.Umask(oldMask)
	}()

	err = cmd.Start()
	if err != nil {
		e.closeFiles(cmd)
		return nil, fmt.Errorf("start failed: %w", err)
	}

	pid := cmd.Process.Pid
	return &Process{PID: pid}, nil
}

func (e *OsProcessExecutor) Wait(ctx context.Context, pid int) (ExitCode, error) {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return -1, fmt.Errorf("find process failed: %w", err)
	}

	select {
	case <-ctx.Done():
		return -1, ctx.Err()
	default:
	}

	type result struct {
		exitCode ExitCode
		err      error
	}
	done := make(chan result, 1)

	go func() {
		state, err := proc.Wait()
		if err != nil {
			done <- result{-1, fmt.Errorf("wait failed: %w", err)}
			return
		}

		exitCode := 0
		if exitStatus, ok := state.Sys().(syscall.WaitStatus); ok {
			if exitStatus.Signaled() {
				exitCode = 128 + int(exitStatus.Signal())
			} else if !state.Success() {
				exitCode = exitStatus.ExitStatus()
			}
		} else if !state.Success() {
			exitCode = 1
		}
		done <- result{ExitCode(exitCode), nil}
	}()

	select {
	case r := <-done:
		return r.exitCode, r.err
	case <-ctx.Done():
		return -1, ctx.Err()
	}
}

func (e *OsProcessExecutor) Signal(ctx context.Context, pid int, signal interface{}) error {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("find process failed: %w", err)
	}
	sig, ok := signal.(os.Signal)
	if !ok {
		return fmt.Errorf("invalid signal type")
	}
	return proc.Signal(sig)
}

func (e *OsProcessExecutor) closeFiles(cmd *exec.Cmd) {
	if f, ok := cmd.Stdout.(io.Closer); ok && f != nil {
		f.Close()
	}
	if f, ok := cmd.Stderr.(io.Closer); ok && f != nil {
		if sf, ok := cmd.Stdout.(*os.File); ok {
			if tf, ok := f.(*os.File); ok && sf == tf {
				return
			}
		}
		f.Close()
	}
}

type CommandBuilder struct{}

func (cb *CommandBuilder) BuildCommand(ctx context.Context, spec config.ConfigSpec) (*exec.Cmd, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	parts := strings.Fields(spec.Cmd)
	if len(parts) == 0 {
		return nil, fmt.Errorf("no command specified")
	}

	cmd := exec.CommandContext(ctx, parts[0], parts[1:]...)

	if spec.Workingdir != "" {
		cmd.Dir = spec.Workingdir
	}

	env := os.Environ()
	if spec.Env != nil {
		for key, value := range spec.Env {
			env = append(env, fmt.Sprintf("%s=%s", key, value))
		}
	}
	cmd.Env = env

	var outF *os.File
	if spec.Stdout != "" {
		var err error
		outF, err = os.OpenFile(spec.Stdout, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
		if err != nil {
			return nil, fmt.Errorf("open stdout %s: %w", spec.Stdout, err)
		}
		cmd.Stdout = outF
	}

	if spec.Stderr != "" {
		errF, err := os.OpenFile(spec.Stderr, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
		if err != nil {
			if outF != nil {
				outF.Close()
			}
			return nil, fmt.Errorf("open stderr %s: %w", spec.Stderr, err)
		}
		cmd.Stderr = errF
	} else if cmd.Stdout != nil {
		cmd.Stderr = cmd.Stdout
	}

	return cmd, nil
}
