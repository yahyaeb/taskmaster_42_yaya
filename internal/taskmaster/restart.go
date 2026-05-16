package taskmaster

import "slices"

const (
	RestartAlways     = "always"
	RestartNever      = "never"
	RestartUnexpected = "unexpected"
)

func RestartPolicy(policy string, exitCode int, okCodes []int) bool {
	switch policy {
	case RestartAlways:
		return true
	case RestartNever:
		return false
	case RestartUnexpected:
		return !slices.Contains(okCodes, exitCode)
	}
	return false
}
