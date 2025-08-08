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
* [`destroy` – Fully delete an instance, optionally including all its data](#restart--restart-instance)

---

## `init` – Interactive Setup Wizard

Use this to create a new HydrAIDE instance.

You will be prompted for:

* Unique instance name (e.g. `prod`, `dev-local`)
* TLS settings: CN, IPs, domains
* Listening port
* Data base path
* Logging level and options
* Optional Graylog integration

The init command creates the config, generates certs, and prepares a new instance for service installation. Note that this command only sets up the instance – it does not start it. To run the instance as a background service, you must follow it with the `service` command.

This command does not require `sudo`. It runs under the current user context and creates the config files in your home or workspace directory.&#x20;

At the end of the initialization, a `certificate` folder is generated inside the chosen instance directory. This folder contains the generated server and client certificates.

The **client certificate** is essential for authenticating your client application. Make sure to extract it and place it in your application's configuration or runtime path.

**Example:**

```bash
hydraidectl init
```

---

## `service` – Set Up Persistent System Service

Registers a systemd service (`hydraserver-<instance>`) for the chosen instance.

It:

* Validates if metadata exists for the instance (must match the name given in the `init` step)
* Writes a service file into the OS
* Prompts to start and enable the service immediately
* At the end, allows you to choose whether to start the instance right away or defer it for later

This command **requires administrative privileges**:

* On Linux: run with `sudo`
* On Windows: run from an Administrator PowerShell session

**Example:**

```bash
sudo hydraidectl service --instance dev-local
```

Useful for persistent background running across reboots.

---

## `start` – Start an Instance

Starts a registered HydrAIDE instance by name. Requires `sudo`.

It:

* Validates that the instance exists
* Starts the system service (`systemctl start hydraserver-<instance>`) **only if it is not already running**

**Example:**

```bash
sudo hydraidectl start --instance dev-local
```

---

## `stop` – Stop a Running Instance

Stops a specific instance cleanly. Also requires `sudo`.

Features:

* Graceful shutdown
* May take longer depending on the number of open Swamps that need to be flushed to disk
* Never forcefully terminate a HydrAIDE instance (e.g., with `kill -9`) — this can result in data corruption

**Example:**

```bash
sudo hydraidectl stop --instance dev-local
```

---

## `restart` – Restart Instance

Combines stop and start in one command. Requires elevated permissions.

* Stops the instance with graceful handling
* Starts it again if no fatal errors
* Logs success or failure per step

This is useful when applying configuration changes, after updates, or for general recovery operations.

**Example:**

```bash
sudo hydraidectl restart --instance dev-local
```

---

## `list` – Show All Instances

Displays all registered HydrAIDE services and their status.

Output options:

* Default (human readable)
* `--quiet` (names only)
* `--json` (machine-readable)

Also detects duplicate services and warns accordingly.

---

## `destroy` – Remove Instance

Destroys the selected instance and optionally purges its data.

Behavior:

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
