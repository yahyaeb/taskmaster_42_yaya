package app

// Defaults for channels, reload/stop timing (keep magic numbers in one place).

const (
	// StatusUpdateChanCapacity is the buffer size for the process status update channel.
	StatusUpdateChanCapacity = 100

	// ReloadDiffGraceMillis is added to spec stoptime when waiting for supervisor exit during reload diff.
	ReloadDiffGraceMillis = 300

	// StopKillVerifySeconds is how long Stop waits after SIGKILL for a terminal status signal.
	StopKillVerifySeconds = 2
)
