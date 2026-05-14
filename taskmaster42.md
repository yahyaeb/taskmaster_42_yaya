# `taskmaster` Program Logic: A New Developer's Guide

This document provides a high-level overview of the `taskmaster` application's core logic, focusing on how configurations are managed, processes are orchestrated, and concurrency is handled using Go's features.

## 1. Introduction to `taskmaster`

`taskmaster` is an application designed to manage the lifecycle of other programs (child processes) based on a YAML configuration file (`config.yml`). It handles starting, stopping, monitoring, and restarting these programs dynamically.

## 2. Configuration Management

The foundation of `taskmaster`'s operation lies in its configuration.

*   **`config.ConfigSpec` (`internal/config/spec.go`)**
    *   Role: This struct defines the precise schema for a single program's configuration as read from `config.yml`. Each field corresponds to a YAML key, making it the blueprint for how `taskmaster` understands a program's requirements.
    *   Key fields: `Program`, `Cmd`, `Numprocs`, `Workingdir`, `Autostart`, `Stopsignal`, `Exitcodes`, `Env`.

*   **`config.YAMLLoader` (`internal/config/yaml_loader.go`)**
    *   Role: This component is responsible for reading the `config.yml` file and parsing its content into one or more `ConfigSpec` structs. It also handles the expansion of the `Numprocs` field, creating multiple `ConfigSpec` instances (e.g., `program:01`, `program:02`) if specified.
    *   `Load` method

## 3. The Central Orchestrator: `app.Manager`

The `app.Manager` struct is the heart of the `taskmaster` application, acting as the central controller and single source of truth for all managed processes.

*   **`app.Manager` Struct (`internal/app/core.go`)**
    *   Role: This struct holds the entire state of the program manager, including all parsed configurations and the runtime status of each process. It is a single, authoritative instance that all management operations refer to.
    *   Key fields:
        *   `Config map[string]*config.ConfigSpec`: Stores the parsed configuration specifications for each program instance, indexed by its unique name (e.g., `nginx:01`).
        *   `Process map[string]*ProcessInstance`: Stores the dynamic runtime state for each running program instance.
        *   `mu sync.Mutex`: A mutex used to protect the shared `Config` and `Process` maps from concurrent access, preventing data races.

*   **`app.ProcessInstance` Struct (`internal/app/core.go`)**
    *   Role: This struct stores the dynamic, runtime-specific information about a managed process. It's distinct from `ConfigSpec` as it contains live operational data rather than static configuration.
    *   Key fields: `Pid`, `Status`, `RetryCount`, `LastStart`, `Intended`.

*   **`engine.Process` Struct (`internal/engine/process.go`)**
    *   Role: This struct represents an actual *running operating system child process*. It holds OS-level identifiers and I/O handlers.
    *   Distinction: It does *not* directly store the configuration data from `config.yml`. Instead, the `ConfigSpec` is passed to the `engine.ProcessExecutor` to configure and launch this OS process.
    *   Key fields: `PID`, `Stdin`, `Stdout`, `Stderr`.

## 4. Concurrency Safety: Mutex Logic

Given that `app.Manager` is a single source of truth and multiple parts of the program operate concurrently, a mechanism for safe access to shared data is essential.

*   **Why `sync.Mutex` is needed**:
    *   **Concurrent Access**: The `Manager`'s `Config` and `Process` maps are shared resources. Various goroutines (e.g., individual `Watchdog` goroutines monitoring processes, RPC handlers responding to client requests, or the `Reload` function) will attempt to read from and write to these maps.
    *   **Data Races**: Without a mutex, these concurrent operations could lead to "data races." A data race occurs when multiple goroutines access the same memory location concurrently, and at least one of the accesses is a write. This can result in unpredictable behavior, corrupted data, or program crashes.
*   **Where it's used (`m.mu.Lock()` and `m.mu.Unlock()`)**:
    *   The `m.mu` mutex (a field within the `Manager` struct) is used to protect any critical section of code that modifies or reads the `Manager`'s shared state (`Config`, `Process`).
    *   Common usage pattern involves `m.mu.Lock()` at the beginning of a critical section and `defer m.mu.Unlock()` to ensure the lock is released when the function exits, even if an error occurs.
    *   Examples of methods employing mutexes: `Start`, `Stop`, `Restart`, `Reload`, `Shutdown`, `GetProcessInfo`, `GetAllProcessInfo`, `applyConfigDiff`, and `handleStatusUpdate`.

