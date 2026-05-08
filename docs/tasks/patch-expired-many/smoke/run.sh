#!/usr/bin/env bash
# Docker smoke runner for PatchExpiredTreasures + PatchManyRequest.Cond.
#
# This script assumes a running HydrAIDE server (docker or systemd) and
# the standard mTLS cert layout. It does NOT spin up the server itself
# — that side of the docker harness is environment-specific (network,
# volume mounts, certs). See README.md for the env vars it consumes and
# the example layout.
#
# Override via env:
#   HYDRAIDE_HOST       (default: localhost:4444)
#   HYDRAIDE_CA_CRT     (default: ./certs/ca.crt)
#   HYDRAIDE_CLIENT_CRT (default: ./certs/client.crt)
#   HYDRAIDE_CLIENT_KEY (default: ./certs/client.key)

set -euo pipefail

cd "$(dirname "$0")"

# The smoke module sits inside the repo but is intentionally not part of
# go.work (it's a standalone consumer of the SDK, not a workspace
# member). Run with GOWORK=off so go uses this module's go.mod
# directly + the replace directive that points back at the local SDK.
export GOWORK=off

# Sanity: the smoke binaries import the local SDK via a replace directive.
go mod tidy >/dev/null

run_one() {
    local name="$1"
    echo "→ ${name}"
    if ! go run "./cmd/${name}"; then
        echo "✗ smoke '${name}' failed" >&2
        exit 1
    fi
}

run_one claim
run_one recovery
run_one condmany
run_one metaonly

echo
echo "All smoke tests passed."
