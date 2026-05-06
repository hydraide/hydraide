# hydraidectl — User Manual

`hydraidectl` is the management CLI for HydrAIDE server instances on a host.
Use it to install instances, change their configuration, monitor their
health, upgrade between versions, run migrations, and back up data.

> **First time here?** The two-command quickstart is at
> [`docs/install/quickstart.md`](../install/quickstart.md). Come back to
> this manual once you have a running instance and want to dig into a
> specific command.

> **Installing the CLI itself** (separate from installing an instance):
> see [`hydraidectl-install.md`](hydraidectl-install.md).

---

## Command map

### Instance lifecycle — [`lifecycle.md`](lifecycle.md)

| Command | Purpose |
|---|---|
| [`init`](lifecycle.md#init--end-to-end-install) | End-to-end install of a new instance: 2 prompts, then cert + binary + systemd unit + start + health-wait. |
| [`edit`](lifecycle.md#edit--reconfigure-an-instance) | Menu-based editor for a running instance: ports, logging, gRPC, TLS SANs, systemd unit. |
| [`start`](lifecycle.md#start--start-an-instance) | Start a stopped instance. |
| [`stop`](lifecycle.md#stop--stop-a-running-instance) | Graceful stop. Asks for clients-stopped confirmation; `--yes` to bypass. |
| [`restart`](lifecycle.md#restart--restart-an-instance) | Stop + start. Same clients-stopped confirmation as `stop`. |
| [`destroy`](lifecycle.md#destroy--remove-an-instance) | Remove instance + service. `--purge` also wipes the data directory. **Irreversible.** |

### Monitoring & status — [`monitoring.md`](monitoring.md)

| Command | Purpose |
|---|---|
| [`list`](monitoring.md#list--show-all-instances) | All registered instances with version, status, health, update-available flags. |
| [`health`](monitoring.md#health--instance-health-probe) | Health probe. Exit codes: `0` healthy, `1` unhealthy, `3` unexpected. |
| [`observe`](monitoring.md#observe--real-time-rpc-dashboard) | Live RPC metrics TUI dashboard (requires `telemetry` enabled). |
| [`explore`](monitoring.md#explore--swamp-hierarchy-browser) | Sanctuary / Realm / Swamp interactive browser. |
| [`telemetry`](monitoring.md#telemetry--enabledisable-telemetry-collection) | Enable / disable per-RPC telemetry used by `observe`. |
| [`version`](monitoring.md#version--cli-version-and-update-check) | CLI version + update-available check. |

### Upgrades & migration — [`upgrades.md`](upgrades.md)

| Command | Purpose |
|---|---|
| [`upgrade`](upgrades.md#upgrade--in-place-binary-upgrade) | Stop + download + replace + auto-restart + health-wait. `--no-start` to defer start; `--force` to reinstall same version. |
| [`migrate v1-to-v2`](upgrades.md#migrate-v1-to-v2--multi-file-to-single-file-format) | Migrate V1 (multi-file) data to V2 (single-file `.hyd`) format. |
| [`migrate v2-migrate-format`](upgrades.md#migrate-v2-migrate-format--upgrade-hyd-headers) | Upgrade `.hyd` headers in-place to the optimized format that embeds the swamp name. Idempotent. |
| [`engine`](upgrades.md#engine--view-or-change-storage-engine-version) | View or change the storage engine version (V1/V2) for an instance. |

### Data management — [`data.md`](data.md)

| Command | Purpose |
|---|---|
| [`backup`](data.md#backup--snapshot-instance-data) | Tar snapshot of instance data. Default: stop → archive → restart. |
| [`restore`](data.md#restore--restore-from-backup) | Restore instance data from a backup archive. |
| [`compact`](data.md#compact--reclaim-space-from-fragmented-swamps) | Force compaction across swamps to reclaim space from dead entries. |
| [`cleanup`](data.md#cleanup--remove-orphaned-storage-files) | Remove obsolete files left over from migrations (V1 chunk folders, pre-format `.hyd` originals). |
| [`size`](data.md#size--show-instance-disk-usage) | Total on-disk size for the instance's data directory. |
| [`stats`](data.md#stats--per-swamp-statistics-and-health) | Per-swamp statistics + health report. |
| [`inspect`](data.md#inspect--low-level-hyd-file-debugger) | Low-level inspection of a single `.hyd` file (header, blocks, entries). |

---

## Conventions used in this manual

Every command page follows the same template:

- **Tagline** — one sentence: what the command does, when to use it.
- **Synopsis** — the `hydraidectl <cmd> ...` invocation with the most common flags.
- **Flags** — table of every flag, with required-marker, default, and effect.
- **Behavior** — numbered steps describing what the command does internally, in order.
- **Examples** — at least one realistic invocation per common scenario.
- **Gotchas** — only present when the command has a footgun worth flagging (e.g. clients must be stopped before `upgrade`).

---

## Privilege requirements

Most lifecycle commands manage `systemd` units and require `sudo`:

`init`, `edit`, `start`, `stop`, `restart`, `destroy`, `upgrade`, `backup`,
`restore`, `compact`, `migrate v1-to-v2`, `migrate v2-migrate-format`,
`cleanup`.

Read-only commands run as a regular user:

`list`, `health`, `observe`, `telemetry`, `stats`, `size`, `inspect`,
`explore`, `version`.

> Interactive `sudo` does not work over non-interactive SSH. To operate
> remotely, either ssh in and run `sudo` directly, or configure passwordless
> `sudo` for the specific commands the operator needs (`NOPASSWD` in
> `/etc/sudoers.d/`) with appropriate restrictions.

---

## Where to look next

| If you want to … | Go to |
|---|---|
| Install your first instance | [Quickstart](../install/quickstart.md) |
| Install the CLI itself | [hydraidectl-install.md](hydraidectl-install.md) |
| Migrate V1 → V2 storage | [Migration guide](hydraidectl-migration.md) |
| Pick a filesystem / hardware | [Install README](../install/README.md) |
| Build apps against an instance | [hydraide skill](../../.claude/skills/hydraide/SKILL.md) and the [Go SDK](../../sdk/go/hydraidego/) |
