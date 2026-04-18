# Taskmaster — Incremental Build Guide

> A step-by-step construction plan that layers features progressively,
> minimizing the need to refactor earlier code as you add new capabilities.

---

## Philosophy

Build a **thin vertical slice** first: one supervised process, one config key, one
shell command. Each phase extends what already exists rather than replacing it.
Interfaces (process abstraction, config schema, logger API) are defined once and
only grow.

---

## Phase 1 — Project Skeleton & Process Abstraction

**Goal:** Launch a single hard-coded command as a child process and keep it alive.
No config file, no shell yet.

### Steps

1. **Choose your language and set up the project structure.**
   Decide on a single-file start vs. package layout. Even if you start with one
   file, name it intentionally (`taskmaster/core.py`, `src/main.rs`, etc.) so
   the structure is already meaningful.

2. **Define a `Process` data structure (class/struct).**
   Fields for now: `name`, `cmd`, `pid`, `state` (an enum: STOPPED, STARTING,
   RUNNING, FATAL). This struct will never be thrown away — only extended.

3. **Implement `spawn(process)`.**
   Fork/exec the command. Store the child PID. Handle the case where the
   executable does not exist (transition to FATAL immediately).

4. **Implement a monitoring loop.**
   A tight loop (or use `waitpid` with WNOHANG) that checks whether each child
   is still alive. If it has exited, transition its state to STOPPED.

5. **Hard-code one process and verify the loop works.**
   Run something like `/bin/sleep 5`, watch it die, confirm your loop detects it.

**Exit criterion:** You can launch a process and your program correctly detects
when it terminates.

---

## Phase 2 — Configuration File Loading

**Goal:** Replace hard-coded values with a YAML (or equivalent) config file,
implementing the full config schema up-front so you never need to revisit it.

### Steps

1. **Define the complete config schema on paper first.**
   All keys from the spec: `cmd`, `numprocs`, `umask`, `workingdir`,
   `autostart`, `autorestart` (always/never/unexpected), `exitcodes`,
   `startretries`, `starttime`, `stopsignal`, `stoptime`, `stdout`, `stderr`,
   `env`. Define sensible defaults for each.

2. **Write a `Config` data structure that mirrors the schema exactly.**
   Even if you only use `cmd` and `numprocs` right now, parsing everything into
   a typed structure means you will not touch this layer again.

3. **Implement `load_config(path)`.**
   Parse the file, validate required fields, apply defaults, return a list of
   program configs. Fail loudly on malformed input.

4. **Replace your hard-coded process with config-driven instantiation.**
   For each program in the config with `autostart: true`, create the
   corresponding `Process` objects (one per `numprocs`) and spawn them.

5. **Verify** with the example config from the appendix (using real programs
   like `sleep`, `cat`, `yes` as stand-ins for nginx/vogsphere).

**Exit criterion:** The program reads a config and manages the correct number
of processes for each program entry.

---

## Phase 3 — Logging System

**Goal:** Add a file-based logger before adding more features, so every
subsequent phase can log from day one.

### Steps

1. **Define log levels:** DEBUG, INFO, WARNING, ERROR.

2. **Implement `log(level, message)`.**
   Write timestamped lines to a log file whose path comes from config or a
   CLI argument. Also print INFO+ to stdout during development.

3. **Add log calls to all existing code paths:**
   process spawned, process died, spawn failed, config loaded.

4. **Ensure the logger is a singleton / globally accessible module** so future
   phases can call it without passing it around.

**Exit criterion:** A log file is written and contains correct entries for
every process lifecycle event you have implemented so far.

---

## Phase 4 — Full Process Lifecycle Management

**Goal:** Implement the complete restart logic and all spawn-time settings.

### Steps

1. **Apply spawn-time settings in `spawn()`:**
   Set `umask`, `chdir` to `workingdir`, inject `env` variables, redirect
   stdout/stderr to files (or `/dev/null`), all before `exec`.

2. **Implement `starttime` / "successfully started" logic.**
   After spawning, wait `starttime` seconds. If the process is still alive at
   that point, transition to RUNNING. If it died before, it counts as a failed
   start.

3. **Implement `autorestart` logic in your monitoring loop:**
   - `never` → do nothing on exit.
   - `always` → restart unconditionally.
   - `unexpected` → restart only if exit code is not in `exitcodes`.

4. **Implement `startretries`.**
   Track a retry counter per process instance. After exceeding the limit,
   transition to FATAL and stop retrying. Log the event.

5. **Implement graceful stop:**
   Send `stopsignal` to the process. Start a `stoptime` countdown. If the
   process has not exited after `stoptime` seconds, send SIGKILL.

6. **Handle `numprocs > 1`.**
   Name instances `programname:00`, `programname:01`, etc. Each instance is an
   independent `Process` object with its own state and retry counter.

**Exit criterion:** A process that keeps crashing eventually reaches FATAL. A
process configured with `autorestart: always` is reliably kept alive.

---

## Phase 5 — Control Shell

**Goal:** Give the user an interactive shell to inspect and control processes.

### Steps

1. **Set up readline (or equivalent) for line editing and history.**
   This is the one place external libraries are explicitly allowed.

2. **Implement the `status` command.**
   Print a table: name, state, PID, uptime for each managed process instance.