## 5. Channels for Communication

Channels are a fundamental Go primitive for communication between concurrently executing goroutines. They allow safe and synchronized data exchange, playing a crucial role in `taskmaster`'s architecture.

*   **`app.ProcessChannels` Struct (`internal/app/core.go`)**
    *   Role: This struct bundles together the channels used for inter-goroutine communication within the `Manager`.
    *   Key fields:
        *   `Status chan bus.ProcessUpdate`: A buffered channel used by `Watchdog` goroutines to send status updates (e.g., `STARTING`, `RUNNING`, `STOPPED`, `FATAL`) for their managed processes back to the `Manager`'s main logic.
        *   `Stop map[string]chan struct{}`: A map where each entry is a channel used to signal a specific `Watchdog` goroutine to stop its associated process. Closing this channel acts as a broadcast signal to that `Watchdog`.

*   **How it works & Where it's used**:
    *   **`Watchdog` -> `Manager` Status Updates**: Each `Watchdog` goroutine sends `bus.ProcessUpdate` messages to the `m.ch.Status` channel to report changes in its child process's state. The `Manager`'s `drainChStatus()` and `handleStatusUpdate()` methods (which are protected by `m.mu`) read from this channel to update the `Manager.Process` map.
    *   **`Manager` -> `Watchdog` Stop Signals**: When the `Manager` needs to stop a process (e.g., via `Stop()` or `Shutdown()`), it closes the corresponding channel in the `m.ch.Stop` map. The `Watchdog` goroutine, which is `select`ing on this channel, will detect the close event and initiate the process termination sequence.
    *   **Synchronization**: Channels provide a safe way to synchronize actions between goroutines. For example, the `Manager` can wait for status updates from `Watchdog`s, and `Watchdog`s can wait for stop signals from the `Manager`, all without explicit locks or shared memory, adhering to Go's "Don't communicate by sharing memory; share memory by communicating" principle.

## 6. Manager's Key Methods

The `app.Manager` struct provides a set of methods for interacting with and controlling the managed processes.

*   **`NewManager()` (`internal/app/core.go`)**
    *   Role: The constructor for creating a new, empty `Manager` instance. It initializes the internal maps (`Config`, `Process`) and sets up default executors and handlers.

*   **`SetShutdownFunc(fn func())` (`internal/app/core.go`)**
    *   Role: Allows an external shutdown function to be registered, which will be called when the `Manager` itself is shut down.

*   **`SetChannels(ch *ProcessChannels)` (`internal/app/core.go`)**
    *   Role: Injects the communication channels (`ProcessChannels`) into the `Manager`, which are essential for `Watchdog` goroutines to report status updates and receive stop signals.

*   **`Start(name string) error` (`internal/app/core.go`)**
    *   Role: Manually starts a specific program instance by its `name`. It retrieves the `ConfigSpec` and launches a `Watchdog` goroutine for it.

*   **`Stop(name string) error` (`internal/app/core.go`)**
    *   Role: Manually stops a running program instance by its `name`. It signals the corresponding `Watchdog` goroutine to terminate the child process.

*   **`Restart(name string) error` (`internal/app/core.go`)**
    *   Role: Restarts a specific program instance. This typically involves stopping it first, waiting for it to fully terminate, and then starting it again.

*   **`Reload() (*ReloadResult, error)` (`internal/app/core.go`)**
    *   Role: Re-reads the `config.yml` file (from the `configPath` stored during initialization) and applies any configuration changes to the running `Manager`. This allows for hot-reloading of configurations.

*   **`ReloadFromConfig(newConfig map[string]*config.ConfigSpec) (*ReloadResult, error)` (`internal/app/core.go`)**
    *   Role: Similar to `Reload()`, but takes a `newConfig` map directly, bypassing the file read. Primarily used internally by `Reload()`.

*   **`Shutdown() error` (`internal/app/core.go`)**
    *   Role: Initiates a graceful shutdown of all managed processes and the `Manager` itself. It sends termination signals to all child processes and cleans up resources.

