/*
FormatInstanceName("nginx", 1)  →  "nginx:01"
FormatInstanceName("nginx", 2)  →  "nginx:02"

Zero-padded two digits. Used in ProcessInfo.Name and ActionRequest.Name.

Instances are program:01, program:02… (not :0). Both developers must use config.FormatInstanceName — never build the string by hand.
*/

package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// ConfigFile represents the top-level structure of the YAML configuration file.
type ConfigFile struct {
	Programs map[string]ConfigSpec `yaml:"programs"`
}

// YAMLLoader implements Loader for YAML configuration files.
type YAMLLoader struct{}

// Load reads a YAML configuration file and returns parsed ConfigSpecs.
// It expands instances based on Numprocs field.
// Returns error if any spec fails validation.
func (l *YAMLLoader) Load(path string) ([]ConfigSpec, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config file %s: %w", path, err)
	}

	var configFile ConfigFile
	err = yaml.Unmarshal(data, &configFile)
	if err != nil {
		return nil, fmt.Errorf("parse yaml: %w", err)
	}

	var specs []ConfigSpec
	for name, spec := range configFile.Programs {
		// Set Program from map key if not already set
		if spec.Program == "" {
			spec.Program = name
		}

		// Validate spec before expansion
		if err := spec.Validate(); err != nil {
			return nil, fmt.Errorf("validate spec %q: %w", name, err)
		}

		// Expand instances
		numprocs := spec.Numprocs
		if numprocs <= 0 {
			numprocs = 1
		}
		for i := 0; i < numprocs; i++ {
			instance := spec
			instance.ProcessName = FormatInstanceName(name, i)
			specs = append(specs, instance)
		}
	}

	return specs, nil
}

// FormatInstanceName formats an instance name (e.g., "program:01").
func FormatInstanceName(base string, index int) string {
	return fmt.Sprintf("%s:%02d", base, index)
}
