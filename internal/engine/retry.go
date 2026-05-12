package engine

// ShouldRestart determines if a process should be restarted based on configuration.
// This replaces the complex RetryStrategy interface + factory pattern with a single function.
//
// Parameters:
//   - autorestart: "always", "never", or "unexpected"
//   - exitCode: the exit code the process returned
//   - exitcodes: list of expected exit codes (for "unexpected" mode)
//
// Returns true if the process should be restarted.
func ShouldRestart(autorestart string, exitCode int, exitcodes []int) bool {
	switch autorestart {
	case "always":
		return true
	case "never":
		return false
	case "unexpected":
		// Check if exit code is in the expected list
		for _, expected := range exitcodes {
			if exitCode == expected {
				return false // Expected exit, don't restart
			}
		}
		return true // Unexpected exit, restart
	default:
		// Unknown autorestart value, default to never restart
		return false
	}
}
