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
* [`destroy` – Fully delete an instance, optionally including all its data](#restart--restart-instance)
* [`cert` – Generate TLS Certificates (without modifying instances)](#cert--generate-tls-certificates-without-modifying-instances)
* [`update` – Update an Instance In‑Place](#update--update-an-instance-inplace-allinone)

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

## `update` – Update an Instance In‑Place (all‑in‑one)

Updates a HydrAIDE instance to the **latest available server binary**.
If an update is available, the command performs the entire flow end‑to‑end:

1. **Gracefully stop** the instance (only if it’s running)
2. **Download** the latest server binary into the instance’s base path (with a progress bar)
3. **Update metadata** and **(re)generate** the service definition
4. **Start** the instance
5. **Wait** until the instance reports **`healthy`** (or until the operation times out)

If the instance is **already on the latest version**, this command is a **no‑op** (it **does not stop** the server).

### Prerequisites

* The instance must have been **initialized** earlier via `hydraidectl init`.
* Starting/stopping services may **require administrative privileges** depending on your OS/service manager.

### Synopsis

```bash
hydraidectl update --instance <name>
```

### Flags

* `--instance` / `-i` **(required)** — the target instance name.

### Behavior & Timeouts

* Version check: compares the instance’s recorded version with the **latest available** version.
* Graceful stop: only if status is not `inactive`/`unknown`.
* Progress: shows a **byte‑accurate progress bar** during download.
* Service file: removes the old service definition and **generates a fresh one** for the updated binary.
* Start: immediately starts the instance after updating.
* Health wait: polls the instance until it becomes **`healthy`**.

    * Overall operation context timeout: **600s**
    * Controller command timeout: **20s**
    * Graceful start/stop timeout: **600s**

### Examples

```bash
# Update an instance named "prod"
hydraidectl update --instance prod
```

**Typical outputs**

* Already up to date:

  ```
  The instance "prod" is already up to date (version X.Y.Z).
  ```
* Successful update + start:

  ```
  Instance "prod" stopped gracefully.
  Downloading  45.2 MB / 45.2 MB
  Instance "prod" has been successfully updated to version X.Y.Z and started.
  Waiting for instance "prod" to become healthy...
  Instance "prod" is now healthy and ready for use. (Waited 7s)
  ```
* Could not determine the latest version:

  ```
  Unable to determine the latest version of HydrAIDE. Please try again later.
  ```

### Exit Codes

* `0` — success **or** no‑op (already up to date)
* `1` — error (metadata access, stop/start failure, download error, health timeout, etc.)
