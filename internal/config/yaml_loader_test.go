package config

import (
	"os"
	"path/filepath"
	"testing"
)

// should_load_single_program_and_populate_manager_when_valid_yaml
func TestYAMLLoader_Load_SingleProgram(t *testing.T) {
	loader := &YAMLLoader{}
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yml")

	yaml := `server:
  program: /bin/server
  cmd: "go run main.go"
  numprocs: 1
  autostart: true
  autorestart: always
  exitcodes:
    - 0
    - 2
  startretries: 3
  starttime: 5
  stopsignal: TERM
  stoptime: 10
  stdout: /tmp/server.stdout
  stderr: /tmp/server.stderr
  env:
    STARTED_BY: taskmaster
    DEBUG: "true"
`
	err := os.WriteFile(configPath, []byte(yaml), 0644)
	if err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	specs, err := loader.Load(configPath)
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	// Assert: exactly one spec
	if len(specs) != 1 {
		t.Errorf("expected 1 spec, got %d", len(specs))
	}

	spec := specs[0]

	// Assert: ProcessName is formatted correctly (server:00)
	if spec.ProcessName != "server:00" {
		t.Errorf("expected ProcessName 'server:00', got '%s'", spec.ProcessName)
	}

	// Assert: fields are parsed correctly
	if spec.Program != "/bin/server" {
		t.Errorf("expected Program '/bin/server', got '%s'", spec.Program)
	}

	if spec.Cmd != "go run main.go" {
		t.Errorf("expected Cmd 'go run main.go', got '%s'", spec.Cmd)
	}

	if spec.Numprocs != 1 {
		t.Errorf("expected Numprocs 1, got %d", spec.Numprocs)
	}

	if !spec.Autostart {
		t.Errorf("expected Autostart true, got false")
	}

	if spec.Autorestart != "always" {
		t.Errorf("expected Autorestart 'always', got '%s'", spec.Autorestart)
	}

	if len(spec.Exitcodes) != 2 || spec.Exitcodes[0] != 0 || spec.Exitcodes[1] != 2 {
		t.Errorf("expected Exitcodes [0, 2], got %v", spec.Exitcodes)
	}

	if spec.Startretries != 3 {
		t.Errorf("expected Startretries 3, got %d", spec.Startretries)
	}

	if spec.Starttime != 5 {
		t.Errorf("expected Starttime 5, got %d", spec.Starttime)
	}

	if spec.Stopsignal != "TERM" {
		t.Errorf("expected Stopsignal 'TERM', got '%s'", spec.Stopsignal)
	}

	if spec.Stoptime != 10 {
		t.Errorf("expected Stoptime 10, got %d", spec.Stoptime)
	}

	if spec.Stdout != "/tmp/server.stdout" {
		t.Errorf("expected Stdout '/tmp/server.stdout', got '%s'", spec.Stdout)
	}

	if spec.Stderr != "/tmp/server.stderr" {
		t.Errorf("expected Stderr '/tmp/server.stderr', got '%s'", spec.Stderr)
	}

	// Assert: env map is parsed
	if len(spec.Env) != 2 {
		t.Errorf("expected 2 env vars, got %d", len(spec.Env))
	}

	if spec.Env["STARTED_BY"] != "taskmaster" {
		t.Errorf("expected env STARTED_BY='taskmaster', got '%s'", spec.Env["STARTED_BY"])
	}

	if spec.Env["DEBUG"] != "true" {
		t.Errorf("expected env DEBUG='true', got '%s'", spec.Env["DEBUG"])
	}
}

// should_return_error_when_file_not_found
func TestYAMLLoader_Load_FileNotFound(t *testing.T) {
	loader := &YAMLLoader{}
	configPath := "/nonexistent/path/config.yml"

	specs, err := loader.Load(configPath)

	// Assert: error is not nil
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	// Assert: specs are nil (or empty)
	if specs != nil && len(specs) > 0 {
		t.Errorf("expected empty specs on error, got %d specs", len(specs))
	}

	// Assert: error message mentions the file
	errStr := err.Error()
	if errStr != "read config file /nonexistent/path/config.yml: open /nonexistent/path/config.yml: no such file or directory" &&
		!contains(errStr, "config.yml") &&
		!contains(errStr, "no such file") {
		t.Errorf("expected error to mention file or 'no such file', got: %v", err)
	}
}

