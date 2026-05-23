package main

import (
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

type Config struct {
	ProcessName   string            `yaml:"process_name"`
	Program       string            `yaml:"program"`
	Cmd           []string          `yaml:"cmd"`
	Numprocs      int               `yaml:"numprocs"`
	NumprocsStart int               `yaml:"numprocs_start"`
	Umask         int               `yaml:"umask"`
	Workingdir    string            `yaml:"workingdir"`
	Autostart     bool              `yaml:"autostart"`
	Autorestart   string            `yaml:"autorestart"`
	Exitcodes     []int             `yaml:"exitcodes"`
	Startretries  int               `yaml:"startretries"`
	Starttime     int               `yaml:"starttime"`
	Stopsignal    string            `yaml:"stopsignal"`
	Stoptime      int               `yaml:"stoptime"`
	Stdout        string            `yaml:"stdout"`
	Stderr        string            `yaml:"stderr"`
	Env           map[string]string `yaml:"env"`
	Uid           *uint32           `yaml:"uid"`
	Gid           *uint32           `yaml:"gid"`
}

func (c *Config) Validate() error {
	// Identity slice: mandatory fields
	if c.Program == "" {
		return fmt.Errorf("program must not be empty")
	}

	if len(c.Cmd) == 0 || strings.TrimSpace(c.Cmd[0]) == "" {
		return fmt.Errorf("cmd must list at least one non-empty executable")
	}

	if c.Numprocs < 1 {
		return fmt.Errorf("numprocs must be >= 1, got %d", c.Numprocs)
	}

	validSignals := map[string]bool{
		"TERM": true, "HUP": true, "INT": true, "QUIT": true,
		"KILL": true, "USR1": true, "USR2": true,
	}

	if c.Stopsignal != "" && !validSignals[c.Stopsignal] {
		return fmt.Errorf("stopsignal must be one of: TERM, HUP, INT, QUIT, KILL, USR1, USR2, got %q", c.Stopsignal)
	}

	if c.Stoptime < 0 {
		return fmt.Errorf("stoptime must be >= 0, got %d", c.Stoptime)
	}

	if c.Starttime < 0 {
		return fmt.Errorf("starttime must be >= 0, got %d", c.Starttime)
	}

	if c.Umask < 0 || c.Umask > 0o777 {
		return fmt.Errorf("umask must be between 0 and 0o777 (0-511 decimal), got %d", c.Umask)
	}

	if c.Workingdir != "" {
		info, err := os.Stat(c.Workingdir)
		if err != nil {
			return fmt.Errorf("workingdir %q does not exist: %w", c.Workingdir, err)
		}
		if !info.IsDir() {
			return fmt.Errorf("workingdir %q is a file, not a directory", c.Workingdir)
		}
	}

	_, err := exec.LookPath(c.Cmd[0])
	if err != nil {
		return fmt.Errorf("executable %q not found in PATH: %w", c.Cmd[0], err)
	}

	return nil
}

func ExpandPrograms(programs map[string]Config) map[string]*Config {
	names := make([]string, 0, len(programs))
	for name := range programs {
		names = append(names, name)
	}
	sort.Strings(names)

	out := make(map[string]*Config)

	for _, name := range names {
		spec := programs[name]
		for i := 0; i < spec.Numprocs; i++ {
			inst := spec
			inst.ProcessName = fmt.Sprintf("%s:%02d", name, i)
			ptr := new(Config)
			*ptr = inst
			out[inst.ProcessName] = ptr
		}
	}
	return out
}

func LoadConfig(path string) (map[string]*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config file %s: %w", path, err)
	}

	var programs struct {
		ConfigMap map[string]Config `yaml:"programs"`
	}

	if err := yaml.Unmarshal(data, &programs); err != nil {
		return nil, fmt.Errorf(`parse yaml: %w`, err)
	}

	if programs.ConfigMap == nil {
		return nil, fmt.Errorf("missing programs")
	}

	for name, config := range programs.ConfigMap {
		config.Program = name
		programs.ConfigMap[name] = config
	}

	for name, config := range programs.ConfigMap {
		if err := config.Validate(); err != nil {
			return nil, fmt.Errorf("validate spec %q: %w", name, err)
		}
	}

	expanded := ExpandPrograms(programs.ConfigMap)

	return expanded, nil
}
