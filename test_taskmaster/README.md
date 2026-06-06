# Taskmaster Evaluation Suite

End-to-end test harness that maps directly to `evaluation_guide.md`. Run from the project root:

```bash
go run ./test_taskmaster
```

Each test block starts a fresh daemon with a restored `config.yml`, so blocks are independent.

## Guide mapping

| Test | Evaluation guide section | What it teaches |
|------|--------------------------|-----------------|
| **TEST 0** | Prerequisites | Harness built; suite scope overview |
| **TEST 1** | Control shell | `start` / `stop` / `restart` / `status` via ctl |
| **TEST 2** | Config file (boot) | `config.yml` loaded at startup; `numprocs` + `autostart` smoke test |
| **TEST 3** | Logging (boot) | Daemon logfile exists; stdout/stderr redirection smoke test |
| **TEST 4** | Hot-reload | `reload` command + SIGHUP; only changed programs get new PID |
| **TEST 5.1** | `cmd` | Launches the configured command |
| **TEST 5.2** | `numprocs` | Expands instances (`dummy:00`, `dummy:01`, …) |
| **TEST 5.3** | `autostart` | `false` leaves program stopped until manual start |
| **TEST 5.4** | `autorestart` + `exitcodes` | `never` / `always` / `unexpected` matrix |
| **TEST 5.5** | `starttime` | Early exit inside window = startup failure |
| **TEST 5.6** | `startretries` | Abort after N attempts, logged in daemon log |
| **TEST 5.7** | `stopsignal` | Graceful stop uses configured signal |
| **TEST 5.8** | `stoptime` | Force-kill after grace period |
| **TEST 5.9** | `stdout` / `stderr` | Redirect process output to files |
| **TEST 5.10** | `env` | Environment variables injected |
| **TEST 5.11** | `workingdir` | Process cwd set correctly |
| **TEST 5.12** | `umask` | File permissions reflect mask |
| **BONUS 1** | uid/gid de-escalation | Process runs as configured user |
| **TEST 6** | Kill scenario | External SIGKILL → autorestart with new PID |
| **TEST 7** | Logging lifecycle | Start/stop/restart events written to daemon log |
| **STABILITY** | Program stability | Daemon survives rapid ctl operations |

## Intentional overlap

Some checks appear twice on purpose:

- **TEST 2 vs 5.2/5.3** — boot smoke test vs isolated option verification
- **TEST 3 vs 5.9** — autostart redirection vs on-demand start redirection

When a boot test fails, check the matching isolated test to pinpoint the option.

## Not covered (manual evaluation)

- Author file, norminette, forbidden libraries
- Bonus: client/server ctl, advanced logging (email/syslog), attach/console
- stdout/stderr discard to `/dev/null`

## Test programs

Defined in `config.yml`, binaries under `testprograms/`:

| Program | Role |
|---------|------|
| `dummy` | Long-running `sleep` — ctl operations |
| `ticker` | Periodic output — autorestart kill test (TEST 6) |
| `lazy` | Manual-start process — config option matrix |
| `crasher` | Exits on demand — autorestart / retry tests |
| `slowstopper` | Signal-aware stop — stopsignal / stoptime |
| `envreporter` | Prints env, cwd, umask — option verification |
| `hello42` | stdout/stderr writer — redirection tests |
