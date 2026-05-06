# 🐳 Running HydrAIDE in Docker

## When to use this

Docker is the lightest path to try HydrAIDE: zero install on the host beyond Docker itself, works on Linux, macOS and Windows (via WSL2 + Docker Desktop), and runs cleanly in CI.

Use Docker for:

- Local development and prototyping
- CI sandboxes
- Containerized deployments (Kubernetes, ECS, Fly.io, …)

For long-lived production services on Linux, the recommended path is `hydraidectl init` — it registers a systemd unit, manages binary upgrades, and is what powers Trendizz. See the [install guide](README.md) and [`hydraidectl` user manual](../hydraidectl/hydraidectl-user-manual.md).

---

## Quickest path: the bundled example tree

If you want a working HydrAIDE in 30 seconds plus runnable Go code to start hacking on, skip the rest of this page and use the bundled example tree:

```bash
git clone https://github.com/hydraide/hydraide
cd hydraide/docs/sdk/go/examples
docker compose up -d
```

That brings up a local HydrAIDE container, generates a fresh TLS cert pair into `./certificate/`, and exposes the server on `localhost:5980`. Nine recipes and three reference HTTP apps are ready to run with `make quickstart` / `make recipe-<name>` / `make app-<name>`. See the [example tree README](../sdk/go/examples/) for the full menu.

The compose file there is the simplest working setup — it's a fine starting point to copy into your own project too.

---

## Custom Docker setup

If you want HydrAIDE in your own docker-compose or Kubernetes stack and the example tree's compose isn't a good fit, here's the minimal end-to-end recipe.

### 1. Generate a TLS cert set

HydrAIDE only accepts TLS-secured gRPC connections, so you need a CA + server cert + client cert. The repo ships [`scripts/gen-dev-certs.sh`](../../scripts/gen-dev-certs.sh) which does exactly that, in a throwaway alpine container so you don't need `openssl` on the host:

```bash
mkdir -p ./certificate
docker run --rm \
    -v "$(pwd)/certificate:/certs" \
    -v "$(pwd)/scripts:/scripts:ro" \
    alpine:3.20 sh /scripts/gen-dev-certs.sh
```

After this `./certificate/` contains:

- `ca.crt` – certificate authority (the SDK uses this to verify the server)
- `server.crt`, `server.key` – the server's identity
- `client.crt`, `client.key` – mTLS client identity (the SDK presents this to the server)

> ⚠️ The script generates a **development** cert set valid for 10 years. For production deployments, use your own PKI or rotate the certs on a meaningful schedule.

### 2. Create the runtime folders

```bash
mkdir -p settings data
```

### 3. Write a `docker-compose.yml`

```yaml
services:
  hydraide:
    image: ghcr.io/hydraide/hydraide:latest
    ports:
      - "5980:4444"
    environment:
      - LOG_LEVEL=info
      - GRPC_MAX_MESSAGE_SIZE=104857600
      - HYDRAIDE_DEFAULT_CLOSE_AFTER_IDLE=600
      - HYDRAIDE_DEFAULT_WRITE_INTERVAL=1
    volumes:
      - ./certificate:/hydraide/certificate
      - ./settings:/hydraide/settings
      - ./data:/hydraide/data
    stop_grace_period: 30s
    healthcheck:
      test: ["CMD", "curl", "--fail", "http://localhost:4445/health"]
      interval: 5s
      timeout: 3s
      start_period: 5s
      retries: 5
```

### 4. Start the container

```bash
docker compose up -d
```

The server is now reachable on `localhost:5980`. Use the `client.crt`, `client.key` and `ca.crt` files from `./certificate/` when connecting from your SDK.

---

## Folder layout

| Folder         | Purpose                                                  |
|----------------|----------------------------------------------------------|
| `certificate/` | TLS material — must include `server.crt`, `server.key`, `ca.crt`. The SDK additionally needs `client.crt`, `client.key`. |
| `settings/`    | Server-level configuration and startup settings          |
| `data/`        | Actual data files (`.hyd`) and Swamps                    |

---

## Environment variables

HydrAIDE supports a wide range of configuration options via environment variables.

