---
name: hydraide-install-and-upgrade
description: Installing, upgrading, and bootstrapping HydrAIDE. Server install on Linux as a systemd service, Docker install for dev or production, Go SDK install in your application, server and SDK upgrades, V1 to V2 storage migration, filesystem and hardware guidance, and troubleshooting common install errors. Use when starting fresh with HydrAIDE, picking an install method, upgrading the server or the SDK, planning a storage migration, or hitting install or setup errors. For day-to-day server operations after install, use the `hydraidectl` skill. For Go application code that uses the SDK, use the `hydraidego` skill. For conceptual questions about how the engine works, use the `hydraide` skill.
---

# HydrAIDE Install and Upgrade

This skill is a router. Pick the matching topic, read the linked doc, and answer grounded in it. Do not fabricate install steps or version numbers from memory.

## When to use this skill vs. its siblings

| User's question shape | Skill |
|---|---|
| "How do I install HydrAIDE", "upgrade the server", "migrate V1 to V2", "go get the SDK", "Docker compose for HydrAIDE" | **`hydraide-install-and-upgrade`** (this skill) |
| "Manage a running instance", "stop/start/restart", "backup/restore", "observe", "compact", "explore" | **`hydraidectl`** |
| "Write Go code that ...", "model this in `hydraidego`", "what filter do I use for ..." | **`hydraidego`** |
| "Explain how X works", "Why does HydrAIDE do Y" | **`hydraide`** |

## Quick decision: which install path?

| Goal | Path |
|---|---|
| Try HydrAIDE locally in 2 minutes | Docker compose quickstart from the example tree (server + auto-generated TLS certs) |
| Production server on Linux | `hydraidectl` single-binary install with a systemd unit |
| Production server in containers | Docker install with the published `ghcr.io/hydraide/hydraide` image |
| Add the Go SDK to my application | `go get github.com/hydraide/hydraide/sdk/go/hydraidego@latest` |
| Upgrade an existing HydrAIDE server | server upgrade page (rolling vs full restart, version compat) |
| Upgrade the Go SDK in my application | `go get -u`, with the version-compatibility table |
| Move a v1 storage to v3 | V1 to V2 storage migration **before** upgrading the running server |
| Pick a filesystem or hardware for production | filesystem and hardware guidance |
| Contribute to HydrAIDE itself | contributor setup with `go.work` and the `replace` directive |

## Topic index

| Topic | When the user is asking about... | Read |
|---|---|---|
| Filesystem and hardware guidance | "What filesystem", "what disk", "production hardware", "ext4 vs xfs vs zfs", IOPS budgeting | [`docs/install/README.md`](../../docs/install/README.md) |
| Linux quickstart | First-time install on a Linux host, end-to-end | [`docs/install/quickstart.md`](../../docs/install/quickstart.md) |
| Docker install (dev or prod) | docker-compose.yml, image tag, persistent volumes, certs in containers | [`docs/install/docker-install.md`](../../docs/install/docker-install.md) |
| `hydraidectl` install (the CLI itself) | Installing the hydraidectl management binary on the host that runs the server | [`docs/hydraidectl/hydraidectl-install.md`](../../docs/hydraidectl/hydraidectl-install.md) |
| Go SDK install + upgrade | `go get`, pinned versions, upgrade command, version compatibility, troubleshooting | [`docs/sdk/go/install.md`](../../docs/sdk/go/install.md) |
| HydrAIDE contributor setup | Working on HydrAIDE itself: `go.work`, `replace` directive, day-to-day build/test, release flow | [`docs/sdk/go/contributor-setup.md`](../../docs/sdk/go/contributor-setup.md) |
| Server upgrade | Bumping the running HydrAIDE server to a newer version, downtime planning | [`docs/hydraidectl/upgrades.md`](../../docs/hydraidectl/upgrades.md) |
| V1 to V2 storage migration | Moving from the legacy chunk-folder layout to the v2 single-file `.hyd` format. Required before running on v3 against v1 data | [`docs/hydraidectl/hydraidectl-migration.md`](../../docs/hydraidectl/hydraidectl-migration.md) |

## Version compatibility (cheat sheet)

| Server | Compatible Go SDK |
|---|---|
| `server/v3.x` | `sdk/go/hydraidego/v3.x` |

The major version of the SDK matches the major version of the server era. A v3 SDK can talk to any v3 server release. Cross-major compatibility is not guaranteed; bump server and SDK in lockstep when crossing a major.

## How to answer

1. Match the user's intent to one row in the topic index.
2. Read the linked file. The plugin keeps these mirrored, so the path resolves locally.
3. Answer the user's actual question, grounded in the file. Quote sparingly; explain in your own words.
4. If the topic spans two docs (for example, "I'm doing a clean Linux install AND I have v1 data"), read both and sequence the steps: filesystem and hardware guidance first, then quickstart, then migration.
5. If the user wants code or runtime configuration after install, hand off:
   - Operating the server day-to-day: `hydraidectl` skill.
   - Writing Go application code: `hydraidego` skill.

## Common install pitfalls (quick reference)

- **TLS cert mismatch**: the `ServerName` your client passes must match the `CN` of the certificate (default `hydraide`). Easy to miss when copying configs between environments.
- **`hydraidectl init` skipped on production**: the systemd unit lives in a known location and the binary expects specific permissions. Re-run `hydraidectl init -i <name>` if the install is in a half-finished state, do not hand-edit the unit file.
- **Docker compose stack vs published image**: `docs/sdk/go/examples/Dockerfile.dev` builds the server straight from the in-repo source. The compose stack in the example tree uses **that** image, so it always matches the SDK in the same checkout. The published image (`ghcr.io/hydraide/hydraide:latest`) lags by the release cadence and may not have unreleased features.
- **Forgot the SDK upgrade after a server major bump**: server users on a new major need a matching SDK major or filter / patch behaviour can drift silently. Bump both.
- **`go get` against a fresh tag returns 410 Gone**: the proxy has not cached the new version yet. Wait a minute, or run `GOPROXY=direct go get ...` once to force a direct pull.
- **V1 to V2 migration on a running server**: do not. Stop the server, run `hydraidectl migrate`, then start the server again. The migration is offline.

## What this skill is not

- **Not the day-to-day operations manual.** Once installed, switch to `hydraidectl` for stop/start/restart, backup/restore, observe, compact, explore.
- **Not application code generation.** For "create a Profile or Catalog model", switch to `hydraidego` (or run the `/hydraide-new-model` slash command).
- **Not a place to invent versions.** If the user asks about a specific version compatibility that is not in the cheat sheet, read the upgrade or install doc; do not guess.
