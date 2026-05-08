package engine

import (
	"testing"
)

func TestRetryStrategyFactory(t *testing.T) {
	tests := []struct {
		name        string
		autorestart string
		exitcodes   []int
		wantType    string
	}{
		{
			name:        "always",
			autorestart: "always",
			exitcodes:   []int{},
			wantType:    "AlwaysRestart",
		},
		{
			name:        "never",
			autorestart: "never",
			exitcodes:   []int{1, 2},
			wantType:    "NeverRestart",
		},
		{
			name:        "unexpected with exit codes",
			autorestart: "unexpected",
			exitcodes:   []int{0, 1},
			wantType:    "UnexpectedOnlyRestart",
		},
		{
			name:        "unknown defaults to never",
			autorestart: "invalid",
			exitcodes:   []int{},
			wantType:    "NeverRestart",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			strategy := RetryStrategyFactory(tt.autorestart, tt.exitcodes)
			switch tt.wantType {
			case "AlwaysRestart":
				if _, ok := strategy.(*AlwaysRestart); !ok {
					t.Errorf("expected AlwaysRestart, got %T", strategy)
				}
			case "NeverRestart":
				if _, ok := strategy.(*NeverRestart); !ok {
					t.Errorf("expected NeverRestart, got %T", strategy)
				}
			case "UnexpectedOnlyRestart":
				u, ok := strategy.(*UnexpectedOnlyRestart)
				if !ok {
					t.Errorf("expected UnexpectedOnlyRestart, got %T", strategy)
				} else {
					// Verify allowed codes match
					if len(u.AllowedCodes) != len(tt.exitcodes) {
						t.Errorf("allowed codes length mismatch: got %d, want %d", len(u.AllowedCodes), len(tt.exitcodes))
					}
				}
			}
		})
	}
}

func TestRetryStrategyFromExpectedCodes(t *testing.T) {
	tests := []struct {
		name         string
		expectedCodes map[int]bool
		wantType     string
		wantAllowed  []int
	}{
		{
			name:         "nil map defaults to unexpected with 0",
			expectedCodes: nil,
			wantType:     "UnexpectedOnlyRestart",
			wantAllowed:  []int{0},
		},
		{
			name:         "empty map -> always restart",
			expectedCodes: map[int]bool{},
			wantType:     "AlwaysRestart",
		},
		{
			name:         "single expected code",
			expectedCodes: map[int]bool{0: true},
			wantType:     "UnexpectedOnlyRestart",
			wantAllowed:  []int{0},
		},
		{
			name:         "multiple expected codes",
			expectedCodes: map[int]bool{0: true, 1: true, 2: false},
			wantType:     "UnexpectedOnlyRestart",
			wantAllowed:  []int{0, 1}, // only true values
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			strategy := RetryStrategyFromExpectedCodes(tt.expectedCodes)
			switch tt.wantType {
			case "AlwaysRestart":
				if _, ok := strategy.(*AlwaysRestart); !ok {
					t.Errorf("expected AlwaysRestart, got %T", strategy)
				}
			case "NeverRestart":
				if _, ok := strategy.(*NeverRestart); !ok {
					t.Errorf("expected NeverRestart, got %T", strategy)
				}
			case "UnexpectedOnlyRestart":
				u, ok := strategy.(*UnexpectedOnlyRestart)
				if !ok {
					t.Errorf("expected UnexpectedOnlyRestart, got %T", strategy)
				} else {
					// Verify allowed codes match
					if len(u.AllowedCodes) != len(tt.wantAllowed) {
						t.Errorf("allowed codes length mismatch: got %d, want %d", len(u.AllowedCodes), len(tt.wantAllowed))
					}
					// Check each allowed code is present
					for _, code := range tt.wantAllowed {
						found := false
						for _, c := range u.AllowedCodes {
							if c == code {
								found = true
								break
							}
						}
						if !found {
							t.Errorf("expected allowed code %d not found in %v", code, u.AllowedCodes)
						}
					}
				}
			}
		})
	}
}