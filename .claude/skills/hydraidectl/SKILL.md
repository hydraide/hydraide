---
name: hydraidectl
description: Operating HydrAIDE instances with the hydraidectl CLI — install, start/stop/restart, upgrade, backup/restore, migrate (V1→V2), inspect, observe, compact, explore, destroy, certs. Use when installing, deploying, upgrading, migrating, backing up, debugging, or otherwise operating a HydrAIDE server instance. For Go SDK usage and data modelling, use the `hydraidego` skill instead.
---

# hydraidectl — Operations Skill

`hydraidectl` is the management CLI for HydrAIDE server instances on a host. This skill is the working reference for operating instances. For Go SDK usage and writing application code against HydrAIDE, see the sibling [`hydraidego` skill](../hydraidego/SKILL.md).

The full per-command flag reference is in [`docs/hydraidectl/`](../../../docs/hydraidectl/README.md) — one focused page per category (lifecycle, monitoring, upgrades, data). This skill is a tighter "what to reach for and when" overview.

---

## 1. Installing the CLI

```bash
# Linux / macOS (install hydraidectl itself)
curl -sSfL https://raw.githubusercontent.com/hydraide/hydraide/main/scripts/install-hydraidectl.sh | bash
```

Full installation guide (including Docker and manual paths): [`docs/hydraidectl/hydraidectl-install.md`](../../../docs/hydraidectl/hydraidectl-install.md).

---

## 2. Command map

### Lifecycle

| Command | Purpose |
|---|---|
| `init` | End-to-end install: 2 prompts (instance name + base path), generates TLS cert, downloads binary, registers systemd unit, starts, waits healthy. Requires `sudo`. `--advanced` exposes the full tunable set; `-i <name>` skips the instance-name prompt. |
| `edit -i <instance>` | Menu-based editor for an existing instance: ports, logging, gRPC, TLS SANs, systemd unit. Saves, restarts, waits healthy. Binary version stays put — use `upgrade` for that. |
| `start -i <instance>` | Start an instance that is currently stopped. |
| `stop -i <instance>` | Gracefully stop a running instance. |
| `restart -i <instance>` | Stop, then start (waits until healthy). |
| `destroy -i <instance>` | Stop, disable, and remove the instance from the host. With `--purge`, also deletes the data directory. **Irreversible.** |

### Inspection and monitoring

| Command | Purpose |
|---|---|
| `list` | List all installed instances on the host with version, status, health, and update-available flags. |
| `health -i <instance>` | Health probe. Exit codes: `0` healthy, `1` unhealthy, `3` unexpected. Use in shell scripts. |
| `observe` | Interactive TUI dashboard for live RPC metrics, requires `telemetry` to be enabled on the instance(s). |
| `telemetry` | Enable / disable telemetry collection used by `observe`. |
| `stats -i <instance>` | Detailed swamp-by-swamp statistics and health report. |
| `size -i <instance>` | Total on-disk size for the instance's data directory. |
| `inspect <swamp-file.hyd>` | Low-level inspection of a single `.hyd` file (header, blocks, entry counts, metadata). For debugging. |
| `explore -i <instance>` | Interactive Sanctuary / Realm / Swamp hierarchy browser. |

### Upgrade and version

| Command | Purpose |
|---|---|
| `upgrade -i <instance>` | Stop → download new binary → update unit → start → wait healthy. |
| `upgrade -i <instance> --force` | Reinstall the current version (e.g. corrupted binary). |
| `upgrade -i <instance> --no-start` | Upgrade without starting (useful before a migration). |
| `version` | Show CLI version and check whether a newer `hydraidectl` is available. |

### Backup and restore

| Command | Purpose |
|---|---|
| `backup -i <instance> --target <path>.tar.gz --compress` | Default: stop instance → tar.gz → restart. |
| `backup ... --no-stop` | Tar a running instance — risky, may capture inconsistent state. Avoid for production data. |
| `restore -i <instance> --source <path>.tar.gz` | Restore from a backup archive. |

### Storage engine and migration

| Command | Purpose |
|---|---|
| `engine -i <instance>` | View or change the active storage engine version on a per-instance basis. |
| `migrate v1-to-v2 -i <instance> --dry-run` | Plan a V1 (multi-file) → V2 (single-file) migration. Reports counts and estimated work without writing. |
| `migrate v1-to-v2 -i <instance> --full` | Execute the migration. |
| `migrate v2-migrate-format -i <instance>` | Upgrade `.hyd` file headers in-place to the optimized format that embeds the swamp name (faster ~100-byte metadata scans). Idempotent. |
| `cleanup -i <instance>` | After a migration, remove the obsolete files (V1 chunk folders or pre-format `.hyd` originals). |

See [`docs/hydraidectl/hydraidectl-migration.md`](../../../docs/hydraidectl/hydraidectl-migration.md) for the full migration procedure.