3. **Implement `start <name>` and `stop <name>`.**
   `start` spawns process instances that are currently STOPPED/FATAL (resetting
   the retry counter). `stop` performs the graceful stop sequence.

4. **Implement `restart <name>`.**
   Stop then start. Re-use your existing stop and start logic exactly.

5. **Implement `quit` / `exit`.**
   Stop all running processes gracefully, then exit the main program.

6. **Run the shell loop in the main thread** while the monitoring loop runs in
   a background thread (or via non-blocking `waitpid` integrated into the
   readline loop using a timer/signal). Define this threading boundary clearly
   and protect shared state with a lock from this point forward.

**Exit criterion:** You can type `status`, `stop nginx:00`, `start nginx:00`,
`restart vogsphere`, and `quit` and each does the right thing.

---

## Phase 6 — Configuration Reload (SIGHUP)

**Goal:** Allow live reconfiguration without restarting unaffected processes.

### Steps

1. **Install a SIGHUP handler** that sets a flag (never do complex work inside
   a signal handler itself).

2. **In your main loop, check the flag** and call `reload_config()` when set.

3. **Implement `reload_config()`:**
   - Parse the new config file.
   - For programs that are new: create and spawn them.
   - For programs that were removed: stop them gracefully.
   - For programs whose config has changed: stop old instances, spawn new ones.
   - For programs whose config is identical: **do nothing** (do not kill them).

4. **Implement `reload` as a shell command** that triggers the same logic
   (useful for testing without needing to send a signal).

5. **Log all reload activity.**

**Exit criterion:** Editing the config file, sending SIGHUP, and running
`status` shows the updated program set without having restarted unchanged
processes.

---

## Phase 7 — Edge Cases & Hardening

**Goal:** Make the program reliable under adversarial conditions (this is what
the grader will test).

### Steps

1. **Process that never starts successfully:**
   Verify FATAL state is reached after `startretries` and no further attempts
   are made.

2. **Process killed externally:**
   Kill a child with `kill -9` from another terminal. Confirm your monitor
   detects it and applies `autorestart` logic correctly.

3. **Process generating massive output:**
   If stdout/stderr are redirected to files, confirm there is no buffering
   issue or deadlock. Test with `yes` or a tight `while true; do echo x; done`.

4. **Rapid restart loops:**
   A process that exits immediately (exit code 1, `autorestart: always`).
   Add a small back-off between retries to avoid CPU spinning.

5. **Config file with errors:**
   Missing required fields, unknown keys, wrong types. Your loader should
   report the error and refuse to apply the bad config (keep the old state).

6. **Signal handling robustness:**
   Ensure SIGINT / SIGTERM on taskmaster itself triggers a clean shutdown
   (same as `quit` command).

7. **PID reuse safety:**
   Never signal a PID without first confirming the process is still one of
   yours (the child may have exited and its PID been reused by the OS).

---

## Phase 8 — Bonus Features (Optional, Additive)

Each bonus is self-contained and builds on the stable core from Phase 7.
Tackle them in any order.

### B1 — Tab Completion in the Shell
Extend readline completion to suggest command names and process names.
No refactoring needed — just add a completion callback.

### B2 — Client / Server Architecture
Split into `taskmaserd` (daemon, no shell) and `taskmasterctl` (shell only,
communicates over a UNIX or TCP socket). The daemon's internal API is already
well-defined by your shell commands — just expose it over a socket.

### B3 — Advanced Logging
Add log rotation (by size or date), syslog output, or alert delivery (HTTP
webhook, email). Plug into the existing `log()` function.

### B4 — Process Attach / Detach
Allow the user to "attach" to a supervised process (connect its stdio to the
terminal) and later "detach" (put it back in the background), similar to
`tmux`. Requires PTY handling.

### B5 — Privilege De-escalation
If started as root, drop to a configured user/group after setup. Add a `user`
key to the config schema (already reserved — just never parsed until now).

---

## Summary Table

| Phase | Feature                        | New files / modules added                   |
|-------|--------------------------------|---------------------------------------------|
| 1     | Process abstraction + spawn    | `process.{ext}`, `main.{ext}`               |
| 2     | Config loading                 | `config.{ext}`                              |
| 3     | Logging                        | `logger.{ext}`                              |
| 4     | Full lifecycle (restart, stop) | extends `process.{ext}`                     |
| 5     | Control shell                  | `shell.{ext}`                               |
| 6     | SIGHUP reload                  | extends `config.{ext}` + signal handler     |
| 7     | Hardening                      | no new modules — fixes & guards throughout  |
| 8     | Bonuses                        | `server.{ext}`, `client.{ext}`, etc.        |

---

## Key Design Decisions to Make Early (Before Phase 1)

- **Threading model:** single-threaded event loop vs. monitor thread + main
  thread. Decide before Phase 5 — changing it later is the biggest refactor risk.
- **Config format:** YAML is the recommended choice. Pick it and stick with it.
- **Process naming convention:** `name:index` (e.g. `nginx:00`). Agree on this
  before Phase 2 so logging and shell commands are consistent throughout.
- **Shared state protection:** if using threads, wrap the process list in a
  mutex from Phase 5 onward — do not retrofit this later.
