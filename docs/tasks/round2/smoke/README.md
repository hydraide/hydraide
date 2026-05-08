# Docker smoke tests — Round 2 (R2-1..R2-7)

Self-contained smoke tests that exercise the Round 2 features end-to-end
against a real HydrAIDE server in a Docker container.

## Layout

```
smoke/
├── README.md       — this file
├── run.sh          — runs all smokes against an already-running server
├── go.mod          — standalone module, replace-points at the local SDK
└── cmd/
    ├── perkeymeta/        — R2-1: per-key Meta on TreasurePatch
    ├── batchbuilder/      — R2-2: builder-reuse PatchManyRequest
    ├── patchexpiredmany/  — R2-3: CatalogPatchExpiredManyFromMany
    ├── patchmany/         — R2-4: CatalogPatchManyToMany
    ├── shiftexpiredmany/  — R2-7: CatalogShiftExpiredManyFromMany
    └── indexexpire/       — R2-6: IndexExpirationTime ASC ordering
```

Each cmd is a self-contained Go program that exits with code `0` on
success and a non-zero code (with a `FAIL: <reason>` line on stderr) on
any assertion failure. `run.sh` chains them, so the first failure stops
the whole run. Each smoke prints a single `PASS: <name>` line on success.

## Prerequisites

* A running HydrAIDE server with the Round 2 wire RPCs (PatchTreasuresMany,
  PatchExpiredTreasuresMany, ShiftExpiredTreasuresMany).
* Go 1.22+ installed (smoke binaries build locally; only the server runs
  in Docker).

## Running

```bash
# from the repo root
cd docs/tasks/round2/smoke
./run.sh
```

The script reads the following env vars (defaults in parens):

* `HYDRAIDE_HOST` (`localhost:4444`) — server address.
* `HYDRAIDE_CA_CRT` (`./certs/ca.crt`) — CA certificate.
* `HYDRAIDE_CLIENT_CRT` (`./certs/client.crt`) — client certificate.
* `HYDRAIDE_CLIENT_KEY` (`./certs/client.key`) — client key.

To run a single smoke against an already-running server:

```bash
HYDRAIDE_HOST=localhost:4444 \
HYDRAIDE_CA_CRT=/path/to/ca.crt \
HYDRAIDE_CLIENT_CRT=/path/to/client.crt \
HYDRAIDE_CLIENT_KEY=/path/to/client.key \
go run ./cmd/perkeymeta
```

## What each smoke proves

| Smoke              | Asserts                                                                                                                   |
| ------------------ | ------------------------------------------------------------------------------------------------------------------------- |
| `perkeymeta`       | Per-key `WithExpiredAt` lands on the wire as `TreasurePatch.Meta`; two keys in one batch get distinct ExpireAt.            |
| `batchbuilder`     | Builder-reuse `PatchManyRequest` carries Set + Inc + Append + IfField + per-key Meta in one batch; CAS bucket reports correctly. |
| `patchexpiredmany` | Multi-swamp `CatalogPatchExpiredManyFromMany` claims expired entries from 2 swamps in one RPC; per-swamp counts match.    |
| `patchmany`        | Multi-swamp `CatalogPatchManyToMany` creates entries across 2 swamps in one RPC; per-swamp creation counts match.         |
| `shiftexpiredmany` | Multi-swamp `CatalogShiftExpiredManyFromMany` shifts and deletes expired entries from 2 swamps in one RPC.                |
| `indexexpire`      | `IndexExpirationTime` + `IndexOrderAsc` returns entries in soonest-first order regardless of insert order.                |
