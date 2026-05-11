package engine

// RetryStrategyFactory creates a RetryStrategyFunc based on autorestart string and exit codes.
func RetryStrategyFactory(autorestart string, exitcodes []int) RetryStrategyFunc {
	switch autorestart {
	case "always":
		return AlwaysRestart()
	case "never":
		return NeverRestart()
	case "unexpected":
		return UnexpectedOnlyRestart(exitcodes)
	default:
		// Default to never restart for unknown values
		return NeverRestart()
	}
}

// RetryStrategyFromExpectedCodes creates a RetryStrategyFunc from an ExpectedCodes map.
// If expectedCodes is nil, only exit code 0 is considered expected (unexpected restart).
// If expectedCodes is empty, all exit codes are unexpected (always restart).
// Otherwise, only the keys with true values are considered expected exit codes.
func RetryStrategyFromExpectedCodes(expectedCodes map[int]bool) RetryStrategyFunc {
	if expectedCodes == nil {
		// nil map → only exit code 0 is expected
		return UnexpectedOnlyRestart([]int{0})
	}
	if len(expectedCodes) == 0 {
		// Empty map → all exit codes are unexpected, always restart
		return AlwaysRestart()
	}
	// Collect keys where value is true
	var allowed []int
	for code, ok := range expectedCodes {
		if ok {
			allowed = append(allowed, code)
		}
	}
	return UnexpectedOnlyRestart(allowed)
}