### Maintenance

| Command | Purpose |
|---|---|
| `compact -i <instance>` | Force compaction across swamps (or scoped via flags) to reclaim space from dead entries. Compaction also runs automatically on Swamp close above the fragmentation threshold. |

> Cert rotation: use `edit -i <instance>` and pick the **TLS SANs** section.
> Certs are always tied to a live instance now; the standalone `cert` command
> was removed.

---

## 3. Operational rules

These are the non-obvious rules that prevent the most common operational mistakes.

### Stop all clients before any lifecycle command that stops the service

`stop`, `restart`, `upgrade` and `edit` (save phase) all need to halt the
running instance to do their job, and HydrAIDE will not shut down gracefully
while clients still hold open TCP connections — the stop phase hangs. All
four commands prompt for explicit "have you stopped your clients?"
confirmation; `stop`, `restart`, and `upgrade` accept a `--yes` / `-y` flag
to bypass the prompt in scripts.

**Symptoms when ignored:** the operation hangs at "Stopping" or "Removing
service" indefinitely.

Procedure for any production upgrade:

1. Stop or pause every service that connects to the instance — including local development APIs and worker processes.
2. Verify with `ss -tn state established` (or equivalent) that no ESTABLISHED connections remain on the instance's port.
3. Run `hydraidectl upgrade`.
4. Wait for `hydraidectl health -i <instance>` to return exit code `0`.
5. Restart clients in reverse dependency order.

When `edit` asks the same confirmation and the user answers `n`, the
configuration is **still saved** to `.env` / regenerated certs — only the
restart is skipped. The instance keeps running with the old config until the
user runs `sudo hydraidectl restart -i <instance>` after quiescing clients.

### Stop all clients before default-mode `backup`

The default `backup` mode also stops the instance to take a consistent snapshot. Same constraint as upgrade — clients must be quiesced first, otherwise the stop phase will hang.

### `upgrade` has no rollback

There is no built-in rollback. Always take a fresh `backup` before upgrading a production instance, so that a `restore` is your fallback path if the new binary misbehaves.

### `--no-stop` backup is a last resort

`backup --no-stop` tars the data directory while the instance keeps writing. The archive may capture an inconsistent point-in-time state and may not restore cleanly. Use only when you genuinely cannot afford the brief downtime, and validate the restore in a non-production environment first.

### `destroy --purge` is irreversible

`destroy` without `--purge` removes the systemd service and config but leaves the data directory intact (you can re-attach the data later). With `--purge`, the data directory is wiped. Always confirm the target instance name before running.

### Multi-instance orchestration

When a host runs multiple instances and you are upgrading or backing up several of them, chain commands with `&&` so a failure in one stops the rest of the batch — never run them in parallel without coordination, because each command may briefly stop a service that another depends on.

```bash
sudo hydraidectl upgrade -i instance-a && \
sudo hydraidectl upgrade -i instance-b && \
sudo hydraidectl upgrade -i instance-c
```

### Filesystem choice

Use **ext4** on the data volume by default. HydrAIDE buffers writes in memory and flushes them in compressed append-only blocks, so a copy-on-write filesystem like ZFS adds metadata and write-amplification overhead without measurable benefit. XFS works equally well. See [`docs/install/README.md`](../../../docs/install/README.md) for the full hardware/filesystem guidance.

---

## 4. Common workflows

### A. First install on a fresh host

```bash
# 1. Install the CLI
curl -sSfL https://raw.githubusercontent.com/hydraide/hydraide/main/scripts/install-hydraidectl.sh | bash

# 2. Install an instance end-to-end (config + cert + binary + service + start)
sudo hydraidectl init -i <instance>

# 3. Verify
hydraidectl list
hydraidectl health -i <instance>
```

`init` only asks for the instance name (skipped when `-i` is provided) and
the base path. Everything else is auto-configured: localhost-only TLS, the
lowest free `4900/4901` pair (auto-bumps by 10 if taken), V2 storage, slog
at `info`. Pass `--advanced` to expose every tunable. To change settings
afterwards: `sudo hydraidectl edit -i <instance>`.

The full quickstart (with the SDK snippet for connecting from an app) is in
[`docs/install/quickstart.md`](../../../docs/install/quickstart.md).

### B. Production upgrade

```bash
# 1. Take a fresh backup (default mode stops the instance)
sudo hydraidectl backup -i <instance> --target /backups/$(date +%F)-pre-upgrade.tar.gz --compress

# 2. Stop all client services first (ssh into client hosts and stop them)
#    Do not skip this — see "Stop all clients before upgrade" above.

# 3. Upgrade
sudo hydraidectl upgrade -i <instance>

# 4. Verify
hydraidectl list
hydraidectl health -i <instance>

# 5. Restart clients
```

### C. Periodic backup

