package engine

import (
	"testing"
)

func TestAlwaysRestart(t *testing.T) {
	strategy := AlwaysRestart{}

	tests := []struct {
		name     string
		exitCode int
		attempt  int
		want     bool
	}{
		{
			name:     "success exit code 0 attempt 0",
			exitCode: 0,
			attempt:  0,
			want:     true,
		},
		{
			name:     "error exit code 1 attempt 0",
			exitCode: 1,
			attempt:  0,
			want:     true,
		},
		{
			name:     "negative exit code attempt 5",
			exitCode: -1,
			attempt:  5,
			want:     true,
		},
		{
			name:     "command not found exit code 127 attempt 10",
			exitCode: 127,
			attempt:  10,
			want:     true,
		},
		{
			name:     "large exit code attempt 99",
			exitCode: 255,
			attempt:  99,
			want:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := strategy.ShouldRestart(tt.exitCode, tt.attempt)
			if got != tt.want {
				t.Errorf("ShouldRestart(%d, %d) = %v, want %v", tt.exitCode, tt.attempt, got, tt.want)
			}
		})
	}
}

func TestNeverRestart(t *testing.T) {
	strategy := NeverRestart{}

	tests := []struct {
		name     string
		exitCode int
		attempt  int
		want     bool
	}{
		{
			name:     "success exit code 0 attempt 0",
			exitCode: 0,
			attempt:  0,
			want:     false,
		},
		{
			name:     "error exit code 1 attempt 0",
			exitCode: 1,
			attempt:  0,
			want:     false,
		},
		{
			name:     "negative exit code attempt 5",
			exitCode: -1,
			attempt:  5,
			want:     false,
		},
		{
			name:     "command not found exit code 127 attempt 10",
			exitCode: 127,
			attempt:  10,
			want:     false,
		},
		{
			name:     "large exit code attempt 99",
			exitCode: 255,
			attempt:  99,
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := strategy.ShouldRestart(tt.exitCode, tt.attempt)
			if got != tt.want {
				t.Errorf("ShouldRestart(%d, %d) = %v, want %v", tt.exitCode, tt.attempt, got, tt.want)
			}
		})
	}
}

func TestUnexpectedOnlyRestart(t *testing.T) {
	tests := []struct {
		name        string
		strategy    UnexpectedOnlyRestart
		exitCode    int
		attempt     int
		want        bool
		description string
	}{
		{
			name:        "success exit code 0 no allowed codes",
			strategy:    UnexpectedOnlyRestart{AllowedCodes: []int{}},
			exitCode:    0,
			attempt:     0,
			want:        false,
			description: "expected: exit code 0 never restarts",
		},
		{
			name:        "error exit code 1 no allowed codes",
			strategy:    UnexpectedOnlyRestart{AllowedCodes: []int{}},
			exitCode:    1,
			attempt:     0,
			want:        true,
			description: "unexpected: exit code 1 restarts when not in allowed list",
		},
		{
			name:        "exit code in allowed list",
			strategy:    UnexpectedOnlyRestart{AllowedCodes: []int{1, 2, 3}},
			exitCode:    2,
			attempt:     0,
			want:        false,
			description: "expected: exit code 2 does not restart when in allowed list",
		},
		{
			name:        "exit code not in allowed list",
			strategy:    UnexpectedOnlyRestart{AllowedCodes: []int{1, 2, 3}},
			exitCode:    5,
			attempt:     0,
			want:        true,
			description: "unexpected: exit code 5 restarts when not in allowed list",
		},
		{
			name:        "zero in allowed codes still no restart for 0",
			strategy:    UnexpectedOnlyRestart{AllowedCodes: []int{0, 1}},
			exitCode:    0,
			attempt:     0,
			want:        false,
			description: "expected: exit code 0 never restarts even if in allowed list",
		},
		{
			name:        "negative exit code unexpected",
			strategy:    UnexpectedOnlyRestart{AllowedCodes: []int{1, 2}},
			exitCode:    -1,
			attempt:     0,
			want:        true,
			description: "unexpected: negative exit code restarts",
		},
		{
			name:        "command not found exit code 127",
			strategy:    UnexpectedOnlyRestart{AllowedCodes: []int{1}},
			exitCode:    127,
			attempt:     0,
			want:        true,
			description: "unexpected: exit code 127 restarts when not in allowed list",
		},
		{
			name:        "large exit code unexpected",
			strategy:    UnexpectedOnlyRestart{AllowedCodes: []int{1, 2}},
			exitCode:    255,
			attempt:     0,
			want:        true,
			description: "unexpected: exit code 255 restarts",
		},
		{
			name:        "exit code 1 in allowed list with multiple codes",
			strategy:    UnexpectedOnlyRestart{AllowedCodes: []int{1, 127, 255}},
			exitCode:    1,
			attempt:     0,
			want:        false,
			description: "expected: exit code 1 does not restart when in allowed list",
		},
		{
			name:        "different attempt numbers",
			strategy:    UnexpectedOnlyRestart{AllowedCodes: []int{1}},
			exitCode:    5,
			attempt:     10,
			want:        true,
			description: "attempt number does not affect restart decision",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.strategy.ShouldRestart(tt.exitCode, tt.attempt)
			if got != tt.want {
				t.Errorf("ShouldRestart(%d, %d) = %v, want %v (%s)", tt.exitCode, tt.attempt, got, tt.want, tt.description)
			}
		})
	}
}