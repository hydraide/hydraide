# Instance lifecycle

Commands that create, configure, run, and dismantle a HydrAIDE instance on
this host. All require `sudo` because they manage `systemd` units.

> Returning to the index? See [`README.md`](README.md).

---

## `init` – End-to-end install

Installs a new instance from scratch in a single command: gathers the
configuration (2 prompts in the default flow), generates TLS certificates,
downloads the latest server binary, registers a `systemd` unit, starts it,
waits until the instance reports healthy, and prints the "Client connection
kit" with paths and a Go SDK config snippet.

### Synopsis

```bash
sudo hydraidectl init [-i <instance>] [--advanced]
```

### Flags

| Flag | Default | Effect |
|---|---|---|
| `--instance` / `-i` | _(prompted)_ | Instance name. When provided, skips the name prompt. |
| `--advanced` | `false` | Show every configuration prompt instead of using defaults. Use to override Graylog, gRPC message size, log level, custom TLS SANs, etc. |

### Behavior

1. Prompts for instance name (skipped when `-i` is given) and base path
   (defaults to `/mnt/hydraide/<instance>`).
2. Picks the lowest free `(grpc, health=grpc+1)` port pair starting at
   `4900/4901`, bumping by 10 if a port is bound or already claimed by
   another registered instance.
3. Generates a localhost-only TLS certificate set (`CN=hydraide`,
   SAN=`localhost`+`127.0.0.1`). With `--advanced`, prompts for CN and
   custom SANs.
4. Downloads the latest server binary into the base path.
5. Writes `.env` and `settings.json` (engine V2).
6. Generates and enables a `systemd` unit (`hydraserver-<instance>`).
7. Starts the service.
8. Polls the health endpoint for up to 30s; aborts with a clear error if
   the instance does not become healthy.
9. Prints the "Client connection kit": paths to `ca.crt`, `client.crt`,
   `client.key`, the connect address, the `ServerName` (which equals the
   cert CN), and a ready-to-paste Go SDK config snippet.

### Defaults applied without `--advanced`

| Setting | Default |
|---|---|
| TLS | localhost-only (`CN=hydraide`, SAN=`localhost`+`127.0.0.1`) |
| gRPC port | lowest free pair from `4900/4901` |
| Health port | `gRPC + 1` (always derived) |
| Storage engine | V2 (single-file, append-only) |
| Log level | `info` (slog) |
| Graylog | off |
| Resource logging | off |
| gRPC max msg size | 10 MB |
| gRPC error logging | on |

### Examples

```bash
# Minimum: 2 prompts (name + base path), defaults everywhere else.
sudo hydraidectl init -i prod

# Full advanced wizard: every tunable exposed.
sudo hydraidectl init -i prod --advanced
```

### Gotchas

- **Requires systemd.** If `/run/systemd/system` is missing, `init` exits
  with a clear error. Container deployments should use the prebuilt Docker
  image instead.
