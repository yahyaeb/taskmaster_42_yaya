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
  │       │   ├── func NewManager
  │       │   ├── func (m *Manager) EventLoop (owns all state)
  │       │   ├── func (m *Manager) handleCommand (inlined: doStart, doStop, doRestart, doReload, doShutdown)
  │       │   ├── func (m *Manager) handleQuery (inlined: get, list)
  │       │   ├── func (m *Manager) handleStatusUpdate
  │       │   ├── func (m *Manager) Watchdog
  │       │   ├── func (m *Manager) GetProcessInfo
  │       │   ├── func (m *Manager) GetAllProcessInfo
  │       │   ├── func (m *Manager) Start
  │       │   ├── func (m *Manager) Stop
  │       │   ├── func (m *Manager) Restart
  │       │   ├── func (m *Manager) Reload
  │       │   ├── func (m *Manager) Shutdown
  │       │   ├── type ReloadResult
  │       │   └── helper functions (formatUptime, closeChannel, etc.)
  │       ├── channels.go
  │       │   ├── type ProcessChannels
  │       │   ├── func NewProcessChannels
  │       │   ├── type ManagerCommand
  │       │   ├── type ManagerQuery
  │       │   ├── type QueryResult
  │       │   └── type ReloadCommandResult
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

### Phases Skipped
- Phase 2: Callbacks already simple, refactoring adds complexity
- Phase 3: Stop map works fine, context tracking adds complexity

### Impact
- Removed ~36 lines of code (type definitions, methods)
- No behavior changes - all 36 e2e tests passing
- Clearer ownership model: EventLoop owns all state
- Simpler type hierarchy

---

## ARCHITECTURE (Updated)

### Command Flow
```
RPC Request → Handler.RouteRequest(ProcessManager)
            ↓
          Manager.{Start|Stop|Restart|Reload|Shutdown}
            ↓
          Send ManagerCommand to commandCh
            ↓
          EventLoop receives → handleCommand
            ↓
          doStart/doStop/doRestart/doReload/doShutdown
            ↓
          Direct state mutation or Watchdog spawn
            ↓
          Status updates via Status channel
            ↓
          handleStatusUpdate applies changes
```

### Query Flow
```
RPC Request → Handler.RouteRequest(ProcessManager)
           ↓
         Manager.GetProcessInfo/GetAllProcessInfo
           ↓
         Send ManagerQuery to queryCh
           ↓
         EventLoop receives → drainStatus() → handleQuery
           ↓
         Direct Process map read (no methods needed)
           ↓
         Send QueryResult
```

### Watchdog Lifecycle
```
EventLoop spawns Watchdog(ConfigSpec, ProcessInstance)
           ↓
Watchdog creates ProcessWatcher with RetryStrategyFunc
           ↓
watcher.Run() with callbacks (unchanged from Phase 1)
           ↓
Callbacks send ProcessUpdate to Status channel
           ↓
EventLoop.handleStatusUpdate applies to Process state
```
