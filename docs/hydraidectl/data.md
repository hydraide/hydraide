# Data management

Commands for backing up, restoring, measuring, and maintaining the data
on disk for a HydrAIDE instance.

> Returning to the index? See [`README.md`](README.md).

---

## `backup` – Snapshot instance data

Creates a tar archive of an instance's full data directory. Default
behavior stops the instance to take a consistent snapshot.

### Synopsis

```bash
sudo hydraidectl backup -i <instance> -t <target> [--compress] [--no-stop]
```

### Flags

| Flag | Default | Effect |
|---|---|---|
| `--instance` / `-i` **(required)** | — | Source instance. |
| `--target` / `-t` **(required)** | — | Target archive path. With `--compress`, end the path in `.tar.gz`; otherwise it's a directory. |
| `--compress` | `false` | Compress the archive as gzip. |
| `--no-stop` | `false` | Don't stop the instance. **Risky** — the snapshot may be inconsistent. Use only when downtime is unacceptable and you've validated restore in a non-production environment. |

### Behavior

1. Validates the instance exists.
2. Unless `--no-stop` is passed: stops the instance gracefully, distinguishing
   "not running" (proceed silently) from real stop errors (abort the
   backup, do not produce an inconsistent snapshot).
3. Walks the base path and writes a tar archive (gzipped if `--compress`).
4. Reports total size and file count.
5. **Does not auto-restart.** Run `hydraidectl start -i <instance>` once
   you've verified the archive.

### Examples

```bash
# Plain tar.
sudo hydraidectl backup -i prod -t /backup/hydraide-20260121

# Gzipped — recommended for production retention.
sudo hydraidectl backup -i prod -t /backup/hydraide.tar.gz --compress

# After backup, manually restart.
sudo hydraidectl start -i prod
```

### Gotchas

- **`--no-stop` snapshots are not guaranteed restorable.** They will be
  the right size but may capture half-flushed data. Treat them as
  best-effort.
- The default-mode backup needs the same "stop your clients first"
  treatment as `stop`/`restart`/`upgrade` — if clients hold open TCP
  connections, the implicit stop phase will hang.

---

## `restore` – Restore from backup

Replaces an instance's data directory with the contents of a backup
archive.

### Synopsis

```bash
sudo hydraidectl restore -i <instance> -s <source> [--force]
```

### Flags

| Flag | Default | Effect |
|---|---|---|
| `--instance` / `-i` **(required)** | — | Target instance (must already exist). |
| `--source` / `-s` **(required)** | — | Source archive — directory or `.tar.gz`. |
| `--force` | `false` | Skip the "are you sure?" confirmation. Use in scripts. |

### Behavior

1. Validates the instance and the source archive.
2. Stops the instance.
3. Replaces the existing data directory with the contents of the archive.
4. Reports the file count restored.
5. **Does not auto-restart.** Run `hydraidectl start -i <instance>` once
   you've verified the data.

### Examples

```bash
# Restore from a directory backup.
sudo hydraidectl restore -i prod -s /backup/hydraide-20260121

# Restore from a tar.gz.
sudo hydraidectl restore -i prod -s /backup/hydraide.tar.gz

# Restore in an automation pipeline.
sudo hydraidectl restore -i prod -s /backup/today.tar.gz --force
```

### Gotchas

- **`restore` overwrites the existing data directory.** There is no
  intermediate snapshot — if the restore turns out to be the wrong
  archive, you've lost the current data. Take a fresh `backup` before
  restoring if there's any doubt.

---

## `compact` – Reclaim space from fragmented swamps

Compacts V2 swamp files in an instance to remove dead entries and reclaim
disk space. Compaction also automatically upgrades file headers to the
optimized format that embeds the swamp name (faster scanning).

### Synopsis

```bash
sudo hydraidectl compact -i <instance> [--threshold N] [--restart] [--dry-run] [--parallel N] [--json]
```

### Flags

