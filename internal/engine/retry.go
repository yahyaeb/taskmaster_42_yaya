package engine

// RetryStrategy defines the interface for determining whether a process should be restarted.
type RetryStrategy interface {
	ShouldRestart(exitCode int, attempt int) bool
}

// AlwaysRestart is a strategy that always restarts the process.
type AlwaysRestart struct{}

// ShouldRestart always returns true.
func (AlwaysRestart) ShouldRestart(exitCode int, attempt int) bool {
	return true
}

// NeverRestart is a strategy that never restarts the process.
type NeverRestart struct{}

// ShouldRestart always returns false.
func (NeverRestart) ShouldRestart(exitCode int, attempt int) bool {
	return false
}

// UnexpectedOnlyRestart is a strategy that restarts only on unexpected exit codes.
type UnexpectedOnlyRestart struct {
	AllowedCodes []int
}

// ShouldRestart returns true only for unexpected exit codes (not 0 or in AllowedCodes).
func (u UnexpectedOnlyRestart) ShouldRestart(exitCode int, attempt int) bool {
	if exitCode == 0 {
		return false
	}

	for _, allowed := range u.AllowedCodes {
		if exitCode == allowed {
			return false
		}
	}

	return true
}
