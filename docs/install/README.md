## 📦 HydrAIDE Installation & Instance Management Guide

The `hydraidectl` CLI is the **recommended way** to install, launch, and manage HydrAIDE. It enables fully isolated 
environments for testing, staging, or production — without containers, and without complex configuration.

---

### 🖥️ Minimal System Requirements

HydrAIDE is designed to be **extremely lightweight** and **zero-impact** when idle.

**Supported Platforms:**

* ✅ Linux (x86\_64 / ARM64) — recommended for production
* ✅ Windows (x86\_64) — recommended for development and testing only

**Minimum Hardware:**

* 🧠 **CPU**: 1-core (x86\_64 or ARM64)
* 🧮 **RAM**: 512 KB free memory (idle)
* 📀 **Disk**: Any POSIX-compatible filesystem (ZFS recommended for production)

> ⚠️ HydrAIDE has **no background processes**, **no idle threads**, and **zero CPU usage** when not actively processing Swamps. It is only active on demand.

**Recommended for Production:**

* SSD storage (HydrAIDE works best with fast I/O)
* Increased file descriptor limits (`ulimit -n 100000`)
* ZFS with snapshot support for safe backups and rollback

---

### ✅ Installing hydraidectl

Before doing anything else, make sure hydraidectl is installed on your system. This CLI tool allows you to create and manage isolated HydrAIDE instances.

#### Linux:

```bash
curl -sSfL https://raw.githubusercontent.com/hydraide/hydraide/main/scripts/install-hydraidectl.sh | bash
```

#### Windows:

```powershell
irm https://raw.githubusercontent.com/hydraide/hydraide/main/scripts/install-hydraidectl.ps1 | iex
```

> ➡️ Hydraidectl full install documentation: [hydraidectl-install.md](../hydraidectl/hydraidectl-install.md)

---

### 🚀 Minimal Setup – Start HydrAIDE in 2 Steps

1. **Initialize a new instance**

```bash
hydraidectl init
```

This interactive setup asks for:

* The instance name (e.g. `hydraide-prod`, `hydraide-test`)
* Folder location for certificates, settings, and data
* Optional port and service tuning

It automatically generates the full folder structure:

```
<your-folder>/
├── certificate/
├── settings/
└── data/
└── logs/
└── binary
└── .env
```

2. **Start as a background service**

```bash
sudo hydraidectl service --instance <your-instance-name>
```

This installs and starts HydrAIDE as a persistent systemd service.

> During initialization, a `certificate/` folder is created, which includes a `client.crt` file. 
> This certificate is required when connecting to HydrAIDE from a client SDK.

You're now ready to connect via gRPC — HydrAIDE is live and secure.

---

- ➡️ Full CLI documentation with all commands: [hydraidectl-user-manual.md](../hydraidectl/hydraidectl-user-manual.md)

---

### 🧪 Multiple Instances = Clean Separation

HydrAIDE supports **any number of isolated instances** on the same machine. Each instance has its own:

* Folder structure
* TLS certificate
* Swamp storage
* Port configuration

This gives you complete flexibility to:

* Run a `test` instance during development
* Deploy a `prod` instance with different settings
* Create throwaway instances for CI pipelines or manual experiments

### ✅ Why it matters

* You don’t need to mock your data layer in tests — just use a real, clean HydrAIDE instance
* No need for container orchestration or complex config switching
* Faster debug and validation cycles for API development
* Clean boundaries between environments (no accidental overlap or conflicts)

> 💡 Best practice: name your instances clearly (`hydraide-dev`, `hydraide-ci`, `hydraide-prod`, etc.) and keep them in separate folders.


---

### 🧠 Think Ahead: Multi-Instance Topologies from Day One

HydrAIDE makes it easy to **prepare for distributed, multi-server environments** — even if you're only starting on a single machine.

Because the client SDK can connect to **multiple servers simultaneously**, you can:

#### 🧪 Emulate a multi-server setup on a single host:

* Create 3 isolated instances: `A`, `B`, and `C` — each in its own folder
* Run all three locally, and connect to them from your client as if they were on separate machines
* Later, you can **migrate any instance’s folder** to a different physical server, with no changes to your data

This lets you **design distributed topologies** from day one — with zero lock-in.

#### 🧩 Or split by logical domain (e.g. user vs. search):

You can also use multiple instances to separate data by purpose:

* 🧑 `user-instance` — stores user profiles, tokens, and permissions (runs on a small server)
* 🔎 `search-instance` — stores high-volume search indexes or analytics (runs on a powerful server)

Your client connects to both simultaneously via **two clean SDK instances** — and it just works.

> 💡 Whether you're building a monolith or a service mesh, HydrAIDE adapts to your architecture, not the other way around.
