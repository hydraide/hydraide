![HydrAIDE – Adaptive Intelligent Data Engine](images/hydraide-banner.jpg)

# HydrAIDE - The Adaptive, Intelligent Data Engine

[![License](https://img.shields.io/badge/license-Apache--2.0-blue?style=for-the-badge)](http://www.apache.org/licenses/LICENSE-2.0)
![Version](https://img.shields.io/badge/version-3.0-informational?style=for-the-badge)
![Status](https://img.shields.io/badge/status-Production%20Ready-brightgreen?style=for-the-badge)
![Speed](https://img.shields.io/badge/Access-O(1)%20Always-ff69b4?style=for-the-badge)
![Go](https://img.shields.io/badge/built%20with-Go-00ADD8?style=for-the-badge&logo=go)
[![Join Discord](https://img.shields.io/discord/1355863821125681193?label=Join%20us%20on%20Discord&logo=discord&style=for-the-badge)](https://discord.gg/xE2YSkzFRm)

---

## 🚀 Major Update: V2 Storage Engine (Version 3.0)

**HydrAIDE 3.0 introduces a completely redesigned storage engine** that delivers:

| Improvement | Gain |
|-------------|------|
| **Write Speed** | 32-112x faster |
| **Storage Size** | 50% smaller |
| **File Count** | 95% fewer files |
| **SSD Lifespan** | 100x longer |

👉 **[Read the full V2 Storage Engine documentation](docs/features/v2-storage-engine.md)** to understand how it works under the hood.

### ✅ Backward Compatible

The V2 engine is **fully backward compatible** with existing V1 data. Both engines work side-by-side:
- **V1 Engine**: Multi-chunk file storage (legacy)
- **V2 Engine**: Single-file append-only storage (recommended)

### ⚠️ Migration Strongly Recommended

While V1 continues to work, we **strongly recommend migrating to V2** for optimal performance.

> **⚡ IMPORTANT: Always create a full backup before migration!**

Use `hydraidectl` to migrate your data:

```bash
# 1. Create a backup first!
# 2. Dry-run to verify (no changes made)
hydraidectl migrate --source /path/to/hydraide/data --dry-run

# 3. Run actual migration
hydraidectl migrate --source /path/to/hydraide/data --workers 4
```

👉 See full migration guide: [hydraidectl Migration Guide](docs/hydraidectl/hydraidectl-migration.md)

---

## 🔍 New: Server-Side Filtering & Streaming Reads

HydrAIDE now supports **server-side query filters** with **nested AND/OR logic** and **streaming reads** for Catalog Swamps:

| Feature | Description |
|---------|-------------|
| **FilterGroup (AND/OR)** | Build complex boolean filter expressions with nested AND/OR groups — evaluated entirely on the server |
| **CatalogReadManyStream** | Streaming variant of CatalogReadMany — results arrive one-by-one via gRPC server-stream instead of a single large response |
| **CatalogReadManyFromMany** | Read from multiple Swamps in a single streaming call with per-Swamp Index and Filters |
| **String Operators** | Contains, NotContains, StartsWith, EndsWith — advanced string matching beyond equality |
| **Server-Side Filters** | Filter Treasures by typed values (int, float, string, bool) directly on the server — non-matching data never leaves the engine |
| **BytesField Filters** | Filter on fields **inside** complex struct values (nested structs, any depth) using dot-separated paths — requires MessagePack encoding |
| **Timestamp Filters** | Filter on CreatedAt, UpdatedAt, ExpiredAt — find recent, expired, or never-updated Treasures |
| **Map Key Existence** | HasKey / HasNotKey — check if a key exists in a map field inside BytesVal |
| **Phrase Search** | FilterPhrase / FilterNotPhrase — find consecutive words in a word-index map for full-text search |
| **Profile Filtering** | Apply server-side filters to profile reads — ForKey() targets specific Treasure fields, with single and batch streaming support |
| **MaxResults** | Post-filter limit for streaming — stop after N matches for both catalog and profile streaming |
| **MessagePack Encoding** | Optional cross-language encoding for complex types, enabling server-side field-level inspection within struct values |
| **CompactSwamp** | Force a full .hyd file rewrite to clean up after encoding migration |

```go
// Stream products with price > 100 AND name contains "Pro", newest first
filters := hydraidego.FilterAND(
    hydraidego.FilterFloat64(hydraidego.GreaterThan, 100.0),
    hydraidego.FilterString(hydraidego.Contains, "Pro"),
)
err := h.CatalogReadManyStream(ctx, swamp, index, filters, Product{}, func(m any) error {
    product := m.(*Product)
    fmt.Println(product.Name, product.Price)
    return nil
})

// Nested AND/OR: price > 100 AND (status == "active" OR status == "pending")
filters := hydraidego.FilterAND(
    hydraidego.FilterFloat64(hydraidego.GreaterThan, 100.0),
    hydraidego.FilterOR(
        hydraidego.FilterString(hydraidego.Equal, "active"),
        hydraidego.FilterString(hydraidego.Equal, "pending"),
    ),
)

// Filter inside a struct field (requires MessagePack encoding):
// Find products where Details.Brand == "Apple" AND Details.Address.City == "Budapest"
filters := hydraidego.FilterAND(
    hydraidego.FilterBytesFieldString(hydraidego.Equal, "Brand", "Apple"),
    hydraidego.FilterBytesFieldString(hydraidego.Equal, "Address.City", "Budapest"),
)

// Timestamp filter: find Treasures created in the last 24 hours
filters := hydraidego.FilterAND(
    hydraidego.FilterCreatedAt(hydraidego.GreaterThan, time.Now().Add(-24*time.Hour)),
)

// Map key existence: find users with "email" in their Metadata map
filters := hydraidego.FilterAND(
    hydraidego.FilterBytesFieldString(hydraidego.HasKey, "Metadata", "email"),
)

// Phrase search: find documents containing "altalanos szerzodesi feltetelek"
filters := hydraidego.FilterAND(
    hydraidego.FilterPhrase("WordIndex", "altalanos", "szerzodesi", "feltetelek"),
)

// Profile filtering: only load profiles where Age > 18 AND Status == "active"
filters := hydraidego.FilterAND(
    hydraidego.FilterInt32(hydraidego.GreaterThan, 18).ForKey("Age"),
    hydraidego.FilterString(hydraidego.Equal, "active").ForKey("Status"),
)
matched, err := h.ProfileReadWithFilter(ctx, swampName, filters, &user)

// Multi-profile streaming with MaxResults: scan 100 profiles, return first 10 matches
h.ProfileReadBatchWithFilter(ctx, swampNames, filters, &UserProfile{}, 10,
    func(sn name.Name, m any, err error) error { /* ... */ })

// MaxResults for catalog streaming: stop after 10 matches
index := &hydraidego.Index{IndexType: hydraidego.IndexCreationTime, MaxResults: 10}
h.CatalogReadManyStream(ctx, swamp, index, filters, Product{}, iterator)
```

> 100% backward compatible — existing APIs work unchanged. Filters are optional. Default encoding remains GOB.

👉 Full documentation: [Server-Side Filtering & Streaming](docs/sdk/go/go-sdk.md#-server-side-filtering--streaming)

---

## 🧠 What is HydrAIDE?

**One engine that replaces your database, cache, and pub/sub — just save your structs.**

No schema design. No queries. No cleanup scripts.
HydrAIDE automatically handles persistence, real-time events, distribution, and memory for you.

For developers who want:

* **Less code and infrastructure** — everything in one place
* **Instant data access** with O(1) folder-based routing
* **Native reactivity** — every change emits a real-time event
* **Memory-efficient operation** — data only lives in RAM when needed
* **Safe concurrency** — built-in per-key distributed locking

With HydrAIDE, you don’t adapt to the database — **the database adapts to your intent**.

---

### ⚙️ What HydrAIDE Does – In One Stack

| Feature                                         | Description                                                                                                                                                                                                                                                                         |
|-------------------------------------------------|-------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| 🗂️ **Database Engine**                         | A NoSQL-like, structure-first data store — no schemas, no query language. Just save your Go structs. [👉 read more](docs/features/database-engine.md)                                                                                                                               |
| 💾 **V2 Storage Engine**                        | Append-only, single-file-per-Swamp storage with 32-112x faster writes, 50% smaller files, and automatic compaction. [👉 read more](docs/features/v2-storage-engine.md)                                                                                                              |
| 🔄 **Built-in Reactivity & Subscription logic** | Native real-time subscriptions on every write/update/delete. Like Redis Pub/Sub, but smarter. [👉 read more](docs/features/reactivity-and-subscription-logic.md)                                                                                                                    |
| ⚡️ **O(1) access**                              | Deterministic, constant-time O(1) access to data — every Swamp name maps directly to a fixed folder location, ensuring instant lookup without indexing or scanning. [👉 read more](docs/features/o1-access.md) |
| 🔐 **Concurrency-Safe**                         | Per-object locking with deadlock-free critical sections. Easy and safe for business rules. [👉 read more](docs/features/concurrency-safe.md)                                                                                                                                        |
| 🛡️ **Built-in business lock**                  | Per-key, distributed locking that works across services and servers — ideal for enforcing business-level rules without race conditions. HydrAIDE automatically queues lock requests (FIFO), applies a TTL to prevent deadlocks, and releases locks safely even if a service crashes [👉 read more](docs/features/built-in-busines-lock.md) |
| 🧠 **Memory-Efficient**                         | Swamps live in memory only when accessed. Instant hydration, automatic disposal when idle. [👉 read more](docs/features/memory-efficient.md)                                                                                                                                        |
| 🧹 **Zero Garbage**                             | No daemons. No cron jobs. No cleanup scripts. Swamps manage themselves via lifecycle logic. [👉 read more](docs/features/zero-garbage.md)                                                                                                                                           |
| 🔍 **Server-Side Filtering & Streaming**        | Built-in query filter system with streaming delivery. Filter Treasures by typed values directly on the server — non-matching data never leaves the engine. Stream millions of results without loading them all into memory. Read from multiple Swamps in a single streaming call with per-Swamp filters. [👉 read more](docs/sdk/go/go-sdk.md#-server-side-filtering--streaming) |
| ✍️ **No More Queries**                          | No SELECT, no WHERE, no JOINS, no Aggregates. Your struct *is* the query. [👉 read more](docs/features/no-more-queries.md)                                                                                                                                                          |
| 🛰️ **Pure gRPC Control**                       | Fully gRPC-native. Works with or without SDKs. Ideal for CLI tools, edge services, and IoT nodes. [👉 read more](docs/features/pure-grpc-control.md)                                                                                                                                |
| 🌐 **Scaling Without Orchestrator**             | Deterministic folder-based distribution. No orchestrators. Just spawn instances where needed. [👉 read more](docs/features/scaing-without-orchestrator.md)                                                                                                                                                                                      |
| 🤯 **Less Infra Headache**                      | No need to combine Redis + Kafka + Mongo + scheduler. HydrAIDE is the backend stack itself. [👉 read more](docs/features/less-infra-headache.md)                                                                                                                                                                                        |

---

## 🚀 Start HydrAIDE in 2 Minutes

The fastest way to run HydrAIDE is using the **`hydraidectl` CLI**.
No config files. No docker. No complexity.

### ✅ Recommended: Install with `hydraidectl`

1. **Download the CLI (Linux):**

   ```bash
   curl -sSfL https://raw.githubusercontent.com/hydraide/hydraide/main/scripts/install-hydraidectl.sh | bash
   ```

   👉 For Windows, and full install guide, see the [How to install hydraidectl](docs/hydraidectl/hydraidectl-install.md)


2. **Create a new instance:**

   ```bash
   hydraidectl init
   ```

   👉 Available command details: [hydraidectl user manual](docs/hydraidectl/hydraidectl-user-manual.md)


3. **Start HydrAIDE as a background service:**

   ```bash
   sudo hydraidectl service --instance <your-instance-name>
   ```

    👉 Read the full installation guide for more details: [How to install HydrAIDE under 2 minutes](docs/install/README.md)

---

> 🐳 **Prefer Docker?**  
> **You can also install and run HydrAIDE using Docker.**  
> 👉  [Docker Installation Guide](docs/install/docker-install.md)

--- 


### 💡 Proven in the Real World

HydrAIDE already powers platforms like [Trendizz.com](https://trendizz.com), indexing millions of websites and 
billions of structured relationships, with real-time search across hundreds of millions of words in under **1 seconds**, 
without preloading.

Read the full story behind the hydrAIDE: [How I Made Europe Searchable From a Single Server - The HydrAIDE Story](https://dev.to/hydraide/how-i-made-europe-searchable-from-a-single-server-the-story-of-hydraide-432h)

> In production for over 2 years.  
> Replaces Redis, MongoDB, Kafka, cron jobs, and their glue code.


---
 
## 🚀 Demo Applications & Model examples

Explore ready-to-run demo applications built in Go to better understand the HydrAIDE Go SDK and its unique data modeling approach.

- All demo apps are located in the [Example Applications in Go](https://github.com/hydraide/hydraide/tree/main/docs/sdk/go/examples/applications) folder.
- Model Examples [CRUD operations, subscriptions, etc.](https://github.com/hydraide/hydraide/tree/main/docs/sdk/go/examples/models)
- Full Go SDK Documentation: [Go SDK Documentation](docs/sdk/go/go-sdk.md)

These examples are a great starting point to learn how to:

* Structure your HydrAIDE-powered services
* Use profile and catalog models 
* Handle real-time, reactive data flows efficiently

---

### ✅ Primary SDK: Go

HydrAIDE is written in Go, and `hydraidego` is the **official SDK powering production at scale**.

- Supports everything: save/read, locking, subscriptions, TTLs, indexes – all native
- Zero boilerplate: just write structs, and it *just works*
- Fast, typed, reactive – built to feel like part of Go itself

> 🧠 Designed for real-time systems.  
> 🔥 Used in live infrastructure today.  
> 📚 Comes with full docs, examples, and patterns.

---

## 🤖 ChatGPT Support: Learn & Build with HydrAIDE Faster

The HydrAIDE documentation is purposefully structured to make it **fully compatible with LLM-based assistants like ChatGPT** — so you can focus on building instead of spending hours reading.

> ⚠️ HydrAIDE was **not created using ChatGPT or any LLM**.
> It is the result of years of real-world engineering experience.
> However, we believe in **leveraging AI tools wherever they can accelerate your work** — especially when learning new architectures or building production-grade systems.

### ✅ Turn ChatGPT into your personal HydrAIDE expert

To do that, simply create a **ChatGPT project**, and upload the following files:

| File Type             | Path                                                                                |
| --------------------- |-------------------------------------------------------------------------------------|
| Installation Guide    | [HydrAIDE installation guide](docs/install/README.md)                               |
| LLM-Friendly Q\&A Set | [hydraide-questions-answers-for-llm.md](docs/hydraide-questions-answers-for-llm.md) |
| Go SDK Documentation  | [go-sdk.md](docs/sdk/go/go-sdk.md)                                                  |
| Go Example Models     | All `.go` files from [models](docs/sdk/go/examples/models)                          |
| Go SDK Core Logic     | [hydraidego.go](sdk/go/hydraidego/hydraidego.go)                                    |

Once uploaded, ChatGPT will be able to:

* answer **any question** about HydrAIDE’s architecture or APIs,
* help you **write HydrAIDE-style Go code** interactively,
* explain example models, functions, and patterns,
* and guide you through debugging, architecture design, or optimization steps.

💡 The documentation is written to be **semantically consumable by AI**, which means ChatGPT will understand not just APIs, but **the design philosophy, naming logic, and intent** behind each HydrAIDE feature.

> A prebuilt ChatGPT is also available for the HydrAIDE Knowledge Engine. You can use it via the ChatGPT store
or directly through this link: https://chatgpt.com/g/g-688779751c988191b975beaf7f68801d-hydraide-knowledge-engine
Feel free to ask it anything! If it can’t answer your question, open an issue, or build your own custom GPT project
with enhanced responses, as we described above.

---

## 📊 Comparisons - HydrAIDE vs Other Databases

Want to see how HydrAIDE compares to the most popular databases and engines?  
We’re building a full series of deep comparisons, mindset-first, not config-first.

* [HydrAIDE vs MongoDB](docs/comparisons/hydraide-vs-mongodb.md)
* [HydrAIDE vs Redis](docs/comparisons/hydraide-vs-redis.md)
* [HydrAIDE vs PostgreSQL](docs/comparisons/hydraide-vs-postgresql.md)
* [HydrAIDE vs MySQL](docs/comparisons/hydraide-vs-mysql.md)
* [HydrAIDE vs SQLite](docs/comparisons/hydraide-vs-sqlite.md)
* [HydrAIDE vs Elasticsearch](docs/comparisons/hydraide-vs-elasticsearch.md)
* [HydrAIDE vs Firebase / Firestore](docs/comparisons/hydraide-vs-firebase.md)
* [HydrAIDE vs DynamoDB](docs/comparisons/hydraide-vs-dynamodb.md)
* [HydrAIDE vs Cassandra](docs/comparisons/hydraide-vs-cassandra.md)
* [HydrAIDE vs ArangoDB](docs/comparisons/hydraide-vs-arangodb.md)
* [HydrAIDE vs InfluxDB](docs/comparisons/hydraide-vs-influxdb.md)
* [HydrAIDE vs ClickHouse](docs/comparisons/hydraide-vs-clickhouse.md)
* [HydrAIDE vs Neo4j](docs/comparisons/hydraide-vs-neo4j.md)
* [HydrAIDE vs TimescaleDB](docs/comparisons/hydraide-vs-timescaledb.md)
* [HydrAIDE vs Apache Kafka](docs/comparisons/hydraide-vs-apachekafka.md)

---

> 🌱 **Every commit builds more than just code. It builds a mindset.**
> HydrAIDE is not just a tool. It’s a way of thinking.
> If you see potential here, don’t just watch — contribute.
> Because we’re not just building a system. We’re building a community of systems thinkers.

Ready to leave your mark? [Join us on Discord](https://discord.gg/xE2YSkzFRm) and let’s build the HydrAIDE together. 🚀

- Start by reading the [Contributor Introduction](/CONTRIBUTORS.md), it explains why HydrAIDE exists, what kind of people we’re looking for, and how you can join.
- Then check out our [Contribution Guide](/CONTRIBUTING.md), it walks you through the practical steps.

Once you're ready, open your first issue or pull request. We’ll be waiting! 🚀

---

## 📩 Contact & Enterprise

HydrAIDE is used in production at [Trendizz.com](https://trendizz.com). 
Interested in enterprise licensing, SDK development, or embedding HydrAIDE in your own platform?

📧 **Peter Gebri** – [peter.gebri@hydraide.io](mailto:peter.gebri@hydraide.io)
(Founder of HydrAIDE & Trendizz)
🌐 **Website** – [https://HydrAIDE.io ](https://hydraide.io) Currently in progress and directly linked to GitHub.

Join the movement. Build different.
