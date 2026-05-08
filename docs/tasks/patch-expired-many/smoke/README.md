# Docker smoke tests — PatchExpiredTreasures + PatchManyRequest.Cond

Self-contained smoke tests that exercise the new RPC end-to-end against
a real HydrAIDE server in a Docker container.

## Layout

```
smoke/
├── README.md       — this file
├── run.sh          — orchestrates the server container + runs all smokes
├── go.mod
└── cmd/
    ├── claim/      — concurrent disjoint-subset claim test
    ├── recovery/   — crash-recovery (lease-elapsed re-claim) test
    ├── condmany/   — per-request Cond on CatalogPatchFieldsMany
    └── metaonly/   — empty-Ops + meta-only ExpireAt slide
```

Each cmd is a self-contained Go program that exits with code `0` on
success and a non-zero code (with a `FAIL: <reason>` line on stderr) on
any assertion failure. `run.sh` chains them, so the first failure stops
the whole run.

## Prerequisites

* Docker installed (server image is built from the repo root).
* Go 1.22+ installed (smoke binaries build locally; only the server
  runs in Docker).

## Running

```bash
# from the repo root
cd docs/tasks/patch-expired-many/smoke
./run.sh
```

`run.sh`:

1. Builds the server Docker image from the current branch.
2. Starts a fresh container with a tmpfs data volume + auto-generated
   mTLS certs (via `hydraidectl init` against the container's data
   dir).
3. Exports `HYDRAIDE_HOST` + cert paths as env vars.
4. Builds and runs each smoke binary in turn.
5. Tears down the container.

Each smoke prints a single `PASS: <name>` line on success.

## Setup notes

The smoke programs read the following env vars (defaults in parens):

* `HYDRAIDE_HOST` (`localhost:4444`) — server address.
* `HYDRAIDE_CA_CRT` (`./certs/ca.crt`) — CA certificate.
* `HYDRAIDE_CLIENT_CRT` (`./certs/client.crt`) — client certificate.
* `HYDRAIDE_CLIENT_KEY` (`./certs/client.key`) — client key.

To run a single smoke against an already-running server (skipping
`run.sh`):

```bash
HYDRAIDE_HOST=localhost:4444 \
HYDRAIDE_CA_CRT=$HOME/.hydraide/ca.crt \
HYDRAIDE_CLIENT_CRT=$HOME/.hydraide/client.crt \
HYDRAIDE_CLIENT_KEY=$HOME/.hydraide/client.key \
go run ./cmd/claim
```
