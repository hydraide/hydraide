# HydrAIDECtl CLI Documentation

## Overview

The `hydraidectl` CLI allows for easy installation, management, and lifecycle control of HydrAIDE server instances.

> **If you haven't installed hydraidectl yet**, you can find the installation guide here: **[HydrAIDECtl Install Guide](hydraidectl-install.md)**

Although `hydraidectl` is stable and production-tested, new features are under development, including:

* `non-interactive init & offline install` (for edge or air-gapped systems)

\* ***If you need a command that is not listed among the current or upcoming features, please create a new issue so it can be considered for implementation***

---

## Available Commands

* [`init` – Initialize a new HydrAIDE instance interactively](#init--interactive-setup-wizard) 
* [`service` – Create and manage a persistent system service](#service--set-up-persistent-system-service)
* [`start` – Start a specific HydrAIDE instance](#start--start-an-instance)
* [`stop` – Gracefully stop an instance](#stop--stop-a-running-instance)
* [`restart` – Restart a running or stopped instance](#restart--restart-instance)
* [`list` – Show all registered HydrAIDE instances on the host](#list--show-all-instances)
* [`health`– Display health of an instance](#health--instance-health)
* [`observe` – Real-time monitoring dashboard for debugging](#observe--real-time-monitoring-dashboard)
* [`telemetry` – Enable/disable telemetry collection](#telemetry--enabledisable-telemetry-collection)
* [`destroy` – Fully delete an instance, optionally including all its data](#restart--restart-instance)
* [`cert` – Generate TLS Certificates (without modifying instances)](#cert--generate-tls-certificates-without-modifying-instances)
* [`upgrade` – Upgrade an Instance In‑Place](#upgrade--upgrade-an-instance-inplace-allinone)
* [`migrate v1-to-v2` – Migrate V1 storage to V2 format](#migrate-v1-to-v2--migrate-v1-storage-to-v2-format)
* [`engine` – View or change storage engine version](#engine--view-or-change-storage-engine-version)
* [`backup` – Create instance backup](#backup--create-instance-backup)
* [`restore` – Restore instance from backup](#restore--restore-instance-from-backup)
* [`size` – Show instance data size](#size--show-instance-data-size)
* [`explore` – Interactive swamp hierarchy explorer](#explore--interactive-swamp-hierarchy-explorer)
* [`stats` – Show detailed swamp statistics and health report](#stats--show-detailed-swamp-statistics-and-health-report)
* [`compact` – Compact swamp files](#compact--compact-swamp-files)
* [`migrate v2-migrate-format` – Upgrade .hyd file headers for faster scanning](#migrate-v2-migrate-format--upgrade-hyd-file-headers-for-faster-scanning)
* [`cleanup` – Remove old storage files](#cleanup--remove-old-storage-files)
* [`version` – Display CLI and optional instance metadata](#version--display-cli-and-optional-instance-metadata)

---

## `init` – Interactive Setup Wizard

Use this command to create a new HydrAIDE instance.

You will be prompted for:

* Unique instance name (e.g. `prod`, `dev-local`)
* TLS settings: CN, IPs, domains
* Listening port
* Data storage path
* Logging level and options
* Optional Graylog integration

The `init` command generates the full instance configuration and prepares all required TLS certificates and keys.
⚠️ Note: `init` only sets up the instance – it does not start it. To run the instance as a background service, follow with the `service` command.

This command does **not** require `sudo`. It runs under the current user context and creates the config and certificate files inside the chosen instance directory.

### Certificate generation

At the end of initialization, a `certificate/` folder is created inside the instance directory.
It contains the following files:

| File             | Purpose                                                                  | Who uses it                                          |
| ---------------- | ------------------------------------------------------------------------ | ---------------------------------------------------- |
| **`ca.crt`**     | Root CA certificate. Used to verify both server and client certificates. | Copy to every client, and keep a copy on the server. |
| **`ca.key`**     | Root CA private key. Used only to sign new server/client certificates.   | **Must remain on the server**. Never share.          |
| **`server.crt`** | TLS certificate for this HydrAIDE server.                                | Used only by the server.                             |
| **`server.key`** | Private key for the server certificate.                                  | **Must remain on the server**.                       |
| **`client.crt`** | Client certificate signed by the CA.                                     | Copy to each client that will connect.               |
| **`client.key`** | Private key for the client certificate.                                  | Copy to each client together with `client.crt`.      |

### What to copy to clients

When setting up a client application, you must copy:

* `ca.crt` → so the client can validate the server’s identity.
* `client.crt` + `client.key` → so the client can authenticate itself to the server.

These three files go into the client’s configuration/runtime path.

### What stays on the server

* `ca.key` → keep strictly private (used only for signing).
* `server.crt` + `server.key` → used by the HydrAIDE server itself.
* The `certificate/` directory should remain intact in the instance folder.

**Example usage:**

```bash
hydraidectl init
```

After initialization:

* Server runs with → `server.crt`, `server.key`, and `ca.crt`
* Client apps must be configured with → `client.crt`, `client.key`, and `ca.crt`

---

## `service` – Set Up Persistent System Service

Registers a systemd service (`hydraserver-<instance>`) for the chosen instance.

It:

* Validates if metadata exists for the instance (must match the name given in the `init` step)
* Writes a service file into the OS
* Prompts to start and enable the service immediately
* At the end, allows you to choose whether to start the instance right away or defer it for later

This command **requires administrative privileges**

- On Linux: run with `sudo`

**Example:**

```bash
sudo hydraidectl service --instance dev-local
```

Useful for persistent background running across reboots.

---

## `start` – Start an Instance

Starts a registered HydrAIDE instance by name. Requires `sudo`.

**Behavior**
* Validates that the instance exists before attempting to start.
* Starts the system service only if it is not already running.
* Uses command timeout 20s, graceful start/stop 10s.

**Flags**
* `--instance` / `-i` (required) — instance name.  
* `--json` / `-j` — produce structured JSON output.  
* `--output` / `-o` — output format (e.g. `json`).
* `--cmd-timeout` — command execution timeout (e.g., 20s).

**CLI examples**
```bash
# Start an instance (interactive/plain output)
sudo hydraidectl start --instance dev-local

# Start an instance and return JSON
sudo hydraidectl start --instance prod --json
```

**JSON success example (produced by `--json`):**
```json
{
  "instance": "prod",
  "action": "start",
  "status": "success",
  "message": "instance started successfully",
  "timestamp": "2025-08-10T14:30:00Z"
}
```

**Error examples (plain output)**
* If the instance does not exist:
  ```
  ❌ Instance "dev-local" not found.
  Use `hydraidectl list` to see available instances.
  ```
* If the instance is already running:
  ```
  🟡 Instance "dev-local" is already running. No action taken.
  ```

**Notes**
* If the command is run without root privileges it prints guidance and exits.
* Return codes are useful for automation (see "Exit codes" section below).
* json output for errors is same as success json output with error message and status 'error'

---


## `stop` – Stop a Running Instance

Stops a specific instance cleanly. Also requires `sudo`.

**Behavior**
* Validates the instance exists before attempting to stop.
* Performs a **graceful shutdown** and may take longer depending on in-memory state (for example, flushing open Swamps to disk).
* Intentionally **never forcefully terminates** the service (no `kill -9`) to avoid data corruption.
* Uses command timeout 20s, graceful stop timeout 10s (prints timeout error post timeout).

**Flags**
* `--instance` / `-i` (required) — instance name.  
* `--json` / `-j` — produce structured JSON output.  
* `--output` / `-o` — output format.
* `--cmd-timeout` — command execution timeout (e.g., 20s). This value must never be shorter than the graceful timeout.
* `--graceful-timeout` — perform a graceful shutdown (default 60s if not specified). It is important to always allow enough time for HydrAIDE to shut down so it can flush the last data from memory to disk. If this timeout is too short, it may lead to data loss. It should never be set below 60 seconds.

**CLI examples**
```bash
# Stop an instance (plain output)
sudo hydraidectl stop --instance dev-local

# Stop an instance and return JSON
sudo hydraidectl stop --instance prod --json
```

**JSON success example:**
```json
{
  "instance": "prod",
  "action": "stop",
  "status": "success",
  "message": "instance stopped successfully",
  "timestamp": "2025-08-10T14:31:00Z"
}
```

**Plain output & user guidance**
* While stopping the CLI prints friendly status and a caution:
  ```
  🟡 Shutting down instance "dev-local"...
  ⚠️  HydrAIDE shutdown in progress... Do not power off or kill the service. Data may be flushing to disk.
  ```
* On success:
  ```
  ✅ Instance "dev-local" has been stopped. Status: inactive
  ```

**Notes**
* Consider using `--json` for automation or CI tasks that must parse the result.
* The stopping operation may take longer if there is significant disk flush or compaction work.

---

## `restart` – Restart Instance

Combines `stop` then `start`. Requires `sudo`.

**Behavior**
* Validates instance existence first.
* Calls `StopInstance` then, if the stop phase did not return a fatal error, calls `StartInstance`.
* Uses `instancerunner` with configured timeouts (common defaults: overall restart timeout 30s, graceful start/stop 10s).

**Flags**
* `--instance` / `-i` (required) — instance name.  
* `--json` / `-j` — produce structured JSON output.  
* `--output` / `-o` — output format.
* `--cmd-timeout` — command execution timeout (e.g., 20s). This value must never be shorter than the graceful timeout.
* `--graceful-timeout` — perform a graceful shutdown (default 60s if not specified). It is important to always allow enough time for HydrAIDE to shut down so it can flush the last data from memory to disk. If this timeout is too short, it may lead to data loss. It should never be set below 60 seconds.

**CLI examples**
```bash
# Restart an instance (plain output)
sudo hydraidectl restart --instance dev-local

# Restart an instance and return JSON
sudo hydraidectl restart --instance test --json
```

**JSON success example:**
```json
{
  "instance": "test",
  "action": "restart",
  "status": "success",
  "message": "instance restarted successfully",
  "timestamp": "2025-08-10T14:33:00Z"
}
```

**JSON error example:**
```json
{
  "instance": "test",
  "action": "restart",
  "status": "error",
  "message": "Service 'test' not found.",
  "timestamp": "2025-08-10T14:32:10Z"
}
```

**Plain output progression**
* On plain restart the CLI prints:
  ```
  🔁 Restarting instance "dev-local"...
  ```
* If stop succeeded:
  ```
  ✅ Instance "dev-local" has been stopped. Status: inactive
  ```
* Then after start finishes:
  ```
  ✅ Restart complete. Status: active
  ```

---

## Exit codes (useful for scripts / automation)

Common exit codes returned by the CLI (useful when scripting):
* `0` — success (start / stop / restart succeeded).
* `1` — instance not found (or related not found errors).
* `2` — no-op condition: instance already running (for `start`) or already stopped (for `stop`).
* `3` — generic/fatal error (permission missing, unsupported OS, unexpected failure).

---

## Implementation notes / error types

The CLI maps certain `instancerunner` error types to friendly messages and specific exit codes:
* `ErrServiceNotFound` → prints “Instance not found” and exits with `1`.
* `ErrServiceAlreadyRunning` → prints “already running” and exits with `2`.
* `ErrServiceNotRunning` → prints “already stopped” and exits with `2`.
* `*instancerunner.CmdError` → when a command produced output and an error, the CLI prints the wrapped command error and its output for debugging.
* `*instancerunner.OperationError` → used (in restart start-phase) to signal start-phase errors and printed accordingly.

---

## `list` – Show All Instances

Displays all registered HydrAIDE instances, their metadata, and runtime status.
Instances are shown in **ascending alphabetical order by name**.

**What it shows:**

* Total number of instances found (all initialized with `init`, even if no service has been created yet)
* The **latest HydrAIDE server version** available on GitHub
* For each instance:

    * `Name` — instance name
    * `Server Port` — listening port
    * `Server Version` — currently running HydrAIDE binary version
    * `Update Available` — whether a newer version than the running one exists

        * `no` → instance is already up to date
        * `yes` → instance can be updated (⚠️ shown in table view)
    * `Service Status` — whether a system service exists and if it’s `active` or `inactive`
    * `Health` — health probe status (`healthy`, `unhealthy`, or `unknown`)
    * `Base Path` — filesystem path where the instance keeps binaries, certificates, environment variables, and data

---

**Example output (plain table, including outdated instances):**

```
Scanning for HydrAIDE instances...
Found 5 HydrAIDE instances:
Latest server version: v2.2.1
Name        Server Port   Server Version   Update Available   Service Status   Health     Base Path
----------------------------------------------------------------------------------------------------
alpha       4777          v2.1.0           ⚠️ yes             active           healthy    /home/user/alpha
beta        4855          v2.2.1           no                 active           healthy    /home/user/beta
gamma       4988          v2.1.0           ⚠️ yes             active           healthy    /home/user/gamma
delta       4322          v2.2.1           no                 active           healthy    /home/user/delta
epsilon     4666          v2.0.1           ⚠️ yes             active           healthy    /home/user/epsilon
```

---

**JSON output example:**

```json
[
  {
    "name": "delta",
    "server_port": "4322",
    "server_version": "v2.2.1",
    "update_available": "no",
    "status": "active",
    "health": "healthy",
    "base_path": "/home/user/delta"
  },
  {
    "name": "epsilon",
    "server_port": "4666",
    "server_version": "v2.0.1",
    "update_available": "yes",
    "status": "active",
    "health": "healthy",
    "base_path": "/home/user/epsilon"
  }
]
```

---

**Flags:**

* `--quiet` — print only instance names (no columns, no health/status)
* `--json` — return full machine-readable JSON with all fields
* `--no-health` — skip health probe for faster listing

**Notes:**

* Health probe uses a 2s timeout against the instance’s configured endpoint.
  If missing or unreachable, health will be `unknown`.
* Instances without a created service are still listed (status will indicate missing service).
* If **update is available**, the table clearly marks it with ⚠️ and JSON will return `"update_available": "yes"`.
* This command is useful both for quick overviews and for automation via JSON output.

**Example:**

```bash
sudo hydraidectl list --json
```
```bash
sudo hydraidectl list --no-health
```

---

## `health` – Instance Health

Checks the runtime health of a specific HydrAIDE instance.

**Synopsis:**
```bash
hydraidectl health --instance <name>
```

**Behavior:**
* Reads the instance’s `.env` file (created by `init`) to locate health settings.
* Performs an HTTP GET request to the configured health endpoint.
* Returns:
  * `healthy` if endpoint returns HTTP 200 OK within 2 seconds
  * `unhealthy` if endpoint returns non-200, times out, or connection fails
* Exit codes:
  * `0` → healthy
  * `1` → unhealthy
  * `3` → instance not found or config missing

**Examples:**
```bash
sudo hydraidectl health --instance dev-local
# healthy
```

```bash
sudo hydraidectl health --instance test
# unhealthy
```

---

## `observe` – Real-time Monitoring Dashboard

The `observe` command provides a real-time TUI (Terminal User Interface) dashboard for monitoring all gRPC calls, errors, and client activity on a HydrAIDE server. This is essential for debugging issues like failed logins, data corruption, or performance problems.

**Quick Start:**
```bash
# 1. Enable telemetry (required for observe to work)
hydraidectl telemetry --instance prod --enable

# 2. Start the monitoring dashboard
hydraidectl observe --instance prod
```

**Synopsis:**
```bash
hydraidectl observe --instance <name> [flags]
```

**Requirements:**
* Telemetry must be enabled on the instance
* If telemetry is not enabled, the command will prompt you to enable it and restart the instance

**Flags:**
| Flag | Description |
|------|-------------|
| `--instance, -i` | Instance name (required) |
| `--errors-only` | Only show error events |
| `--filter` | Filter by swamp pattern (e.g., `auth/*`) |
| `--simple` | Simple text output instead of TUI |
| `--stats` | Show statistics only (no streaming) |

**TUI Features:**
* **Live Tab** - Real-time stream of all gRPC calls with timing and status
* **Errors Tab** - Filtered view showing only errors
* **Stats Tab** - Aggregated statistics (total calls, error rate, top swamps)
* **Pause/Resume** - Press `P` to pause the stream and examine events
* **Error Details** - Press `Enter` on any event to see full details

**TUI Keyboard Shortcuts:**
| Key | Action |
|-----|--------|
| `1` | Switch to Live view |
| `2` | Switch to Errors view |
| `3` | Switch to Stats view |
| `P` | Pause/Resume stream |
| `C` | Clear all events |
| `E` | Toggle errors-only filter |
| `↑/↓` or `j/k` | Navigate events |
| `Enter` | View error details |
| `Esc` | Close detail view |
| `?` or `H` | Show help |
| `Q` | Quit |

**Examples:**

Start the TUI dashboard:
```bash
hydraidectl observe --instance prod
```

Simple text output (for scripting or logging):
```bash
hydraidectl observe --instance prod --simple
```

Show only errors:
```bash
hydraidectl observe --instance prod --errors-only
```

Get statistics snapshot:
```bash
hydraidectl observe --instance prod --stats
```

Filter by swamp pattern:
```bash
hydraidectl observe --instance prod --filter "auth/*"
```

**Automatic Telemetry Enable:**

If telemetry is not enabled, the command will prompt:
```
⚠️  Telemetry is not enabled on this instance.

To use observe, telemetry must be enabled and the instance must be restarted.
Enable telemetry and restart now? [y/N]: y
✅ Telemetry enabled
🔄 Restarting instance 'prod'...
✅ Instance restarted
⏳ Waiting for server to be ready...
```

**Example Output (Simple Mode):**
```
HydrAIDE Observe - Simple Mode
==============================
Streaming events... (Press Ctrl+C to stop)

14:23:01.234 | Get      | user/sessions/abc123               |    2ms | OK
14:23:01.456 | Set      | cache/products/item-x              |    1ms | OK
14:23:01.789 | Get      | auth/tokens/xyz                    |    5ms | ERR FailedPrecondition
         +-- decompression failed: invalid data format
14:23:02.012 | Delete   | temp/uploads/file                  |    0ms | OK
```

---

## `telemetry` – Enable/Disable Telemetry Collection

The `telemetry` command controls real-time monitoring data collection on the HydrAIDE server. When enabled, the server collects detailed information about all gRPC calls, which is required for the `observe` command.

**Synopsis:**
```bash
hydraidectl telemetry --instance <name> [--enable | --disable]
```

**What Telemetry Collects:**
* All gRPC call details (method, swamp, duration, status)
* Error information with categorization
* Client connection statistics
* Timing metrics for performance analysis

**Flags:**
| Flag | Description |
|------|-------------|
| `--instance, -i` | Instance name (required) |
| `--enable` | Enable telemetry collection |
| `--disable` | Disable telemetry collection |
| `--json, -j` | Output as JSON |

**Examples:**

Check current telemetry status:
```bash
hydraidectl telemetry --instance prod
# Instance:  prod
# Telemetry: disabled
#
# Enable with: hydraidectl telemetry --instance prod --enable
```

Enable telemetry:
```bash
hydraidectl telemetry --instance prod --enable
# ✅ Telemetry enabled
# Restart instance now for changes to take effect? [Y/n]: y
# 🔄 Restarting instance 'prod'...
# ✅ Instance 'prod' restarted successfully
```

Disable telemetry:
```bash
hydraidectl telemetry --instance prod --disable
# ✅ Telemetry disabled
# Restart instance now for changes to take effect? [Y/n]: y
# 🔄 Restarting instance 'prod'...
# ✅ Instance 'prod' restarted successfully
```

JSON output (for scripting):
```bash
hydraidectl telemetry --instance prod --json
# {
#   "instance": "prod",
#   "telemetry_enabled": true
# }
```

**Performance Considerations:**
* Telemetry has minimal performance impact (< 1% overhead)
* Data is stored in a ring buffer (last 30 minutes)
* No data is persisted to disk - telemetry is memory-only
* Recommended to enable only when debugging

---

## `explore` – Interactive Swamp Hierarchy Explorer

The `explore` command provides an interactive TUI (Terminal User Interface) for browsing the Sanctuary / Realm / Swamp hierarchy of your HydrAIDE data. It scans `.hyd` files directly from disk and builds an in-memory index, so **no running server is needed** for browsing.

When used with `--instance` on a running server, you can also **delete** Sanctuaries, Realms, or individual Swamps directly from the explorer.

**Quick Start:**
```bash
# Browse an instance's data (supports deletion if server is running)
hydraidectl explore --instance prod

# Browse a data directory directly (read-only, no deletion)
hydraidectl explore --data-path /var/hydraide/data
```

**Synopsis:**
```bash
hydraidectl explore [--instance <name> | --data-path <path>]
```

**Flags:**
| Flag | Description |
|------|-------------|
| `--instance, -i` | Instance name (reads data path from instance config, enables deletion) |
| `--data-path, -d` | Direct path to data directory (read-only, no deletion) |

**How It Works:**

On launch, the explorer scans all `.hyd` files in the data directory. For files with embedded swamp names, only ~100 bytes per file are read (the 64-byte header + swamp name), making the scan extremely fast even for large datasets.

The swamp names are parsed into a three-level hierarchy: **Sanctuary / Realm / Swamp** (e.g., `users/profiles/alice`). You can then browse this hierarchy interactively.

**Navigation:**

| Key | Action |
|-----|--------|
| `j/k` or `Up/Down` | Navigate list |
| `Enter` or `Right` | Drill down into selected item |
| `Esc` or `Left` | Go back one level |
| `/` | Search/filter current list |
| `PgUp/PgDown` | Scroll pages |
| `Home/End` | Jump to top/bottom |
| `r` | Rescan data directory |
| `d` | Delete selected item (instance mode only) |
| `q` | Quit |

**Hierarchy Levels:**

1. **Sanctuaries** — Top-level grouping. Shows realm count, swamp count, total size.
2. **Realms** — Second level within a sanctuary. Shows swamp count, total size.
3. **Swamps** — Individual swamp files. Shows file size, entry count, format version.
4. **Detail** — Full metadata for a single swamp: file path, creation/modification time, block count, island ID.

**Deletion (Instance Mode Only):**

When launched with `--instance` and the server is running, you can delete data at any level by pressing `d`:

- **Sanctuary level:** Deletes all Realms and all Swamps within the selected Sanctuary.
- **Realm level:** Deletes all Swamps within the selected Realm.
- **Swamp level / Detail:** Deletes the individual Swamp.

Deletion requires **double confirmation** to prevent accidental data loss:

1. **First confirmation:** Shows a summary of what will be deleted (target name, swamp count, total size) and requires typing a randomly generated 4-character code.
2. **Second confirmation:** Shows a stronger warning emphasizing that the operation is **irreversible**, with a new random code to type.

After both confirmations, the explorer uses the server's `DestroyBulk` gRPC streaming API to delete all swamps with parallel workers, showing a real-time progress bar. The Destroy operation cleans up both in-memory state and disk files, including empty parent directories.

> **Warning:** Deletion is permanent and cannot be undone. The data is removed from both memory and disk.

> **Note:** Deletion is not available in `--data-path` mode because the server needs to handle cleanup of in-memory state and file locks properly.

**Examples:**

Browse instance data interactively (with deletion support):
```bash
hydraidectl explore --instance prod
```

Browse a local data directory (read-only, useful for development/testing):
```bash
hydraidectl explore --data-path /tmp/hydraide-test-data
```

---

## `destroy` – Remove Instance

Destroys the selected instance and optionally purges its data.

**Behavior:**
* Gracefully stops instance (if running)
* Removes service definition
* If `--purge` flag is passed, deletes base directory (irreversible)
* Manual confirmation required for data deletion

> ⚠️ Use with caution! `--purge` wipes all binaries, logs, certs, and state

**Examples:**

Destroy without deleting data:

```bash
sudo hydraidectl destroy --instance dev-local
```

Destroy with full purge:

```bash
sudo hydraidectl destroy --instance dev-local --purge
```

---

## `cert` – Generate TLS Certificates (without modifying instances)

The `cert` command is used to generate new TLS certificates without altering or reinitializing an existing HydrAIDE instance.
This is useful when:

* Certificates have expired and must be renewed.
* You want to rotate certificates for security reasons.
* You need to generate certificates specifically for a **Docker-based deployment**, where the server and client certificates will be mounted into containers.

⚠️ **Important:**
This command does **not replace** the `init` process. During initialization, certificate generation already occurs automatically.
`cert` is intended only for later re-generation or for Docker setups where you need the certs separately.

### How it works

1. Prompts you to enter the **target folder path** where certificates should be placed.
   (The folder must exist and be writable.)
2. Asks the same certificate questions as `init` (CN, DNS SANs, IP SANs).
3. Generates a new CA, server, and client certificate set.
4. Copies all generated certificate files into the specified folder.

This allows you to safely regenerate and distribute TLS material without touching the running instance.

### Certificate generation

The following files are created:

| File             | Purpose                                                                  | Who uses it                                          |
| ---------------- | ------------------------------------------------------------------------ | ---------------------------------------------------- |
| **`ca.crt`**     | Root CA certificate. Used to verify both server and client certificates. | Copy to every client, and keep a copy on the server. |
| **`ca.key`**     | Root CA private key. Used only to sign new server/client certificates.   | **Must remain on the server**. Never share.          |
| **`server.crt`** | TLS certificate for this HydrAIDE server.                                | Used only by the server.                             |
| **`server.key`** | Private key for the server certificate.                                  | **Must remain on the server**.                       |
| **`client.crt`** | Client certificate signed by the CA.                                     | Copy to each client that will connect.               |
| **`client.key`** | Private key for the client certificate.                                  | Copy to each client together with `client.crt`.      |

### What to copy to clients

When setting up a client application, copy:

* `ca.crt` → so the client can validate the server’s identity.
* `client.crt` + `client.key` → so the client can authenticate itself to the server.

These three files must be placed in the client’s configuration/runtime path.

### What stays on the server

* `ca.key` → keep strictly private (used only for signing).
* `server.crt` + `server.key` → used by the HydrAIDE server itself.
* The full set of certificates should remain intact in the chosen folder.

**Example usage:**

```bash
hydraidectl cert
```

---

## `upgrade` – Upgrade an Instance In‑Place (all‑in‑one)

Updates a HydrAIDE instance to the **latest available server binary**.
If an update is available, the command performs the entire flow end‑to‑end:

1. **Gracefully stop** the instance (only if it's running)
2. **Download** the latest server binary into the instance's base path (with a progress bar)
3. **Update metadata** and **(re)generate** the service definition
4. **Optionally start** the instance (unless `--no-start` is used)
5. **Wait** until the instance reports **`healthy`** (if started)

If the instance is **already on the latest version**, this command is a **no‑op** (it **does not stop** the server).
Use `--force` to bypass the version check and force a full re‑download and reinstall.

### Prerequisites

* The instance must have been **initialized** earlier via `hydraidectl init`.
* Starting/stopping services may **require administrative privileges** depending on your OS/service manager.

### Synopsis

```bash
hydraidectl upgrade --instance <name> [--no-start] [--force]
```

### Flags

* `--instance` / `-i` **(required)** — the target instance name.
* `--no-start` — update the binary without starting the server (useful before migration).
* `--force` — force re‑download and reinstall even if the instance is already on the latest version. Clears the download cache to ensure a fresh binary.

### Behavior & Timeouts

* Version check: compares the instance's recorded version with the **latest available** version.
* Graceful stop: only if status is not `inactive`/`unknown`.
* Progress: shows a **byte‑accurate progress bar** during download.
* Service file: removes the old service definition and **generates a fresh one** for the updated binary.
* Start: immediately starts the instance after updating (unless `--no-start` is set).
* Health wait: polls the instance until it becomes **`healthy`** (if started).

    * Overall operation context timeout: **600s**
    * Controller command timeout: **90s** (enough time for graceful data flush on stop)
    * Graceful start/stop timeout: **600s**
    * Service removal timeouts: **30s** per systemctl operation (stop, disable, daemon‑reload)

### Examples

```bash
# Update an instance named "prod" and start it
hydraidectl upgrade --instance prod

# Update without starting (for migration scenarios)
sudo hydraidectl upgrade --instance prod --no-start

# Force reinstall even if already on the latest version
sudo hydraidectl upgrade --instance prod --force
```

**Typical outputs**

* Already up to date:

  ```
  The instance "prod" is already up to date (version X.Y.Z).
  Use --force to re-download and reinstall the current version.
  ```
* Force upgrade (same version):

  ```
  Force upgrade requested for instance "prod" (current: X.Y.Z, latest: X.Y.Z).
  Instance "prod" stopped gracefully.
  Downloading  45.2 MB / 45.2 MB
  Instance "prod" has been successfully updated to version X.Y.Z and started.
  ```
* Successful update + start:

  ```
  Instance "prod" stopped gracefully.
  Downloading  45.2 MB / 45.2 MB
  Instance "prod" has been successfully updated to version X.Y.Z and started.
  Waiting for instance "prod" to become healthy...
  Instance "prod" is now healthy and ready for use. (Waited 7s)
  ```
* Successful update without start (--no-start):

  ```
  Instance "prod" stopped gracefully.
  Downloading  45.2 MB / 45.2 MB
  Instance "prod" has been successfully updated to version X.Y.Z.
  The instance was NOT started (--no-start flag). Start it manually with:
    sudo hydraidectl start --instance prod
  ```
* Could not determine the latest version:

  ```
  Unable to determine the latest version of HydrAIDE. Please try again later.
  ```

### Exit Codes

* `0` — success **or** no‑op (already up to date)
* `1` — error (metadata access, stop/start failure, download error, health timeout, etc.)

## `version` – Display CLI and Optional Instance Metadata

Prints the current `hydraidectl` build information and, optionally, the version recorded in a single instance’s `metadata.json`. This command never queries running services—use [`list`](#list--show-all-instances) for fleet status.

**Behavior**
- Default output shows CLI version, commit, build date, platform, and whether a newer release exists.
- `--instance <name>` reads only the local metadata for that instance and appends its recorded version (no health checks, no remote lookups).
- When an update is found, the CLI suggests reinstalling via the official installer script instead of `self-update`.
- Pass `--no-remote` to skip the GitHub release check (useful for air-gapped hosts) and `--pre` to compare against pre-release builds.

**Flags**
- `--instance`, `-i` — instance name whose metadata version should be shown.
- `--json`, `-j` — emit structured JSON containing `cli`, optional `instance`, and optional `update` objects.
- `--no-remote` — disable the GitHub release check.
- `--pre` — include pre-releases when checking for newer builds.
- `--timeout` — network timeout in seconds for the release check (default `3`).

**Examples**
```bash
# CLI build only
hydraidectl version

# CLI + instance metadata (no service status)
hydraidectl version --instance prod

# JSON output suitable for automation
hydraidectl version --json --no-remote
```

**Update message**

When a newer release is available you will see:

```
Update: vX.Y.Z available → run:
  curl -sSfL https://raw.githubusercontent.com/hydraide/hydraide/main/scripts/install-hydraidectl.sh | bash
```

Use that command to reinstall the CLI with the latest stable binary.

---

## `migrate v1-to-v2` – Migrate V1 Storage to V2 Format

**⚠️ IMPORTANT: Always create a full backup before migration!**

Migrates HydrAIDE data from the legacy V1 multi-chunk storage format to the new V2 append-only single-file format.

The V2 storage engine provides:
- **32-112x faster** write operations
- **50% smaller** storage footprint
- **95% fewer** files on disk
- **100x longer** SSD lifespan

**Flags**
- `--instance`, `-i` — Instance name (recommended, auto-handles stop/start)
- `--data-path` — Path to HydrAIDE data directory (manual mode)
- `--full` — Complete migration: stop → migrate → set V2 → cleanup → start
- `--dry-run` — Simulate migration without making changes
- `--verify` — Verify data integrity after each swamp migration
- `--delete-old` — Delete V1 files after successful migration
- `--parallel` — Number of parallel workers (default: 4)
- `--json` — Output result as JSON

**Examples**

```bash
# Recommended: Full automated migration
hydraidectl backup --instance prod --target /backup/pre-migration
hydraidectl migrate v1-to-v2 --instance prod --full

# Manual migration with data path
hydraidectl migrate v1-to-v2 --data-path /path/to/data --verify --delete-old

# Dry-run to see what would be migrated
hydraidectl migrate v1-to-v2 --instance prod --dry-run
```

---

## `engine` – View or Change Storage Engine Version

View or change the storage engine version for a HydrAIDE instance.

**Engine Versions:**
- **V1** — Legacy multi-chunk file storage (default, backward compatible)
- **V2** — New append-only single-file storage (32-112x faster, 50% smaller)

**⚠️ IMPORTANT:** Before switching to V2, you MUST migrate your data first!

**Flags**
- `--instance`, `-i` — Instance name (**required**)
- `--set` — Set engine version (`V1` or `V2`)
- `--json`, `-j` — Output as JSON

**Examples**

```bash
# View current engine
hydraidectl engine --instance prod

# Switch to V2 (after migration)
hydraidectl engine --instance prod --set V2

# Switch back to V1 (after restore)
hydraidectl engine --instance prod --set V1
```

---

## `backup` – Create Instance Backup

Create a backup of HydrAIDE instance data.

**Behavior:**
- The instance is automatically stopped before backup (unless `--no-stop` is used)
- After backup completes, the instance is **NOT** restarted automatically
- You must manually start the instance when ready

**Flags**
- `--instance`, `-i` — Instance name (**required**)
- `--target`, `-t` — Target backup path (**required**)
- `--compress` — Compress backup as tar.gz
- `--no-stop` — Don't stop instance (warning: data may be inconsistent)

**Examples**

```bash
# Simple backup
sudo hydraidectl backup --instance prod --target /backup/hydraide-20260121

# Compressed backup
sudo hydraidectl backup --instance prod --target /backup/hydraide.tar.gz --compress

# Start the instance after backup
sudo hydraidectl start --instance prod
```

---

## `restore` – Restore Instance from Backup

Restore HydrAIDE instance data from a backup.

**⚠️ WARNING:** This will REPLACE all current data!

**Behavior:**
- The instance is automatically stopped before restore
- After restore completes, the instance is **NOT** restarted automatically
- You must manually start the instance when ready

**Flags**
- `--instance`, `-i` — Instance name (**required**)
- `--source`, `-s` — Source backup path (**required**)
- `--force` — Skip confirmation prompt

**Examples**

```bash
# Restore from directory
sudo hydraidectl restore --instance prod --source /backup/hydraide-20260121

# Restore from tar.gz
sudo hydraidectl restore --instance prod --source /backup/hydraide.tar.gz

# Start the instance after restore
sudo hydraidectl start --instance prod
```

---

## `size` – Show Instance Data Size

Show size of HydrAIDE instance data with V1/V2 breakdown.

**Flags**
- `--instance`, `-i` — Instance name (**required**)
- `--detailed` — Show top 10 largest swamps
- `--json`, `-j` — Output as JSON

**Examples**

```bash
# Basic size info
hydraidectl size --instance prod

# Detailed view with top swamps
hydraidectl size --instance prod --detailed
```

**Output Example:**

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
   2. domains/metadata               8.45 MB
   ...
```

---

## `stats` – Show Detailed Swamp Statistics and Health Report

Analyzes all V2 swamps in a HydrAIDE instance and provides comprehensive statistics including fragmentation levels, compaction recommendations, and size information.

**Flags**
- `--instance`, `-i` — Instance name (**required**)
- `--json`, `-j` — Output as JSON format
- `--latest`, `-l` — Show the last saved report instead of running a new scan
- `--parallel`, `-p` — Number of parallel workers (default: 4)

**Examples**

```bash
# Run a full scan and display report
hydraidectl stats --instance prod

# Output as JSON (useful for automation)
hydraidectl stats --instance prod --json

# Show the last saved report (no new scan)
hydraidectl stats --instance prod --latest

# Use 8 parallel workers for faster scanning
hydraidectl stats --instance prod --parallel 8
```

**Output Example:**

```
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
  💠 HydrAIDE Swamp Statistics - prod
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

📊 SUMMARY
────────────────────────────────────────────────────────────
  Total Database Size              │ 1.25 GB
  Total Swamps                     │ 1234
  Total Live Records               │ 456.7K
  Total Entries (incl. deleted)    │ 512.3K
  Dead Entries                     │ 55.6K
  Avg Records/Swamp                │ 370.1
  Median Records/Swamp             │ 245
  Avg Swamp Size                   │ 1.04 MB
  Scan Duration                    │ 2.345s

🔧 FRAGMENTATION & COMPACTION
────────────────────────────────────────────────────────────
  Average Fragmentation            │ ✅ 10.8%
  Swamps Needing Compaction        │ 23 (>20% fragmented)
  Estimated Reclaimable Space      │ 45.67 MB

📅 TIMELINE
────────────────────────────────────────────────────────────
  Oldest Swamp                     │ words/common (2024-01-15 10:30)
  Newest Swamp                     │ analytics/events (2026-01-22 14:45)

📦 TOP 10 LARGEST SWAMPS
────────────────────────────────────────────────────────────
  #    Swamp                                Size       Records
────────────────────────────────────────────────────────────
  1    words/index                       15.32 MB      45.2K
  2    domains/metadata                   8.45 MB      12.1K
  ...

⚡ TOP 10 MOST FRAGMENTED SWAMPS
────────────────────────────────────────────────────────────
  #    Swamp                          Frag%      Dead      Live  Compact?
────────────────────────────────────────────────────────────
  1    temp/cache                      65.2%      1234       567  ⚠️
  2    sessions/expired                45.8%       890       321  ⚠️
  ...

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
  Generated: 2026-01-22T15:30:45+01:00
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

💡 RECOMMENDATIONS
────────────────────────────────────────────────────────────
   23 swamp(s) have >20% fragmentation.
   Estimated 45.67 MB can be reclaimed with compaction.
```

**Report Storage:**

The stats command automatically saves reports to `<instance_base_path>/.hydraide/stats-report-latest.json`. Use `--latest` to quickly view the last report without rescanning.

**Understanding Fragmentation:**

- **0-20%**: ✅ Healthy - No action needed
- **20-50%**: ⚠️ Moderate - Consider compaction
- **50%+**: 🔴 High - Compaction recommended

Fragmentation occurs when records are updated or deleted. Dead entries remain in the file until compaction reclaims the space.

---

## `compact` – Compact Swamp Files

Compacts all V2 swamp files in a HydrAIDE instance to remove dead entries and reclaim disk space. The instance is automatically stopped during compaction.

**Compaction also automatically upgrades file headers** (embeds the swamp name in plain text after the header for fast scanning).

**Flags**
| Flag | Description | Default |
|------|-------------|---------|
| `--instance, -i` | Instance name (**required**) | - |
| `--parallel, -p` | Number of parallel workers | `4` |
| `--threshold, -t` | Fragmentation threshold percentage | `20` |
| `--restart, -r` | Restart instance after compaction | `false` |
| `--dry-run` | Only analyze, don't compact | `false` |
| `--json, -j` | Output as JSON | `false` |

**Examples**

```bash
# Analyze fragmentation without compacting
hydraidectl compact --instance prod --dry-run

# Compact all swamps above 20% fragmentation
hydraidectl compact --instance prod

# Compact with lower threshold and restart after
hydraidectl compact --instance prod --threshold 10 --restart

# Use more workers for faster processing
hydraidectl compact --instance prod --parallel 8 --restart
```

**The compaction process:**
1. Stops the instance (if running)
2. Scans all swamp files for fragmentation
3. Compacts swamps above the threshold (outputs optimized format with embedded name)
4. Reports space savings
5. Optionally restarts the instance (`--restart`)

**File header upgrade:** Compaction always outputs files with the swamp name embedded in the header. This means running `compact` on an instance will automatically upgrade all compacted file headers. The embedded name (stored in plain text after the 64-byte header) enables the `explore` command to scan metadata at ~100 bytes per file without decompressing any blocks. No manual migration step is needed — the upgrade is seamless and backward-compatible.

---

## `cleanup` – Remove Old Storage Files

Remove old V1 or V2 files after migration or rollback.

**Flags**
- `--instance`, `-i` — Instance name (**required**)
- `--v1-files` — Remove V1 chunk files/folders
- `--v2-files` — Remove V2 .hyd files
- `--dry-run` — Show what would be deleted without deleting

**Examples**

```bash
# Dry-run to see what would be deleted
hydraidectl cleanup --instance prod --v1-files --dry-run

# Remove V1 files after V2 migration
hydraidectl cleanup --instance prod --v1-files

# Remove V2 files after rollback to V1
hydraidectl cleanup --instance prod --v2-files
```

---

## Complete V2 Migration Workflow

Here's the recommended step-by-step workflow for safely migrating to V2 storage:

### Pre-Migration Checklist

Before starting, ensure:
- ✅ You have the latest `hydraidectl` installed
- ✅ You have sufficient disk space for backup
- ✅ No critical operations are running

### Step-by-Step Migration

```bash
# 1. Check for hydraidectl updates
hydraidectl version

# 2. Update hydraidectl if needed
curl -sSfL https://raw.githubusercontent.com/hydraide/hydraide/main/scripts/install-hydraidectl.sh | bash

# 3. Stop the HydrAIDE server
sudo hydraidectl stop --instance prod

# 4. Create a compressed backup of your data
sudo hydraidectl backup --instance prod --output /backup/pre-migration --compress

# 5. Update the server WITHOUT starting it
sudo hydraidectl upgrade --instance prod --no-start

# 6. Run the full migration
sudo hydraidectl migrate v1-to-v2 --instance prod --full

# 7. Verify migration results (check the output above for any errors)
hydraidectl size --instance prod

# 8. Start the server manually after verification
sudo hydraidectl start --instance prod

# 9. Check server health
hydraidectl health --instance prod
```

### Why This Order?

1. **Stop first** - Ensures no data is being written during backup or migration
2. **Backup before update** - Your backup contains the current working version
3. **Update with --no-start** - Gets latest server binary without starting
4. **Migrate** - Converts V1 data to V2 format
5. **Manual start** - Gives you control to verify before starting

**Rollback procedure:**

```bash
# 1. Stop instance
hydraidectl stop --instance prod

# 2. Restore from backup
hydraidectl restore --instance prod --source /backup/pre-migration.tar.gz

# 3. Set engine back to V1
hydraidectl engine --instance prod --set V1

# 4. Start instance
hydraidectl start --instance prod
```

---

## `migrate v2-migrate-format` – Upgrade .hyd File Headers for Faster Scanning

Rewrites `.hyd` swamp file headers to embed the swamp name directly after the 64-byte file header. This enables **~100x faster scanning** for tools like `explore` and `stats`, because the swamp name can be read in ~100 bytes without decompressing any data blocks.

This is a V2 engine internal format optimization — no engine version change is involved. The server reads both old and new format files transparently. Files that already have the embedded name are skipped automatically.

| Flag | Description | Default |
|------|-------------|---------|
| `--instance, -i` | Instance name (required) | – |
| `--parallel, -p` | Number of parallel workers | `4` |
| `--restart, -r` | Restart instance after upgrade | `false` |
| `--dry-run` | Only analyze, don't upgrade | `false` |
| `--json, -j` | Output as JSON | `false` |

```bash
# Check how many files need upgrading
hydraidectl migrate v2-migrate-format --instance prod --dry-run

# Upgrade all files
hydraidectl migrate v2-migrate-format --instance prod --restart

# Upgrade with more workers for faster processing
hydraidectl migrate v2-migrate-format --instance prod --parallel 8 --restart
```

**The upgrade process:**

1. Stops the instance (if running)
2. Scans all `.hyd` files and identifies files without embedded swamp name
3. Rewrites each file with the swamp name stored in the header
4. Reports upgrade results (files upgraded, space changes)
5. Optionally restarts the instance

> **Note:** Compaction (`hydraidectl compact`) also automatically upgrades file headers during the compaction process. The `migrate v2-migrate-format` command is useful when you want to upgrade all files without waiting for compaction thresholds.

---

## V2 File Header Optimization

Starting with server **v3.3.0** and hydraidectl **v2.4.0**, HydrAIDE embeds the swamp name as plain text immediately after the 64-byte file header. This enables fast metadata scanning (~100 bytes per file) without decompressing any blocks.

The header upgrade happens through multiple paths:

- **New swamp files** are created with embedded names immediately.
- **Existing files** are upgraded during compaction (automatic or manual).
- **Dedicated upgrade command** for immediate, full-instance conversion:

```bash
# Upgrade all file headers in one step
hydraidectl migrate v2-migrate-format --instance prod --restart
```

The upgrade is fully backward-compatible — the server and hydraidectl can read both old and new format files without any configuration changes.
