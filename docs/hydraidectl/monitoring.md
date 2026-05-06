# Monitoring & status

Read-only commands for inspecting state, browsing data, and watching live
RPC traffic. None of these require `sudo`.

> Returning to the index? See [`README.md`](README.md).

---

## `list` – Show all instances

Lists every registered HydrAIDE instance on this host with version, status,
health, and update-available flags. Instances appear in ascending
alphabetical order.

### Synopsis

```bash
hydraidectl list [--quiet | --json | --no-health]
```

### Flags

| Flag | Default | Effect |
|---|---|---|
| `--quiet` | `false` | Print only instance names, one per line. |
| `--json` | `false` | Machine-readable JSON output (full record per instance). |
| `--no-health` | `false` | Skip the health probe. Faster on hosts with many instances or unreachable services. |

### Output columns

| Column | Meaning |
|---|---|
| `Name` | Instance name. |
| `Server Port` | gRPC listening port. |
| `Server Version` | Currently installed binary version. |
| `Update Available` | `yes` (with ⚠️) when a newer release exists, `no` otherwise. |
| `Service Status` | `active` / `inactive` (or "no service" when the unit was never installed). |
| `Health` | `healthy` / `unhealthy` / `unknown` (omitted with `--no-health`). |
| `Base Path` | Filesystem path of the instance directory. |

### Examples

```bash
# Plain table.
hydraidectl list

# Just the names — handy for scripts.
hydraidectl list --quiet

# Full JSON for automation.
hydraidectl list --json

# Fast listing on hosts with many instances.
hydraidectl list --no-health
```

JSON record shape:

```json
{
  "name": "prod",
  "server_port": "4900",
  "server_version": "v3.10.2",
  "update_available": "no",
  "status": "active",
  "health": "healthy",
  "base_path": "/mnt/hydraide/prod"
}
```

### Notes

- The health probe uses a 2s timeout. Unreachable endpoints show
  `health=unknown` rather than blocking the listing.
- The "latest available version" is fetched once from GitHub releases.
  Failures are silent; the column simply shows the local version without
  the update flag.

---

## `health` – Instance health probe

Runs an HTTP health check against a single instance. Designed for shell
scripts: returns a clean exit code, no required parsing.

### Synopsis

```bash
hydraidectl health -i <instance>
```

### Flags

| Flag | Default | Effect |
|---|---|---|
| `--instance` / `-i` **(required)** | — | Instance to probe. |

### Exit codes

| Code | Meaning |
|---|---|
| `0` | Healthy (HTTP 200 within 2s). |
| `1` | Unhealthy (non-200, timeout, connection refused). |
| `3` | Instance not found, or its `.env` is missing. |

### Behavior

1. Reads the instance's `.env` to find the configured health port.
2. Issues `GET http://localhost:<health-port>/health` with a 2s timeout.
3. Maps the result to an exit code (above) and prints `healthy` /
   `unhealthy` / a brief diagnostic line.

### Examples

```bash
# Quick check.
hydraidectl health -i prod

# In a shell script.
if hydraidectl health -i prod >/dev/null; then
    echo "alive"
else
    echo "down or missing"
fi
```

---

## `observe` – Real-time RPC dashboard

Live TUI dashboard that streams every gRPC call hitting an instance —
method, swamp, latency, status. Indispensable when debugging client
errors, slow queries, or unexpected traffic patterns.

`observe` requires telemetry to be enabled on the instance. If it isn't,
the command offers to enable it and restart the instance for you.

### Synopsis

```bash
hydraidectl observe -i <instance> [flags]
```

### Flags

| Flag | Default | Effect |
|---|---|---|
| `--instance` / `-i` **(required)** | — | Instance to observe. |
| `--errors-only` | `false` | Only show error events. |
| `--filter` | _(empty)_ | Filter by swamp pattern (e.g. `auth/*`). |
| `--simple` | `false` | Plain text streaming output instead of TUI. Useful for piping to `tee` or grep. |
| `--stats` | `false` | Print one statistics snapshot and exit (no streaming). |

### TUI shortcuts

| Key | Action |
|---|---|
| `1` / `2` / `3` | Switch to Live / Errors / Stats view. |
| `P` | Pause / resume the stream. |
| `C` | Clear the current event buffer. |
| `E` | Toggle the errors-only filter. |
| `↑` `↓` or `j` `k` | Navigate events. |
| `Enter` | Open the detail view for the selected event. |
| `Esc` | Close the detail view. |
| `?` or `H` | Show in-app help. |
| `Q` | Quit. |

### Examples

```bash
# Full TUI.
hydraidectl observe -i prod

# Plain text stream — pipe into your favourite tool.
hydraidectl observe -i prod --simple | tee /tmp/rpc-trace.log

# Only errors, only auth traffic.
hydraidectl observe -i prod --errors-only --filter "auth/*"

# One-shot stats snapshot.
hydraidectl observe -i prod --stats
```

### Simple-mode output

```
14:23:01.234 | Get      | user/sessions/abc123               |    2ms | OK
14:23:01.456 | Set      | cache/products/item-x              |    1ms | OK
14:23:01.789 | Get      | auth/tokens/xyz                    |    5ms | ERR FailedPrecondition
         +-- decompression failed: invalid data format
14:23:02.012 | Delete   | temp/uploads/file                  |    0ms | OK
```

### Notes

- Telemetry is **memory-only** — events live in a 30-minute ring buffer
  on the server, never persisted to disk.
- Overhead is well under 1%, but production hygiene is to leave telemetry
  off and turn it on only when investigating an issue.

---

## `explore` – Swamp hierarchy browser

