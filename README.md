![HydrAIDE – Adaptive Intelligent Data Engine](images/hydraide-banner.jpg)

# HydrAIDE

**One engine that replaces your database, cache, and pub/sub — just save your structs.**

[![License](https://img.shields.io/badge/license-Apache--2.0-blue?style=for-the-badge)](http://www.apache.org/licenses/LICENSE-2.0)
![Go](https://img.shields.io/badge/built%20with-Go-00ADD8?style=for-the-badge&logo=go)
![Status](https://img.shields.io/badge/status-Production%20Ready-brightgreen?style=for-the-badge)
[![Discord](https://img.shields.io/discord/1355863821125681193?label=Discord&logo=discord&style=for-the-badge)](https://discord.gg/xE2YSkzFRm)

---

## Why HydrAIDE?

Most backends are a patchwork: a database for persistence, Redis for caching, Kafka for events, cron jobs for cleanup, and glue code to hold it all together. Every layer adds latency, failure modes, and operational overhead.

HydrAIDE collapses that entire stack into a single, self-managing data engine:

- **No queries, no schemas** — your Go struct _is_ the data model. Save it, read it, done.
- **O(1) access, always** — deterministic folder-based routing, no indexing, no scanning.
- **Built-in reactivity** — every write emits a real-time event. No separate pub/sub needed.
- **Self-managing** — data loads into memory on access, evicts when idle. No cache invalidation logic.
- **Safe concurrency** — per-key distributed locking with automatic deadlock prevention.

> HydrAIDE already powers [Trendizz.com](https://trendizz.com) — indexing millions of websites and searching hundreds of millions of words in under 1 second, from a single server. In production for over 2 years.
>
> [Read the full story on dev.to →](https://dev.to/hydraide/how-i-made-europe-searchable-from-a-single-server-the-story-of-hydraide-432h)

---

## Quick Start

```bash
# Install the CLI
curl -sSfL https://raw.githubusercontent.com/hydraide/hydraide/main/scripts/install-hydraidectl.sh | bash

# Create and start an instance
hydraidectl init
sudo hydraidectl service --instance <your-instance-name>
```

That's it. No config files, no Docker required.

> Docker also supported → [Docker Installation Guide](docs/install/docker-install.md)

---

## Features

| | Feature | What it does |
|---|---------|-------------|
| 🗂️ | [Database Engine](docs/features/database-engine.md) | NoSQL-like, structure-first data store — no schemas, no query language |
| 💾 | [V2 Storage Engine](docs/features/v2-storage-engine.md) | Append-only single-file storage — 32–112x faster writes, 50% smaller, automatic compaction |
| 🔄 | [Reactivity & Subscriptions](docs/features/reactivity-and-subscription-logic.md) | Native real-time events on every write/update/delete |
| ⚡ | [O(1) Access](docs/features/o1-access.md) | Deterministic constant-time routing — no indexes needed |
| 🔐 | [Concurrency Safety](docs/features/concurrency-safe.md) | Per-object locking with deadlock-free critical sections |
| 🛡️ | [Business Locks](docs/features/built-in-busines-lock.md) | Distributed per-key locking with FIFO queuing and TTL |
| 🔍 | [Server-Side Filtering](docs/sdk/go/go-sdk.md#-server-side-filtering--streaming) | AND/OR filter expressions, streaming delivery, field-level inspection |
| 🧠 | [Memory Efficiency](docs/features/memory-efficient.md) | Data lives in RAM only when accessed, auto-evicts when idle |
| 🧹 | [Zero Garbage](docs/features/zero-garbage.md) | No daemons, no cron jobs — lifecycle is self-managed |
| 🌐 | [Scaling](docs/features/scaing-without-orchestrator.md) | Deterministic distribution — no orchestrators, just spawn instances |
| 🛰️ | [Pure gRPC](docs/features/pure-grpc-control.md) | Fully gRPC-native with mTLS — works with or without SDKs |

---

## Documentation

| | Resource | |
|---|----------|---|
| 📦 | **[Installation Guide](docs/install/README.md)** | Full setup instructions (CLI, Docker, manual) |
| 📘 | **[Go SDK](docs/sdk/go/go-sdk.md)** | Complete SDK reference with examples |
| 🔧 | **[hydraidectl CLI](docs/hydraidectl/hydraidectl-user-manual.md)** | Instance management, monitoring, migration |
| 🔄 | **[Migration Guide](docs/hydraidectl/hydraidectl-migration.md)** | V1→V2 and V2→V3 format migration |
| 🚀 | **[Example Applications](docs/sdk/go/examples/applications)** | Ready-to-run demo apps |
| 🧩 | **[Model Examples](docs/sdk/go/examples/models)** | CRUD, subscriptions, profiles, catalogs |
| 📊 | **[Comparisons](docs/comparisons)** | HydrAIDE vs MongoDB, Redis, PostgreSQL, Kafka, and more |
| 🤖 | **[LLM Integration](docs/hydraide-questions-answers-for-llm.md)** | Use ChatGPT/Claude as your HydrAIDE expert |

---

## Community

We're building something different — a data engine that thinks like a developer.

- 💬 [Join us on Discord](https://discord.gg/xE2YSkzFRm)
- 📖 [Contributor Introduction](CONTRIBUTORS.md) — why HydrAIDE exists and who we're looking for
- 🛠️ [Contribution Guide](CONTRIBUTING.md) — practical steps to get started

---

## Contact

HydrAIDE is created by **Peter Gebri** — founder of [Trendizz.com](https://trendizz.com).

📧 [peter.gebri@trendizz.com](mailto:peter.gebri@trendizz.com) · 🌐 [hydraide.io](https://hydraide.io)

---

<sub>Licensed under [Apache 2.0](http://www.apache.org/licenses/LICENSE-2.0)</sub>
