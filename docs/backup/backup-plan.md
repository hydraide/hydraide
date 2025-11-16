# HydrAIDE Backup Plan

This document outlines a practical, incremental backup approach for HydrAIDE, based on the existing filesystem layout and the meta file (`meta`) each Swamp folder contains.

Goal: Zero-downtime, safe, incremental backups that copy only updated Swamps to a remote HydrAIDE-compatible storage, with simple disaster recovery.

## Key Concepts

- Each Swamp resides under: `HYDRA_DATA_ROOT/<IslandID>/<hash-folders...>/<swamp-folder>/`.
- Each Swamp folder contains a `meta` file (GOB) with:
  - `SwampName` (canonical: `sanctuary/realm/swamp`)
  - `CreatedAt`, `UpdatedAt`, `BackupAt` timestamps
- New Hydra API: `SummonSwampByFolderPath(ctx, absFolderPath)` can reconstruct and open a Swamp from a folder, using the meta file.
- We leverage `UpdatedAt` vs `BackupAt` to detect whether a Swamp needs backup.

## Modes of Operation

We define two complementary transport modes. Both are gated by meta timestamps (UpdatedAt vs BackupAt):

- Mode A — Filesystem-native incremental copy
  - Copy Swamp folders/files using rsync/ZFS/btrfs send.
  - Optional local state DB (hash/mtime/size) to reduce bandwidth.
  - Pros: fastest raw throughput. Cons: coupled to storage layout; be careful with in-progress writes (mitigated by flush).

- Mode B — Logical HydrAIDE→HydrAIDE streaming
  - Stream Treasures from source HydrAIDE directly to a destination HydrAIDE instance via client API.
  - Avoids copying files that may be mid-write or partially flushed; always deals with consistent in-memory state.
  - Pros: storage-agnostic, safer semantics. Cons: more CPU/network. Excellent for correctness.

Recommendation: start with Mode A for immediate practicality, and add Mode B for high-integrity, cross-platform backups.

## High-level Flow (Common)

1. Filesystem scan (single pass):
   - Iterate Island folders (digits under `HYDRA_DATA_ROOT`).
   - Traverse to each Swamp folder (leaf with `meta`).
   - Read meta; if `UpdatedAt > BackupAt`, enqueue for backup.
2. Backup execution per Swamp:
   - Open Swamp via `SummonSwampByFolderPath(ctx, folder)` (safe if already open).
   - For permanence: optionally call `WriteTreasuresToFilesystem()` to flush pending writes before copy (Mode A).
   - Perform the chosen transport (Mode A or Mode B).
   - On success: `meta.SetBackupAt(now)` and `meta.SaveToFile()`.

## Logical HydrAIDE→HydrAIDE Streaming (Mode B)

- Reader side (source):
  - Open Swamp, set BeginVigil(), iterate Treasures (`GetTreasuresByBeacon`/`CloneTreasures`) in batches.
  - Serialize as wire messages via SDK and send to destination.
  - CeaseVigil() quickly after each batch.
- Writer side (destination):
  - Receive stream per Swamp, `SummonSwamp(ctx, island, name)`, then upsert Treasures.
  - Ensure idempotence on duplicate batches (based on Treasure keys and timestamps).
- Consistency:
  - Optionally stamp batches with a snapshot watermark (e.g., source meta UpdatedAt at start) for auditing.
  - If transfer fails, nothing is overwritten because we are writing a new version (see below).

## Versioned Backups and Immutability

Industry-aligned practices to avoid clobbering data and to enable controlled recovery:

- Do not overwrite previous backups. Create new backup versions atomically, then promote.
- Retain multiple versions based on policy (e.g., GFS: daily/weekly/monthly) and enforce retention with compaction.
- Immutability options:
  - Object storage with write-once / object lock (WORM) for a defined period.
  - Append-only backup store for the version’s lifetime.
