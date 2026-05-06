## 📦 HydrAIDE Installation & Instance Management Guide

The `hydraidectl` CLI is the **recommended way** to install, launch, and manage HydrAIDE. It enables fully isolated 
environments for testing, staging, or production — without containers, and without complex configuration.

> 🐳 **Prefer Docker?**  
> **You can also install and run HydrAIDE using Docker.**  
> 👉  [Docker Installation Guide](docker-install.md)  

---

### 🖥️ Minimal System Requirements

HydrAIDE is designed to be **extremely lightweight** and **zero-impact** when idle.

**Supported Platforms:**

* ✅ Linux (**x86\_64 / ARM64**) — recommended for production
* ⚠️ Windows (**x86\_64**) — only via WSL2 with Ubuntu distribution (no native Windows support)

> ⚠️ **HydrAIDE supports only 64-bit systems.**
> 32-bit support has been explicitly discussed and declined. For more details, see: [Issue #151](https://github.com/hydraide/hydraide/issues/151)

**Minimum Hardware:**

* 🧠 **CPU**: 1-core (x86\_64 or ARM64)
* 🧮 **RAM**: 15MB free memory (idle)
* 📀 **Disk**: Any POSIX-compatible filesystem. ext4 is the recommended default — HydrAIDE buffers writes in memory and flushes them as compressed append-only blocks, so a copy-on-write filesystem like ZFS adds metadata and write-amplification overhead without measurable benefit.

> ⚠️ HydrAIDE has **no background processes**, **no idle threads**, and **zero CPU usage** when not actively processing Swamps. It is only active on demand.

**Recommended for Production:**

* SSD storage (HydrAIDE works best with fast I/O)
* Increased file descriptor limits (`ulimit -n 100000`)
* ext4 on the data volume (preferred). XFS works as well; ZFS and other COW filesystems work but add unnecessary overhead for append-only block writes — pick a snapshot strategy at the volume or block layer instead.

---

### ✅ Installing hydraidectl

Before doing anything else, make sure hydraidectl is installed on your system. This CLI tool allows you to create and manage isolated HydrAIDE instances.

#### Linux:

```bash
curl -sSfL https://raw.githubusercontent.com/hydraide/hydraide/main/scripts/install-hydraidectl.sh | bash
```

#### Windows:

HydrAIDE does **not** support native Windows execution.
For development purposes, it can be run inside **WSL2** using an Ubuntu distribution.
Inside your Ubuntu (WSL2) terminal, run the Linux installer (see above).

---

### 🚀 Minimal Setup – Start HydrAIDE in one command

```bash
sudo hydraidectl init -i <your-instance-name>
```

`init` is end-to-end: it asks for the instance name (skipped when `-i` is given) and a base folder, then generates the TLS cert, downloads the binary, registers the systemd unit, starts the service, and waits for it to become healthy.

The folder structure created on disk:

```
<base-folder>/<instance-name>/
├── certificate/         ca.crt, server.crt/key, client.crt/key
├── settings/            generated server config
├── data/                .hyd files (Swamp data)
├── logs/                instance log files
├── binary               the HydrAIDE server binary
└── .env                 systemd environment file
```

Use the `client.crt`, `client.key` and `ca.crt` from `certificate/` when connecting to this instance from any HydrAIDE SDK.

For the full prompt sequence and tunable options, see [`docs/install/quickstart.md`](quickstart.md). Use `sudo hydraidectl init --advanced` to expose every tunable.

---

- ➡️ Full CLI documentation with all commands: [hydraidectl user manual](../hydraidectl/README.md)

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