| ENV variable                        | Purpose                                                                                                                                            | Allowed / expected values (incl. default)                                                                         | Example                                |
|-------------------------------------|----------------------------------------------------------------------------------------------------------------------------------------------------|-------------------------------------------------------------------------------------------------------------------|----------------------------------------|
| `HYDRAIDE_SERVER_PORT`              | Port for the TLS gRPC HydrAIDE server.                                                                                                             | Integer TCP port (1–65535). **Default:** `4444`. Must be a number (no quotes/letters).                            | `HYDRAIDE_SERVER_PORT=4444`            |
| `HEALTH_CHECK_PORT`                 | Port for the HTTP `/health` endpoint.                                                                                                              | Integer TCP port (1–65535). **Default:** `4445`. Must be a number.                                                | `HEALTH_CHECK_PORT=4445`               |
| `HYDRAIDE_ROOT_PATH`                | Root folder HydrAIDE uses to locate certs and other assets. Must contain `certificate/server.crt`, `certificate/server.key`, `certificate/ca.crt`. | Absolute path. If unset, the app sets it to **`/hydraide`** at startup.                                           | `HYDRAIDE_ROOT_PATH=/hydraide`         |
| `LOG_LEVEL`                         | Global log verbosity for `slog`.                                                                                                                   | One of: `debug`, `info`, `warn`, `error`. **Default:** `debug`. (Unknown values fall back to `info`.)             | `LOG_LEVEL=info`                       |
| `SYSTEM_RESOURCE_LOGGING`           | Enables periodic system resource logging.                                                                                                          | `true` to enable; anything else = disabled. **Default:** disabled.                                                | `SYSTEM_RESOURCE_LOGGING=true`         |
| `GRAYLOG_ENABLED`                   | Turns on Graylog logging pipeline (with local fallback if Graylog goes down).                                                                      | `true` or `false`. **Default:** `false` (console only).                                                           | `GRAYLOG_ENABLED=true`                 |
| `GRAYLOG_SERVER`                    | Graylog TCP endpoint the logger should send to. Used only when `GRAYLOG_ENABLED=true`.                                                             | `<host>:<port>` (e.g., `graylog:12201`). If empty, Graylog is treated as unavailable and logs go to console only. | `GRAYLOG_SERVER=graylog:12201`         |
| `GRAYLOG_SERVICE_NAME`              | Service name tag reported to Graylog.                                                                                                              | Any non-empty string. **Default:** `HydrAIDE-Server`.                                                             | `GRAYLOG_SERVICE_NAME=HydrAIDE-Server` |
| `GRPC_MAX_MESSAGE_SIZE`             | Maximum gRPC message size the server accepts (bytes).                                                                                              | Positive integer (bytes). **Default:** `104857600` (100 MB). Must be a number.                                    | `GRPC_MAX_MESSAGE_SIZE=104857600`      |
| `HYDRAIDE_DEFAULT_CLOSE_AFTER_IDLE` | Default in-memory idle timeout before a Swamp is closed/flushed (seconds).                                                                         | Positive integer (seconds). **Default:** `1`. Must be a number.                                                   | `HYDRAIDE_DEFAULT_CLOSE_AFTER_IDLE=60` |
| `HYDRAIDE_DEFAULT_WRITE_INTERVAL`   | Default disk flush/write interval for persistent Swamps (seconds).                                                                                 | Positive integer (seconds). **Default:** `10`. Must be a number.                                                  | `HYDRAIDE_DEFAULT_WRITE_INTERVAL=10`   |
| `HYDRAIDE_DEFAULT_FILE_SIZE`        | Default max chunk file size for persistent Swamps (bytes).                                                                                         | Positive integer (bytes). **Default:** `8192` (8 KB). Must be a number.                                           | `HYDRAIDE_DEFAULT_FILE_SIZE=8388608`   |
| `GRPC_SERVER_ERROR_LOGGING`         | Enable logging of gRPC server errors when set to true.                                                                                             | `true` to enable; anything else/empty = disabled. **Default:** disabled.                                          | `GRPC_SERVER_ERROR_LOGGING=true`       |

---

## Running multiple instances on one host

You can run multiple HydrAIDE containers on different ports with different mounted folders. For each instance, copy the compose snippet and change:

- The external port (e.g. `5981:4444`, `5982:4444`)
- The mounted folders (e.g. `instance-1/certificate`, `instance-2/certificate`)

Each instance is fully isolated — separate cert set, separate data, separate process.

---

## Need help?

- Discord: <https://discord.gg/xE2YSkzFRm>
- GitHub issues: <https://github.com/hydraide/hydraide/issues>
