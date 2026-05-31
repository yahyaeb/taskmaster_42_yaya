#!/usr/bin/env bash
# =============================================================================
# Taskmaster Evaluation — Automated Tests (Points 0, 1, 2)
# =============================================================================
# 0. Control shell   — start, stop, restart, status commands
# 1. Config file     — programs loaded from config, status consultable
# 2. Logging         — events logged: start, stop, restart, unexpected exit
# =============================================================================
set -euo pipefail

# --- Colors ---
RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'; BOLD='\033[1m'; NC='\033[0m'
PASS=0; FAIL=0; TOTAL=0

say()        { echo -e "$@"; }
header()     { say "\n${BOLD}━━━ $1 ━━━${NC}"; }
pass()       { PASS=$((PASS + 1)); TOTAL=$((TOTAL + 1)); say "  ${GREEN}✓${NC} $1"; }
fail()       { FAIL=$((FAIL + 1)); TOTAL=$((TOTAL + 1)); say "  ${RED}✗${NC} $1"; }

# --- Paths ---
ROOT="$(cd "$(dirname "$0")" && pwd)"
DAEMON="$ROOT/taskmasterd"
CTL="$ROOT/taskmasterctl"
SOCKET="/tmp/eval_taskmaster.sock"
LOGFILE="/tmp/eval_taskmaster.log"
CONFIG="/tmp/eval_config.yml"
CRASHER="$ROOT/testprograms/crasher/crasher"
DAEMON_PID=""

# Instance names (ExpandPrograms adds :00 suffix for numprocs=1)
SLEEPER="test_sleeper:00"
LOOPER="test_looper:00"
CRASHER_NAME="test_crasher:00"

# =============================================================================
# JSON-RPC helper (Python over Unix socket)
# =============================================================================
rpc_call() {
    local method="$1"
    local name="${2:-}"
    local json
    if [[ -n "$name" ]]; then
        json="{\"id\":1,\"method\":\"$method\",\"params\":{\"name\":\"$name\"}}"
    else
        json="{\"id\":1,\"method\":\"$method\"}"
    fi
    python3 -c "
import socket, json, sys
try:
    sock = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
    sock.settimeout(5)
    sock.connect('$SOCKET')
    sock.sendall(json.dumps($json).encode())
    sock.shutdown(socket.SHUT_WR)
    data = b''
    while True:
        chunk = sock.recv(4096)
        if not chunk: break
        data += chunk
    sock.close()
    result = json.loads(data)
    if result.get('error'):
        print('ERROR:' + result['error'])
        sys.exit(1)
    print(json.dumps(result.get('result', '')))
except Exception as e:
    print('EXCEPTION:' + str(e))
    sys.exit(2)
"
}

rpc_status()  { rpc_call "status"; }
rpc_start()   { rpc_call "start" "$1"; }
rpc_stop()    { rpc_call "stop" "$1"; }
rpc_restart() { rpc_call "restart" "$1"; }
rpc_shutdown(){ rpc_call "shutdown"; }

# =============================================================================
# Helpers
# =============================================================================
wait_for_daemon() {
    local max=30 i=0
    while [[ $i -lt $max ]]; do
        if [[ -S "$SOCKET" ]]; then
            local out
            out=$(rpc_status 2>/dev/null || true)
            if [[ -n "$out" ]] && ! echo "$out" | grep -q "EXCEPTION"; then
                return 0
            fi
        fi
        sleep 0.2
        ((i++)) || true
    done
    return 1
}

log_contains() {
    local pattern="$1"
    [[ -f "$LOGFILE" ]] && grep -q "$pattern" "$LOGFILE"
}

# status_has: checks if program <name> exists in status, optionally with <state>
# StatusReport fields are uppercase: Name, Status, Pid, ExitCode, Uptime
status_has() {
    local name="$1" state="${2:-}"
    local out
    out=$(rpc_status 2>/dev/null) || return 1
    if [[ -n "$state" ]]; then
        echo "$out" | python3 -c "
import json, sys
try:
    reports = json.load(sys.stdin)
    for r in reports:
        if r['Name'] == '$name' and r['Status'] == '$state':
            sys.exit(0)
    sys.exit(1)
except: sys.exit(1)
" 2>/dev/null
    else
        echo "$out" | python3 -c "
import json, sys
try:
    reports = json.load(sys.stdin)
    for r in reports:
        if r['Name'] == '$name':
            sys.exit(0)
    sys.exit(1)
except: sys.exit(1)
" 2>/dev/null
    fi
}

# get_field: extract a field from status for a given program
get_field() {
    local name="$1" field="$2"
    rpc_status 2>/dev/null | python3 -c "
import json, sys
for r in json.load(sys.stdin):
    if r['Name'] == '$name':
        print(r.get('$field', ''))
        break
" 2>/dev/null
}

# =============================================================================
# Cleanup
# =============================================================================
cleanup() {
    say ""
    if [[ -n "$DAEMON_PID" ]] && kill -0 "$DAEMON_PID" 2>/dev/null; then
        rpc_shutdown 2>/dev/null || true
        sleep 0.5
        kill "$DAEMON_PID" 2>/dev/null || true
        wait "$DAEMON_PID" 2>/dev/null || true
    fi
    rm -f "$SOCKET" "$CONFIG" "$LOGFILE"
    rm -f /tmp/eval_*.stdout /tmp/eval_*.stderr
}
trap cleanup EXIT

# =============================================================================
# MAIN
# =============================================================================
say "${BOLD}══════════════════════════════════════════════${NC}"
say "${BOLD}  Taskmaster Evaluation — Points 0, 1, 2${NC}"
say "${BOLD}══════════════════════════════════════════════${NC}"

# ---- Build ----
header "Building binaries"
make build -C "$ROOT" > /dev/null 2>&1
if [[ ! -x "$DAEMON" ]] || [[ ! -x "$CTL" ]]; then
    fail "Build failed — missing taskmasterd or taskmasterctl"
    exit 1
fi
pass "taskmasterd and taskmasterctl built"

# Build crasher
mkdir -p "$(dirname "$CRASHER")"
go build -o "$CRASHER" "$ROOT/testprograms/crasher/main.go" 2>/dev/null && \
    pass "crasher binary built" || \
    fail "crasher build failed"

# ---- Test config ----
header "Creating test configuration"
cat > "$CONFIG" << 'YAML'
programs:
  test_sleeper:
    cmd: ["sleep", "999"]
    numprocs: 1
    autostart: false
    autorestart: unexpected
    exitcodes: [0]
    startretries: 0
    starttime: 1
    stopsignal: TERM
    stoptime: 2
    stdout: /tmp/eval_sleeper.stdout
    stderr: /tmp/eval_sleeper.stderr
    env: {}
    workingdir: /tmp
  test_looper:
    cmd: ["sleep", "999"]
    numprocs: 1
    autostart: true
    autorestart: unexpected
    exitcodes: [0]
    startretries: 0
    starttime: 1
    stopsignal: TERM
    stoptime: 2
    stdout: /tmp/eval_looper.stdout
    stderr: /tmp/eval_looper.stderr
    env: {}
    workingdir: /tmp
  test_crasher:
    cmd: ["REPLACE_CRASHER", "1"]
    numprocs: 1
    autostart: false
    autorestart: always
    exitcodes: [0]
    startretries: 2
    starttime: 1
    stopsignal: TERM
    stoptime: 2
    stdout: /tmp/eval_crasher.stdout
    stderr: /tmp/eval_crasher.stderr
    env: {}
    workingdir: /tmp
YAML
sed -i "s|REPLACE_CRASHER|$CRASHER|" "$CONFIG"
pass "Test config written to $CONFIG"

# ---- Start daemon ----
header "Starting taskmaster daemon"
rm -f "$SOCKET" "$LOGFILE"
"$DAEMON" -config "$CONFIG" -socket "$SOCKET" -log "$LOGFILE" &
DAEMON_PID=$!

if ! wait_for_daemon; then
    fail "Daemon failed to start within timeout"
    exit 1
fi
pass "Daemon started (pid $DAEMON_PID)"

sleep 0.5

# =============================================================================
# POINT 0 — Control Shell
# =============================================================================
header "POINT 0 — Control Shell"

# 0.1: Start a program via RPC (the control shell)
if rpc_start "$SLEEPER" > /dev/null 2>&1; then
    pass "0.1 start $SLEEPER → success"
else
    fail "0.1 start $SLEEPER → failed"
fi
sleep 0.3

# 0.2: Status shows program as RUNNING after start
if status_has "$SLEEPER" "running"; then
    pass "0.2 status shows $SLEEPER as RUNNING"
else
    fail "0.2 status does NOT show $SLEEPER as RUNNING"
fi

# 0.3: Stop a running program
if rpc_stop "$SLEEPER" > /dev/null 2>&1; then
    pass "0.3 stop $SLEEPER → success"
else
    fail "0.3 stop $SLEEPER → failed"
fi
sleep 0.5

# 0.4: Status shows program as STOPPED after stop
if status_has "$SLEEPER" "stopped"; then
    pass "0.4 status shows $SLEEPER as STOPPED"
else
    fail "0.4 status does NOT show $SLEEPER as STOPPED"
fi

# 0.5: Restart a stopped program
if rpc_restart "$SLEEPER" > /dev/null 2>&1; then
    pass "0.5 restart $SLEEPER → success"
else
    fail "0.5 restart $SLEEPER → failed"
fi
sleep 0.3

# 0.6: Status shows program as RUNNING after restart
if status_has "$SLEEPER" "running"; then
    pass "0.6 status shows $SLEEPER as RUNNING after restart"
else
    fail "0.6 status does NOT show $SLEEPER as RUNNING after restart"
fi

# 0.7: Unknown process returns error
if ! rpc_start "no_such_process" > /dev/null 2>&1; then
    pass "0.7 unknown process → error (expected)"
else
    fail "0.7 unknown process should return error"
fi

# 0.8: Status returns full process list (3 programs × numprocs=1 = 3 instances)
ALL_STATUS=$(rpc_status 2>/dev/null)
PROC_COUNT=$(echo "$ALL_STATUS" | python3 -c "import json,sys; print(len(json.load(sys.stdin)))" 2>/dev/null || echo 0)
if [[ "$PROC_COUNT" -ge 3 ]]; then
    pass "0.8 status returns all 3 configured instances"
else
    fail "0.8 status returned $PROC_COUNT instances (expected ≥3)"
fi

# =============================================================================
# POINT 1 — Configuration File
# =============================================================================
header "POINT 1 — Configuration File"

# 1.1: Autostart program is running
if status_has "$LOOPER" "running"; then
    pass "1.1 autostart program ($LOOPER) is RUNNING"
else
    fail "1.1 autostart program ($LOOPER) is NOT running"
fi

# 1.2: Non-autostart program exists in status
if status_has "$CRASHER_NAME"; then
    pass "1.2 non-autostart program ($CRASHER_NAME) present in status"
else
    fail "1.2 $CRASHER_NAME missing from status"
fi

# 1.3: Running program shows valid PID
RUNNING_PID=$(get_field "$LOOPER" "Pid")
if [[ "${RUNNING_PID:-0}" -gt 0 ]]; then
    pass "1.3 $LOOPER has valid PID ($RUNNING_PID)"
else
    fail "1.3 $LOOPER PID is 0 or missing"
fi

# 1.4: Status shows uptime for running program
UPTIME=$(get_field "$LOOPER" "Uptime")
if [[ -n "$UPTIME" ]]; then
    pass "1.4 $LOOPER shows uptime ($UPTIME)"
else
    fail "1.4 $LOOPER uptime missing"
fi

# =============================================================================
# POINT 2 — Logging
# =============================================================================
header "POINT 2 — Logging"

# 2.1: Log file exists and is non-empty
if [[ -f "$LOGFILE" ]] && [[ -s "$LOGFILE" ]]; then
    pass "2.1 log file exists and is non-empty"
else
    fail "2.1 log file missing or empty"
fi

# 2.2: Log contains STARTING for autostart program
if log_contains "$LOOPER.*STARTING"; then
    pass "2.2 log contains STARTING event for $LOOPER"
else
    fail "2.2 missing STARTING event for $LOOPER"
fi

# 2.3: Log contains RUNNING state entry
if log_contains "$LOOPER.*RUNNING"; then
    pass "2.3 log contains RUNNING event for $LOOPER"
else
    fail "2.3 missing RUNNING event for $LOOPER"
fi

# 2.4: Log contains STOPPED/exited event after manual stop
if log_contains "$SLEEPER.*exited"; then
    pass "2.4 log contains exited event for $SLEEPER"
else
    fail "2.4 missing exited event for $SLEEPER"
fi

# 2.5: Start crasher, wait for crash+retry cycle → BACKOFF/FATAL logged
rpc_start "$CRASHER_NAME" > /dev/null 2>&1
sleep 4  # crasher exits after 1s, then retries, eventually backoff/fatal

if log_contains "$CRASHER_NAME.*BACKOFF\|$CRASHER_NAME.*FATAL"; then
    pass "2.5 log contains BACKOFF/FATAL for $CRASHER_NAME"
else
    fail "2.5 missing BACKOFF/FATAL for $CRASHER_NAME"
fi

# 2.6: Log contains unexpected exit (exit status != 0, "not expected")
if log_contains "exit status.*not expected"; then
    pass "2.6 log contains 'unexpected exit' entry"
else
    fail "2.6 missing unexpected exit entry"
fi

# 2.7: Log entries include timestamps
LOG_SAMPLE=$(head -5 "$LOGFILE" 2>/dev/null || true)
if echo "$LOG_SAMPLE" | grep -qE '[0-9]{4}-[0-9]{2}-[0-9]{2}|[0-9]{2}:[0-9]{2}:[0-9]{2}'; then
    pass "2.7 log entries are timestamped"
else
    fail "2.7 log entries lack timestamps"
fi

# =============================================================================
# REPORT
# =============================================================================
header "Results"
say ""
say "  ${GREEN}Passed:${NC} $PASS"
say "  ${RED}Failed:${NC} $FAIL"
say "  Total:    $TOTAL"
say ""

if [[ $FAIL -eq 0 ]]; then
    say "  ${GREEN}${BOLD}All evaluation points (0, 1, 2) PASSED ✓${NC}"
    exit 0
else
    say "  ${RED}${BOLD}$FAIL test(s) FAILED — review output above${NC}"
    say ""
    say "  Debug: log file at $LOGFILE"
    exit 1
fi
