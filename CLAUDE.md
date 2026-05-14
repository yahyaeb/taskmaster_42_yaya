Always edit the project structure after each file edit / write.

Root:
  ├── main.go
  │   └── (no top-level defs)
  ├── go.mod
  ├── go.sum
  ├── config.yml
  ├── daemon (executable binary)
  ├── README.md (concise; daemon and test instructions)
  ├── taskmaster.md
  ├── 42_project.md
  ├── guide_lines.txt
  ├── .gitignore
  ├── .claude/
  │   └── settings.local.json
  ├── internal/
  │   ├── engine/
  │   │   ├── process.go (~150 lines)
  │   │   │   ├── type Process
  │   │   │   ├── type ExitCode
  │   │   │   ├── type ProcessExecutor (interface)
  │   │   │   ├── type OsProcessExecutor
  │   │   │   ├── func NewOsProcessExecutor
  │   │   │   └── type CommandBuilder
  │   │   ├── signal.go (~50 lines)
  │   │   │   ├── type SignalHandler (interface)
  │   │   │   ├── type OSSignalHandler
  │   │   │   ├── func SignalFromString
  │   │   │   └── func StopProcess
  │   │   ├── lifecycle.go (~130 lines)
  │   │   │   ├── type ProcessWatcher
  │   │   │   ├── func Executor
  │   │   │   ├── func watchForEarlyExit
  │   │   │   ├── func (*ProcessWatcher) Run
  │   │   │   ├── func ShouldRestart
  │   │   └── func procState
  │   │   └── *_test.go (none)
  │   ├── engine_exports.go
  │   │   └── (re-exports from engine)
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
  │   ├── config_exports.go
  │   │   └── (re-exports from config)
  │   ├── bus/
  │   │   ├── event.go
  │   │   │   ├── type Status
  │   │   │   ├── type ProcessUpdate
  │   │   │   └── type Updates
  │   │   └── *_test.go
  │   ├── bus_exports.go
  │   │   └── (re-exports from bus)
  │   ├── protocol/
  │   │   └── jsonrpc.go
  │   │       ├── const (error codes)
  │   │       ├── type RPCRequest
  │   │       ├── type RPCResponse
  │   │       ├── type RPCError
  │   │       ├── type ProcessInfo
  │   │       ├── type ActionRequest
  │   │       ├── type ActionResponse
  │   │       ├── type ReloadResponse
  │   │       ├── func NewErrorResponse
  │   │       └── func NewSuccessResponse
  │   └── app/
  │       ├── manager.go (~280 lines)
  │       │   ├── type ProcessChannels
  │       │   ├── type ReloadResult
  │       │   ├── type ProcessManager (interface)
  │       │   ├── type ProcessInstance
  │       │   │   ├── Stopped chan struct{} (per-process notification)
  │       │   ├── type Manager
  │       │   │   ├── mu sync.Mutex (protects all state)
  │       │   │   ├── Config map[string]*config.ConfigSpec
  │       │   │   └── Process map[string]*ProcessInstance
  │       │   ├── func NewManager
  │       │   ├── func (m *Manager) Start/Stop/Restart/Reload/Shutdown
  │       │   ├── func (m *Manager) GetProcessInfo/GetAllProcessInfo
  │       │   ├── func (m *Manager) StopAll/Spawn
  │       │   ├── func NewManagerFromConfig
  │       │   ├── func (m *Manager) runStatusLoop (background goroutine)
  │       │   ├── func (m *Manager) applyUpdate
  │       │   └── helpers (isRunning, formatUptime, closeChannel)
  │       ├── watchdog.go (~200 lines)
  │       │   ├── func (m *Manager) startWatchdog
  │       │   ├── func (m *Manager) Watchdog (goroutine loop per process)
  │       │   ├── func (m *Manager) spawnRun/killProcess/waitForExit
  │       │   ├── func (m *Manager) launchAndWait/evaluateExit
  │       │   └── helpers (validateWatchdog, resolveMaxRetries, isStopped, pidAfterStart)
  │       ├── config_diff.go (~80 lines)
  │       │   ├── func (m *Manager) applyConfigDiff
  │       │   ├── func configChanged
  │       │   └── func slicesEqual
  │       ├── rpc.go (~200 lines)
  │       │   ├── type SocketListener
  │       │   ├── func NewSocketListener / StartSocketListener (ProcessManager)
  │       │   ├── func (*SocketListener) serve / handleConn
  │       │   ├── func HandleConnection / RouteRequest (ProcessManager)
  │       │   ├── handler registry (table-driven dispatch)
  │       │   └── handlers (handleStart, handleStop, etc.)
  │       └── *_test.go (none - e2e tests only)
  ├── cmd/
  │   ├── daemon/
  │   │   └── main.go (~50 lines)
  │   │       ├── func main
  │   │       ├── SIGHUP handling for config reload
  │   │       └── manager.Spawn() for autostart
  │   └── ctl/
  │       └── main.go
  │           ├── func main
  │           └── func printUsage
  ├── e2e/
  │   ├── daemon_test.go
  │   ├── e2e_test.go
  │   ├── logs/
  │   │   └── (test run logs - timestamped)
  │   └── programs/
  │       ├── crasher/
  │       │   └── main.go
  │       ├── printer/
  │       │   └── main.go
  │       └── signal_trap/
  │           └── main.go
  ├── tmp/
  │   └── taskmaster.sock
  └── .git/

---