| Flag | Default | Effect |
|---|---|---|
| `--instance` / `-i` **(required)** | — | Target instance. |
| `--threshold` / `-t` | `20` | Fragmentation percentage above which a swamp is compacted. Lower is more aggressive. |
| `--parallel` / `-p` | `4` | Number of parallel compaction workers. |
| `--restart` / `-r` | `false` | Restart the instance after compaction. |
| `--dry-run` | `false` | Analyse only — report what would be compacted, don't write. |
| `--json` / `-j` | `false` | JSON output. |

### Behavior

1. Stops the instance (if running).
2. Scans every swamp file for fragmentation.
3. Compacts every swamp whose fragmentation is above `--threshold`. The
   output uses the optimized header format (swamp name embedded after the
   64-byte header).
4. Reports the total space reclaimed.
5. Restarts the instance when `--restart` is passed.

### Examples

```bash
# Just see what's fragmented.
hydraidectl compact -i prod --dry-run

# Compact above 20% fragmentation, leave instance stopped.
sudo hydraidectl compact -i prod

# Aggressive: compact above 10% fragmentation, then restart.
sudo hydraidectl compact -i prod --threshold 10 --restart

# Faster on large datasets.
sudo hydraidectl compact -i prod --parallel 8 --restart
```

### Notes

- HydrAIDE also runs auto-compaction on Swamp close above the configured
  fragmentation threshold. Manual `compact` is for batch maintenance, not
  a constant requirement.
