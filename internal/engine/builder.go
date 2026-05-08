package engine

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"taskmaster/internal/config"
)

type CommandBuilder struct{}

func (cb *CommandBuilder) BuildCommand(spec config.ConfigSpec) (*exec.Cmd, error) {
	parts := strings.Fields(spec.Cmd)
	if len(parts) == 0 {
		return nil, fmt.Errorf("no command specified")
	}

	cmd := exec.Command(parts[0], parts[1:]...)

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
