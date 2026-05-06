#!/usr/bin/env bash
# tests/smoke/run-smoke.sh
#
# End-to-end smoke test for hydraidectl install/edit flow.
# Drives the binary from /tmp/hydraidectl through the 9-step verification
# checklist in plan/hydraidectl-install-simplification.md.
#
# Usage: ./run-smoke.sh                   (uses /tmp/hydraidectl, stops on first fail)
#        BIN=/path/to/hydraidectl ./run-smoke.sh
#
# Idempotent: every test creates uniquely-named instances under
# /tmp/hydraide-smoke/<instance>, and the EXIT trap tears them down.

set -u
set -o pipefail

BIN="${BIN:-/tmp/hydraidectl}"
TS="$(date +%s)"
INST1="smoke-${TS}-a"
INST2="smoke-${TS}-b"
BASE_ROOT="/tmp/hydraide-smoke"
BASE1="${BASE_ROOT}/${INST1}"
BASE2="${BASE_ROOT}/${INST2}"

PASS=0
FAIL=0
STEP=0
FAILED_STEPS=()

color_ok="\033[32m"
color_fail="\033[31m"
color_dim="\033[2m"
color_off="\033[0m"

log()  { echo -e "${color_dim}    $*${color_off}"; }
ok()   { echo -e "${color_ok}✓${color_off} $*"; PASS=$((PASS+1)); }
fail() { echo -e "${color_fail}✗${color_off} $*"; FAIL=$((FAIL+1)); FAILED_STEPS+=("step ${STEP}: $*"); }

step() { STEP=$((STEP+1)); echo; echo "── Step ${STEP}: $* ──"; }

# Run a command quietly, capture exit code, stream stdout/stderr to a log.
run_quiet() {
    local label="$1"; shift
    local logfile; logfile="$(mktemp)"
    if "$@" >"$logfile" 2>&1; then
        log "${label} ok"
        rm -f "$logfile"
        return 0
    else
        local rc=$?
        echo "    ${label} FAILED (exit ${rc}). Log:"
        sed 's/^/      /' "$logfile"
        rm -f "$logfile"
        return $rc
    fi
}

cleanup() {
    local rc=$?
    echo
    echo "── Cleanup ──"
    for inst in "$INST1" "$INST2"; do
        if sudo "$BIN" list 2>/dev/null | grep -q "$inst"; then
            log "destroying $inst"
            sudo "$BIN" destroy -i "$inst" --purge <<<"$inst" >/dev/null 2>&1 || true
        fi
        # belt-and-braces: remove any straggling unit / data
        sudo rm -f "/etc/systemd/system/hydraserver-${inst}.service" 2>/dev/null || true
        sudo rm -rf "${BASE_ROOT}/${inst}" 2>/dev/null || true
    done
    sudo systemctl daemon-reload 2>/dev/null || true
    sudo rm -rf "$BASE_ROOT" 2>/dev/null || true
    echo
    if [[ $FAIL -gt 0 ]]; then
        echo -e "${color_fail}SMOKE FAILED${color_off} — ${PASS} passed, ${FAIL} failed"
        for s in "${FAILED_STEPS[@]}"; do echo "  - $s"; done
        exit 1
    elif [[ $rc -ne 0 ]]; then
        echo -e "${color_fail}SMOKE ABORTED${color_off} (exit ${rc}) — ${PASS} passed before abort"
        exit "$rc"
    else
        echo -e "${color_ok}SMOKE PASSED${color_off} — ${PASS} steps green"
        exit 0
    fi
}
trap cleanup EXIT

require() {
    [[ -x "$BIN" ]] || { echo "Binary $BIN not found or not executable"; exit 2; }
    sudo -n true 2>/dev/null || { echo "Need passwordless sudo"; exit 2; }
    [[ -d /run/systemd/system ]] || { echo "Need systemd"; exit 2; }
}

# pre_cleanup wipes any leftover smoke-* state from previous runs that
# crashed before their EXIT trap could fire. Without this, stale
# metadata.json entries reserve port pairs and the next run picks unexpected
# defaults.
pre_cleanup() {
    log "pre-run cleanup: stopping any stale smoke services"
    while read -r unit; do
        [[ -z "$unit" ]] && continue
        sudo systemctl stop "$unit" 2>/dev/null || true
        sudo systemctl disable "$unit" 2>/dev/null || true
    done < <(systemctl list-units --all 'hydraserver-smoke-*' --no-legend 2>/dev/null | awk '{print $1}')
    sudo rm -f /etc/systemd/system/hydraserver-smoke-*.service 2>/dev/null || true
    sudo systemctl daemon-reload 2>/dev/null || true
    sudo pkill -9 -f '/tmp/hydraide-smoke' 2>/dev/null || true
    sleep 1
    sudo rm -rf "$BASE_ROOT" 2>/dev/null || true
    # Wipe smoke-* entries from buildmetadata so the next run starts clean.
    if [[ -f "$HOME/.hydraide/metadata.json" ]] && command -v python3 >/dev/null; then
        python3 -c "
import json, pathlib
p = pathlib.Path('$HOME/.hydraide/metadata.json')
try:
    data = json.loads(p.read_text() or '{}')
except Exception:
    data = {}
data = {k: v for k, v in data.items() if not k.startswith('smoke-')}
p.write_text(json.dumps(data, indent=2))
"
    fi
}

