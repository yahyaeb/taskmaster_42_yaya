package app

import (
	"os"
	"path/filepath"
	"testing"

	"taskmaster/internal/bus"
	"taskmaster/internal/config"
)

// should_populate_manager_config_and_process_from_loaded_specs
func TestManager_Load_PopulatesConfigAndProcess(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yml")

	yaml := `programs:
  server:
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
`
	err := os.WriteFile(configPath, []byte(yaml), 0644)
	if err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	manager, err := NewManagerFromConfig(configPath)
	if err != nil {
		t.Fatalf("NewManagerFromConfig() failed: %v", err)
	}

	// Assert: manager is not nil
	if manager == nil {
		t.Fatal("expected Manager, got nil")
	}

	// Assert: Config map has 1 entry
	if len(manager.Config) != 1 {
		t.Errorf("expected 1 config entry, got %d", len(manager.Config))
	}

	// Assert: Config map has the correct key
	spec, exists := manager.Config["server:00"]
	if !exists {
		t.Fatalf("expected config key 'server:00', got keys: %v", keysConfig(manager.Config))
	}

	// Assert: ConfigSpec is populated correctly
	if spec.Program != "/bin/server" {
		t.Errorf("expected Program '/bin/server', got '%s'", spec.Program)
	}

	if spec.Cmd != "go run main.go" {
		t.Errorf("expected Cmd 'go run main.go', got '%s'", spec.Cmd)
	}

	if spec.Autostart != true {
		t.Errorf("expected Autostart true, got false")
	}

	// Assert: Process map has 1 entry
	if len(manager.Process) != 1 {
		t.Errorf("expected 1 process entry, got %d", len(manager.Process))
	}

	// Assert: Process map has the correct key
	proc, exists := manager.Process["server:00"]
	if !exists {
		t.Fatalf("expected process key 'server:00', got keys: %v", keysProc(manager.Process))
	}

	// Assert: ProcessInstance is initialized correctly
	if proc.Status != bus.STOPPED {
		t.Errorf("expected initial Status STOPPED, got %s", proc.Status)
	}

	// Assert: Intended flag matches autostart
	if proc.Intended != true {
		t.Errorf("expected Intended true (from autostart), got false")
	}

	if proc.RetryCount != 0 {
		t.Errorf("expected RetryCount 0, got %d", proc.RetryCount)
	}
}

// should_populate_manager_with_multiple_programs_and_instances
func TestManager_Load_MultipleProgramsAndInstances(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yml")

	yaml := `programs:
  server:
    program: /bin/server
    cmd: "server"
    numprocs: 2
    autostart: true
    autorestart: always
    stopsignal: TERM
    stoptime: 10

  worker:
    program: /usr/bin/worker
    cmd: "worker"
    numprocs: 3
    autostart: false
    autorestart: never
    stopsignal: INT
    stoptime: 5
`
	err := os.WriteFile(configPath, []byte(yaml), 0644)
	if err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	manager, err := NewManagerFromConfig(configPath)
	if err != nil {
		t.Fatalf("NewManagerFromConfig() failed: %v", err)
	}

	// Assert: Config map has 5 entries (2 servers + 3 workers)
	if len(manager.Config) != 5 {
		t.Fatalf("expected 5 config entries, got %d", len(manager.Config))
	}

	// Assert: Process map has 5 entries
	if len(manager.Process) != 5 {
		t.Fatalf("expected 5 process entries, got %d", len(manager.Process))
	}

	// Assert: server instances
	for i := 0; i < 2; i++ {
		key := formatKey("server", i)
		spec, exists := manager.Config[key]
		if !exists {
			t.Errorf("expected config key '%s', not found", key)
			continue
		}

		if spec.Program != "/bin/server" {
			t.Errorf("server instance %d: expected Program '/bin/server', got '%s'", i, spec.Program)
		}

		proc, procExists := manager.Process[key]
		if !procExists {
			t.Errorf("expected process key '%s', not found", key)
			continue
		}

		if proc.Status != bus.STOPPED {
			t.Errorf("server instance %d: expected Status STOPPED, got %s", i, proc.Status)
		}

		if proc.Intended != true {
			t.Errorf("server instance %d: expected Intended true (autostart), got false", i)
		}
	}

	// Assert: worker instances
	for i := 0; i < 3; i++ {
		key := formatKey("worker", i)
		spec, exists := manager.Config[key]
		if !exists {
			t.Errorf("expected config key '%s', not found", key)
			continue
		}

		if spec.Program != "/usr/bin/worker" {
			t.Errorf("worker instance %d: expected Program '/usr/bin/worker', got '%s'", i, spec.Program)
		}

		proc, procExists := manager.Process[key]
		if !procExists {
			t.Errorf("expected process key '%s', not found", key)
			continue
		}

		if proc.Status != bus.STOPPED {
			t.Errorf("worker instance %d: expected Status STOPPED, got %s", i, proc.Status)
		}

		if proc.Intended != false {
			t.Errorf("worker instance %d: expected Intended false (no autostart), got true", i)
		}
	}
}

// should_respect_autostart_flag_in_process_instance_intended
func TestManager_Load_RespectAutostart(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yml")

	yaml := `programs:
  auto:
    program: /bin/auto
    cmd: "auto"
    numprocs: 1
    autostart: true

  manual:
    program: /bin/manual
    cmd: "manual"
    numprocs: 1
    autostart: false
`
	err := os.WriteFile(configPath, []byte(yaml), 0644)
	if err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	manager, err := NewManagerFromConfig(configPath)
	if err != nil {
		t.Fatalf("NewManagerFromConfig() failed: %v", err)
	}

	// Assert: auto:00 has Intended=true
	autoProcAny := manager.Process["auto:00"]
	if autoProcAny == nil {
		t.Fatal("expected process 'auto:00'")
	}
	if !autoProcAny.Intended {
		t.Errorf("expected auto:00.Intended=true, got false")
	}

	// Assert: manual:00 has Intended=false
	manualProcAny := manager.Process["manual:00"]
	if manualProcAny == nil {
		t.Fatal("expected process 'manual:00'")
	}
	if manualProcAny.Intended {
		t.Errorf("expected manual:00.Intended=false, got true")
	}
}

// Helpers

func keysConfig(m map[string]*config.ConfigSpec) []string {
	var result []string
	for k := range m {
		result = append(result, k)
	}
	return result
}

func keysProc(m map[string]*ProcessInstance) []string {
	var result []string
	for k := range m {
		result = append(result, k)
	}
	return result
}

func formatKey(base string, index int) string {
	return config.FormatInstanceName(base, index)
}
