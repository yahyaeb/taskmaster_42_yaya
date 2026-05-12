package engine

import (
	"testing"
)

func TestShouldRestart_always(t *testing.T) {
	tests := []struct {
		name     string
		exitCode int
		want     bool
	}{
		{"exit code 0", 0, true},
		{"exit code 1", 1, true},
		{"negative exit code", -1, true},
		{"exit code 127", 127, true},
		{"exit code 255", 255, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ShouldRestart("always", tt.exitCode, nil)
			if got != tt.want {
				t.Errorf("ShouldRestart('always', %d, nil) = %v, want %v", tt.exitCode, got, tt.want)
			}
		})
	}
}

func TestShouldRestart_never(t *testing.T) {
	tests := []struct {
		name     string
		exitCode int
		want     bool
	}{
		{"exit code 0", 0, false},
		{"exit code 1", 1, false},
		{"negative exit code", -1, false},
		{"exit code 127", 127, false},
		{"exit code 255", 255, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ShouldRestart("never", tt.exitCode, nil)
			if got != tt.want {
				t.Errorf("ShouldRestart('never', %d, nil) = %v, want %v", tt.exitCode, got, tt.want)
			}
		})
	}
}

func TestShouldRestart_unexpected(t *testing.T) {
	tests := []struct {
		name     string
		exitCode int
		exitcodes []int
		want     bool
	}{
		{"exit code 0, no expected codes", 0, []int{}, false},
		{"exit code 1, no expected codes", 1, []int{}, true},
		{"exit code in allowed list", 2, []int{1, 2, 3}, false},
		{"exit code not in allowed list", 5, []int{1, 2, 3}, true},
		{"zero in allowed list still expected", 0, []int{0, 1}, false},
		{"negative exit code unexpected", -1, []int{1, 2}, true},
		{"exit code 127 not in list", 127, []int{1}, true},
		{"exit code 255 not in list", 255, []int{1, 2}, true},
		{"exit code in list with multiple", 1, []int{1, 127, 255}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ShouldRestart("unexpected", tt.exitCode, tt.exitcodes)
			if got != tt.want {
				t.Errorf("ShouldRestart('unexpected', %d, %v) = %v, want %v", tt.exitCode, tt.exitcodes, got, tt.want)
			}
		})
	}
}

func TestShouldRestart_default(t *testing.T) {
	// Unknown autorestart values default to never restart
	tests := []struct {
		autorestart string
		exitCode    int
		want        bool
	}{
		{"", 0, false},
		{"unknown", 1, false},
		{"invalid", 255, false},
	}

	for _, tt := range tests {
		t.Run(tt.autorestart, func(t *testing.T) {
			got := ShouldRestart(tt.autorestart, tt.exitCode, nil)
			if got != tt.want {
				t.Errorf("ShouldRestart(%q, %d, nil) = %v, want %v", tt.autorestart, tt.exitCode, got, tt.want)
			}
		})
	}
}
