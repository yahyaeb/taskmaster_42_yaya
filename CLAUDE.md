Always edit the project structure after each file edit / write.

Root:
  ├── main.go
  │   └── (no top-level defs)
  ├── go.mod
  ├── go.sum
  ├── config.yml
  ├── .context/
  │   ├── watchdog_legacy.go
  │   │   ├── func legacyWatchdog
  │   │   └── struct legacyWatcher
  │   ├── watchdog_legacy_test.go
  │   │   └── func TestLegacyWatchdog
  │   └── integration_test_legacy.go
  │       └── func TestLegacyIntegration
  ├── internal/
  │   ├── engine/
  │   │   ├── executor.go
  │   │   │   ├── type ExitCode
  │   │   │   ├── type Process
  │   │   │   └── type ProcessExecutor
  │   │   ├── watcher.go
  │   │   │   ├── type RetryConfig
  │   │   │   ├── type ProcessWatcher
  │   │   │   │   ├── OnProcessStarted func(pid int)
  │   │   │   │   ├── OnProcessRunning func(pid int)
  │   │   │   │   ├── OnBackoff func(attempt int)
  │   │   │   │   ├── OnSpawnFailed func(attempt int)
  │   │   │   │   ├── OnStarting func()
  │   │   │   │   └── StarttimeSec int
  │   │   │   ├── func NewProcessWatcher
  │   │   │   ├── func NewProcessWatcherWithStrategy
  │   │   │   ├── type ProcessSpawner
  │   │   │   ├── func (pw *ProcessWatcher) Run
  │   │   │   └── func procState
  │   │   ├── stopper.go
  │   │   │   ├── type ProcessStopper
  │   │   │   ├── func NewProcessStopper
  │   │   │   └── func (ps *ProcessStopper) Stop
  │   │   ├── retry.go
  │   │   │   ├── type RetryStrategy
  │   │   │   ├── type AlwaysRestart
  │   │   │   ├── func (AlwaysRestart) ShouldRestart
  │   │   │   ├── type NeverRestart
  │   │   │   ├── func (NeverRestart) ShouldRestart
  │   │   │   ├── type UnexpectedOnlyRestart
  │   │   │   └── func (u UnexpectedOnlyRestart) ShouldRestart
  │   │   ├── retry_factory.go
  │   │   │   ├── func RetryStrategyFactory
  │   │   │   └── func RetryStrategyFromExpectedCodes
  │   │   ├── signaler.go
  │   │   │   ├── type SignalHandler
  │   │   │   ├── type OSSignalHandler
  │   │   │   └── func (h *OSSignalHandler) Send
  │   │   ├── builder.go
  │   │   │   ├── type CommandBuilder
  │   │   │   └── func (cb *CommandBuilder) BuildCommand
  │   │   ├── os_executor.go
  │   │   │   ├── var umaskLock
  │   │   │   ├── type OsProcessExecutor
  │   │   │   ├── func NewOsProcessExecutor
  │   │   │   ├── func (e *OsProcessExecutor) Start
  │   │   │   ├── func (e *OsProcessExecutor) Wait
  │   │   │   ├── func (e *OsProcessExecutor) Signal
  │   │   │   ├── func (e *OsProcessExecutor) closeFilesForPID
  │   │   │   └── func (e *OsProcessExecutor) closeFiles
  │   │   └── *_test.go
  │   ├── config/
  │   │   ├── spec.go
  │   │   │   ├── type ConfigSpec
  │   │   │   ├── type Loader
  │   │   │   └── func (c *ConfigSpec) Validate
  │   │   ├── yaml_loader.go
  │   │   │   ├── type ConfigFile
  │   │   │   ├── type YAMLLoader
  │   │   │   ├── func (l *YAMLLoader) Load
  │   │   │   └── func FormatInstanceName
  │   │   └── *_test.go
  │   ├── bus/
  │   │   ├── event.go
  │   │   │   ├── type Status
  │   │   │   ├── type ProcessUpdate
  │   │   │   └── type Updates
  │   │   └── *_test.go
  │   └── app/
  │       ├── handler.go
  │       │   ├── func HandleConnection
  │       │   ├── func RouteRequest
  │       │   ├── func withRecovery
  │       │   ├── func getNameFromParams
  │       │   ├── func handleGetStatus
  │       │   ├── func handleStart
  │       │   ├── func handleStop
  │       │   ├── func handleRestart
  │       │   ├── func handleReload
  │       │   └── func handleShutdown
 │       ├── manager.go
 │       │   ├── type ProcessInstance
 │       │   ├── func (pi *ProcessInstance) GetStatus
 │       │   ├── func (pi *ProcessInstance) SetStatus
 │       │   ├── func (pi *ProcessInstance) GetPid
 │       │   ├── func (pi *ProcessInstance) SetPid
 │       │   ├── func (pi *ProcessInstance) SetStateOnStart
 │       │   ├── func (pi *ProcessInstance) SetStateOnRunning
 │       │   ├── func (pi *ProcessInstance) SetStateOnBackoff
 │       │   ├── func (pi *ProcessInstance) State
 │       │   ├── type Manager
 │       │   ├── func NewManager
 │       │   ├── func (m *Manager) Watchdog
 │       │   ├── func sendFinalUpdate
 │       │   ├── func Stop
 │       │   ├── func Spawn
 │       │   ├── func Load
 │       │   └── func closeChannel
  │       └── *_test.go
  ├── cmd/
  │   ├── daemon/
  │   │   └── main.go
  │   │       ├── func read
  │   │       └── func main
  │   └── ctl/
  │       └── main.go
  │           ├── func main
  │           └── func printUsage
  ├── tmp/
  │   └── taskmaster.sock
  └── .git/
