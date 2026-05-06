# 🔧 hydraidectl Installation Guide

`hydraidectl` is a standalone CLI tool to manage your HydrAIDE server lifecycle.
The commands below automatically download, install, and make the latest version globally available on your system — with a single line.

To learn about available `hydraidectl` commands and usage examples, refer to the [**hydraidectl user manual**](README.md).

---

## 🐧 Linux

Run the following command in your terminal:

```bash
curl -sSfL https://raw.githubusercontent.com/hydraide/hydraide/main/scripts/install-hydraidectl.sh | bash
```

This script will:

* Detect your architecture (`amd64` or `arm64`)
* Download the latest binary release of `hydraidectl`
* Set it as executable
* Move it to `/usr/local/bin/hydraidectl` (requires `sudo`)
* Make it immediately available in your shell

---

## 🪠 Windows (WSL2 + Ubuntu only)

HydrAIDE does **not** support native Windows execution. 
For development purposes, it can be run inside **WSL2** using an Ubuntu distribution.

Inside your Ubuntu (WSL2) terminal, run the Linux installer (see above)

---

## ✅ Verify Installation

After installation, you can run:

```bash
hydraidectl --help
```

---

## ♻️ Updating hydraidectl

To update to the latest version at any time, simply re-run the same install command.
It will download and replace the previous version automatically.

---

## 📁 Source Scripts

* Linux script: [`scripts/install-hydraidectl.sh`](https://github.com/hydraide/hydraide/blob/main/scripts/install-hydraidectl.sh)

---

### 📖 Next Steps

You can now manage your HydrAIDE instances with `hydraidectl`.  

## Available Commands

* [`init`](lifecycle.md#init--end-to-end-install) – Install a new instance end-to-end (config + cert + service + start)
* [`edit`](lifecycle.md#edit--reconfigure-an-instance) – Reconfigure ports, logging, gRPC, TLS SANs or repair the systemd unit
* [`start`](lifecycle.md#start--start-an-instance) – Start a specific HydrAIDE instance
* [`stop`](lifecycle.md#stop--stop-a-running-instance) – Gracefully stop an instance
* [`restart`](lifecycle.md#restart--restart-an-instance) – Restart a running or stopped instance
* [`list`](monitoring.md#list--show-all-instances) – Show all registered HydrAIDE instances on the host
* [`health`](monitoring.md#health--instance-health-probe) – Display health of an instance
* [`destroy`](lifecycle.md#destroy--remove-an-instance) – Fully delete an instance, optionally including all its data

> Looking for the shortest path from "fresh host" to "running instance"?
> The two-command quickstart lives at [`docs/install/quickstart.md`](../install/quickstart.md).

For details, see the [hydraidectl user manual](README.md).

---
