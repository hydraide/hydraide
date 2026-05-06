# HydrAIDE Quickstart

Get a working HydrAIDE instance running on a Linux host in two commands.
This guide is the shortest path. For options, edge cases, and the full
command reference, see the [user manual](../hydraidectl/README.md).

## Prerequisites

- Linux with `systemd` (Ubuntu, Debian, RHEL, Fedora, Arch, …).
- `sudo` access on the target host.
- Outbound HTTPS to GitHub releases (for the binary download).

> Containers / non-systemd hosts: use the prebuilt Docker image instead. This
> guide assumes a regular Linux host.

## 1. Install the CLI

```bash
curl -sSfL https://raw.githubusercontent.com/hydraide/hydraide/main/scripts/install-hydraidectl.sh | bash
```

Verify:

```bash
hydraidectl --help
```

## 2. Install an instance

```bash
sudo hydraidectl init -i myhydra
```

The wizard asks for two things:

- **Instance name** — already provided via `-i`, so it is skipped.
- **Base path** — defaults to `/mnt/hydraide/<instance>`. Press Enter to accept.

Everything else is auto-configured:

| Setting | Default |
|---|---|
| TLS | localhost-only (`CN=hydraide`, SAN=`localhost`+`127.0.0.1`) |
| gRPC port | lowest free pair from `4900/4901` (auto-bumps by 10 if taken) |
| Storage engine | V2 (single-file, append-only) |
| Logging | slog at `info` level, no Graylog, no resource logging |
| gRPC max msg size | 10 MB |
| systemd service | installed and started automatically |

The CLI then downloads the latest server binary, registers a `systemd` unit,
starts the instance, and waits until it reports healthy. After that the
"Client connection kit" block prints the three files your application needs.

If you want every tunable exposed (Graylog, message size, log level, custom
TLS SANs, etc.), pass `--advanced`:

```bash
sudo hydraidectl init -i prod --advanced
```

## 3. Connect from your application

The `init` output lists three files inside `<base_path>/certificate/`:

```
/mnt/hydraide/myhydra/certificate/ca.crt
/mnt/hydraide/myhydra/certificate/client.crt
/mnt/hydraide/myhydra/certificate/client.key
```

Copy these three (any way you like — `scp`, `cp`, your config-management
tool) to the host running your application. The Go SDK then takes them like
this:

```go
import "github.com/hydraide/hydraide/sdk/go/hydraidego/client"

client := hydraidego.New(&client.Config{
    ServerHost: "localhost:4900",
    CACertPath: "/path/to/ca.crt",
    ClientCert: "/path/to/client.crt",
    ClientKey:  "/path/to/client.key",
    ServerName: "hydraide",
})
```

`ServerName` must match the `CN` of the certificate — `hydraide` by default.
`ServerHost` must match one of the certificate SANs (so by default,
`localhost` or `127.0.0.1`).

> Connecting from a different machine? Re-run `hydraidectl edit -i myhydra`,
> pick **TLS SANs**, and add the host's IP or DNS name to the certificate
> before you copy the new files.

## 4. Useful follow-ups

```bash
# Show all instances on this host with their status.
hydraidectl list

# Tweak any setting after install — ports, logging, TLS SANs, gRPC.
sudo hydraidectl edit -i myhydra

# Health probe (exit code 0 = healthy).
hydraidectl health -i myhydra

# Stop / start / restart.
sudo hydraidectl stop    -i myhydra
sudo hydraidectl start   -i myhydra
sudo hydraidectl restart -i myhydra

# Upgrade to the latest server version.
sudo hydraidectl upgrade -i myhydra
```

## Troubleshooting

| Symptom | Fix |
|---|---|
| `init` exits with "HydrAIDE requires systemd" | Run on a systemd-based host, or use the Docker image for container setups. |
| `init` says port 4900 is in use | The auto-finder already picked the next free pair. If you need a specific port, re-run with `--advanced`. |
| Service is installed but never goes healthy | `journalctl -u hydraserver-<instance> -n 100` shows the server's logs. |
| Client cannot connect ("certificate is valid for X, not Y") | The `ServerHost` you used is not in the cert's SAN list. Run `hydraidectl edit -i <instance>` and add the host. |