- The instance name must not already exist on this host (`hydraidectl list`
  shows what's there).

---

## `edit` – Reconfigure an instance

Section-based editor for an existing instance. Opens a menu where each
entry shows the current value and lets you change it. After saving the
changes, the instance is restarted and a health check is performed.

### Synopsis

```bash
sudo hydraidectl edit -i <instance>
```

### Flags

| Flag | Default | Effect |
|---|---|---|
| `--instance` / `-i` **(required)** | — | Instance to edit. |

### Menu

```
[1] Ports          gRPC=…, health=…
[2] Logging        level, graylog, resource logging
[3] gRPC           max message size, error logging
[4] TLS SANs       CN, DNS SANs, IP SANs
[5] systemd unit   [unit OK / MISSING — reinstall recommended / reinstalled]
[s] Save and restart
[q] Quit without saving
```

### Behavior

1. Loads the current `.env`, parses the live `server.crt` for SAN values,
   checks the systemd unit existence.
2. Lets you edit one or more sections. Within each section, every prompt
   shows the **current** value; pressing Enter keeps it, anything else
   replaces it. Validation runs the same as during `init` (port availability,
   log level whitelist, message-size parsing).
3. **TLS SAN edit** regenerates the certificate. The editor warns that
   every client must replace `ca.crt` / `client.crt` / `client.key`, and
   requires typing `rotate` to confirm.
4. **systemd unit reinstall** is offered when the unit file is missing or
   corrupt. Reinstall touches only the unit and `.env`; it never changes
   the binary or the data directory.
5. On `[s] Save and restart`: writes `.env`, regenerates certs if needed,
   asks the standard "have you stopped your clients?" confirmation, then
   triggers a proper stop+start via the instance controller and polls
   health for up to 30s.

### Examples

```bash
# Change the gRPC port.
sudo hydraidectl edit -i prod
# → pick [1], enter new port, [s], confirm clients-stopped.

# Add a new IP SAN to the TLS certificate.
sudo hydraidectl edit -i prod
# → pick [4], confirm rotation, type the full SAN list, type 'rotate', [s].

# Repair a missing systemd unit (e.g. after manual deletion).
sudo hydraidectl edit -i prod
# → pick [5] (only visible when unit is missing), [s].
```

### Gotchas

- **`edit` does not change the binary version.** Use [`upgrade`](upgrades.md#upgrade--in-place-binary-upgrade) for that.
- **Stop your clients before saving.** When you choose `[s]`, the service
  is stopped and re-started. Open TCP connections will hang the graceful
  shutdown. Answer `n` to the clients-stopped prompt and the configuration
  is **still saved** to disk; the running instance keeps the old config
  until you stop the clients and run
  `sudo hydraidectl restart -i <instance>` manually.
- **SAN editing replaces the SAN list, not amends it.** The current SANs
  are pre-filled in the prompt (read from the live `server.crt`); type the
  full final list, not just the additions.

---

## `start` – Start an instance

Starts a registered instance that is currently stopped.

### Synopsis

```bash
sudo hydraidectl start -i <instance>
```

### Flags

| Flag | Default | Effect |
|---|---|---|
| `--instance` / `-i` **(required)** | — | Instance to start. |
| `--cmd-timeout` | `20s` | Overall command timeout. |
| `--json` / `-j` | `false` | Structured JSON output. |
| `--output` / `-o` | _(empty)_ | Output format (`json` is equivalent to `--json`). |

### Behavior

1. Validates the instance exists.
2. No-op if the service is already running.
3. Otherwise issues `systemctl start` and waits for the unit to report
   active.

### Examples

```bash
# Plain output.
sudo hydraidectl start -i prod

# JSON output for scripts.
sudo hydraidectl start -i prod --json
```

JSON success example:

```json
{
  "instance": "prod",
  "action": "start",
  "status": "success",
  "message": "instance started successfully",
  "timestamp": "2025-08-10T14:30:00Z"
}
```

JSON errors use the same shape with `"status": "error"` and an explanatory
`"message"`.

---

## `stop` – Stop a running instance

Issues a graceful shutdown. The CLI **never** force-kills the process —
HydrAIDE may still be flushing in-memory swamps to disk.

### Synopsis

```bash
sudo hydraidectl stop -i <instance> [--yes]
```

### Flags

| Flag | Default | Effect |
|---|---|---|
| `--instance` / `-i` **(required)** | — | Instance to stop. |
| `--cmd-timeout` | `20s` | Overall command timeout. Must be ≥ graceful-timeout. |
| `--graceful-timeout` | `60s` | Time to wait for HydrAIDE's flush-to-disk before declaring the stop a failure. **Never set below 60s** — risk of data loss. |
| `--yes` / `-y` | `false` | Skip the interactive clients-stopped confirmation. Use in scripts. |
| `--json` / `-j` | `false` | Structured JSON output. |
| `--output` / `-o` | _(empty)_ | Output format (`json` is equivalent to `--json`). |

### Behavior

1. Asks the standard "have you stopped your clients?" confirmation
   (skippable with `--yes`).
2. Validates the instance exists.
3. Issues `systemctl stop` and polls until the unit becomes inactive or
   the graceful-timeout elapses.

### Examples

```bash
# Interactive — prompts for clients-stopped confirmation.
sudo hydraidectl stop -i prod

# Scripted — skip the prompt.
sudo hydraidectl stop -i prod --yes
```

### Gotchas

- **Stop your clients first.** HydrAIDE protects in-flight data and refuses
  to shut down gracefully while clients hold open TCP connections — the
  stop phase will hang. The prompt is there to remind you. `--yes` only
  skips the prompt; it does not kill clients for you.

---

## `restart` – Restart an instance

`stop` followed by `start`, with the same clients-stopped concern.

### Synopsis

```bash
sudo hydraidectl restart -i <instance> [--yes]
```

### Flags

| Flag | Default | Effect |
|---|---|---|
| `--instance` / `-i` **(required)** | — | Instance to restart. |
| `--cmd-timeout` | `30s` | Overall command timeout. |
| `--graceful-timeout` | `60s` | Time to wait for graceful shutdown. **Never set below 60s.** |
| `--yes` / `-y` | `false` | Skip the clients-stopped confirmation. |
| `--json` / `-j` | `false` | Structured JSON output. |
| `--output` / `-o` | _(empty)_ | Output format. |

### Behavior

1. Asks clients-stopped confirmation (skippable with `--yes`).
2. Stops the instance gracefully.
3. Starts it again.
4. Returns success once the unit reports active.

### Gotchas

Same as `stop` — open client connections will hang the stop phase.

---

## `destroy` – Remove an instance

Stops the instance, removes the `systemd` unit, and unregisters the
instance from `metadata.json`. With `--purge`, also deletes the entire
base path.

### Synopsis

```bash
sudo hydraidectl destroy -i <instance> [--purge]
```

### Flags

| Flag | Default | Effect |
|---|---|---|
| `--instance` / `-i` **(required)** | — | Instance to remove. |
| `--purge` | `false` | Also delete the data directory. **Irreversible.** Requires re-typing the instance name as confirmation. |

### Behavior

1. Stops the instance gracefully (no-op if already stopped).
2. Removes the `systemd` unit and reloads the daemon.
3. Removes the instance entry from `~/.hydraide/metadata.json`.
4. With `--purge`: prompts for the instance name as confirmation, then
   deletes the entire base path (data, certificates, settings, binary,
   `.env`).

### Examples

```bash
# Keep the data on disk — you can re-attach by re-running init with the same base path.
sudo hydraidectl destroy -i dev-local

# Wipe everything, including data.
sudo hydraidectl destroy -i dev-local --purge
# → prompts: "To confirm, type the full instance name ('dev-local'):"
```

### Gotchas

- **`--purge` is irreversible.** No backup is taken automatically. Run
  [`backup`](data.md#backup--snapshot-instance-data) first if you might
  need the data.
- Without `--purge`, the data directory survives. You can recreate the
  instance later by pointing `init` at the same base path (it will warn
  about existing folders and ask for explicit confirmation before
  proceeding).

---

## Exit codes (lifecycle commands)

| Code | Meaning |
|---|---|
| `0` | Success. |
| `1` | Generic failure (instance not found, validation error, command failed). |
| `3` | Privilege / pre-flight failure (missing `sudo`, unsupported OS, missing systemd). |

JSON output mirrors the same outcome via the `"status"` field
(`"success"` / `"error"`); the exit code is still set so shell scripts
can branch on `$?` without parsing JSON.