- 3-2-1 rule: 3 copies, 2 media, 1 offsite is recommended for critical deployments.
- Integrity checks: store and verify checksums (per-chunk/per-file) on write and periodically.

## Storage Format for Efficiency

To avoid “many small files” and enable fast restore:

- Swamp-level pack files per backup version:
  - `swamp-pack-<sanctuary>_<realm>_<swamp>-<version>.hpack` (container), plus an index/catalog JSON.
  - Container contents: length-prefixed Treasure records (similar to on-disk file parts), optional compression (Snappy/Zstd), optional encryption.
  - Index maps Treasure keys → offsets, timestamps, optional metadata (for partial restore and search).
- Global catalog per backup cycle:
  - `backup-index-<version>.json` listing all Swamps, counts, sizes, checksums, and time ranges.
- Benefits:
  - Fewer, larger files → lower syscall overhead, better throughput.
  - Index enables partial restore by Swamp or pattern, and time-bounded restores.

Mode A mapping:
- If using filesystem copy, you may still pack files during backup to reduce the small-file problem on the target, leaving the source untouched.

Mode B mapping:
- The receiver can directly write to pack files rather than raw Swamp folders for a pure backup repository.

## Restore Scenarios

- Full restore (DR): restore the entire `HYDRA_DATA_ROOT` from the latest version or a chosen version.
- Partial restore by Swamp name/pattern: use the catalog to select matching Swamps and unpack only those.
- Point-in-time restore: choose the backup version whose timestamp is closest to the requested time; later support for time window filtering via index metadata.
- Dry-run and verification: list and validate (checksums) without writing.

## hydraidectl Integration

Backups and DR must be operable from hydraidectl. Proposed commands:

- `hydraidectl backup run --mode=[fs|logical] --target=<URL|path> [--parallel=N] [--since=<duration>] [--dry-run]`
  - Scans, decides (UpdatedAt vs BackupAt), executes per Swamp.
- `hydraidectl backup list-versions --target=<URL|path>`
- `hydraidectl backup show --version=<id> --target=<URL|path>`
- `hydraidectl backup verify --version=<id> --target=<URL|path> [--swamp=<pattern>]`
- `hydraidectl backup restore --version=<id> --target=<URL|path> [--swamp=<pattern>] [--to=<local-root>] [--dry-run]`
- `hydraidectl backup policy set --retention="7d,4w,12m" --target=<URL|path>`
- `hydraidectl backup purge --target=<URL|path>`

Notes:
- Mode B requires destination HydrAIDE endpoint configuration (auth, TLS, tenant, etc.).
- All destructive operations should be protected by confirmation flags and dry-runs.

## Incremental Detection Details

- Primary gate: `UpdatedAt` vs `BackupAt` from meta.
- Mode A enhancement: maintain a state DB with per-file size/mtime/hash to avoid copying unchanged files.
- Mode B enhancement: send only Treasures newer than the destination’s recorded watermark (optional).

## Error Handling and Safety

- Never update `BackupAt` on failure.
- Use retries with exponential backoff; isolate failing Swamps.
- Keep a quarantine report for invalid metas or corrupt containers.
- Use bounded worker pools and SafeGo runners.

## Observability

- Metrics: queued/completed/failed jobs, bytes transferred, latency, throughput.
- Logs: per Swamp decisions (skipped vs backed up), version IDs, checksums, errors.

## Security

- Optional encryption at rest for pack files.
- TLS and auth for Mode B transport.
- Avoid leaking Swamp names and keys in logs; redact where appropriate.

## Roadmap

- Phase 1: Mode A (filesystem) + meta gating + optional packing on target.
- Phase 2: Mode B (logical streaming) + receiver service; write to pack format and catalog.
- Phase 3: Advanced features — deduplication, point-in-time filtering, swamp-pattern selective restore, background verify.

---

This plan achieves automated, low-impact backups, supports both storage-native and logical streaming modes, and lays the groundwork for versioned, immutable backups with efficient partial restores and hydraidectl-driven disaster recovery.
