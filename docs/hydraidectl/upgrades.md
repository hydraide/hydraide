# Upgrades & migration

Commands for moving an instance to a newer server version, converting data
between storage formats, and switching the active storage engine.

> Returning to the index? See [`README.md`](README.md). For the full
> step-by-step migration runbook with rollback, see
> [`hydraidectl-migration.md`](hydraidectl-migration.md).

---

## `upgrade` – In-place binary upgrade

End-to-end upgrade of a HydrAIDE instance to the latest server version.
Default flow is **fully automated**: stop → download → replace binary →
auto-restart → wait healthy.

### Synopsis

```bash
sudo hydraidectl upgrade -i <instance> [--no-start] [--force] [--yes]
```

### Flags

| Flag | Default | Effect |
|---|---|---|
| `--instance` / `-i` **(required)** | — | Instance to upgrade. |
| `--no-start` | `false` | Update the binary without starting the server. Use before a migration when you want the new binary in place but the server quiet. |
| `--force` | `false` | Re-download and reinstall even when already on the latest version. Clears the local download cache to ensure a fresh binary. |
| `--yes` / `-y` | `false` | Skip the interactive clients-stopped confirmation. Use in scripts. |

### Behavior

1. Asks the standard "have you stopped your clients?" confirmation.
2. Compares the recorded instance version with the latest GitHub release.
   No-op (no stop, no download) when already up to date — unless `--force`.
3. Stops the instance gracefully (only if it is currently active).
4. Downloads the latest binary into the instance's base path. Shows a
   byte-accurate progress bar.
5. Updates `metadata.json` with the new version.
6. Removes the old `systemd` unit and generates a fresh one for the new
   binary.
7. **Default**: enables and starts the new unit, then polls the health
   endpoint until the instance reports healthy.
8. **With `--no-start`**: registers the unit but leaves it stopped, prints
   the manual start command and exits.

### Timeouts

| Phase | Default |
|---|---|
| Overall operation | 600s |
| Controller command | 90s (allows graceful flush on stop) |
| Graceful start/stop | 600s |
| `systemctl` operations during service replacement | 30s each |

### Examples

```bash
# Standard upgrade — auto-restart, wait for healthy.
sudo hydraidectl upgrade -i prod --yes

# Upgrade and stay stopped (e.g. before running a migration).
sudo hydraidectl upgrade -i prod --no-start --yes

# Reinstall the same version (e.g. corrupted binary).
sudo hydraidectl upgrade -i prod --force --yes
```

### Gotchas

- **Stop your clients first.** As with `stop` / `restart` / `edit`-save,
  the upgrade stops the running service and HydrAIDE refuses to shut down
  gracefully while clients hold open TCP connections — the stop phase
  hangs. The CLI prompts before stopping; pass `--yes` to bypass in
  scripts.