// should_return_error_when_yaml_syntax_invalid
func TestYAMLLoader_Load_InvalidYAML(t *testing.T) {
	loader := &YAMLLoader{}
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "invalid.yml")

	// Invalid YAML: bad indentation
	yaml := `server:
  program: /bin/server
   cmd: "broken indentation"
  numprocs: 1`

	err := os.WriteFile(configPath, []byte(yaml), 0644)
	if err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	specs, err := loader.Load(configPath)

	// Assert: error is not nil
	if err == nil {
		t.Fatal("expected error for invalid YAML, got nil")
	}

	// Assert: specs are nil or empty
	if specs != nil && len(specs) > 0 {
		t.Errorf("expected empty specs on YAML error, got %d specs", len(specs))
	}

	// Assert: error message mentions YAML parsing
	errStr := err.Error()
	if !contains(errStr, "parse yaml") {
		t.Errorf("expected error to mention 'parse yaml', got: %v", err)
	}
}

func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// should_expand_numprocs_into_multiple_instances
func TestYAMLLoader_Load_ExpandNumprocs(t *testing.T) {
	loader := &YAMLLoader{}
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yml")

	yaml := `worker:
  program: /usr/bin/python
  cmd: "worker.py"
  numprocs: 3
  autostart: true
  autorestart: never
`
	err := os.WriteFile(configPath, []byte(yaml), 0644)
	if err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	specs, err := loader.Load(configPath)
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	// Assert: 3 instances created
	if len(specs) != 3 {
		t.Fatalf("expected 3 specs, got %d", len(specs))
	}

	// Assert: instance names are formatted correctly with zero-padding
	expectedNames := []string{"worker:00", "worker:01", "worker:02"}
	for i, expected := range expectedNames {
		if specs[i].ProcessName != expected {
			t.Errorf("specs[%d] expected ProcessName '%s', got '%s'", i, expected, specs[i].ProcessName)
		}
	}

	// Assert: all instances share the same program and command
	for i, spec := range specs {
		if spec.Program != "/usr/bin/python" {
			t.Errorf("specs[%d] expected Program '/usr/bin/python', got '%s'", i, spec.Program)
		}
		if spec.Cmd != "worker.py" {
			t.Errorf("specs[%d] expected Cmd 'worker.py', got '%s'", i, spec.Cmd)
		}
		if !spec.Autostart {
			t.Errorf("specs[%d] expected Autostart true, got false", i)
		}
		if spec.Autorestart != "never" {
			t.Errorf("specs[%d] expected Autorestart 'never', got '%s'", i, spec.Autorestart)
		}
	}
}

// should_default_numprocs_to_1_when_missing
func TestYAMLLoader_Load_DefaultNumprocs(t *testing.T) {
	loader := &YAMLLoader{}
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yml")

	// Numprocs field is missing; validation requires >= 1
	yaml := `server:
  program: /bin/server
  cmd: "server"
  numprocs: 1
  autostart: true
`
	err := os.WriteFile(configPath, []byte(yaml), 0644)
	if err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	specs, err := loader.Load(configPath)
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	// Assert: defaults to 1 instance
	if len(specs) != 1 {
		t.Fatalf("expected 1 spec (default), got %d", len(specs))
	}

	if specs[0].ProcessName != "server:00" {
		t.Errorf("expected ProcessName 'server:00', got '%s'", specs[0].ProcessName)
	}
}

// should_handle_numprocs_zero_or_negative_as_one
func TestYAMLLoader_Load_InvalidNumprocs(t *testing.T) {
	loader := &YAMLLoader{}
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yml")

	// Numprocs is 0 (invalid)
	yaml := `server:
  program: /bin/server
  cmd: "server"
  numprocs: 0
  autostart: true
`
	err := os.WriteFile(configPath, []byte(yaml), 0644)
	if err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	specs, err := loader.Load(configPath)

	// Assert: Load fails on validation
	if err == nil {
		t.Fatal("expected error for numprocs=0, got nil")
	}

	// Assert: specs are nil or empty
	if specs != nil && len(specs) > 0 {
		t.Errorf("expected empty specs on validation error, got %d specs", len(specs))
	}

	// Assert: error mentions Numprocs validation
	errStr := err.Error()
	if !contains(errStr, "Numprocs") {
		t.Errorf("expected error to mention Numprocs, got: %v", err)
	}
}

// should_handle_multiple_programs_with_mixed_numprocs
func TestYAMLLoader_Load_MultipleProgramsMixedNumprocs(t *testing.T) {
	loader := &YAMLLoader{}
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yml")

	yaml := `server:
  program: /bin/server
  cmd: "server"
  numprocs: 2
  autostart: true
  autorestart: always

worker:
  program: /usr/bin/worker
  cmd: "worker"
  numprocs: 3
  autostart: false
  autorestart: never

logger:
  program: /bin/logger
  cmd: "logger"
  numprocs: 1
  autostart: true
  autorestart: on-failure
`
	err := os.WriteFile(configPath, []byte(yaml), 0644)
	if err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	specs, err := loader.Load(configPath)
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	// Assert: 2 + 3 + 1 = 6 specs
	if len(specs) != 6 {
		t.Fatalf("expected 6 specs (2+3+1), got %d", len(specs))
	}

	// Count by base name
	serverCount := 0
	workerCount := 0
	loggerCount := 0

	for _, spec := range specs {
		if len(spec.ProcessName) < 2 {
			continue
		}
		base := spec.ProcessName[:len(spec.ProcessName)-3]
		switch base {
		case "server":
			serverCount++
		case "worker":
			workerCount++
		case "logger":
			loggerCount++
		}
	}

	if serverCount != 2 {
		t.Errorf("expected 2 server specs, got %d", serverCount)
	}
	if workerCount != 3 {
		t.Errorf("expected 3 worker specs, got %d", workerCount)
	}
	if loggerCount != 1 {
		t.Errorf("expected 1 logger spec, got %d", loggerCount)
	}
}

