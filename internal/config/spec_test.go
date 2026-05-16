package config

import (
	"testing"
)

// Identity Slice Tests

// should_return_nil_when_identity_slice_valid
func TestConfigSpec_Validate_ValidIdentity(t *testing.T) {
	spec := ConfigSpec{
		Program:     "nginx",
		Cmd:         "nginx -g daemon off;",
		Numprocs:    1,
		Autostart:   true,
		Autorestart: "always",
	}

	err := spec.Validate()
	if err != nil {
		t.Errorf("expected nil error for valid spec, got: %v", err)
	}
}

// should_return_error_when_program_empty
func TestConfigSpec_Validate_ProgramEmpty(t *testing.T) {
	spec := ConfigSpec{
		Program:     "",
		Cmd:         "nginx",
		Numprocs:    1,
		Autostart:   true,
		Autorestart: "always",
	}

	err := spec.Validate()
	if err == nil {
		t.Fatal("expected error for empty Program, got nil")
	}

	if err.Error() != "program must not be empty" {
		t.Errorf("expected error 'program must not be empty', got: %v", err)
	}
}

// should_return_error_when_cmd_empty
func TestConfigSpec_Validate_CmdEmpty(t *testing.T) {
	spec := ConfigSpec{
		Program:     "nginx",
		Cmd:         "",
		Numprocs:    1,
		Autostart:   true,
		Autorestart: "always",
	}

	err := spec.Validate()
	if err == nil {
		t.Fatal("expected error for empty Cmd, got nil")
	}

	if err.Error() != "cmd must not be empty" {
		t.Errorf("expected error 'cmd must not be empty', got: %v", err)
	}
}

// should_return_error_when_numprocs_zero
func TestConfigSpec_Validate_NumprocsZero(t *testing.T) {
	spec := ConfigSpec{
		Program:     "nginx",
		Cmd:         "nginx",
		Numprocs:    0,
		Autostart:   true,
		Autorestart: "always",
	}

	err := spec.Validate()
	if err == nil {
		t.Fatal("expected error for Numprocs=0, got nil")
	}

	if err.Error() != "numprocs must be >= 1, got 0" {
		t.Errorf("expected error 'numprocs must be >= 1, got 0', got: %v", err)
	}
}

// should_return_error_when_numprocs_negative
func TestConfigSpec_Validate_NumprocsNegative(t *testing.T) {
	spec := ConfigSpec{
		Program:     "nginx",
		Cmd:         "nginx",
		Numprocs:    -5,
		Autostart:   true,
		Autorestart: "always",
	}

	err := spec.Validate()
	if err == nil {
		t.Fatal("expected error for Numprocs=-5, got nil")
	}

	if err.Error() != "numprocs must be >= 1, got -5" {
		t.Errorf("expected error 'numprocs must be >= 1, got -5', got: %v", err)
	}
}

// Control Slice Tests

// should_return_nil_when_control_slice_valid
func TestConfigSpec_Validate_ValidControl(t *testing.T) {
	spec := ConfigSpec{
		Program:     "nginx",
		Cmd:         "nginx",
		Numprocs:    1,
		Stopsignal:  "TERM",
		Stoptime:    10,
		Starttime:   5,
		Autostart:   true,
		Autorestart: "always",
	}

	err := spec.Validate()
	if err != nil {
		t.Errorf("expected nil error for valid control spec, got: %v", err)
	}
}

// should_return_error_when_stopsignal_invalid
func TestConfigSpec_Validate_InvalidStopsignal(t *testing.T) {
	spec := ConfigSpec{
		Program:    "nginx",
		Cmd:        "nginx",
		Numprocs:   1,
		Stopsignal: "SIGGG",
		Stoptime:   10,
		Starttime:  5,
	}

	err := spec.Validate()
	if err == nil {
		t.Fatal("expected error for invalid stopsignal, got nil")
	}

	errStr := err.Error()
	if !contains(errStr, "stopsignal") {
		t.Errorf("expected error mentioning stopsignal, got: %v", err)
	}
}

// should_return_error_when_stoptime_negative
func TestConfigSpec_Validate_NegativeStoptime(t *testing.T) {
	spec := ConfigSpec{
		Program:    "nginx",
		Cmd:        "nginx",
		Numprocs:   1,
		Stopsignal: "TERM",
		Stoptime:   -5,
		Starttime:  5,
	}

	err := spec.Validate()
	if err == nil {
		t.Fatal("expected error for negative stoptime, got nil")
	}

	if !contains(err.Error(), "stoptime") {
		t.Errorf("expected error mentioning stoptime, got: %v", err)
	}
}

// should_return_error_when_starttime_negative
func TestConfigSpec_Validate_NegativeStarttime(t *testing.T) {
	spec := ConfigSpec{
		Program:    "nginx",
		Cmd:        "nginx",
		Numprocs:   1,
		Stopsignal: "TERM",
		Stoptime:   10,
		Starttime:  -3,
	}

	err := spec.Validate()
	if err == nil {
		t.Fatal("expected error for negative starttime, got nil")
	}

	if !contains(err.Error(), "starttime") {
		t.Errorf("expected error mentioning starttime, got: %v", err)
	}
}

// should_accept_all_valid_stop_signals
func TestConfigSpec_Validate_AllValidSignals(t *testing.T) {
	validSignals := []string{"TERM", "HUP", "INT", "QUIT", "KILL", "USR1", "USR2"}

	for _, sig := range validSignals {
		spec := ConfigSpec{
			Program:    "app",
			Cmd:        "app",
			Numprocs:   1,
			Stopsignal: sig,
			Stoptime:   10,
			Starttime:  5,
		}

		err := spec.Validate()
		if err != nil {
			t.Errorf("signal %s should be valid, got error: %v", sig, err)
		}
	}
}

// Context Slice Tests

// should_return_nil_when_context_slice_valid
func TestConfigSpec_Validate_ValidContext(t *testing.T) {
	spec := ConfigSpec{
		Program:    "app",
		Cmd:        "app",
		Numprocs:   1,
		Stopsignal: "TERM",
		Stoptime:   10,
		Starttime:  5,
		Workingdir: "/tmp",
		Umask:      0o022,
		Stdout:     "/var/log/app.stdout",
		Stderr:     "/var/log/app.stderr",
	}

	err := spec.Validate()
	if err != nil {
		t.Errorf("expected nil error for valid context spec, got: %v", err)
	}
}

// should_return_error_when_umask_out_of_range
func TestConfigSpec_Validate_InvalidUmask(t *testing.T) {
	tests := []struct {
		name   string
		umask  int
		should bool // true = should error
	}{
		{"0o000", 0o000, false},
		{"0o022", 0o022, false},
		{"0o777", 0o777, false},
		{"-1", -1, true},
		{"512", 512, true},
	}

	for _, test := range tests {
		spec := ConfigSpec{
			Program:    "app",
			Cmd:        "app",
			Numprocs:   1,
			Stopsignal: "TERM",
			Stoptime:   10,
			Starttime:  5,
			Workingdir: "/tmp",
			Umask:      test.umask,
			Stdout:     "/var/log/app.stdout",
			Stderr:     "/var/log/app.stderr",
		}

		err := spec.Validate()

		if test.should && err == nil {
			t.Errorf("umask %s (%d): expected error, got nil", test.name, test.umask)
		}

		if !test.should && err != nil {
			t.Errorf("umask %s (%d): expected no error, got: %v", test.name, test.umask, err)
		}

		if err != nil && !contains(err.Error(), "umask") {
			t.Errorf("umask %s: expected error mentioning umask, got: %v", test.name, err)
		}
	}
}
