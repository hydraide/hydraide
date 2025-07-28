# 🚀 HydrAIDE Installation Guide

Welcome to the official installation guide for **HydrAIDE** designed for zero-maintenance, secure, and fast deployments.

---

## 🧠 Summary

HydrAIDE ships as a Docker-native system, with both standalone and cluster-ready configurations. 
Whether you're testing locally or deploying to production, setup is deterministic and infrastructure-agnostic. 
No daemons. No cron jobs. No surprises.

---

## 🖥️ System Requirements

### Minimum Requirements

HydrAIDE is extremely lightweight:

* Runs on a **single-core CPU**
* Uses as little as **512 KB of memory**
* Does nothing until called no idle RAM/CPU usage

### Recommended Setup

For production:

* External volume mount
* **ZFS** filesystem with snapshot support (for backups & fast recovery)
* Separate disks for OS and HydrAIDE data
* Optionally RAID-1 or RAID-Z2 for fault tolerance
* **SSD storage is strongly recommended**. HydrAIDE performs best on high-speed SSDs due to its frequent small-file access patterns. While it can technically run on spinning disks (HDD), this is discouraged in production because of significant I/O penalties and potential latency spikes.
* **Memory sizing**: provision your server RAM based on your largest expected Swamp. As a rule of thumb, allocate memory capacity to hold **at least 10× the size** of your largest Swamp. This allows multiple Swamps to be hydrated in memory simultaneously. You'll quickly observe optimal sizing needs during real-world usage.
* **Linux OS is recommended** for running HydrAIDE in production environments. 
  Popular distributions like Ubuntu, Debian, or Rocky Linux are particularly well-suited. 
  HydrAIDE uses folder-based disk structures and real-time file hydration, which rely on efficient inode handling and predictable I/O scheduling. 
  Native support for filesystems such as **ZFS**, **ext4**, or **xfs** in Linux ensures optimal performance.
  > ⚠️ While HydrAIDE may run on Windows via Docker Desktop or WSL2, this is not recommended for production due to inconsistent file lock behavior and volume mount latency.

#### File System Recommendation

While HydrAIDE works with any file system, **ZFS is strongly recommended** due to:

* Atomic snapshots (`zfs snapshot`)
* Instant rollback (`zfs rollback`)
* Better disk I/O consistency

