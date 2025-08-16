# 🔧 hydraidectl Installation Guide

`hydraidectl` is a standalone CLI tool to manage your HydrAIDE server lifecycle.
The commands below automatically download, install, and make the latest version globally available on your system — with a single line.

To learn about available `hydraidectl` commands and usage examples, refer to the [**Hydraidectl User Manual**](hydraidectl-user-manual.md) document.

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

* [`init`](hydraidectl-user-manual.md#init--interactive-setup-wizard) – Initialize a new HydrAIDE instance interactively
* [`service`](hydraidectl-user-manual.md#service--set-up-persistent-system-service) – Create and manage a persistent system service
* [`start`](hydraidectl-user-manual.md#start--start-an-instance) – Start a specific HydrAIDE instance
* [`stop`](hydraidectl-user-manual.md#stop--stop-a-running-instance) – Gracefully stop an instance
* [`restart`](hydraidectl-user-manual.md#restart--restart-instance) – Restart a running or stopped instance
* [`list`](hydraidectl-user-manual.md#list--show-all-instances) – Show all registered HydrAIDE instances on the host
* [`health`](hydraidectl-user-manual.md#health--instance-health) – Display health of an instance
* [`destroy`](hydraidectl-user-manual.md#destroy--remove-instance) – Fully delete an instance, optionally including all its data

For details, see the [HydrAIDECtl User Manual](hydraidectl-user-manual.md).

---
