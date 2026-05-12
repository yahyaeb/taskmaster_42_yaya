### yel-bouk
1. CTL shell (cmd/ctl/main.go)
Connect to the daemon and give the user a shell. Use a readline library for line editing + history. Commands: status, start <name>, stop <name>, restart <name>, reload, exit.

net.Dial("unix", "/tmp/taskmaster.sock")
Encode RPCRequest as JSON, decode RPCResponse
status prints a table: name / status / pid / uptime / retries

2. Hot-reload diff engine (called on SIGHUP)
Compare old config map to new config map. Three cases:

Program exists in new but not old → start it
Program exists in old but not new → stop it
Program exists in both → compare fields, only restart if something actually changed

3. Logger
Subscribe to chan bus.ProcessUpdate. On every update, append a line to a log file:
[2024-01-15 14:32:01] nginx:01 → running (pid 4821)
4. stopsignal config
Parse the stopsignal string from ConfigSpec ("TERM", "USR1", "KILL"…) and convert it to an os.Signal before passing it to ProcessStopper. Right now it's hardcoded SIGTERM.
5. SIGHUP wiring
Catch SIGHUP in the daemon's main loop using signal.Notify. When received: reload config from disk, run the diff engine, apply changes.
6. Shutdown command
Taskmaster.Shutdown RPC → stop all running processes cleanly → exit the daemon.

---

## TODO (Evaluation Completion)

### Mandatory
**CTL Interactive Shell** (`cmd/ctl/main.go`) - Readline library for line editing + history. Commands: `status`, `start <name>`, `stop <name>`, `restart <name>`, `reload`, `exit`. Status prints table: name/status/pid/uptime/retries.
**Logger** - Subscribe to `chan bus.ProcessUpdate`, append to log file: `[2024-01-15 14:32:01] nginx:01 → running (pid 4821)`. Must log: start, stop, restart, unexpected exit, abort after max retries.

### Bonus
**Privilege De-escalation** - Config option to run program as specific user (setuid/setgid).
**Client/Server Architecture** - Daemon + separate control program via unix socket (already partially done - verify full separation).
**Advanced Logging** - Email alerts, HTTP webhooks, or syslog integration on critical events.
**Attach Console** (+2 points) - Like `screen`/`tmux`: attach to supervised process stdin/stdout in current terminal.

### Evaluation Checklist
Kill supervised process → verifies auto-restart works.
Supervise failing process → verifies abort after max retries.
SIGHUP triggers hot-reload without affecting unchanged processes.
All 13 config options from evaluation guide tested and working.

---

## Architecture Overview

```
┌─────────────────────────────────────────────────────────────────┐
│                         USER LAYER                               │
│  ┌──────────────┐          ┌──────────────┐                    │
│  │  taskmasterctl │          │   SIGHUP     │                    │
│  │  (cmd/ctl/)    │          │   signal     │                    │
│  └───────┬────────┘          └──────┬───────┘                    │
└──────────┼──────────────────────────┼────────────────────────────┘
           │                          │
           │   Unix Socket            │
           │   /tmp/taskmaster.sock   │
           ▼                          ▼
┌─────────────────────────────────────────────────────────────────┐
│                      DAEMON LAYER (cmd/daemon/)                 │
│                                                                 │
│  ┌─────────────────────────────────────────────────────────┐    │
│  │  SocketListener  (internal/app/rpc.go)               │    │
│  │  - Accepts RPC connections                              │    │
│  │  - Routes to Manager                                   │    │
│  └─────────────────────┬───────────────────────────────────┘    │
│                        │                                        │
│                        ▼                                        │
│  ┌─────────────────────────────────────────────────────────┐    │
│  │  Manager  (internal/app/core.go)                        │    │
│  │  - sync.Mutex protects all state                        │    │
│  │  - Holds: Config[process] → ProcessInstance             │    │
│  │  - Methods: Start/Stop/Restart/Reload/Shutdown          │    │
│  └─────────────────────┬───────────────────────────────────┘    │
│                        │                                        │
│           ┌────────────┼────────────┐                        │
│           │            │            │                         │
│           ▼            ▼            ▼                         │
│  ┌──────────────┐ ┌──────────┐ ┌──────────────┐               │
│  │   Watchdog   │ │  Reload  │ │   Status     │               │
│  │   (per proc) │ │  (SIGHUP)│ │   Channel    │               │
│  └──────────────┘ └──────────┘ └──────────────┘               │
│           │                                        │            │
│           ▼                                        ▼            │
│  ┌─────────────────────────────────────────────────────────┐    │
│  │           ENGINE LAYER (internal/engine/)               │    │
│  │  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐    │    │
│  │  │ process.go   │  │  signal.go   │  │ lifecycle.go │    │    │
│  │  │ - Executor   │  │ - Signal     │  │ - Watcher    │    │    │
│  │  │ - Command    │  │ - Stopper    │  │ - ShouldRestart│   │    │
│  │  └──────────────┘  └──────────────┘  └──────────────┘    │    │
│  └─────────────────────────────────────────────────────────┘    │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘

DATA FLOW
=========
1. User runs "taskmasterctl start nginx"
   → RPC Request → SocketListener → Manager.Start("nginx")

2. Manager spawns Watchdog goroutine
   → Watchdog creates process via engine/process.go
   → Sends status updates via Status channel

3. Logger (to be implement) reads Status channel
   → Writes to /var/log/taskmaster/taskmaster.log

DIRECTORY GUIDE
===============
cmd/daemon/         → Daemon entry point (main loop, SIGHUP handler)
cmd/ctl/            → Control program (your readline shell goes here)
internal/app/       → Core orchestration (Manager + RPC handlers)
internal/engine/    → Process execution primitives
internal/bus/       → Event types (ProcessUpdate, Status enum)
internal/config/    → YAML loading + config spec
e2e/                → End-to-end tests (run these!)
config.yml          → Example configuration
```