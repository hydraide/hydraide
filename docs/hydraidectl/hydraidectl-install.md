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

## 🪠 Windows (PowerShell)

Run the following command in an **Administrator PowerShell** window:

```powershell
irm https://raw.githubusercontent.com/hydraide/hydraide/main/scripts/install-hydraidectl.ps1 | iex
```

This script will:

* Download the latest `hydraidectl-windows-amd64.exe`
* Copy it to `C:\Program Files\Hydraide\`
* Add this folder to the system PATH (if not already there)
* Make `hydraidectl` globally available via PowerShell or CMD

---

## ✅ Verify Installation

After installation, you can run:

```bash
hydraidectl --help
```

Or on Windows:

```powershell
hydraidectl --help
```

---

## ♻️ Updating hydraidectl

To update to the latest version at any time, simply re-run the same install command.
It will download and replace the previous version automatically.

---

## 📁 Source Scripts

* Linux script: [`scripts/install-hydraidectl.sh`](https://github.com/hydraide/hydraide/blob/main/scripts/install-hydraidectl.sh)
* Windows script: [`scripts/install-hydraidectl.ps1`](https://github.com/hydraide/hydraide/blob/main/scripts/install-hydraidectl.ps1)

---

For more CLI usage, see the full `hydraidectl` documentation.