*   **`GetProcessInfo(name string) (protocol.ProcessInfo, error)` (`internal/app/core.go`)**
    *   Role: Retrieves detailed runtime information (PID, status, uptime, retries) for a specific program instance.

*   **`GetAllProcessInfo() ([]protocol.ProcessInfo, error)` (`internal/app/core.go`)**
    *   Role: Returns a list containing runtime information for all currently managed program instances.

*   **`Watchdog(setting *config.ConfigSpec, proc *ProcessInstance)` (`internal/app/core.go`)**
    *   Role: This is the core method run by a dedicated goroutine for each program instance. It continuously monitors the associated child process, handles restarts, retries, and communicates status updates.

*   **`Spawn(prev *Manager)` (`internal/app/core.go`)**
    *   Role: Used during initial setup or configuration reloads to compare the current desired state (`curr`) with a previous state (`prev`). It launches new `Watchdog` goroutines for newly added programs and stops `Watchdog` goroutines for removed programs.

*   **`applyConfigDiff(newConfig map[string]*config.ConfigSpec) (*ReloadResult, error)` (`internal/app/core.go`)**
    *   Role: An internal helper method called by `Reload` and `ReloadFromConfig`. It performs the actual comparison between the old and new configurations and applies the necessary changes (add, remove, restart processes).

## 7. Program Lifecycle and Concurrency Model

`taskmaster` leverages Go's goroutines to efficiently manage multiple programs concurrently.

*   **Initialization: `NewManagerFromConfig` (`internal/app/core.go`)**
    *   Role: This function is called once at the application's startup (`cmd/daemon/main.go`) to create and initialize a new `Manager` instance from the specified `config.yml` file. It sets up the initial `Config` and `Process` states.

*   **Dynamic Updates: `Reload` (`internal/app/core.go`)**
    *   Role: This method allows `taskmaster` to re-read its configuration file from disk and apply any changes (add new programs, remove old ones, restart modified ones) to the *existing* `Manager` instance without requiring a full application restart.

*   **Process Spawning: `Spawn` (`internal/app/core.go`)**
    *   Role: This method is responsible for initiating the `Watchdog` goroutines based on the current configuration. It's called after initial configuration loading or during a `Reload` operation to reconcile the set of running processes with the desired configuration.

*   **`Watchdog` Goroutines (`internal/app/core.go`)**
    *   Role: For *each configured program instance* (e.g., `program:01`, `program:02`), a **dedicated goroutine** is launched to execute the `m.Watchdog` method. This goroutine is the primary mechanism for managing an individual program's lifecycle.
    *   Responsibilities: Starting the child process, continuously monitoring its status, applying `autorestart` logic, managing `startretries`, handling `stopsignal` and `stoptime`, and reporting status updates back to the `Manager` via channels.

*   **Go Goroutines vs. OS Child Processes**:
    *   A **Go goroutine** is a lightweight, concurrent execution unit *within the Go runtime*. It's managed by Go's scheduler.
    *   An **OS child process** is a separate, heavier-weight execution unit managed by the *operating system*. It has its own PID, memory space, and resources.
    *   Relationship: Each `Watchdog` **goroutine** is responsible for managing the lifecycle of one specific **OS child process**.

## 8. Architectural Rationale

The design choice of launching a dedicated `Watchdog` goroutine for each program instance, rather than a single method managing all processes, offers several significant advantages:

*   **Concurrency and Responsiveness**: Each process can be monitored and managed independently, ensuring that the system remains responsive even if one program misbehaves or takes a long time to respond.
*   **Isolation and Fault Tolerance**: Problems or panics within one `Watchdog` goroutine are isolated, preventing them from affecting the management of other programs or the `Manager` itself. This improves the overall robustness of `taskmaster`.
*   **Simplicity of Logic per Unit**: The code within the `Watchdog` function is simpler as it only needs to consider the state and logic for a single program instance, rather than juggling multiple programs' states in a complex, monolithic loop.
*   **Scalability**: The design naturally scales with the number of managed programs. Adding more entries to `config.yml` simply means more `Watchdog` goroutines are launched, leveraging Go's efficient concurrency model.
*   **Idiomatic Go Concurrency**: This pattern aligns with Go's philosophy of Communicating Sequential Processes (CSP), where independent goroutines communicate via channels, making the system easier to understand, reason about, and maintain.