- Compaction is a no-op when fragmentation is already low — see
  [`stats`](#stats--per-swamp-statistics-and-health) for the current
  level before running.

---

## `cleanup` – Remove orphaned storage files

Deletes files left over from a migration or a rollback. Used after
`migrate v1-to-v2` to remove the V1 chunk folders, or after rolling back
to V1 to remove leftover V2 `.hyd` files.

### Synopsis

```bash
sudo hydraidectl cleanup -i <instance> [--v1-files | --v2-files] [--dry-run]
```

### Flags

| Flag | Default | Effect |
|---|---|---|
| `--instance` / `-i` **(required)** | — | Target instance. |
| `--v1-files` | `false` | Remove V1 chunk files and folders. |
| `--v2-files` | `false` | Remove V2 `.hyd` files. |
| `--dry-run` | `false` | Show what would be deleted, write nothing. |

Pass exactly one of `--v1-files` or `--v2-files`.

### Examples

```bash
# Plan — list what would be deleted.
hydraidectl cleanup -i prod --v1-files --dry-run

# Remove V1 leftovers after a successful V1→V2 migration.
sudo hydraidectl cleanup -i prod --v1-files

# Remove V2 leftovers after rolling back to V1.
sudo hydraidectl cleanup -i prod --v2-files
```

### Gotchas

- **Always `--dry-run` first.** Cleanup is irreversible.
- Don't mix the two: deleting V1 files when the active engine is V1, or
  deleting V2 files when the active engine is V2, will destroy your
  data. Verify the active engine with [`engine -i <instance>`](upgrades.md#engine--view-or-change-storage-engine-version)
  before running.

---

## `size` – Show instance disk usage

Reports the total on-disk size for an instance's data directory, with a
V1 vs V2 breakdown.

### Synopsis

```bash
hydraidectl size -i <instance> [--detailed] [--json]
```

### Flags

| Flag | Default | Effect |
|---|---|---|
| `--instance` / `-i` **(required)** | — | Target instance. |
| `--detailed` | `false` | Append a "top 10 largest swamps" list. |
| `--json` / `-j` | `false` | JSON output. |

### Examples

```bash
# Quick total.
hydraidectl size -i prod

# Top 10 largest swamps too.
hydraidectl size -i prod --detailed
```

Output sample:

```
HydrAIDE Instance: prod
========================================
Data Path:   /var/hydraide/data
Total Size:  45.23 MB
Total Files: 1234

V1 Files:    0 (0.00 MB)
V2 Files:    50 (45.23 MB)

Top 10 Largest Swamps:
   1. words/index                    15.32 MB
   2. domains/metadata                8.45 MB
   ...
```

---

## `stats` – Per-swamp statistics and health

Scans every V2 swamp in the instance and reports fragmentation, dead
entry counts, compaction recommendations, and the largest / most
fragmented swamps. Saves the report to disk so subsequent `--latest`
calls are instant.

### Synopsis

```bash
hydraidectl stats -i <instance> [--latest] [--parallel N] [--json]
```

### Flags

| Flag | Default | Effect |
|---|---|---|
| `--instance` / `-i` **(required)** | — | Target instance. |
| `--latest` / `-l` | `false` | Print the last saved report instead of running a new scan. |
| `--parallel` / `-p` | `4` | Number of parallel scan workers. |
| `--json` / `-j` | `false` | JSON output. |

### Behavior

1. Walks every `.hyd` file in the data directory in parallel.
2. Computes per-swamp metrics: live records, dead entries, fragmentation
   percentage, file size, oldest/newest record timestamps.
3. Aggregates into the report shown below.
4. Saves the report to
   `<instance_base_path>/.hydraide/stats-report-latest.json` so
   `--latest` can return it without rescanning.

### Examples

```bash
# Full scan + report.
hydraidectl stats -i prod

# Last saved report (no new scan).
hydraidectl stats -i prod --latest

# JSON for automation.
hydraidectl stats -i prod --json

# Faster scan on a big instance.
hydraidectl stats -i prod --parallel 8
```

### Sample output

```
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
  💠 HydrAIDE Swamp Statistics - prod
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

📊 SUMMARY
────────────────────────────────────────────────────────────
  Total Database Size              │ 1.25 GB
  Total Swamps                     │ 1234
  Total Live Records               │ 456.7K
  Total Entries (incl. deleted)    │ 512.3K
  Dead Entries                     │ 55.6K
  Avg Records/Swamp                │ 370.1
  Avg Swamp Size                   │ 1.04 MB
  Scan Duration                    │ 2.345s

🔧 FRAGMENTATION & COMPACTION
────────────────────────────────────────────────────────────
  Average Fragmentation            │ ✅ 10.8%
  Swamps Needing Compaction        │ 23 (>20% fragmented)
  Estimated Reclaimable Space      │ 45.67 MB

📦 TOP 10 LARGEST SWAMPS / ⚡ TOP 10 MOST FRAGMENTED
   …
```

### Reading fragmentation

| Range | Verdict | Action |
|---|---|---|
| 0 – 20% | ✅ Healthy | None. |
| 20 – 50% | ⚠️ Moderate | Consider `compact`. |
| 50%+ | 🔴 High | Compaction recommended. |

Fragmentation grows when records are updated or deleted. Dead entries stay
in the file until compaction reclaims the space.

---

## `inspect` – Low-level `.hyd` file debugger

Inspects a single V2 swamp file and lists every entry: operation type,
key, data size, and (for inserts/updates) the treasure metadata. This is
a debugging tool — you reach for it when `stats` and `explore` aren't
enough to figure out what happened to a specific swamp.

### Synopsis

```bash
hydraidectl inspect -i <instance> -s <swamp-path> [--page N] [--per-page N] [--json] [-o <file>]
```

### Flags

| Flag | Default | Effect |
|---|---|---|
| `--instance` / `-i` **(required)** | — | Instance whose data directory to look in. |
| `--swamp` / `-s` | — | Relative path to the swamp **without** the `.hyd` extension. |
| `--page` | `1` | Page number for pagination through entries. |
| `--per-page` | `20` | Entries per page. |
| `--json` / `-j` | `false` | JSON output. |
| `--output` / `-o` | _(stdout)_ | Write JSON to a file instead of stdout. |

### What it shows

- File header info (created, block count, entry count).
- The swamp name parsed from metadata.
- Every entry with its operation type (`INSERT` / `UPDATE` / `DELETE`),
  key, and data size.
- Per-entry treasure metadata for inserts and updates: timestamps,
  creator, expiry.
- Per-file fragmentation analysis.

### Examples

```bash
# Page through entries on stdout.
hydraidectl inspect -i prod -s users/profiles/alice

# Dump everything to a JSON file for offline analysis.
hydraidectl inspect -i prod -s users/profiles/alice --json -o /tmp/alice.json
```