# Drive an interactive `init` with stdin: instance name (always via -i flag),
# then base path, then confirmation. We run with -i so only the base path and
# confirmation prompts remain — both default-friendly.
init_default() {
    local inst="$1" base="$2"
    sudo "$BIN" init -i "$inst" <<EOF
$base
y
EOF
}

# Edit driver helpers: each one drives the menu in batch mode.
# The menu loops until 's' (save), 'q' (quit), or unknown; we feed a sequence.
edit_change_port() {
    local inst="$1" newport="$2"
    # menu choice 1, new port, save, confirm clients-stopped
    sudo "$BIN" edit -i "$inst" <<EOF
1
$newport
s
y
EOF
}

edit_rotate_san() {
    local inst="$1" extra_ip="$2"
    # menu 4, confirm rotate, keep CN (enter), DNS empty (keeps), IPs +extra, type "rotate", save, confirm clients-stopped
    sudo "$BIN" edit -i "$inst" <<EOF
4
y

localhost
127.0.0.1,$extra_ip
rotate
s
y
EOF
}

edit_repair_unit() {
    local inst="$1"
    # menu 5, save (no clients-stopped prompt — service was just (re)started by reinstall)
    sudo "$BIN" edit -i "$inst" <<EOF
5
s
EOF
}

# ─── tests ──────────────────────────────────────────────────────────────────

require
echo "Smoke run with bin=$BIN, instances=$INST1,$INST2, base_root=$BASE_ROOT"
pre_cleanup

step "init -i $INST1 (default flow, 2 prompts)"
init_default "$INST1" "$BASE1" >/tmp/smoke-init-1.log 2>&1
init_rc=$?
if [[ $init_rc -eq 0 && -f "$BASE1/.env" && -f "$BASE1/certificate/server.crt" ]]; then
    ok "instance ${INST1} created at ${BASE1}"
else
    fail "init failed (rc=$init_rc) or missing files; log:"
    sed 's/^/    /' /tmp/smoke-init-1.log
    exit 1
fi

step "health -i $INST1 (expect exit 0)"
if sudo "$BIN" health -i "$INST1" >/tmp/smoke-health-1.log 2>&1; then
    ok "health=0 for ${INST1}"
else
    fail "health did not return 0; log:"
    sed 's/^/    /' /tmp/smoke-health-1.log
fi

step "list shows $INST1 as healthy"
if sudo "$BIN" list 2>&1 | tee /tmp/smoke-list-1.log | grep -q "$INST1"; then
    ok "list contains ${INST1}"
else
    fail "list does not contain ${INST1}"
    sed 's/^/    /' /tmp/smoke-list-1.log
fi

step "init -i $INST2 — second instance must auto-bump ports"
if init_default "$INST2" "$BASE2" >/tmp/smoke-init-2.log 2>&1; then
    p1=$(grep '^HYDRAIDE_SERVER_PORT=' "$BASE1/.env" | cut -d= -f2)
    p2=$(grep '^HYDRAIDE_SERVER_PORT=' "$BASE2/.env" | cut -d= -f2)
    if [[ -n "$p1" && -n "$p2" && "$p1" != "$p2" ]]; then
        ok "ports differ: ${INST1}=${p1}, ${INST2}=${p2}"
    else
        fail "expected different ports, got '${p1}' and '${p2}'"
    fi
else
    fail "second init failed"
    sed 's/^/    /' /tmp/smoke-init-2.log
fi

step "edit ports on $INST1, restart, check health"
# pick a definitely-free port pair
NEWPORT=4950
edit_change_port "$INST1" "$NEWPORT" >/tmp/smoke-edit-port.log 2>&1
new_p=$(grep '^HYDRAIDE_SERVER_PORT=' "$BASE1/.env" | cut -d= -f2 || true)
if [[ "$new_p" == "$NEWPORT" ]]; then
    sleep 2
    if sudo "$BIN" health -i "$INST1" >/dev/null 2>&1; then
        ok "ports edited to ${NEWPORT} and instance still healthy"
    else
        fail "instance unhealthy after port edit"
        sed 's/^/    /' /tmp/smoke-edit-port.log
    fi