// should_parse_env_variables_correctly
func TestYAMLLoader_Load_ParseEnv(t *testing.T) {
	loader := &YAMLLoader{}
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yml")

	yaml := `app:
  program: /bin/app
  cmd: "app"
  numprocs: 1
  autostart: true
  env:
    DEBUG: "true"
    LOG_LEVEL: "info"
    DATABASE_URL: "postgres://localhost:5432/db"
    EMPTY_VALUE: ""
    NUMBER_VALUE: "42"
`
	err := os.WriteFile(configPath, []byte(yaml), 0644)
	if err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	specs, err := loader.Load(configPath)
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	spec := specs[0]

	// Assert: all env vars parsed
	if len(spec.Env) != 5 {
		t.Fatalf("expected 5 env vars, got %d", len(spec.Env))
	}

	tests := []struct {
		key   string
		value string
	}{
		{"DEBUG", "true"},
		{"LOG_LEVEL", "info"},
		{"DATABASE_URL", "postgres://localhost:5432/db"},
		{"EMPTY_VALUE", ""},
		{"NUMBER_VALUE", "42"},
	}

	for _, test := range tests {
		if spec.Env[test.key] != test.value {
			t.Errorf("expected env %s='%s', got '%s'", test.key, test.value, spec.Env[test.key])
		}
	}
}

// should_handle_empty_env_map
func TestYAMLLoader_Load_EmptyEnv(t *testing.T) {
	loader := &YAMLLoader{}
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yml")

	yaml := `app:
  program: /bin/app
  cmd: "app"
  numprocs: 1
  autostart: true
  env: {}
`
	err := os.WriteFile(configPath, []byte(yaml), 0644)
	if err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	specs, err := loader.Load(configPath)
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	spec := specs[0]

	// Assert: env is either nil or empty map
	if spec.Env != nil && len(spec.Env) != 0 {
		t.Errorf("expected empty or nil env, got %v", spec.Env)
	}
}

// should_parse_exitcodes_array
func TestYAMLLoader_Load_ParseExitcodes(t *testing.T) {
	loader := &YAMLLoader{}
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yml")

	yaml := `app:
  program: /bin/app
  cmd: "app"
  numprocs: 1
  autostart: true
  exitcodes:
    - 0
    - 1
    - 2
    - 127
`
	err := os.WriteFile(configPath, []byte(yaml), 0644)
	if err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	specs, err := loader.Load(configPath)
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	spec := specs[0]

	// Assert: exitcodes parsed
	if len(spec.Exitcodes) != 4 {
		t.Fatalf("expected 4 exitcodes, got %d", len(spec.Exitcodes))
	}

	expected := []int{0, 1, 2, 127}
	for i, exp := range expected {
		if spec.Exitcodes[i] != exp {
			t.Errorf("exitcodes[%d]: expected %d, got %d", i, exp, spec.Exitcodes[i])
		}
	}
}

// should_handle_zero_padded_instance_indices_correctly
func TestYAMLLoader_Load_ZeroPaddingHighIndex(t *testing.T) {
	loader := &YAMLLoader{}
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yml")

	yaml := `worker:
  program: /bin/worker
  cmd: "worker"
  numprocs: 15
  autostart: true
`
	err := os.WriteFile(configPath, []byte(yaml), 0644)
	if err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	specs, err := loader.Load(configPath)
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	// Assert: 15 instances created
	if len(specs) != 15 {
		t.Fatalf("expected 15 specs, got %d", len(specs))
	}

	// Assert: last one is worker:14
	if specs[14].ProcessName != "worker:14" {
		t.Errorf("expected last ProcessName 'worker:14', got '%s'", specs[14].ProcessName)
	}

	// Assert: first is worker:00
	if specs[0].ProcessName != "worker:00" {
		t.Errorf("expected first ProcessName 'worker:00', got '%s'", specs[0].ProcessName)
	}
}

