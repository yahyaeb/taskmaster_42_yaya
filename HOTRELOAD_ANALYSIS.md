# How Supervisor Handles Hot-Reload: Field Parsing & Differentiated Management

## The Three-Layer Architecture

### **Layer 1: Configuration Fields (YAML → Struct)**
All fields are parsed once from `config.yml` via `yaml.Unmarshal()` into a single `Settings` struct:

```
YAML Configuration Fields:
├── Command Spec: cmd, numprocs, numprocs_start, workingdir
├── Lifecycle: autostart, autorestart, starttime, stoptime, startretries
├── Signals: stopsignal
├── Exit Handling: exitcodes
├── Limits: umask
├── Streams: stdout, stderr
└── Environment: env (map[string]string)
```

These 16 fields live in the `Settings` struct with `yaml` tags. **No field parsing is separated or grouped during load.**

---

## **Layer 2: Field Separation at Runtime**

The magic happens AFTER parsing, when fields are used by two separate goroutine systems:

### **Group A: Watchdog-Managed Fields** (run by `watchdog()` goroutine)
These fields are **read per-loop iteration**, alive only during process lifecycle:

- `Cmd` — parsed with `strings.Fields()`, executed via `exec.Command()`
- `Numprocs` — loop counter for spawning N process instances
- `Workingdir` — set as `cmd.Dir`
- `Env` — appended to `cmd.Env` as `KEY=VALUE`
- `Autostart` — gates whether the watchdog loop starts at all
- `Autorestart`, `Exitcodes`, `Startretries`, `Starttime`, `Stoptime` — govern retry/backoff logic
- `Stopsignal` — determines signal sent on stop (TERM, KILL, etc.)

**Management Style:** Watchdog reads these on-demand, applies them in real-time. No state caching—each loop iteration re-reads values.

### **Group B: Central Controller Fields** (run by `main()` select loop)
These fields are **monitored for changes** during hot-reload, primarily for discovery:

- `Program` — program name, used as map key to detect new programs
- `ProcessName` — instance identifier (e.g., "server:00", "server:01")
- `Stdout`, `Stderr` — stream redirection (currently not implemented in watchdog)

**Management Style:** On SIGHUP, the controller reloads config, scans for NEW program names, and spawns new watchdogs. **Existing watchdogs continue unchanged** (they see the old config snapshot).

### **Group C: Runtime State (Not Config)**
These live in the embedded `Process` struct—**never from YAML**:

- `Status` — current process state (STARTING, RUNNING, STOPPED, FATAL)
- `Pid` — process ID, updated via channel
- `retries` — retry counter (private to watchdog)
- `lastStart` — last start timestamp (private to watchdog)
- `intended` — desired state (declared but unused)

**Management Style:** Updated exclusively via the `updates` channel from watchdog goroutines. Central controller modifies only via `msg.Status` and `msg.Pid`.

---

## **The Hot-Reload Flow**

### **SIGHUP Reception (main loop)**
```
1. Signal arrives → ctl.sighup channel
2. configLoad() reads fresh config.yml
3. New Settings structs created from YAML
4. Diff against existing programs map:
   - If NEW program name exists:
     → Spawn new watchdog(newMgr, updates)
     → "Started new program X"
   - If EXISTING program name:
     → No action—old watchdog keeps running
     → Old watchdog continues using OLD Settings snapshot
5. programs = newPrograms (global map replaced)
```

### **Critical Insight: Watchdog Isolation**
Each watchdog goroutine captures `m *Manager` at spawn time. When hot-reload happens:
- **Old watchdogs**: Keep using their original Settings, unchanged
- **New watchdogs**: Get fresh Settings from reloaded config
- **Central state**: Only `Status` and `Pid` flow back via channel

**This means:** Changing `cmd`, `numprocs`, `autostart` in config.yml does NOT affect running watchdogs. Only NEW programs spawn with new config. **Existing processes keep running with their original config.**

---

## **Field Parsing Mechanics**

### **YAML → Struct (One Pass)**
```go
yaml.Unmarshal(data, &raw)  // All 16 fields parsed once
```

### **Struct → Instance Expansion**
```go
for name, s := range raw {
    for i := 0; i < s.Numprocs; i++ {
        ns := *s  // Shallow copy of Settings
        instName := formatProcessName(name, i)  // "server:00", "server:01"
        ns.ProcessName = instName
        process[instName] = &ns
    }
}
```

**Each process instance gets its own Settings copy.** Modifying one doesn't affect siblings.

### **Field-by-Field Usage in Watchdog**

| Field | Parse Location | Usage | Grouping |
|-------|---|---|---|
| `Cmd` | `strings.Fields(cmd)` into `[0]` and `[1:]` | `exec.Command(parts[0], parts[1:]...)` | Group A |
| `Workingdir` | Direct string | `cmd.Dir = settings.Workingdir` | Group A |
| `Env` | Iterated as map | `cmd.Env = append(cmd.Env, "K=V")` | Group A |
| `Autostart` | Direct bool | `if !settings.Autostart { return }` | Group A |
| `Numprocs` | Loop counter | `for i := 0; i < s.Numprocs; i++` | Group A |
| `Autorestart` | String enum check | Determines retry behavior | Group A |
| `Exitcodes` | List check | `if contains(exitcodes, exitCode)` | Group A |
| `Status`, `Pid` | Channel update | `updates <- Settings{Status: ..., Pid: ...}` | Group C |

---

## **Why Fields Are Grouped This Way**

**Group A (Watchdog-Managed):** These are **execution parameters**. Each must be applied at the moment of `cmd.Start()`. They're read fresh per loop to respect the running process's needs.

**Group B (Controller-Managed):** These are **topology parameters**. They determine which programs exist and which watchdogs to spawn. Changes trigger new watchdog creation, not modification of running ones.

**Group C (Runtime State):** These are **not from config**. They're ephemeral—status, PID, retries. They flow through channels to keep central state in sync.

---

## **Design Philosophy**

The supervisor implements a **process-group-per-watchdog** pattern:
- Each watchdog is a long-lived goroutine managing `Numprocs` instances
- Config changes don't interrupt running watchdogs (graceful degradation)
- New programs spawn fresh—old ones finish naturally
- State flows back centrally via channels, config stays local to goroutines

This is **not a full hot-reload** (changes don't propagate to running processes). It's a **partial reload** (new programs adopt new config; old programs complete with old config). This avoids deadlocks and race conditions at the cost of requiring a process restart to pick up config changes.

