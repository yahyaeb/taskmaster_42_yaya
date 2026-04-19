# Taskmaster - Implementation Tasks

## Core Process Management
- [x] Start jobs as child processes
- [x] Keep processes alive and restarting as needed
- [x] Track process status accurately (alive/dead)
- [x] Implement numprocs (multiple instances per program)

## Configuration Management
- [x] Load configuration file at startup (YAML format)
- [x] Reload configuration on SIGHUP signal
- [x] Add programs from configuration
- [x] Remove programs when no longer in config
- [x] Preserve unchanged processes during reload
- [x] Implement autostart flag (start at launch or not)
- [ ] Implement autorestart modes (always/never/unexpected)
- [ ] Implement starttime (grace period to consider started)
- [ ] Implement startretries (max restart attempts)
- [ ] Implement exitcodes (expected exit codes)
- [ ] Implement stopsignal (signal to gracefully stop)
- [ ] Implement stoptime (timeout before killing)
- [ ] Implement umask setting
- [ ] Implement working directory setting
- [ ] Implement environment variables
- [ ] Implement stdout redirection
- [ ] Implement stderr redirection

## Control Shell
- [x] Provide interactive foreground shell
- [x] Status command (show all programs)
- [ ] Start command (start a specific program)
- [ ] Stop command (stop a specific program)
- [ ] Restart command (restart a specific program)
- [x] Reload command (reload configuration)
- [x] Exit command (stop main program)
- [ ] Improve shell with line editing
- [ ] Implement command history
- [ ] Implement tab completion (bonus)

## Logging System
- [ ] Log program startup events
- [ ] Log program stop events
- [ ] Log program restart events
- [ ] Log unexpected crashes
- [ ] Log configuration reload events
- [ ] Implement log file output
- [ ] Implement log rotation (bonus)

## Bonus Features
- [ ] Client/server architecture (separate daemon and control program)
- [ ] Privilege de-escalation on launch (requires root)
- [ ] Advanced logging (email/HTTP/syslog alerts)
- [ ] Process attach/detach capability (like tmux/screen)
- [ ] Process resource monitoring
- [ ] Web dashboard