```bash
TODAY=$(date +%F)
sudo mkdir -p /backups/$TODAY
sudo hydraidectl backup -i <instance> --target /backups/$TODAY/<instance>.tar.gz --compress
ls -lh /backups/$TODAY/
```

To restore from one:

```bash
sudo hydraidectl restore -i <instance> --source /backups/<date>/<instance>.tar.gz
```

### D. Migrating V1 storage to V2

```bash
# 1. Dry-run to see what would happen
sudo hydraidectl migrate v1-to-v2 -i <instance> --dry-run

# 2. Take a backup first
sudo hydraidectl backup -i <instance> --target /backups/pre-v2-migration.tar.gz --compress

# 3. Run the full migration
sudo hydraidectl migrate v1-to-v2 -i <instance> --full

# 4. Once verified, remove the old V1 files
sudo hydraidectl cleanup -i <instance>
```

For a V2 instance that predates the embedded-swamp-name header:

```bash
sudo hydraidectl migrate v2-migrate-format -i <instance>
```

This is idempotent and safe to re-run.

### E. Investigating a misbehaving swamp

```bash
# 1. High-level health
hydraidectl health -i <instance>

# 2. Per-swamp statistics
hydraidectl stats -i <instance>

# 3. Live metrics (requires telemetry enabled)
hydraidectl telemetry --enable -i <instance>
hydraidectl observe

# 4. Browse the namespace interactively
hydraidectl explore -i <instance>

# 5. Low-level look at a single .hyd file (rarely needed)
hydraidectl inspect /path/to/<swamp_hash>.hyd

# 6. If fragmentation is high, force compaction
sudo hydraidectl compact -i <instance>
```

### F. Decommissioning an instance

```bash
# Keep the data (re-attach later by re-creating the instance with the same data dir)
sudo hydraidectl destroy -i <instance>

# Wipe everything
sudo hydraidectl destroy -i <instance> --purge
```

---

## 5. Sudo and SSH considerations

Several `hydraidectl` commands manage systemd units (`init`, `edit`, `start`, `stop`, `restart`, `upgrade`, `backup`, `restore`, `destroy`, `compact`, `migrate`, `cleanup`) and require `sudo`. Some of these prompt for the password interactively.

This means **interactive `sudo` does not work over non-interactive SSH**. Two options when operating remotely:

1. SSH into the host first, then run the command with `sudo` directly.
2. Configure passwordless `sudo` for the specific commands the operator needs (NOPASSWD in `/etc/sudoers.d/`), with appropriate restrictions.

Read-only commands (`list`, `health`, `observe`, `telemetry`, `stats`, `size`, `inspect`, `explore`, `version`) typically do not require `sudo` and can run cleanly over non-interactive SSH.

---

## 6. Troubleshooting

| Symptom | Likely cause | Action |
|---|---|---|
| `upgrade` hangs at "Removing service" | A client still holds an open TCP connection | Find the client (`ss -tnp`), stop it, retry the upgrade. |
| Instance does not start after `upgrade` | New binary mismatch, config drift, port conflict | `journalctl -u hydraserver-<instance> -n 100`. Try `restart`. If still broken, `restore` from the pre-upgrade backup. |
| Backup file is 0 bytes | Insufficient disk space, stop phase failed | Check `df -h` on the backup target. Re-run `backup`. |
| `health` returns exit code `1` | Instance is unhealthy (port not open, internal error, recent crash) | `journalctl -u hydraserver-<instance>`. Check disk space and recent ops. `restart`. |
| `list` shows "update available" but the instance was just upgraded | Stale cache or version detection lag | `hydraidectl version` to refresh; re-run `list`. |
| Compaction did not reduce file size | Fragmentation was below threshold or the file was already compact | Check `stats` for fragmentation percentage. Compaction is a no-op when fragmentation is low. |

For deeper diagnostics, the `journalctl` log of the instance unit is the first place to look.

---

## 7. What lives where

| What | Where |
|---|---|
| CLI source | [`app/hydraidectl/cmd/`](../../../app/hydraidectl/cmd/) |
| Full per-command flag reference | [`docs/hydraidectl/README.md`](../../../docs/hydraidectl/README.md) (index → 4 category pages) |
| Installation guide | [`docs/hydraidectl/hydraidectl-install.md`](../../../docs/hydraidectl/hydraidectl-install.md) |
| Migration guide | [`docs/hydraidectl/hydraidectl-migration.md`](../../../docs/hydraidectl/hydraidectl-migration.md) |
| Filesystem and hardware guidance | [`docs/install/README.md`](../../../docs/install/README.md) |
| Storage engine internals | [`docs/features/v2-storage-engine.md`](../../../docs/features/v2-storage-engine.md) |
| Go SDK modelling, filters, patches | [`hydraidego` skill](../hydraidego/SKILL.md) |
