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
  │       ├── channels.go
  │       │   ├── type ProcessChannels
  │       │   ├── func NewProcessChannels
  │       │   ├── type Command
  │       │   ├── type ReloadCommandResult
  │       │   ├── type ManagerCommand
  │       │   ├── type ManagerQuery
  │       │   └── type QueryResult
  │       ├── listener.go
  │       │   ├── type SocketListener
  │       │   ├── func NewSocketListener
  │       │   ├── func (sl *SocketListener) serve
  │       │   ├── func (sl *SocketListener) Stop
  │       │   ├── func (sl *SocketListener) Addr
  │       │   └── func StartSocketListener
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

## ARCHITECTURE & DEPENDENCY MAP

### Layer 1: Entry Points (cmd/)
```
daemon/main      ─────┐
ctl/main         ─────┼─ internal/app/Manager (orchestrator)
                      │   └─ via: channels.go (command queue)
                      │   └─ via: listener.go (socket RPC)
                      └─ Handler.RouteRequest()
```

### Layer 2: Core App (internal/app/)
**Contracts & Orchestration**
```
ProcessChannels
  ├─ type Command (structs for manager actions)
  ├─ type ManagerCommand (command dispatch)
  ├─ type ManagerQuery (query dispatch)
  └─ used by: Manager.Watchdog()

SocketListener
  ├─ NewSocketListener(path, Manager)
  ├─ serve() → Handler.HandleConnection()
  └─ used by: cmd/daemon/main

Handler (RPC dispatcher)
  ├─ RouteRequest(RPCRequest) → RPCResponse
  ├─ handleGetStatus() ─→ Manager.GetStatus()
  ├─ handleStart() ─────→ Manager.Spawn()
  ├─ handleStop() ──────→ Manager.Stop()
  ├─ handleRestart() ───→ Manager.Spawn() + Manager.Stop()
  ├─ handleReload() ────→ Manager.Load() + refresh channels
  └─ handleShutdown() ──→ Manager.Stop() (all) + exit

Manager (process orchestrator)
  ├─ Load(config) → create ProcessInstance per process
  ├─ Spawn(name) → call Watcher.Run()
  ├─ Stop(name) → call ProcessStopper.Stop()
  ├─ Watchdog() → listen on channels, dispatch commands
  └─ GetStatus() → collect state from all instances
```

### Layer 3: Engine (process lifecycle) — internal/engine/
**Interfaces (contracts) & Implementations**

```
[ProcessExecutor] (interface: Start, Wait, Signal)
  └─ OsProcessExecutor (impl)
     ├─ Start(cmd, cwd, env, umask) → spawn OS process
     ├─ Wait() → block until process exits
     ├─ Signal(sig) → send signal to process
     └─ used by: ProcessWatcher.Run()

[RetryStrategy] (interface: ShouldRestart)
  ├─ AlwaysRestart (impl)
  ├─ NeverRestart (impl)
  └─ UnexpectedOnlyRestart (impl)
     └─ used by: ProcessWatcher.Run()
     └─ created by: RetryStrategyFactory(config.AutoRestart, config.ExitCodes)

[SignalHandler] (interface: Send)
  └─ OSSignalHandler (impl)
     └─ used by: ProcessStopper.Stop()

ProcessWatcher (high-level process supervisor)
  ├─ Run(executor, strategy, callbacks)
  ├─ spawns process via: ProcessExecutor.Start()
  ├─ decides restart via: RetryStrategy.ShouldRestart()
  ├─ fires callbacks: OnProcessStarted, OnProcessRunning, OnBackoff, OnSpawnFailed
  └─ used by: Manager.Spawn() → calls watcher.Run() in goroutine

ProcessStopper (graceful shutdown)
  ├─ Stop(pid, signal, stopTime)
  ├─ sends signal via: SignalHandler.Send()
  ├─ kills if timeout via: SignalHandler.Send(SIGKILL)
  └─ used by: Manager.Stop()

CommandBuilder (command assembly)
  ├─ BuildCommand(program, args) → *os.Cmd
  └─ used by: OsProcessExecutor.Start()
```

### Layer 4: Support (configuration, events, protocol) — internal/
**Factories, Config, Events**

```
RetryStrategyFactory
  ├─ RetryStrategyFactory(autoRestart string) → RetryStrategy
  ├─ RetryStrategyFromExpectedCodes(exitCodes) → RetryStrategy
  └─ used by: Manager.Load() (per process config)

ConfigSpec + YAMLLoader
  ├─ ConfigSpec.Validate() → errors
  ├─ YAMLLoader.Load(file) → ConfigFile → []ConfigSpec
  └─ used by: Manager.Load()

ProcessUpdate (event bus)
  ├─ type Status (starting, running, backoff, fatal, stopped)
  ├─ type ProcessUpdate (name, status, pid, timestamp)
  ├─ type Updates (channel of ProcessUpdate)
  └─ emitted by: Manager callbacks → UpdateBus

JSONRPCProtocol (schema)
  ├─ type RPCRequest (jsonrpc 2.0: method, params, id)
  ├─ type RPCResponse (result | error, id)
  ├─ type ProcessInfo (name, status, pid)
  ├─ type ActionRequest/Response (for start/stop/restart/reload)
  └─ used by: Handler.RouteRequest() ↔ ctl/main
```

---

## REFACTORING IMPACT ZONES

**If you change ProcessExecutor interface:**
  → Only OsProcessExecutor + ProcessWatcher affected  
  → Safe to change (isolated behind interface)

**If you change RetryStrategy interface:**
  → Only implementations + RetryStrategyFactory + ProcessWatcher affected  
  → Safe to refactor (contract is clear)

**If you change Manager interface:**
  → Handler + cmd/daemon/main affected  
  → Ripple to channels.go (command dispatch)  
  → ⚠️ Larger refactor zone

**If you change RPC protocol (jsonrpc.go):**
  → Handler + ctl/main affected  
  → Protocol is the daemon-to-client contract  
  → ⚠️ Breaking change (version carefully)

**If you change ProcessUpdate (bus/event.go):**
  → Any listener on UpdateBus affected  
  → ctl/main listens for status updates  
  → ⚠️ May break clients

---

## KEY DESIGN PRINCIPLES

1. **Interfaces as boundaries** — ProcessExecutor, RetryStrategy, SignalHandler are contracts, not coupling
2. **Single responsibility per layer** — cmd only knows Manager; app only knows engine; engine knows nothing of RPC
3. **Dependency injection** — Watcher doesn't create executor; it receives it (testable, flexible)
4. **Event-driven state** — Manager emits ProcessUpdate events; handlers listen (loose coupling)
5. **Graceful shutdown** — ProcessStopper manages signal→SIGKILL flow (timeout handling)