else
    fail "expected port=${NEWPORT}, got '${new_p}'"
    sed 's/^/    /' /tmp/smoke-edit-port.log
fi

step "edit TLS SANs on $INST1, add 10.0.0.42, verify cert contains it"
edit_rotate_san "$INST1" "10.0.0.42" >/tmp/smoke-edit-san.log 2>&1
if openssl x509 -in "$BASE1/certificate/server.crt" -noout -text 2>/dev/null | grep -q "10.0.0.42"; then
    ok "server.crt now contains 10.0.0.42"
else
    fail "10.0.0.42 not found in regenerated cert"
    sed 's/^/    /' /tmp/smoke-edit-san.log
fi

step "systemctl status hydraserver-$INST1 — active?"
if systemctl is-active "hydraserver-$INST1" >/dev/null 2>&1; then
    ok "service active"
else
    fail "service not active"
    sudo systemctl status "hydraserver-$INST1" --no-pager | sed 's/^/    /' || true
fi

step "upgrade --force --yes auto-restarts the instance"
sudo "$BIN" upgrade -i "$INST1" --force --yes >/tmp/smoke-upgrade.log 2>&1
upgrade_rc=$?
if [[ $upgrade_rc -eq 0 ]] && sudo "$BIN" health -i "$INST1" >/dev/null 2>&1; then
    ok "upgrade succeeded and instance is healthy (auto-restart confirmed)"
else
    fail "upgrade rc=$upgrade_rc or post-upgrade health failed; log:"
    sed 's/^/    /' /tmp/smoke-upgrade.log
fi

step "stop --yes then start, instance comes back healthy"
sudo "$BIN" stop -i "$INST1" --yes >/tmp/smoke-stop.log 2>&1 || true
sleep 2
if systemctl is-active "hydraserver-$INST1" >/dev/null 2>&1; then
    fail "service still active after stop"
    sed 's/^/    /' /tmp/smoke-stop.log
else
    sudo "$BIN" start -i "$INST1" >/tmp/smoke-start.log 2>&1 || true
    sleep 3
    if sudo "$BIN" health -i "$INST1" >/dev/null 2>&1; then
        ok "stop/start cycle works with --yes"
    else
        fail "instance not healthy after start"
        sed 's/^/    /' /tmp/smoke-start.log
    fi
fi

step "simulate broken unit, repair via edit"
sudo systemctl stop "hydraserver-$INST1" 2>/dev/null || true
sudo rm -f "/etc/systemd/system/hydraserver-${INST1}.service"
sudo systemctl daemon-reload
edit_repair_unit "$INST1" >/tmp/smoke-edit-repair.log 2>&1
sleep 2
if [[ -f "/etc/systemd/system/hydraserver-${INST1}.service" ]] && systemctl is-active "hydraserver-$INST1" >/dev/null 2>&1; then
    ok "unit reinstalled and service active"
else
    fail "unit reinstall did not restore the service"
    sed 's/^/    /' /tmp/smoke-edit-repair.log
fi

step "destroy --purge leaves no residue"
# destroy --purge prompts the user to retype the instance name as confirmation.
sudo "$BIN" destroy -i "$INST1" --purge <<<"$INST1" >/tmp/smoke-destroy-1.log 2>&1 || true
sudo "$BIN" destroy -i "$INST2" --purge <<<"$INST2" >/tmp/smoke-destroy-2.log 2>&1 || true
residue=0
problems=()
[[ -d "$BASE1" ]] && { residue=$((residue+1)); problems+=("$BASE1 still exists"); }
[[ -d "$BASE2" ]] && { residue=$((residue+1)); problems+=("$BASE2 still exists"); }
[[ -f "/etc/systemd/system/hydraserver-${INST1}.service" ]] && { residue=$((residue+1)); problems+=("unit ${INST1} present"); }
[[ -f "/etc/systemd/system/hydraserver-${INST2}.service" ]] && { residue=$((residue+1)); problems+=("unit ${INST2} present"); }
if grep -q "$INST1\|$INST2" "$HOME/.hydraide/metadata.json" 2>/dev/null; then
    residue=$((residue+1)); problems+=("metadata.json still references one of the instances")
fi
if [[ $residue -eq 0 ]]; then
    ok "no residue after destroy --purge"
else
    fail "${residue} leftover artefacts after destroy: ${problems[*]}"
    echo "    destroy log INST1:"
    sed 's/^/      /' /tmp/smoke-destroy-1.log
fi
