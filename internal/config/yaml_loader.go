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

	var programs map[string]ConfigSpec

	// Try nested format first: {programs: {name: spec}}
	var nested struct {
		Programs map[string]ConfigSpec `yaml:"programs"`
	}
	if err := yaml.Unmarshal(data, &nested); err == nil && nested.Programs != nil {
		programs = nested.Programs
	} else {
		// Fall back to flat format: {name: spec}
		err = yaml.Unmarshal(data, &programs)
		if err != nil {
			return nil, fmt.Errorf("parse yaml: %w", err)
		}
	}

	var specs []ConfigSpec
	for name, spec := range programs {
		// Set Program from map key if not already set
		if spec.Program == "" {
			spec.Program = name
		}

		// Apply default for Numprocs before validation
		if spec.Numprocs <= 0 {
			spec.Numprocs = 1
		}

		// Validate spec before expansion
		if err := spec.Validate(); err != nil {
			return nil, fmt.Errorf("validate spec %q: %w", name, err)
		}

		// Expand instances
		for i := 0; i < spec.Numprocs; i++ {
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
