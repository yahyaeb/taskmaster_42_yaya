package internal

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildCommandEnvMerge(t *testing.T) {
	cb := &commandBuilder{}

	spec := ConfigSpec{
		Cmd: "echo test",
		Env: map[string]string{
			"CUSTOM_VAR": "custom_value",
			"TEST_VAR":   "test_value",
		},
	}

	cmd, err := cb.BuildCommand(spec)
	if err != nil {
		t.Fatalf("buildCommand failed: %v", err)
	}

	if cmd == nil {
		t.Fatal("buildCommand returned nil command")
	}

	// Check that custom vars are in the command environment
	foundCustom := false
	foundTest := false
	for _, env := range cmd.Env {
		if env == "CUSTOM_VAR=custom_value" {
			foundCustom = true
		}
		if env == "TEST_VAR=test_value" {
			foundTest = true
		}
	}

	if !foundCustom {
		t.Error("CUSTOM_VAR not found in merged environment")
	}
	if !foundTest {
		t.Error("TEST_VAR not found in merged environment")
	}

	// Verify that at least one inherited environment variable exists
	if len(cmd.Env) < 3 {
		t.Errorf("Expected at least 3 environment variables (2 custom + at least 1 inherited), got %d", len(cmd.Env))
	}
}

func TestBuildCommandStdio(t *testing.T) {
	cb := &commandBuilder{}
	tmpDir := t.TempDir()

	stdoutFile := filepath.Join(tmpDir, "stdout.log")
	stderrFile := filepath.Join(tmpDir, "stderr.log")

	tests := []struct {
		name      string
		spec      ConfigSpec
		checkErr  bool
		checkOut  bool
		checkErr2 bool
	}{
		{
			name: "stdout and stderr files",
			spec: ConfigSpec{
				Cmd:    "echo test",
				Stdout: stdoutFile,
				Stderr: stderrFile,
			},
			checkErr:  false,
			checkOut:  true,
			checkErr2: true,
		},
		{
			name: "stdout only, stderr defaults to stdout",
			spec: ConfigSpec{
				Cmd:    "echo test",
				Stdout: stdoutFile,
			},
			checkErr:  false,
			checkOut:  true,
			checkErr2: false,
		},
		{
			name: "no stdout or stderr",
			spec: ConfigSpec{
				Cmd: "echo test",
			},
			checkErr:  false,
			checkOut:  false,
			checkErr2: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd, err := cb.BuildCommand(tt.spec)
			if err != nil {
				t.Fatalf("buildCommand failed: %v", err)
			}

			if cmd == nil {
				t.Fatal("buildCommand returned nil command")
			}

			if tt.checkOut {
				if cmd.Stdout == nil {
					t.Error("Stdout should not be nil")
				}
			} else {
				if cmd.Stdout != nil {
					t.Error("Stdout should be nil")
				}
			}

			if tt.checkErr2 {
				if cmd.Stderr == nil {
					t.Error("Stderr should not be nil")
				}
				// Verify stderr is different from stdout when both are specified
				if cmd.Stdout != nil && cmd.Stderr == cmd.Stdout {
					t.Error("Stderr should be different from Stdout when both are specified")
				}
			} else if tt.checkOut && !tt.checkErr2 {
				// When only stdout is specified, stderr should equal stdout
				if cmd.Stderr != cmd.Stdout {
					t.Error("Stderr should equal Stdout when stderr is not specified")
				}
			}

			// Clean up file handles
			if f, ok := cmd.Stdout.(*os.File); ok {
				f.Close()
			}
			if f, ok := cmd.Stderr.(*os.File); ok && cmd.Stderr != cmd.Stdout {
				f.Close()
			}
		})
	}
}

func TestBuildCommandStdioInvalidPath(t *testing.T) {
	cb := &commandBuilder{}

	spec := ConfigSpec{
		Cmd:    "echo test",
		Stdout: "/invalid/nonexistent/path/stdout.log",
	}

	_, err := cb.BuildCommand(spec)
	if err == nil {
		t.Error("Expected error for invalid stdout path")
	}
}

