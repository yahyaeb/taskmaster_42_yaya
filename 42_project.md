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

Shared contracts:

Socket: /tmp/taskmaster.sock
Instance names: nginx:01, nginx:02 (always use config.FormatInstanceName)
Statuses: "starting" "running" "backoff" "stopped" "fatal"
RPC method format: Taskmaster.GetStatus, Taskmaster.Start, etc.
Params: {"name": "nginx:01"} — use "all" to target everything


### bde-albu
Shared contracts:

Socket: /tmp/taskmaster.sock
Instance names: nginx:01, nginx:02 (always config.FormatInstanceName)
Statuses: "starting" "running" "backoff" "stopped" "fatal"
RPC format: RPCRequest / RPCResponse from internal/protocol/jsonrpc.go
Event bus: chan bus.ProcessUpdate — write to it on every state change