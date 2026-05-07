![HydrAIDE](images/hydraide-banner-2.jpg)

# HydrAIDE

<sub>**Pronunciation:** /haɪˈdreɪd/ ("hi-DRAYD")</sub>

A structure-first data engine for workloads that scale by namespace. One Swamp per tenant, user, device, agent, or other natural unit. Reactive, gRPC-native, single binary.

[![License](https://img.shields.io/badge/license-Apache--2.0-blue?style=for-the-badge)](http://www.apache.org/licenses/LICENSE-2.0)
[![Go](https://img.shields.io/badge/built%20with-Go-00ADD8?style=for-the-badge&logo=go)](https://go.dev)
[![Powers Trendizz since 2024](https://img.shields.io/badge/powers-Trendizz%20since%202024-purple?style=for-the-badge)](https://trendizz.com)
[![Claude Code Friendly](https://img.shields.io/badge/Claude%20Code-friendly-7c3aed?style=for-the-badge)](docs/claude-friendly.md)
[![Discord](https://img.shields.io/discord/1355863821125681193?label=Discord&logo=discord&style=for-the-badge)](https://discord.gg/xE2YSkzFRm)

---

## What HydrAIDE is

HydrAIDE organises data into **Swamps** — independent namespaces (think "shard with its own file, lifecycle and lock domain") that live in their own files on disk and load into memory only when accessed. You give every natural unit of your domain its own Swamp: per tenant, per user, per device, per agent, per crawled domain. There is no shared global table. There is no central coordinator. Each Swamp is reached in O(1) by deterministic hashing — the client computes where the data lives without consulting metadata.

Inside a Swamp, data is stored as typed key/value **Treasures**. The Go SDK lets you save and load native structs directly; the wire protocol is gRPC, so any language with a protoc-generated client can use HydrAIDE without an SDK. Every write emits a real-time event over a Subscribe stream — there is no separate pub/sub layer to operate.

HydrAIDE has powered [Trendizz.com](https://trendizz.com) since 2024 — indexing millions of European websites and serving keyword search from a single server. Concrete numbers: 100K inserts in ~46 ms, 10K updates in ~3.75 ms, ~15.4 bytes per entry on disk on a Threadripper + NVMe. See [benchmarks](docs/benchmarks/V2_RESULTS_SUMMARY.md).

---

## Why HydrAIDE exists

HydrAIDE was built because, for one specific workload, every off-the-shelf option ran out of room — for different reasons, all at the same time.

In 2021 we were indexing 2M+ European websites for [Trendizz](https://trendizz.com). Every word was stored in its own shard, and every shard listed the domains where the word appears — tens of millions of independent storage units, multiple terabytes on disk, on a single server with 128 GB of RAM and a sub-3-second search budget across the entire corpus. PostgreSQL deadlocked under the concurrent writes and slowed past tens of millions of rows. MongoDB and any RAM-resident engine wanted the working set in memory. Cloud and per-CPU-licensed engines were ruled out on cost. Disk B-trees were too slow for the latency target.

The shared mismatch was structural: every engine assumed the data lives in one logical place and is reached through a query language. Our data was already split into millions of independent units by nature, and the access pattern was *open the right small store, do work, close it*. So we stopped combining tools and built a single one.

[Full origin story — what we tried, what broke, what we built →](docs/why-hydraide.md) · [Personal narrative on dev.to →](https://dev.to/hydraide/how-i-made-europe-searchable-from-a-single-server-the-story-of-hydraide-432h)

---

## Quick Start

Two paths, depending on what you want.

### Try it locally (any platform, 2 minutes)

Best for learning the SDK, prototyping, or kicking the tires before committing to anything. Brings up a local HydrAIDE with auto-generated TLS certs and a working Go example, in two commands.

```bash
git clone https://github.com/hydraide/hydraide
cd hydraide/docs/sdk/go/examples
docker compose up -d
make quickstart
```

From here the [example tree](docs/sdk/go/examples/) has 9 focused recipes (TTL queues, atomic patches, subscriptions, distributed locks, server-side filters, …) plus 3 reference HTTP apps (`todo-api`, `url-shortener`, `multi-tenant-saas`) with ready-to-import Postman collections. Pick one, run it, copy from it.

Works on Linux, macOS and Windows. On Windows, run from inside WSL2 with Docker Desktop's WSL2 integration enabled — the `Makefile` and the `docker compose` flow are identical.

→ More: [example tree README](docs/sdk/go/examples/), [testing your own models](docs/sdk/go/testing.md)

### Install for real (Linux service, single binary)

Best for staging, production, or anywhere you want HydrAIDE running as a long-lived service. No config files, no Docker required.

```bash
# Install the CLI
curl -sSfL https://raw.githubusercontent.com/hydraide/hydraide/main/scripts/install-hydraidectl.sh | bash

# End-to-end install: generates TLS cert, downloads the binary, registers
# the systemd unit, starts the service, and waits until it is healthy.
sudo hydraidectl init -i <your-instance-name>
```

→ More: [install guide](docs/install/), [install quickstart](docs/install/quickstart.md), [Docker install](docs/install/docker-install.md), [`hydraidectl` user manual](docs/hydraidectl/hydraidectl-user-manual.md)

---

## What HydrAIDE does well

- **Per-namespace isolation at scale** — millions of independent Swamps on a single server, each with its own lifecycle, lock domain and disk file. Natural fit for multi-tenant SaaS, IoT fleets, per-agent state, search indexes.
- **Hot/cold tiering without a cache layer** — a Swamp loads into memory on first access and evicts itself after a configurable idle window. No external cache, no invalidation logic.
- **Indexes built on demand, not maintained forever** — internal indexes for a Swamp (e.g. ordering by creation or update time) are built when a read or filter needs them, and discarded when the Swamp evicts. No persistent index files; no disk space spent on indexes nobody is currently using.
- **Native subscriptions on every write** — gRPC streams deliver insert/update/delete events in FIFO order. No Kafka, no Redis pub/sub.
- **Server-side filtering and queries** — AND/OR filter expressions, vector similarity, geographic distance and field-level inspection are evaluated on the server and streamed back. See [Query engine](docs/features/query-engine.md).
- **Atomic field-level patches** — `PatchTreasures` mutates individual fields inside a typed MessagePack Treasure on the server, without a read-modify-write round-trip. See [Structural MessagePack patch](docs/features/structural-msgpack-patch.md).
- **Append-only single-file storage** — one `.hyd` file per Swamp with automatic compaction. On a Threadripper 2950X + Samsung 990 PRO: 100K inserts in ~46 ms, 10K updates in ~3.75 ms, 10K deletes in ~1.66 ms, ~15.4 bytes per entry on disk. See [benchmark results](docs/benchmarks/V2_RESULTS_SUMMARY.md) and [run instructions](docs/benchmarks/CHRONICLER_BENCHMARKS.md).
- **Per-key FIFO locking** — concurrent writes on different keys run in parallel; same-key writes are queued. Deadlocks are not possible by construction.
- **gRPC-native, SDK-optional** — the [proto file](proto/hydraide.proto) is the source of truth. The Go SDK is a convenience layer; any language with protoc support can talk to a HydrAIDE server directly.

---

## Good fit for

The features list above is what HydrAIDE *does*. Here's what to *build* with it:

| You're building | Why HydrAIDE fits |
|---|---|
| **Multi-tenant SaaS** | One Swamp per tenant. Isolation, GDPR delete, eviction — by construction, no row-level security to write or audit. See the [`multi-tenant-saas` reference app](docs/sdk/go/examples/03-reference-apps/multi-tenant-saas/). |
| **IoT / device fleet state** | A device's data only lives in RAM while it's talking. Idle = zero CPU, zero memory. Scales to millions of devices on one server. |
| **Per-agent / per-user AI state** | Native subscriptions on every write feed agent loops without Kafka. One Swamp per agent gives independent lifecycle and lock domain. |
| **Real-time collaborative apps** | The gRPC subscribe stream replaces a separate event bus. One binary, one wire protocol. See [`live-subscribe`](docs/sdk/go/examples/02-recipes/live-subscribe/). |
| **Search and keyword indexes** | What HydrAIDE was originally built for. Powers Trendizz across millions of European websites today. |

---

## What HydrAIDE is not for

Honesty up front, so you don't pick the wrong tool:

- **Not an OLAP engine.** No columnar storage, no cross-Swamp aggregation, no analytical query planner.
- **No multi-key transactions across Swamps.** Atomicity is per-key (and within a Swamp via patches). If you need cross-shard ACID, use Postgres or CockroachDB.
- **No SQL surface.** Filtering and queries are expressed via the gRPC API, not SQL. There is no dialect-compatibility shim.
- **Not a drop-in replacement for relational schemas with enforced foreign keys.** Integrity lives in the application layer.
- **Not a hosted service.** You run the binary. There is no managed cloud offering.

---

## Documentation

### Concepts and features

The features below are grouped to follow the order you'd want to read them in: the data model first, then how you work with the data, then runtime behaviour, then how it scales, then the wire protocol.

#### Foundation — the data model

| | Resource | |
|---|---|---|
| 🗂️ | [Database engine](docs/features/database-engine.md) | Sanctuary/Realm/Swamp/Treasure model |
| 🧬 | [Struct-first data model](docs/features/struct-first-data-model.md) | Why your Go struct is the schema |

#### Working with data — the surface area

| | Resource | |
|---|---|---|
| 🔍 | [Query engine](docs/features/query-engine.md) | Server-side filters, vector and geo |
| 🪄 | [Structural MessagePack patch](docs/features/structural-msgpack-patch.md) | Atomic field-level mutations on the server |
| 🔄 | [Reactivity & subscriptions](docs/features/reactivity-and-subscription-logic.md) | Real-time events on every write |

#### Runtime — what happens during execution

| | Resource | |
|---|---|---|
| 🔐 | [Concurrency safety](docs/features/concurrency-safe.md) | Per-key FIFO locks, deadlock-free |
| 🛡️ | [Business locks](docs/features/built-in-business-lock.md) | Application-level distributed locks with TTL |
| 🧠 | [Swamp lifecycle](docs/features/swamp-lifecycle.md) | Idle eviction from RAM, empty-Swamp removal from disk |

#### Scale — where data lives

| | Resource | |
|---|---|---|
| ⚡ | [Deterministic addressing](docs/features/deterministic-addressing.md) | Swamp name → folder → island → server, all O(1) |
| 💾 | [V2 storage engine](docs/features/v2-storage-engine.md) | Append-only `.hyd` format with measurements |

#### Protocol

| | Resource | |
|---|---|---|
| 🛰️ | [Pure gRPC control](docs/features/pure-grpc-control.md) | Wire protocol as the contract |

### SDK, CLI, examples

| | Resource | |
|---|---|---|
| 📥 | [Go SDK install + upgrade](docs/sdk/go/install.md) | `go get`, version compatibility, troubleshooting |
| 📘 | [Go SDK reference](docs/sdk/go/go-sdk.md) | Full API with code samples |
| 🔧 | [hydraidectl CLI](docs/hydraidectl/README.md) | Instance management, monitoring, migration |
| 🔄 | [Migration guide](docs/hydraidectl/hydraidectl-migration.md) | V1→V2 format migration |
| 🚀 | [Go examples (runnable)](docs/sdk/go/examples) | Quickstart, recipes, and reference apps with integration tests |
| 🧪 | [Testing HydrAIDE models](docs/sdk/go/testing.md) | How to test against a live instance, not mocks |
| 🤔 | [Why we built it](docs/why-hydraide.md) | The workload that broke every off-the-shelf database we tried |
| 📈 | [Benchmarks](docs/benchmarks) | Raw measurements, methodology, run scripts |

### Working with Claude Code

Install the [HydrAIDE Claude Code plugin](https://github.com/hydraide/claude) to get three skills (Go SDK reference, server operations, conceptual explanations) and three slash commands ready to use:

```
/plugin marketplace add hydraide/claude
/plugin install hydraide
```

The skills activate automatically when you ask a HydrAIDE-related question. Slash commands: `/hydraide-new-model` (interactive Profile/Catalog wizard), `/hydraide-review` (pitfall checklist code review), `/hydraide-debug` (diagnostic flow). Full breakdown of what the plugin does in [Claude Code-friendly notes](docs/claude-friendly.md).

| | Resource | |
|---|---|---|
| 🤖 | [Claude Code-friendly notes](docs/claude-friendly.md) | What the plugin does, install, skills, slash commands |
| 📒 | [`CLAUDE.md`](CLAUDE.md) | Project-level guidance auto-loaded by Claude Code (architecture, conventions, build) |
| 📜 | [`hydraidego` skill](.claude/skills/hydraidego/SKILL.md) | Go SDK reference: modelling, filters, patches, locks, subscriptions |
| 🛠️ | [`hydraidectl` skill](.claude/skills/hydraidectl/SKILL.md) | Operations reference: install, upgrade, backup/restore, migrate, observe |
| 🧠 | [`hydraide` skill](.claude/skills/hydraide/SKILL.md) | Conceptual / "how does X work" router into the feature docs |

---

## Trying HydrAIDE?

If you're picking this up and something doesn't fit — the docs miss a thing, the install hits a wall, a model design isn't clicking, or the workload just doesn't quite map — say so. Open a Discord thread, file an issue, or email directly. Real people answer.

- 💬 [Discord](https://discord.gg/xE2YSkzFRm) — fastest channel; drop a question any time
- 🐙 [GitHub issues](https://github.com/hydraide/hydraide/issues) — bugs and feature requests
- 📧 [peter.gebri@trendizz.com](mailto:peter.gebri@trendizz.com) — direct line

Want to contribute? Start with [Contributors](CONTRIBUTORS.md) and the [Contribution Guide](CONTRIBUTING.md).

---

## Author and contact

HydrAIDE is built by **Peter Gebri**, founder of [Trendizz.com](https://trendizz.com), and used in production at Trendizz since 2024.

📧 [peter.gebri@trendizz.com](mailto:peter.gebri@trendizz.com) · 🌐 [hydraide.io](https://hydraide.io)

---

<sub>Licensed under [Apache 2.0](http://www.apache.org/licenses/LICENSE-2.0)</sub>
