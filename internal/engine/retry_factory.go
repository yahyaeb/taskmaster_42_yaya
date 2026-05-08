package engine

// RetryStrategyFactory creates a RetryStrategy based on autorestart string and exit codes.
func RetryStrategyFactory(autorestart string, exitcodes []int) RetryStrategy {
	switch autorestart {
	case "always":
		return &AlwaysRestart{}
	case "never":
		return &NeverRestart{}
	case "unexpected":
		return &UnexpectedOnlyRestart{AllowedCodes: exitcodes}
	default:
		// Default to never restart for unknown values
		return &NeverRestart{}
	}
}

// RetryStrategyFromExpectedCodes creates a RetryStrategy from an ExpectedCodes map.
// If expectedCodes is nil, only exit code 0 is considered expected (unexpected restart).
// If expectedCodes is empty, all exit codes are unexpected (always restart).
// Otherwise, only the keys with true values are considered expected exit codes.
func RetryStrategyFromExpectedCodes(expectedCodes map[int]bool) RetryStrategy {
	if expectedCodes == nil {
		// Legacy behavior: nil means only exit code 0 is expected
		return &UnexpectedOnlyRestart{AllowedCodes: []int{0}}
	}
	if len(expectedCodes) == 0 {
		// No expected codes → always restart
		return &AlwaysRestart{}
	}
	// Collect keys where value is true
	var allowed []int
	for code, ok := range expectedCodes {
		if ok {
			allowed = append(allowed, code)
		}
	}
	return &UnexpectedOnlyRestart{AllowedCodes: allowed}
}