func TestBuildCommandEdgeCases(t *testing.T) {
	cb := &commandBuilder{}
	tmpDir := t.TempDir()

	tests := []struct {
		name      string
		spec      ConfigSpec
		shouldErr bool
		validate  func(*testing.T, *exec.Cmd)
	}{
		{
			name: "empty command",
			spec: ConfigSpec{
				Cmd: "",
			},
			shouldErr: true,
		},
		{
			name: "command with only whitespace",
			spec: ConfigSpec{
				Cmd: "   ",
			},
			shouldErr: true,
		},
		{
			name: "command with special characters in arguments",
			spec: ConfigSpec{
				Cmd: `echo "hello world" $VAR`,
			},
			shouldErr: false,
			validate: func(t *testing.T, cmd *exec.Cmd) {
				if len(cmd.Args) < 2 {
					t.Error("Expected multiple arguments from parsed command")
				}
			},
		},
		{
			name: "missing workdir (should be valid, uses current dir)",
			spec: ConfigSpec{
				Cmd:        "echo test",
				Workingdir: "",
			},
			shouldErr: false,
			validate: func(t *testing.T, cmd *exec.Cmd) {
				if cmd.Dir != "" {
					t.Error("Expected empty Dir when Workingdir not specified")
				}
			},
		},
		{
			name: "environment variable with special characters",
			spec: ConfigSpec{
				Cmd: "echo test",
				Env: map[string]string{
					"SPECIAL_VAR": "value$with!special@chars&symbols",
					"PATH_VAR":    "/path/to/dir:another/path",
				},
			},
			shouldErr: false,
			validate: func(t *testing.T, cmd *exec.Cmd) {
				foundSpecial := false
				foundPath := false
				for _, env := range cmd.Env {
					if strings.Contains(env, "SPECIAL_VAR=") && strings.Contains(env, "value$with!special@chars&symbols") {
						foundSpecial = true
					}
					if strings.Contains(env, "PATH_VAR=") && strings.Contains(env, "/path/to/dir:another/path") {
						foundPath = true
					}
				}
				if !foundSpecial {
					t.Error("Special character env var not properly set")
				}
				if !foundPath {
					t.Error("Path env var with colons not properly set")
				}
			},
		},
		{
			name: "empty env map",
			spec: ConfigSpec{
				Cmd: "echo test",
				Env: map[string]string{},
			},
			shouldErr: false,
			validate: func(t *testing.T, cmd *exec.Cmd) {
				if len(cmd.Env) == 0 {
					t.Error("Expected at least inherited environment variables")
				}
			},
		},
		{
			name: "nil env map",
			spec: ConfigSpec{
				Cmd: "echo test",
				Env: nil,
			},
			shouldErr: false,
			validate: func(t *testing.T, cmd *exec.Cmd) {
				if len(cmd.Env) == 0 {
					t.Error("Expected at least inherited environment variables")
				}
			},
		},
		{
			name: "command with empty string args",
			spec: ConfigSpec{
				Cmd: "echo  test",
			},
			shouldErr: false,
			validate: func(t *testing.T, cmd *exec.Cmd) {
				if cmd.Path == "" || cmd.Args == nil {
					t.Error("Command should be properly parsed despite extra spaces")
				}
			},
		},
		{
			name: "workdir set but empty",
			spec: ConfigSpec{
				Cmd:        "echo test",
				Workingdir: "",
			},
			shouldErr: false,
			validate: func(t *testing.T, cmd *exec.Cmd) {
				if cmd.Dir != "" {
					t.Error("Dir should remain empty string when Workingdir is empty")
				}
			},
		},
		{
			name: "valid workdir path",
			spec: ConfigSpec{
				Cmd:        "echo test",
				Workingdir: tmpDir,
			},
			shouldErr: false,
			validate: func(t *testing.T, cmd *exec.Cmd) {
				if cmd.Dir != tmpDir {
					t.Errorf("Expected Dir to be %s, got %s", tmpDir, cmd.Dir)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd, err := cb.BuildCommand(tt.spec)

			if tt.shouldErr {
				if err == nil {
					t.Error("Expected error but got none")
				}
				return
			}

			if err != nil {
				t.Fatalf("buildCommand failed: %v", err)
			}

			if cmd == nil {
				t.Fatal("buildCommand returned nil command")
			}

			if tt.validate != nil {
				tt.validate(t, cmd)
			}

			// Clean up file handles
			if f, ok := cmd.Stdout.(*os.File); ok {
				f.Close()
			}
			if f, ok := cmd.Stderr.(*os.File); ok && cmd.Stderr != cmd.Stdout {
				f.Close()
			}
		})
	}
}
