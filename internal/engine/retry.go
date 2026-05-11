package engine

// RetryStrategyFunc defines the decision logic for restarting a process.
// Returns true if the process should be restarted given the exit code and retry attempt number.
type RetryStrategyFunc func(exitCode int, attempt int) bool

// AlwaysRestart strategy always restarts the process.
func AlwaysRestart() RetryStrategyFunc {
	return func(exitCode int, attempt int) bool {
		return true
	}
}

// NeverRestart strategy never restarts the process.
func NeverRestart() RetryStrategyFunc {
	return func(exitCode int, attempt int) bool {
		return false
	}
}

// UnexpectedOnlyRestart strategy restarts only on unexpected exit codes.
func UnexpectedOnlyRestart(allowedCodes []int) RetryStrategyFunc {
	return func(exitCode int, attempt int) bool {
		if exitCode == 0 {
			return false
		}
		for _, allowed := range allowedCodes {
			if exitCode == allowed {
				return false
			}
		}
		return true
	}
}
