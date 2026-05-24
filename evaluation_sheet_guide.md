# Taskmaster — Ordered Evaluation Walkthrough

This guide follows the evaluation sheet in the same order as the grader.
For each topic it lists:

- what to demonstrate
- what to change in the config, if needed
- what to type in `taskmasterctl`
- what to show on screen
- what the evaluator should observe

## 0. Before starting

### Show
- `author` exists and is valid
- the project builds cleanly
- no segfaults or crashes during the demo
- external library use is limited to config parsing and the client/server bonus

### Commands
```bash
cat author
make clean && make build
```

### Expected
- build succeeds
- binaries are generated
- no warnings or runtime errors appear

---

## 1. Control shell

### Goal
Prove that the control shell can start, stop, restart, and inspect programs.

### Config
Use any program with `autostart: true`, for example `longrunner:00`.

### Demo
1. Start the daemon.
2. Open the controller shell.
3. Show `status`.
4. Stop one program.
5. Show `status` again.
6. Start it again.
7. Restart it.
8. Show `status` after each command.

### Commands
**Terminal 1**
```bash
./taskmasterd
```

**Terminal 2**
```bash
./taskmasterctl
```

Inside the shell:
```text
status
stop longrunner:00:00
status
start longrunner:00:00
status
restart longrunner:00:00
status
```

### Show
- prompt is interactive
- status changes from `running` to `stopped` and back
- PID changes after restart

### Expected
- `start`, `stop`, `restart`, and `status` all work from the shell

---

## 2. Configuration file

### Goal
Prove programs are loaded from YAML and expanded by `numprocs`.

### Config
Use `dummy-program` from `config.yml`.

### Demo
- show the config file
- show that all declared programs appear in `status`
- show that `numprocs` creates multiple instances

### Commands
```bash
cat config.yml
```

In `taskmasterctl`:
```text
status
```

### Show
- `dummy-program:00` to `dummy-program:03`

### Expected
- every program in YAML appears in the shell
- instance expansion matches `numprocs`

---

## 3. Logging

### Goal
Prove the log file records meaningful lifecycle events.

### Config
Use at least one program that can be started, stopped, and restarted.

### Demo
- tail the log file
- trigger stop, start, restart from the shell
- point at new log lines

### Commands
```bash
tail -f taskmaster.log
```

In `taskmasterctl`:
```text
stop dummy-program:00
start dummy-program:00
restart dummy-program:01
```

### Show
- timestamps
- process name
- status
- PID
- exit code

### Expected
- start, stop, restart, and exit events are logged
- log updates while the shell is used

---

## 4. Hot reload

### Goal
Prove reload works from the shell and from `SIGHUP`.
Also prove unchanged programs are not restarted.

### Config plan
Use one program as the reload target, for example `server`.
Keep another program unchanged, for example `dummy-program`.

### Demo A: reload from shell
1. Capture current PIDs with `status`.
2. Edit only one program in `config.yml`.
3. Run `reload` in `taskmasterctl`.
4. Run `status` again.
5. Show the changed program got a new PID.
6. Show the unchanged program kept the same PID.

### Commands
In `taskmasterctl`:
```text
status
reload
status
```

### Show
- the modified program restarts
- the untouched program keeps the same PID

### Demo B: reload from `SIGHUP`
1. Edit the same program again.
2. Send `SIGHUP` to the daemon.
3. Show the daemon logs mention reload.
4. Show `status` again.

### Commands
```bash
kill -SIGHUP $(pgrep -f taskmasterd | head -n1)
```

### Expected
- reload works both ways
- only affected programs restart

---

## 5. Configuration options

Use one dedicated program per behavior when possible. This keeps the demo readable.

### 5.1 `cmd`

### Goal
Show the command used to launch a program.

### Show
- the `cmd` field in `config.yml`
- the program actually starts with that command

### Expected
- the configured command is executed

---

### 5.2 `numprocs`

### Goal
Show a single declaration becomes multiple supervised instances.

### Show
- `status` lists `dummy-program:00` through `dummy-program:03`

### Expected
- instance count matches `numprocs`

---

### 5.3 `autostart`

### Goal
Show both `true` and `false` behaviors.

### Demo
- set one program to `autostart: false`
- restart the daemon
- show the program stays stopped
- set it back to `true`
- restart the daemon again
- show it starts automatically

### Expected
- `autostart` controls startup behavior correctly

---

### 5.4 `autorestart` and `exitcodes`

### Goal
Show `never`, `always`, and `unexpected`.

### Recommended config
Use `crasher`.

### Demo
- `autorestart: never` → program exits and stays stopped
- `autorestart: always` → program is restarted repeatedly
- `autorestart: unexpected` with `exitcodes: [0]` → exit code `1` causes restart
- `autorestart: unexpected` with `exitcodes: [1]` → exit code `1` does not restart

