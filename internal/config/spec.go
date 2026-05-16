package config

import "fmt"

type ConfigSpec struct {
	ProcessName   string            `yaml:"process_name"`
	Program       string            `yaml:"program"`
	Cmd           string            `yaml:"cmd"`
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
}

type Loader interface {
	Load(path string) ([]ConfigSpec, error)
}

// Validate checks ConfigSpec for required fields and valid values.
// Returns error if spec is invalid, nil if valid.
func (c *ConfigSpec) Validate() error {
	// Identity slice: mandatory fields
	if c.Program == "" {
		return fmt.Errorf("program must not be empty")
	}

	if c.Cmd == "" {
		return fmt.Errorf("cmd must not be empty")
	}

	if c.Numprocs < 1 {
		return fmt.Errorf("numprocs must be >= 1, got %d", c.Numprocs)
	}

	// Control slice: signals + timeouts
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

	// Context slice: execution environment (all optional, empty values skipped)
	// Only validate Umask always; others only if non-empty (provided)
	if c.Umask < 0 || c.Umask > 0o777 {
		return fmt.Errorf("umask must be between 0 and 0o777 (0-511 decimal), got %d", c.Umask)
	}

	return nil
}