Interactive TUI for browsing the **Sanctuary / Realm / Swamp** hierarchy
of an instance's stored data. Reads `.hyd` files directly from disk and
builds an in-memory index, so it works **without a running server**.

When pointed at a running instance via `--instance`, the explorer can
also delete Sanctuaries, Realms, and individual Swamps through the
server's `DestroyBulk` API. In `--data-path` mode (no server contact) it
is read-only.

### Synopsis

```bash
hydraidectl explore [-i <instance> | -d <data-path>]
```

### Flags

| Flag | Default | Effect |
|---|---|---|
| `--instance` / `-i` | — | Instance name. Reads its data path from buildmetadata; enables deletion. |
| `--data-path` / `-d` | — | Direct path to a data directory. Read-only — no deletion. |

Pass exactly one of `--instance` or `--data-path`.

### TUI shortcuts

| Key | Action |
|---|---|
| `j` `k` or `↑` `↓` | Navigate the current list. |
| `Enter` or `→` | Drill down into the selected item. |
| `Esc` or `←` | Back one level. |
| `/` | Filter the current list. |
| `PgUp` / `PgDn` | Page through long lists. |
| `Home` / `End` | Jump to the first / last item. |
| `r` | Rescan the data directory. |
| `d` | Delete the selected item (instance mode only). |
| `q` | Quit. |

### Hierarchy levels

1. **Sanctuaries** — top-level grouping; shows realm count, swamp count,
   total size.
2. **Realms** — second level; shows swamp count, total size.
3. **Swamps** — individual swamp files; shows file size, entry count,
   format version.
4. **Detail** — full metadata for one swamp: file path, modification time,
   block count, island ID.

### Deletion safety

When you press `d` in instance mode, the explorer asks for **two
independent confirmations** — each requires typing a fresh randomly
generated 4-character code — and shows a summary of what will be deleted
before each. The actual delete uses the server's `DestroyBulk` streaming
RPC with parallel workers and shows a progress bar. Both in-memory state
and on-disk files are cleaned up, including empty parent directories.

### Examples

```bash
# Browse a running instance, with deletion enabled.
hydraidectl explore -i prod

# Read-only browse of a backup or copy of the data directory.
hydraidectl explore -d /var/hydraide/data
```

### Notes

- The disk scan reads only ~100 bytes per `.hyd` file (header + embedded
  swamp name) on the optimized V2 format, so even datasets with millions
  of swamps load quickly.
- For older `.hyd` files without the embedded name, the explorer falls
  back to decompressing the first block — slower, but transparent.

---

## `telemetry` – Enable/disable telemetry collection

Controls per-RPC telemetry on an instance. `observe` requires telemetry to
be enabled; everything else works without it.

### Synopsis

```bash
hydraidectl telemetry -i <instance> [--enable | --disable] [--json]
```

### Flags

| Flag | Default | Effect |
|---|---|---|
| `--instance` / `-i` **(required)** | — | Target instance. |
| `--enable` | — | Turn telemetry on. Prompts for a restart so the change takes effect. |
| `--disable` | — | Turn telemetry off. Same restart prompt. |
| `--json` / `-j` | `false` | JSON output. |

When neither `--enable` nor `--disable` is passed, the command prints the
current state.

### What telemetry collects

- gRPC method, swamp, duration, status code for every call.
- Error categorization (e.g. `FailedPrecondition`, `Internal`).
- Connection statistics per client.
- Latency metrics for performance analysis.

All of this is held in a **30-minute ring buffer in memory**. Nothing is
persisted to disk.

### Examples

```bash
# Show current state.
hydraidectl telemetry -i prod

# Enable + restart now.
hydraidectl telemetry -i prod --enable
# Y at the restart prompt.

# JSON output for scripts.
hydraidectl telemetry -i prod --json
# {"instance": "prod", "telemetry_enabled": true}
```

### Notes

- Recommended posture: leave telemetry **off** in production and turn it
  on only when diagnosing a specific issue.
- Overhead is < 1%, but every byte of memory matters at scale.

---

## `version` – CLI version and update check

Prints the local `hydraidectl` build information and, optionally, the
version recorded for a specific instance. Never contacts a running
service — use [`list`](#list--show-all-instances) for fleet status.

### Synopsis

```bash
hydraidectl version [--instance <name>] [--json] [--no-remote] [--pre] [--timeout <sec>]
```

### Flags

| Flag | Default | Effect |
|---|---|---|
| `--instance` / `-i` | — | Append the recorded version of the named instance to the output. |
| `--json` / `-j` | `false` | Structured JSON: `cli`, optional `instance`, optional `update`. |
| `--no-remote` | `false` | Skip the GitHub release check (useful on air-gapped hosts). |
| `--pre` | `false` | Include pre-release builds when comparing. |
| `--timeout` | `3` | Network timeout in seconds for the release check. |

### Behavior

1. Prints CLI version, commit, build date, platform.
2. With `--instance`, reads `~/.hydraide/metadata.json` for that instance
   and appends its recorded version.
3. With remote check enabled, queries GitHub for the latest release and
   appends an "update available" line if a newer build exists.

### Examples

```bash
# CLI build only.
hydraidectl version

# CLI + the version recorded for one instance.
hydraidectl version --instance prod

# Air-gapped: skip the release check.
hydraidectl version --no-remote --json
```

### Update notice format

```
Update: v2.6.0 available → run:
  curl -sSfL https://raw.githubusercontent.com/hydraide/hydraide/main/scripts/install-hydraidectl.sh | bash
```

The CLI does not self-update; the install script handles it.
