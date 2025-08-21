# 🐳 Installing HydrAIDE with Docker

This guide explains how to install and run a **single-node HydrAIDE server** using Docker and `docker-compose`.  
It is ideal for local development, testing, or lightweight production setups.

---

## 🚀 Quickstart with `docker-compose`

1. **Create a new directory** to hold your HydrAIDE server files:

```bash
mkdir -p ~/hydraide-instance
cd ~/hydraide-instance
```

2. **Create the following folder structure** inside this directory:

```bash
mkdir -p certificate settings data
```

3. **Generate certificates** using the `hydraidectl` CLI:

```bash
hydraidectl cert
```

> 👉  How to [install hydraidectl](../hydraidectl/hydraidectl-install.md)

This will generate:

- `server.crt` – server certificate
- `server.key` – server private key
- `ca.crt` – certificate authority file
- `client.crt` – client certificate to connect securely from SDK
- `client.key` – client key to connect securely from SDK

These files must be present in the `certificate/` folder.

---

4. **Create your `docker-compose.yml` file:**

```yaml
services:
  hydraide:
    image: ghcr.io/hydraide/hydraide:latest
    ports:
      - "5980:4444"
    environment:
      - LOG_LEVEL=trace
      - LOG_TIME_FORMAT=2006-01-02 15:04:05
      - SYSTEM_RESOURCE_LOGGING=true
      - GRAYLOG_ENABLED=false
      - GRPC_MAX_MESSAGE_SIZE=5368709120
      - GRPC_SERVER_ERROR_LOGGING=true
      - HYDRAIDE_DEFAULT_CLOSE_AFTER_IDLE=10
      - HYDRAIDE_DEFAULT_WRITE_INTERVAL=5
      - HYDRAIDE_DEFAULT_FILE_SIZE=8192
    volumes:
      - ./certificate:/hydraide/certificate
      - ./settings:/hydraide/settings
      - ./data:/hydraide/data
    stop_grace_period: 10m
```

> This file maps required folders into the container and starts HydrAIDE on port `5980` (mapped to `4444` inside the container).

---

5. **Start the HydrAIDE container:**

```bash
docker-compose up -d
```

You can now connect to the server on `localhost:5980` using any HydrAIDE SDK.

---

## 📁 Folder Requirements

The following folders must exist and be mounted into the container:

| Folder         | Purpose                          |
|----------------|----------------------------------|
| `certificate/` | Must include `server.crt`, `server.key`, and `ca.crt` |
| `settings/`    | Contains server-level configuration and startup settings |
| `data/`        | Actual data files and Swamps     |

---

## ⚙️ Environment Variables

HydrAIDE supports a wide range of configuration options via environment variables.

| ENV variable                        | Purpose (EN)                                                                                                                                       | Allowed / expected values (incl. default)                                                                         | Example                                |
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
| `GRPC_SERVER_ERROR_LOGGING`         | Enable logging of gRPC server errors when set to true.                                                                                             | true to enable; anything else/empty = disabled. Default: disabled.                                                | `GRPC_SERVER_ERROR_LOGGING=true  `     |

---

## 🧪 Testing Multiple Instances

You can run multiple HydrAIDE containers on different ports and with different mounted folders.  
Just copy your setup and change:

- The external port (e.g. `5981:4444`, `5982:4444`)
- The folder name (e.g. `hydraide-instance-1`, `hydraide-instance-2`)

Each instance runs fully isolated.

---

## 🔒 Security Note

HydrAIDE only accepts TLS-secured gRPC connections. 

Always use the `client.crt client.key ca.crt` generated by `hydraidectl cert` to connect securely from your SDK.

---

## Need Help?

- Discord: https://discord.gg/xE2YSkzFRm
