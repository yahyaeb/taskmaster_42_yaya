package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestYAMLConfigLoader_Load(t *testing.T) {
	loader := &YAMLLoader{}
	tmpDir := t.TempDir()

	tests := []struct {
		name          string
		yamlContent   string
		wantCount     int
		wantErr       bool
		checkInstance func(t *testing.T, specs []ConfigSpec)
	}{
		{
			name: "valid YAML with single process (Numprocs=1)",
			yamlContent: `example:
  program: /bin/sleep
  cmd: "5"
  numprocs: 1
  autostart: true
  autorestart: always`,
			wantCount: 1,
			checkInstance: func(t *testing.T, specs []ConfigSpec) {
				if len(specs) != 1 {
					t.Fatalf("expected 1 spec, got %d", len(specs))
				}
				if specs[0].ProcessName != "example:00" {
					t.Errorf("expected process name 'example:00', got %s", specs[0].ProcessName)
				}
				if specs[0].Program != "/bin/sleep" {
					t.Errorf("expected program '/bin/sleep', got %s", specs[0].Program)
				}
				if specs[0].Cmd != "5" {
					t.Errorf("expected cmd '5', got %s", specs[0].Cmd)
				}
			},
		},
		{
			name: "valid YAML with Numprocs > 1 (instance expansion)",
			yamlContent: `worker:
  program: /usr/bin/python
  cmd: "worker.py"
  numprocs: 3
  autostart: false
  autorestart: unexpected`,
			wantCount: 3,
			checkInstance: func(t *testing.T, specs []ConfigSpec) {
				if len(specs) != 3 {
					t.Fatalf("expected 3 specs, got %d", len(specs))
				}
				expectedNames := map[string]bool{
					"worker:00": true,
					"worker:01": true,
					"worker:02": true,
				}
				for _, spec := range specs {
					if !expectedNames[spec.ProcessName] {
						t.Errorf("unexpected process name %s", spec.ProcessName)
					}
				}
			},
		},
		{
			name: "empty YAML (zero processes)",
			yamlContent: `{}`,
			wantCount: 0,
		},
		{
			name: "negative Numprocs (should default to 1)",
			yamlContent: `test:
  program: /bin/echo
  numprocs: -5`,
			wantCount: 1,
			checkInstance: func(t *testing.T, specs []ConfigSpec) {
				if specs[0].ProcessName != "test:00" {
					t.Errorf("expected process name 'test:00', got %s", specs[0].ProcessName)
				}
			},
		},
		{
			name: "zero Numprocs (should default to 1)",
			yamlContent: `test:
  program: /bin/echo
  numprocs: 0`,
			wantCount: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpFile := filepath.Join(tmpDir, "config.yaml")
			if err := os.WriteFile(tmpFile, []byte(tt.yamlContent), 0644); err != nil {
				t.Fatalf("failed to write temp file: %v", err)
			}

			specs, err := loader.Load(tmpFile)
			if (err != nil) != tt.wantErr {
				t.Fatalf("YAMLConfigLoader.Load() error = %v, wantErr %v", err, tt.wantErr)
			}
			if len(specs) != tt.wantCount {
				t.Errorf("YAMLConfigLoader.Load() returned %d specs, want %d", len(specs), tt.wantCount)
			}
			if tt.checkInstance != nil {
				tt.checkInstance(t, specs)
			}
		})
	}
}

func TestYAMLConfigLoader_Load_MalformedYAML(t *testing.T) {
	loader := &YAMLLoader{}
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "bad.yaml")
	// Invalid YAML: missing colon
	if err := os.WriteFile(tmpFile, []byte("program /bin/echo"), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := loader.Load(tmpFile)
	if err == nil {
		t.Error("expected error for malformed YAML, got nil")
	}
}

func TestYAMLConfigLoader_Load_MissingFile(t *testing.T) {
	loader := &YAMLLoader{}
	_, err := loader.Load("/nonexistent/path/to/config.yaml")
	if err == nil {
		t.Error("expected error for missing file, got nil")
	}
	if err != nil {
		// Ensure error message includes the path
		errStr := err.Error()
		if errStr[:16] != "read config file" {
			t.Errorf("error should start with 'read config file', got: %s", errStr)
		}
	}
}

func TestFormatInstanceName(t *testing.T) {
	tests := []struct {
		base   string
		index  int
		want   string
	}{
		{"program", 0, "program:00"},
		{"program", 1, "program:01"},
		{"program", 10, "program:10"},
		{"program", 99, "program:99"},
		{"program", 100, "program:100"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := FormatInstanceName(tt.base, tt.index); got != tt.want {
				t.Errorf("FormatInstanceName(%q, %d) = %q, want %q", tt.base, tt.index, got, tt.want)
			}
		})
	}
}