### Expected
- restart policy matches the config

---

### 5.5 `starttime`

### Goal
Show the success window is enforced.

### Recommended config
Use a flaky program that crashes before startup completes.

### Demo
- set `starttime` to a value larger than the crash delay
- start the program
- show the process is treated as a startup failure

### Expected
- crashes inside the startup window count as failed starts

---

### 5.6 `startretries`

### Goal
Show the program aborts after too many failed starts.

### Recommended config
Use `crasher` or another failing helper.

### Demo
- set `startretries` to a small number
- show repeated failed attempts
- show final fatal/abort state

### Expected
- retries stop at the configured limit

---

### 5.7 `stopsignal`

### Goal
Show the configured stop signal is sent.

### Recommended config
Use `slowstopper`.

### Demo
- set `stopsignal: USR1`
- stop the process
- show it exits cleanly on that signal
- change to `stopsignal: TERM`
- show the stop behavior changes accordingly

### Expected
- the configured signal is used

---

### 5.8 `stoptime`

### Goal
Show graceful stop escalates to `SIGKILL` after timeout.

### Recommended config
Use `slowstopper`.

### Demo
- set `stopsignal: TERM`
- set a short `stoptime`
- stop the program
- show it is killed after the timeout

### Expected
- graceful stop is attempted first
- escalation happens after `stoptime`

---

### 5.9 `stdout` and `stderr`

### Goal
Show redirection works.

### Demo
- start a program that prints output
- read the configured output files

### Commands
```bash
cat /tmp/dummy.stdout
cat /tmp/dummy.stderr
```

### Expected
- output goes to the configured files

---

### 5.10 `env`

### Goal
Show environment variables are injected into the child process.

### Recommended config
Use `envreporter`.

### Demo
- set a few `TM_*` variables
- show they appear in the child output

### Commands
```bash
cat /tmp/envreporter.out
```

### Expected
- configured variables are present in the child environment

---

### 5.11 `workingdir`

### Goal
Show the child process starts in the configured directory.

### Recommended config
Use `envreporter` or another program that prints `pwd`.

### Expected
- the child current directory matches `workingdir`

---

### 5.12 `umask`

### Goal
Show the child uses the configured umask.

### Recommended config
Use `envreporter` or another helper that creates a file.

### Show
- file mode on the created file

### Expected
- permissions reflect the configured umask

---

## 6. Stability tests

### 6.1 Kill a supervised process

### Goal
Show the manager restarts a killed child.

### Demo
- find the PID of `ticker`
- kill it with `SIGKILL`
- show it restarts with a new PID

### Expected
- the process is supervised continuously

---

### 6.2 Fail forever, then abort

### Goal
Show a process that keeps failing eventually aborts.

### Demo
- use `crasher`
- show the retry loop
- show the final abort state

### Expected
- the daemon does not loop forever

---

### 6.3 Shell stress

### Goal
Show the daemon remains stable under rapid commands.

### Demo
- send several `restart`, `stop`, `start`, and `status` commands quickly
- show the daemon keeps responding

### Expected
- no hang
- no crash
- final state is coherent

---

### 6.4 Broken YAML reload

### Goal
Show bad configuration does not crash the daemon.

### Demo
- break indentation or syntax in `config.yml`
- run `reload`
- show an error is returned
- restore the file

### Expected
- reload fails gracefully
- daemon stays alive

---

### 6.5 Clean shutdown

### Goal
Show the daemon stops all children cleanly.

### Demo
- run `shutdown` from `taskmasterctl`
- show the daemon exits
- show no supervised processes remain

### Expected
- no orphaned supervised children

---

## 7. Bonuses

### 7.1 Client/server architecture

### Show
- `taskmasterd` is the daemon
- `taskmasterctl` is a separate client
- they communicate over `/tmp/taskmaster.sock`

### Expected
- the bonus is visible from the architecture itself

---

### 7.2 Privilege de-escalation

### Show
- `uid` and `gid` are present in the config
- the child process runs with the requested credentials

### Expected
- the process drops privileges correctly

---

## Suggested config workflow

Use the config in a way that keeps each program focused on one requirement:

- `dummy-program` → shell, `numprocs`, stdout/stderr
- `server` → autostart, reload, env, workingdir
- `crasher` → restart policy, `exitcodes`, retries, abort
- `slowstopper` → `stopsignal`, `stoptime`
- `envreporter` → env, `workingdir`, `umask`
- `ticker` or `longrunner` → kill and auto-restart

Add comments above each program in `config.yml` explaining what it proves.
That makes the demo much easier to follow.

---

## Recommended demo order

1. preliminaries
2. control shell
3. config loading
4. logging
5. reload from shell
6. reload from `SIGHUP`
7. configuration options
8. stability tests
9. bonuses