> 📎 ZFS configuration is **not** covered in this guide. Please follow [this external guide](https://openzfs.github.io/) for setup.

#### Best Practice for ZFS Setup

While ZFS is flexible, here are recommended property settings for optimal performance with HydrAIDE:

| Property       | Recommended | Reason |
|----------------|-------------|--------|
| `compression`  | `lz4`       | Fast and low-overhead block-level compression |
| `recordsize`   | `8K`        | Matches HydrAIDE's small file size pattern |
| `atime`        | `off`       | Prevents unnecessary disk writes on access |
| `relatime`     | `on`        | Balanced timestamp update strategy |
| `logbias`      | `throughput` | Prioritizes sequential write performance (optional for SSDs) |
| `primarycache` | `all`       | Enables RAM caching of both metadata and data |

> 💡 You can set these during dataset creation or update them later via `zfs set`:
>
> ```bash
> zfs set atime=off hydraide/data
> zfs set logbias=throughput hydraide/data
> ```

Snapshotting is encouraged. Use `zfs snapshot` for consistent backups, and optionally replicate using `zfs send` and `zfs recv`.

We recommend a **dedicated dataset** (e.g. `hydraide/data`) for `/hydraide/data`, and optionally separate ones for `/settings` and `/certificate`, depending on your HA/backup strategy.

---

## Increase Max Open Files

HydrAIDE may open many Swamps in parallel. Each one corresponding to at least one open file descriptor.
The number of allowed open files determines how many Swamps (or files within them) can be active simultaneously.

We recommend setting this value to **at least 100,000** for production systems, especially if you're planning
to run **thousands or millions of Swamps**. This ensures that HydrAIDE can hydrate and operate multiple
Swamps concurrently without hitting OS-imposed limits.

However, for smaller-scale setups (e.g. local development, low Swamp count), you can safely start with:

- **10,000 open files** as a baseline
- And increase as your Swamp usage grows


#### 🔧 How to check your current limits:

```bash
ulimit -n                  # Shows current soft limit for the user
sudo sysctl fs.file-max    # Shows the system-wide file descriptor cap
````

#### 🛠️ How to increase:

1. Add or modify the following in `/etc/security/limits.conf`:

```text
youruser soft nofile 100000
youruser hard nofile 200000
```

2. Update system-wide file-max if needed:

```bash
sudo sysctl -w fs.file-max=200000
```

3. If using systemd (e.g. for Docker), set in override:

```bash
sudo systemctl edit docker
```

Add:

```ini
[Service]
LimitNOFILE=200000
```

Then:

```bash
sudo systemctl daemon-reexec
sudo systemctl restart docker
```

> 💡 If the open file limit is too low, HydrAIDE may fail to start or load Swamps with
> `too many open files` errors, especially when scaling.

---

## 🔐 Create Certificate

HydrAIDE only speaks **gRPC over TLS**. You must generate valid certificates before launching.

### Why Certificates?

TLS ensures:

* Encrypted client-server communication
* Protection against MITM attacks
* Trust-based access to HydrAIDE instances

### Steps to Generate:

1. Copy the contents of `/installation/certificate` to your local machine.
2. Open `certificate-generator.sh` and edit the `CA_SUBJECT` variable.
3. Copy `openssl-example.sh` to a new file called `openssl.sh`.
4. Fill in the certificate subject values inside `openssl.sh`.
5. Run `certificate-generator.sh`. It will create all required certificate files.
6. Place the resulting files inside a mountable folder.

---

## 📁 Create Folders for Docker Mounts

Before starting HydrAIDE, make sure to prepare the required folders that will be mounted into the Docker container.

Whether you're using a regular file system or a dedicated ZFS dataset, the following three folders must exist on the host:

```bash
sudo mkdir -p /mnt/hydraide/data
sudo mkdir -p /mnt/hydraide/certificate
sudo mkdir -p /mnt/hydraide/settings
````

#### 💡 ZFS Users

If you're using ZFS, we recommend creating separate datasets for each mount point:

```bash
sudo zfs create yourpool/hydraide-data
sudo zfs set mountpoint=/mnt/hydraide/data yourpool/hydraide-data

sudo zfs create yourpool/hydraide-settings
sudo zfs set mountpoint=/mnt/hydraide/settings yourpool/hydraide-settings

sudo zfs create yourpool/hydraide-certificate
sudo zfs set mountpoint=/mnt/hydraide/certificate yourpool/hydraide-certificate
```

> ✅ Make sure all three folders are writable by the user running Docker.

---

## 🔐 Place Certificates

After generating your TLS certificates, copy them into the appropriate folder:

```bash
cp server.crt /mnt/hydraide/certificate/
cp server.key /mnt/hydraide/certificate/
```

These files will be mounted to `/hydraide/certificate` inside the container and used by 
HydrAIDE for secure TLS communication.

---

## 🧪 Standalone Docker Install

Use the provided compose file:

1. Copy `/installation/docker/docker-compose.local.yml` to your machine.
2. Rename it to `docker-compose.yml`.
3. Edit all required fields as per the in-file comments.
4. Run:

```bash
docker-compose up -d
````

This will start HydrAIDE locally with your mounted volumes and certs.

### 🧾 Example `docker-compose.yml` snippet

```yaml
version: "3.8"

services:
  hydraide-test-server:
    image: ghcr.io/hydraide/hydraide:latest
    ports:
      - "4900:4444"
    volumes:
      - /mnt/hydraide/certificate:/hydraide/certificate
      - /mnt/hydraide/settings:/hydraide/settings
      - /mnt/hydraide/data:/hydraide/data
    environment:
      - LOG_LEVEL=debug
      - GRPC_SERVER_ERROR_LOGGING=true
      - HYDRAIDE_DEFAULT_CLOSE_AFTER_IDLE=10
      - HYDRAIDE_DEFAULT_WRITE_INTERVAL=5
      - HYDRAIDE_DEFAULT_FILE_SIZE=8192
    stop_grace_period: 10m
```

> 🧠 You can customize additional environment variables for logging, gRPC behavior, and Swamp defaults.
> See full documentation inside the provided `docker-compose.local.yml` file.

---

## 🐳 Swarm Docker Services Install

For clustered environments:

1. Copy `/installation/docker/docker-compose.swarm.yml` to your machine.
2. Rename it to `docker-compose.yml`.
3. Fill in the config values and cert mount paths.
4. Deploy with:

```bash
docker stack deploy -c docker-compose.yml <stack_name>
```

This deploys the HydrAIDE service across your Docker Swarm cluster.

---

## ☁️ Kubernetes Support

HydrAIDE Kubernetes installation is coming soon.

---

Happy hydrating! 💧