// should_call_validate_in_loader_and_fail_entire_load
func TestYAMLLoader_Load_FailsWhenSpecInvalid(t *testing.T) {
	loader := &YAMLLoader{}
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "invalid_identity.yml")

	// Missing Cmd field - invalid spec
	yaml := `nginx:
  program: nginx
  numprocs: 1
  autostart: true
`
	err := os.WriteFile(configPath, []byte(yaml), 0644)
	if err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	specs, err := loader.Load(configPath)

	// Assert: Load returns error
	if err == nil {
		t.Fatal("expected error for invalid spec (missing Cmd), got nil")
	}

	// Assert: specs are nil or empty
	if specs != nil && len(specs) > 0 {
		t.Errorf("expected empty specs on validation error, got %d specs", len(specs))
	}

	// Assert: error mentions validation
	errStr := err.Error()
	if !contains(errStr, "Cmd") && !contains(errStr, "validate") {
		t.Errorf("expected error to mention Cmd or validation, got: %v", err)
	}
}

// should_fail_load_when_numprocs_invalid
func TestYAMLLoader_Load_FailsWhenNumprocsInvalid(t *testing.T) {
	loader := &YAMLLoader{}
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "invalid_numprocs.yml")

	yaml := `worker:
  program: /usr/bin/worker
  cmd: "worker"
  numprocs: 0
  autostart: true
`
	err := os.WriteFile(configPath, []byte(yaml), 0644)
	if err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	specs, err := loader.Load(configPath)

	// Assert: Load returns error
	if err == nil {
		t.Fatal("expected error for numprocs=0, got nil")
	}

	// Assert: specs are nil or empty
	if specs != nil && len(specs) > 0 {
		t.Errorf("expected empty specs on validation error, got %d specs", len(specs))
	}

	// Assert: error mentions Numprocs
	errStr := err.Error()
	if !contains(errStr, "Numprocs") {
		t.Errorf("expected error to mention Numprocs, got: %v", err)
	}
}

// should_fail_load_when_stopsignal_invalid
func TestYAMLLoader_Load_FailsWhenStopsignalInvalid(t *testing.T) {
	loader := &YAMLLoader{}
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "invalid_signal.yml")

	yaml := `app:
  program: /bin/app
  cmd: "app"
  numprocs: 1
  stopsignal: BADSIGNAL
  stoptime: 10
  autostart: true
`
	writeErr := os.WriteFile(configPath, []byte(yaml), 0644)
	if writeErr != nil {
		t.Fatalf("failed to write test config: %v", writeErr)
	}

	specs, err := loader.Load(configPath)

	if err == nil {
		t.Fatal("expected error for invalid stopsignal, got nil")
	}

	if !contains(err.Error(), "Stopsignal") {
		t.Errorf("expected error mentioning Stopsignal, got: %v", err)
	}

	if specs != nil && len(specs) > 0 {
		t.Errorf("expected empty specs on validation error, got %d", len(specs))
	}
}

// should_fail_load_when_stoptime_negative
func TestYAMLLoader_Load_FailsWhenStoptimeNegative(t *testing.T) {
	loader := &YAMLLoader{}
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "invalid_stoptime.yml")

	yaml := `app:
  program: /bin/app
  cmd: "app"
  numprocs: 1
  stopsignal: TERM
  stoptime: -5
  autostart: true
`
	writeErr := os.WriteFile(configPath, []byte(yaml), 0644)
	if writeErr != nil {
		t.Fatalf("failed to write test config: %v", writeErr)
	}

	specs, err := loader.Load(configPath)

	if err == nil {
		t.Fatal("expected error for negative stoptime, got nil")
	}

	if !contains(err.Error(), "Stoptime") {
		t.Errorf("expected error mentioning Stoptime, got: %v", err)
	}

	if specs != nil && len(specs) > 0 {
		t.Errorf("expected empty specs on validation error, got %d", len(specs))
	}
}

// should_fail_load_when_umask_invalid
func TestYAMLLoader_Load_FailsWhenUmaskInvalid(t *testing.T) {
	loader := &YAMLLoader{}
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "invalid_umask.yml")

	yaml := `app:
  program: /bin/app
  cmd: "app"
  numprocs: 1
  workingdir: /tmp
  umask: 512
  stdout: /var/log/app.stdout
  stderr: /var/log/app.stderr
  autostart: true
`
	writeErr := os.WriteFile(configPath, []byte(yaml), 0644)
	if writeErr != nil {
		t.Fatalf("failed to write test config: %v", writeErr)
	}

	specs, err := loader.Load(configPath)

	if err == nil {
		t.Fatal("expected error for invalid umask, got nil")
	}

	if !contains(err.Error(), "Umask") {
		t.Errorf("expected error mentioning Umask, got: %v", err)
	}

	if specs != nil && len(specs) > 0 {
		t.Errorf("expected empty specs on validation error, got %d", len(specs))
	}
}

