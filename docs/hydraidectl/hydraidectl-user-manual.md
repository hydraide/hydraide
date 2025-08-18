# HydrAIDECtl CLI Documentation

## Overview

The `hydraidectl` CLI allows for easy installation, management, and lifecycle control of HydrAIDE server instances.

> **If you haven't installed hydraidectl yet**, you can find the installation guide here: **[HydrAIDECtl Install Guide](hydraidectl-install.md)**

Although `hydraidectl` is stable and production-tested, new features are under development, including:

* `update` (binary or config upgrade)
* `healthcheck` (status monitoring)
* `log view` (log file reader)
* `non-interactive init` (headless installs in AWS or scripts)
* `offline install` (for edge or air-gapped systems)

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
* [`cert` – Fully delete an instance, optionally including all its data](#restart--restart-instance)

---

Nagyon jó, hogy ezt kiemeled, mert az **`init`** dokumentáció így most nem adja át elég tisztán, hogy pontosan **milyen fájlok keletkeznek, mire jók, és hova kell őket másolni**.
Az új mTLS-es felépítés szerint így érdemes átírni:

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

Displays all registered HydrAIDE services, their Status, and Health.

Output options:

* Default (human-readable table with `Name`, `Status`, `Health`)
* `--quiet` (names only, skips health/status)
* `--json` (machine-readable, includes `"health": "healthy|unhealthy|unknown"`)
* `--no-health` (skip health probing for faster listing)

The `Health` column is determined by running a short health probe (2s timeout) against each instance’s configured health endpoint. If the configuration is missing or the check times out, `unknown` is shown.

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