- **No automatic rollback.** Always run [`backup`](data.md#backup--snapshot-instance-data)
  before upgrading a production instance — `restore` is the rollback path
  if the new binary misbehaves.
- **Auto-restart is the default.** `--no-start` is opt-in and intentional.
  After an `--no-start` upgrade you must run `sudo hydraidectl start -i <instance>`
  yourself.

---

## `migrate v1-to-v2` – Multi-file to single-file format

Migrates HydrAIDE data from the legacy V1 multi-chunk storage format to
the V2 append-only single-file `.hyd` format.

V2 vs V1 (measured on the reference benchmark — see
[`docs/benchmarks/V2_RESULTS_SUMMARY.md`](../benchmarks/V2_RESULTS_SUMMARY.md)):

- 32–112× faster write operations
- ~50% smaller storage footprint
- ~95% fewer files on disk
- Significantly lower SSD wear

### Synopsis

```bash
sudo hydraidectl migrate v1-to-v2 -i <instance> [--full | --dry-run] [flags]
```

### Flags

| Flag | Default | Effect |
|---|---|---|
| `--instance` / `-i` | — | Instance name. Recommended — auto-handles stop/start, engine version, cleanup. |
| `--data-path` | — | Path to a data directory. Manual mode for instances not registered with `hydraidectl`. |
| `--full` | `false` | Complete automated migration: stop → migrate → set engine to V2 → cleanup → start. |
| `--dry-run` | `false` | Plan the migration without writing anything. Reports counts and estimated work. |
| `--verify` | `false` | Verify data integrity after each swamp migration. |
| `--delete-old` | `false` | Delete V1 files after successful migration (alternative to running `cleanup` afterwards). |
| `--parallel` | `4` | Number of parallel migration workers. |
| `--json` | `false` | JSON output. |

Pass exactly one of `--instance` or `--data-path`.

### Behavior

`--full` (recommended) does everything in order:

1. Stops the instance.
2. Walks every V1 swamp folder and writes a V2 `.hyd` equivalent.
3. Verifies the new file (when `--verify` is passed).
4. Updates `settings.json` to set engine = V2.
5. Removes V1 chunks (when `--delete-old` is passed; otherwise leaves them
   for [`cleanup`](data.md#cleanup--remove-orphaned-storage-files)).
6. Starts the instance back up.

`--dry-run` skips everything from step 2 onward and reports counts only.

### Examples

```bash
# Recommended end-to-end migration with backup safety net.
sudo hydraidectl backup -i prod --target /backup/pre-migration --compress
sudo hydraidectl migrate v1-to-v2 -i prod --full

# See what would happen, no writes.
hydraidectl migrate v1-to-v2 -i prod --dry-run

# Manual mode (no instance registration required).
sudo hydraidectl migrate v1-to-v2 --data-path /var/hydraide/data --verify --delete-old
```

### Gotchas

- **Always back up first.** No automatic rollback — only `restore` from
  the pre-migration backup will get you back to V1.
- The full V2 migration runbook with rollback steps lives in
  [`hydraidectl-migration.md`](hydraidectl-migration.md). Read it before
  running `--full` on production data.

---

## `migrate v2-migrate-format` – Upgrade `.hyd` headers

Rewrites V2 `.hyd` headers in place to embed the swamp name as plain text
right after the 64-byte file header. Once embedded, `explore` and `stats`
can scan metadata at ~100 bytes per file without decompressing any data
blocks — roughly 100× faster on large datasets.

This is a **V2-internal format optimization**, not an engine change. The
server reads both old and new headers transparently; files that already
have the embedded name are skipped. The command is fully idempotent.

### Synopsis

```bash
sudo hydraidectl migrate v2-migrate-format -i <instance> [--restart] [--dry-run] [--parallel N] [--json]
```

### Flags

| Flag | Default | Effect |
|---|---|---|
| `--instance` / `-i` **(required)** | — | Target instance. |
| `--parallel` / `-p` | `4` | Number of parallel workers. |
| `--restart` / `-r` | `false` | Restart the instance after the upgrade. |
| `--dry-run` | `false` | Plan only — count files needing upgrade, write nothing. |
| `--json` / `-j` | `false` | JSON output. |

### Behavior

1. Stops the instance (if running).
2. Walks all `.hyd` files and identifies the ones still using the legacy
   header format.
3. Rewrites each one with the swamp name embedded.
4. Reports counts and any space delta.
5. Restarts the instance when `--restart` is passed.

### Examples

```bash
# Count files that would be touched.
hydraidectl migrate v2-migrate-format -i prod --dry-run

# Upgrade and restart in one go.
sudo hydraidectl migrate v2-migrate-format -i prod --restart

# Faster processing on large datasets.
sudo hydraidectl migrate v2-migrate-format -i prod --parallel 8 --restart
```

### Notes

- Compaction (`hydraidectl compact`) also upgrades the header format on
  any file it touches. Use `migrate v2-migrate-format` when you want to
  upgrade everything **now** without waiting for the compaction threshold
  to trip per-swamp.
- Available since server **v3.3.0** and hydraidectl **v2.4.0**.

---

## `engine` – View or change storage engine version

Shows the active storage engine version, or switches between V1 and V2.
Switching does **not** convert data — that is the job of
[`migrate v1-to-v2`](#migrate-v1-to-v2--multi-file-to-single-file-format).

### Synopsis

```bash
sudo hydraidectl engine -i <instance> [--set V1|V2] [--json]
```

### Flags

| Flag | Default | Effect |
|---|---|---|
| `--instance` / `-i` **(required)** | — | Target instance. |
| `--set` | — | Switch to `V1` or `V2`. Without this flag, the command just prints the current engine. |
| `--json` / `-j` | `false` | JSON output. |

### Examples

```bash
# Show current engine.
hydraidectl engine -i prod

# Switch to V2 (after running migrate v1-to-v2).
sudo hydraidectl engine -i prod --set V2

# Switch back to V1 (after restoring from a pre-migration backup).
sudo hydraidectl engine -i prod --set V1
```

### Gotchas

- **Switching the engine does not migrate data.** If you point the V2
  engine at a V1 data directory the server will not start. Always run the
  migrate command first, then switch the engine.
- New instances installed with the recent `init` flow already default to
  V2 — you typically never need this command unless you are operating an
  old V1 instance.

---

## End-to-end migration workflow

The recommended sequence for migrating a production instance from V1 to
V2, including a rollback path. The full guide with edge cases is in
[`hydraidectl-migration.md`](hydraidectl-migration.md).

```bash
# 1. Verify CLI is up to date.
hydraidectl version

# 2. Stop all clients connecting to the instance, then stop HydrAIDE.
sudo hydraidectl stop -i prod

# 3. Take a compressed backup as a rollback safety net.
sudo hydraidectl backup -i prod --target /backup/pre-migration --compress

# 4. Upgrade the server binary without starting (so the new binary
#    handles the migration).
sudo hydraidectl upgrade -i prod --no-start --yes

# 5. Run the full migration.
sudo hydraidectl migrate v1-to-v2 -i prod --full

# 6. Verify the new sizes look reasonable.
hydraidectl size -i prod --detailed

# 7. Start the instance.
sudo hydraidectl start -i prod

# 8. Health-check.
hydraidectl health -i prod
```

### Rollback

```bash
# 1. Stop the instance.
sudo hydraidectl stop -i prod

# 2. Restore from the pre-migration backup.
sudo hydraidectl restore -i prod --source /backup/pre-migration.tar.gz

# 3. Switch the engine back to V1.
sudo hydraidectl engine -i prod --set V1

# 4. Start.
sudo hydraidectl start -i prod
```
