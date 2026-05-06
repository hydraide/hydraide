# CLAUDE.md

Guidance for Claude Code (and other AI coding assistants) working on this repository.

Two bundled skills cover the depth:

- [`.claude/skills/hydraide/SKILL.md`](.claude/skills/hydraide/SKILL.md) — application side: modelling, filters, patches, locks, subscriptions, common pitfalls.
- [`.claude/skills/hydraidectl/SKILL.md`](.claude/skills/hydraidectl/SKILL.md) — operations side: install, upgrade, backup/restore, migrate, observe.

## Communication

- All code, comments, documentation, and commit messages are in English.
- Ask before generating code if a requirement is ambiguous.

## Project overview

HydrAIDE is a structure-first data engine written in Go. Data is organised as **Sanctuary / Realm / Swamp** (a 3-level namespace). Each Swamp is a storage unit holding key-value **Treasures**.

The server speaks gRPC with mTLS. Clients use the Go SDK (`hydraidego`) or raw protoc-generated clients in any language.

## Architecture

### Key layers (top-down)

| Layer | Path | Role |
|---|---|---|
| **Proto** | `proto/hydraide.proto` | gRPC service definition, the single source of truth for the wire API |
| **Generated** | `generated/hydraidepbgo/` | Go protobuf/gRPC stubs |
| **Server** | `app/server/` | Entry point (`main.go`), gRPC server setup (`server/server.go`), request handling (`gateway/gateway.go`) |
| **Zeus** | `app/core/zeus/` | Orchestrator — owns Hydra, Settings, SafeOps |
| **Hydra** | `app/core/hydra/hydra.go` | Core engine — manages Swamps in memory, summoning, subscriptions, locking |
| **Swamp** | `app/core/hydra/swamp/` | A storage unit with Treasures, Beacon (index), Chronicler (disk I/O), Metadata, Vigil (lifecycle) |
| **Chronicler V2** | `app/core/hydra/swamp/chronicler/chronicler_v2.go` + `v2/` | Append-only single `.hyd` file storage — the current engine |
| **Chronicler V1** | `app/core/hydra/swamp/chronicler/chronicler.go` | Legacy multi-chunk storage; kept for migration support via `hydraidectl migrate` |
| **MsgPack patch** | `app/core/hydra/swamp/treasure/msgpackpatch/` | Server-side atomic field-level mutations on msgpack Treasures |
| **Settings** | `app/core/settings/` | Runtime config, swamp patterns |
| **Name** | `app/name/name.go` | Deterministic namespace hashing: `Sanctuary/Realm/Swamp` → IslandID + folder path |
| **Go SDK** | `sdk/go/hydraidego/` | Client library wrapping gRPC calls |
| **CLI** | `app/hydraidectl/` | Management CLI (cobra-based): init, start, stop, upgrade, backup, restore, observe, migrate |
| **Telemetry** | `app/server/telemetry/` | Real-time gRPC call monitoring, streamed via `SubscribeToTelemetry` |

### Data flow

1. Client builds a `name.Name` (Sanctuary/Realm/Swamp) and computes `IslandID` via consistent hashing.
2. Client sends a gRPC request with `IslandID` + `SwampName` to the server.
3. Gateway dispatches to Zeus → Hydra → `SummonSwamp()` loads the Swamp into memory if needed.
4. Swamp uses Chronicler to read/write the `.hyd` file on disk.
5. Swamp auto-closes after the configured idle timeout, flushing data via Chronicler.

### File system layout

```
<data_root>/<islandID>/<hash_subfolder>/<swamp_hash>.hyd   (V2 — current)
<data_root>/<islandID>/<hash_subfolder>/<swamp_hash>/      (V1 legacy — folder with chunk files + meta)
```

- `IslandID` range is configurable (typically 1–1000) and computed client-side.
- Hash sub-folders are one level deep, up to 1000 folders per level.
- `.hyd` files contain: 64-byte FileHeader (`HYDR` magic) → swamp name (plain UTF-8) → compressed blocks → entries (insert / update / delete).
- The swamp name is stored in plain text right after the header in the optimized format, enabling fast ~100 byte/file metadata scans without decompressing any block. Older files use an `OpMetadata` entry inside the first compressed block as fallback; compaction auto-upgrades the header.

For storage measurements (insert / update / delete / read latencies, on-disk size), see [`docs/benchmarks/V2_RESULTS_SUMMARY.md`](docs/benchmarks/V2_RESULTS_SUMMARY.md).

## Build commands

The `Makefile` covers local developer workflows only. Production builds (server
binaries, multi-arch Docker images, hydraidectl release binaries) run in
GitHub Actions under `.github/workflows/` and do not go through `make`.

```bash
# Regenerate Go gRPC stubs after editing proto/hydraide.proto
make proto-go

# Build the hydraidectl CLI locally (with version/commit/build-date ldflags)
make build-hydraidectl

# Build the server binary locally (no Makefile target — use go build directly)
cd app/server && go build -o ../../hydraide .

# Run Go tests
go test ./app/core/hydra/...
go test ./app/core/hydra/swamp/chronicler/v2/...
go test -run TestSpecificName ./app/core/hydra/...

# Tidy dependencies
go mod tidy
```

## Code conventions

- **Go version:** see `go.mod`.
- **Error handling:** always check errors with `if err != nil`. Use `errors.Is()` for specific error checks.
- **Never use `go func()` directly.** Always use `panichandler.SafeGo("label", func() { ... })` so panics in goroutines do not crash the server.
- **No `panic()` for error handling.** Reserve `panic()` for truly unrecoverable startup conditions.
- **Interface-based design.** Components communicate through interfaces. Constructor functions accept all dependencies.
- **Commit messages:** [Conventional Commits](https://www.conventionalcommits.org/), enforced by a pre-commit hook with `--strict`. Do **not** add `Co-Authored-By` lines or any reference to AI / Claude generation in commits, code comments, or PR descriptions. Commit messages describe the change itself, nothing else.
- **Language:** code, comments, logs, and commits in English. User-facing CLI output may use emoji for formatting.
- **Proto is the source of truth.** When changing `proto/hydraide.proto`, run `make proto-go` to regenerate stubs.

## Where to look first

| Looking for | Start here |
|---|---|
| Wire-level API | [`proto/hydraide.proto`](proto/hydraide.proto) |
| Modelling, filters, patches, locks, subscriptions | [`.claude/skills/hydraide/SKILL.md`](.claude/skills/hydraide/SKILL.md) |
| Operating an instance (`hydraidectl`) | [`.claude/skills/hydraidectl/SKILL.md`](.claude/skills/hydraidectl/SKILL.md) |
| Concept docs | [`docs/features/`](docs/features/) |
| Benchmarks and methodology | [`docs/benchmarks/`](docs/benchmarks/) |
| Install and operate | [`docs/install/`](docs/install/), [`docs/hydraidectl/`](docs/hydraidectl/) |
| Go SDK | [`sdk/go/hydraidego/`](sdk/go/hydraidego/) |
