Always edit the project structure after each file edit / write.

Root:
  ├── main.go
  │   └── (no top-level defs)
  ├── go.mod
  ├── go.sum
  ├── config.yml
  ├── daemon (executable binary)
  ├── README.md
  ├── taskmaster.md
  ├── 42_project.md
  ├── guide_lines.txt
  ├── .gitignore
  ├── .claude/
  │   └── settings.local.json
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
  │   │   │   ├── type RetryStrategyFunc (function type)
  │   │   │   ├── func AlwaysRestart() RetryStrategyFunc
  │   │   │   ├── func NeverRestart() RetryStrategyFunc
  │   │   │   └── func UnexpectedOnlyRestart(allowedCodes) RetryStrategyFunc
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
  │       ├── handler.go
  │       │   ├── func HandleConnection(conn, manager ProcessManager)
  │       │   ├── func RouteRequest(req, manager ProcessManager)
  │       │   ├── func withRecovery
  │       │   ├── func getNameFromParams
  │       │   ├── func handleGetStatus
  │       │   ├── func handleStart
  │       │   ├── func handleStop
  │       │   ├── func handleRestart
  │       │   ├── func handleReload
  │       │   └── func handleShutdown
  │       ├── manager.go
  │       │   ├── type ProcessManager (interface)
  │       │   ├── type ProcessInstance
  │       │   ├── type Manager
  │       │   │   ├── mu sync.Mutex (protects all state)
  │       │   │   ├── Config map[string]*config.ConfigSpec
  │       │   │   └── Process map[string]*ProcessInstance
  │       │   ├── func NewManager
  │       │   ├── func (m *Manager) Start (mutex-protected)
  │       │   ├── func (m *Manager) Stop (mutex-protected)
  │       │   ├── func (m *Manager) Restart (mutex-protected)
  │       │   ├── func (m *Manager) Reload (mutex-protected)
  │       │   ├── func (m *Manager) Shutdown (mutex-protected)
  │       │   ├── func (m *Manager) GetProcessInfo (mutex-protected)
  │       │   ├── func (m *Manager) GetAllProcessInfo (mutex-protected)
  │       │   ├── func (m *Manager) Watchdog (runs in goroutine per process)
  │       │   ├── func (m *Manager) applyConfigDiff
  │       │   └── helper functions (formatUptime, closeChannel, etc.)
  │       ├── channels.go
  │       │   ├── type ProcessChannels
  │       │   │   ├── Status chan bus.ProcessUpdate (Watchdog → Manager)
  │       │   │   └── Stop map[string]chan struct{} (Manager → Watchdog)
  │       │   ├── func NewProcessChannels
  │       │   ├── type ReloadResult
  │       │   └── type ProcessManager (interface)
  │       ├── listener.go
  │       │   ├── type SocketListener
  │       │   ├── func NewSocketListener(path, manager ProcessManager)
  │       │   ├── func (sl *SocketListener) serve(m ProcessManager)
  │       │   ├── func (sl *SocketListener) Stop
  │       │   ├── func (sl *SocketListener) Addr
  │       │   └── func StartSocketListener(path, manager ProcessManager)
  │       └── *_test.go
  ├── cmd/
  │   ├── daemon/
  │   │   └── main.go
  │   │       ├── type ManagerReference (atomic swap wrapper)
  │   │       ├── func NewManagerReference
  │   │       ├── func (mr *ManagerReference) GetManager
  │   │       ├── func (mr *ManagerReference) SetManager
  │   │       ├── func (mr *ManagerReference) [Implements ProcessManager methods]
  │   │       ├── func main
  │   │       └── SIGHUP handling for config reload
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

## REFACTORING COMPLETE

### Phase 1 ✓: Flatten Manager
- Removed double-method pattern (Start → startInternal)
- All *Internal methods inlined into handleCommand/handleQuery
- Removed ProcessQuerier, ProcessController, DaemonController interfaces
- Single ProcessManager interface for RPC boundary
- EventLoop directly owns and modifies state

### Phase 4 ✓: Function-based retry strategies
- RetryStrategy interface → RetryStrategyFunc (function type)
- AlwaysRestart, NeverRestart, UnexpectedOnlyRestart: types → factory functions
- Cleaner factory pattern, reduced type count

### Phase 5 ✓: Remove Channel Dispatch (SIMPLIFICATION)
- Replaced reqCh + EventLoop dispatch with sync.Mutex
- Removed Request/Response types from channels.go
- Public methods now directly acquire mutex and execute
- Removed ~60 lines of indirection code
- Student-friendly: trace `Start()` → lock → logic → unlock → return

### Impact
- Removed ~100+ lines of code (types, channel plumbing, EventLoop)
- No behavior changes - all e2e tests passing
- Clearer ownership model: mutex protects all state
- Simpler type hierarchy
- Direct method calls instead of string-based dispatch

---

## ARCHITECTURE (Simplified - No Channel Dispatch)

### Command Flow (Direct)
```
RPC Request → Handler.RouteRequest(ProcessManager)
            ↓
          Manager.{Start|Stop|Restart|Reload|Shutdown}
            ↓
          mu.Lock() → direct state mutation → mu.Unlock()
            ↓
          Return result directly (no channels)
```

### Watchdog Lifecycle
```
Manager.Start() spawns Watchdog(ConfigSpec, ProcessInstance)
           ↓
Watchdog creates ProcessWatcher with RetryStrategyFunc
           ↓
watcher.Run() with callbacks
           ↓
Callbacks send ProcessUpdate to Status channel (async)
           ↓
Manager.handleStatusUpdate applies to Process state
           ↓
(mu.Lock() for mutation, mu.Unlock())
```

### Key Design Change
**Before:** String dispatch via channel → EventLoop switch → handler
**After:** Direct method call → mutex.Lock() → logic → mutex.Unlock()

Benefits:
- No "magic strings" for command routing
- Stack traces show actual method names
- Standard Go concurrency pattern (mutex)
- Easier to debug (no hidden goroutine for dispatch)
