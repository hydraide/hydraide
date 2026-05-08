#!/usr/bin/env bash
# Docker smoke runner for Round 2 (R2-1..R2-7).
#
# Assumes a running HydrAIDE server (docker or systemd) and the standard
# mTLS cert layout. The shape mirrors patch-expired-many/smoke/run.sh —
# this script does NOT spin up the server itself.
#
# Override via env:
#   HYDRAIDE_HOST       (default: localhost:4444)
#   HYDRAIDE_CA_CRT     (default: ./certs/ca.crt)
#   HYDRAIDE_CLIENT_CRT (default: ./certs/client.crt)
#   HYDRAIDE_CLIENT_KEY (default: ./certs/client.key)

set -euo pipefail

cd "$(dirname "$0")"

# Standalone smoke module — see go.mod replace directive.
export GOWORK=off

go mod tidy >/dev/null

run_one() {
    local name="$1"
    echo "→ ${name}"
    if ! go run "./cmd/${name}"; then
        echo "✗ smoke '${name}' failed" >&2
        exit 1
    fi
}

run_one perkeymeta
run_one batchbuilder
run_one patchexpiredmany
run_one patchmany
run_one shiftexpiredmany
run_one indexexpire

echo
echo "All Round 2 smoke tests passed."
