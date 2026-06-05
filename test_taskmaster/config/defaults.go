package config

import (
	"os"
	"path/filepath"
	"time"
)

// Paths — resolved relative to the project root (cwd at runtime).
const (
	DaemonBin  = "taskmasterd"
	CtlBin     = "taskmasterctl"
	ConfigFile = "config.yml"
	LogFile    = "taskmaster.log"
)

// Temp paths
const (
	SocketPath       = "/tmp/taskmaster.sock"
	BackupConfigName = "taskmaster_config_backup.yml"
	DaemonStdout     = "taskmasterd_eval.stdout"
	DaemonStderr     = "taskmasterd_eval.stderr"
)

// Process log paths used by the hello program in config.yml
const (
	HelloStdoutLog       = "/tmp/hello.stdout"
	HelloStderrLog       = "/tmp/hello.stderr"
	SlowstopperStdoutLog = "/tmp/slowstopper.stdout"
	EnvReporterStdoutLog = "/tmp/envreporter.out"
	EnvReporterStderrLog = "/tmp/envreporter.err"
	EnvReporterProbePath = "/tmp/envreporter_probe"
	ServerUIDOut         = "/tmp/server.uid.out"
	ServerGIDOut         = "/tmp/server.gid.out"
	ServerUIDGIDProbe    = "/tmp/server.uidgid_probe"
)

// Timeouts
const (
	DaemonReadyTimeout   = 15 * time.Second
	StatusPollInterval   = 200 * time.Millisecond
	DefaultWaitTimeout   = 10 * time.Second
	ReloadWaitTimeout    = 8 * time.Second
	RestartWaitTimeout   = 4 * time.Second
	AutostartWaitTimeout = 2 * time.Second
	StopWaitTimeout      = 1 * time.Second
	LogFlushWait         = 2 * time.Second
	StopSettleWait       = 300 * time.Millisecond
)

// Build targets
const (
	DaemonBuildTarget = "./cmd/daemon"
	CtlBuildTarget    = "./cmd/ctl"
)

// MinExpectedPrograms is the minimum number of entries status must return in point1.
const MinExpectedPrograms = 3

// BackupPath returns the absolute path for the config backup in the OS temp dir.
func BackupPath() string {
	return filepath.Join(os.TempDir(), BackupConfigName)
}
