package main

import (
	"fmt"
	"os"
	"os/exec"
	"slices"
	"strings"
	"syscall"
)

func environment(extra map[string]string) []string {
	env := slices.Clone(os.Environ())
	if len(extra) == 0 {
		return env
	}
	idx := make(map[string]int, len(env))
	for i, kv := range env {
		if j := strings.IndexByte(kv, '='); j >= 0 {
			idx[kv[:j]] = i
		}
	}
	for k, v := range extra {
		p := k + "=" + v
		if i, ok := idx[k]; ok {
			env[i] = p
		} else {
			idx[k] = len(env)
			env = append(env, p)
		}
	}
	return env
}

func buildCommand(config *Config) *exec.Cmd {

	if config.Umask == 0 {
		return exec.Command(config.Cmd[0], config.Cmd[1:]...)
	}

	umask := fmt.Sprintf("%o", config.Umask)

	args := []string{"-c", "umask " + umask + " && exec \"$@\"", "--"}
	args = append(args, config.Cmd...)
	return exec.Command("/bin/sh", args...)
}

func privileged(config *Config, cmd *exec.Cmd) {
	if config.Uid == nil && config.Gid == nil {
		return
	}
	cred := &syscall.Credential{}
	if config.Uid != nil {
		cred.Uid = *config.Uid
	}
	if config.Gid != nil {
		cred.Gid = *config.Gid
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{Credential: cred}
}

func startProcess(config *Config) (*exec.Cmd, error) {

	cmd := buildCommand(config)

	privileged(config, cmd)

	cmd.Dir = config.Workingdir
	cmd.Env = environment(config.Env)

	var closers []func()
	fail := func() {
		for i := len(closers) - 1; i >= 0; i-- {
			closers[i]()
		}
	}

	if config.Stdout != "" {
		f, err := os.OpenFile(config.Stdout, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
		if err != nil {
			fail()
			return nil, fmt.Errorf("open stdout %s: %w", config.Stdout, err)
		}
		cmd.Stdout = f
		closers = append(closers, func() { _ = f.Close() })
	}

	if config.Stderr != "" {
		f, err := os.OpenFile(config.Stderr, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
		if err != nil {
			fail()
			return nil, fmt.Errorf("open stderr %s: %w", config.Stderr, err)
		}
		cmd.Stderr = f
		closers = append(closers, func() { _ = f.Close() })
	} else if cmd.Stdout != nil {
		cmd.Stderr = cmd.Stdout
	}

	closeFiles := func() {
		for i := len(closers) - 1; i >= 0; i-- {
			closers[i]()
		}
	}

	err := cmd.Start()
	if err != nil {
		closeFiles()
		return nil, err
	}

	fmt.Println("Process started with Name:", config.ProcessName, "and PID:", cmd.Process.Pid)

	closeFiles()

	return cmd, nil
}
