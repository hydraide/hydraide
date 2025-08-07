![HydrAIDE – Adaptive Intelligent Data Engine](images/hydraide-banner.jpg)

# HydrAIDE - The Adaptive, Intelligent Data Engine

[![License](https://img.shields.io/badge/license-Apache--2.0-blue?style=for-the-badge)](http://www.apache.org/licenses/LICENSE-2.0)
![Version](https://img.shields.io/badge/version-2.0-informational?style=for-the-badge)
![Status](https://img.shields.io/badge/status-Production%20Ready-brightgreen?style=for-the-badge)
![Speed](https://img.shields.io/badge/Access-O(1)%20Always-ff69b4?style=for-the-badge)
![Go](https://img.shields.io/badge/built%20with-Go-00ADD8?style=for-the-badge&logo=go)
[![Join Discord](https://img.shields.io/discord/1355863821125681193?label=Join%20us%20on%20Discord&logo=discord&style=for-the-badge)](https://discord.gg/tYjgwFaZ)

## 🧠 What is HydrAIDE?

**HydrAIDE is a real-time data engine that unifies multiple critical layers into one.**

With HydrAIDE, you no longer need to run a separate database, cache, pub/sub system, or worry about cleaning up stale data.  
It’s a purpose-built engine that replaces traditional architecture with clean, reactive, and developer-native logic.

---

### ⚙️ What HydrAIDE Does – In One Stack

| Feature                   | Description |
|---------------------------|-------------|
| 🗂️ **Database Engine**     | A NoSQL-like, structure-first data store — no schemas, no query language. Just save your Go structs. |
| 🔄 **Built-in Reactivity** | Native real-time subscriptions on every write/update/delete. Like Redis Pub/Sub, but smarter. |
| 📡 **Subscriber Logic**    | Built-in event-awareness for all data. Like Firebase listeners — but deterministic and controlled. |
| 🧠 **Memory-Efficient**    | Swamps live in memory only when accessed. Instant hydration, automatic disposal when idle. |
| ✍️ **No More Queries**     | No SELECT, no WHERE. Your struct *is* the query. |
| 🛰️ **Pure gRPC Control**   | Fully gRPC-native. Works with or without SDKs. Ideal for CLI tools, edge services, and IoT nodes. |
| 🧹 **Zero Garbage**        | No daemons. No cron jobs. No cleanup scripts. Swamps manage themselves via lifecycle logic. |
| 🌐 **Effortless Scaling**  | Deterministic folder-based distribution. No orchestrators. Just spawn instances where needed. |
| 🔒 **Concurrency-Safe**    | Per-object locking with deadlock-free critical sections. Easy and safe for business rules. |
| 💵 **Cost-Efficient**      | Minimal RAM usage. No cache layers. Fewer components = fewer servers. |
| 🔍 **Search Optimized**    | Great for search engines and ML pipelines — but not limited to them. Perfect for dashboards and reactive apps. |
| 🤯 **Less Infra Headache** | No need to combine Redis + Kafka + Mongo + scheduler. HydrAIDE is the backend stack itself. |


---

### 💡 Proven in the Real World

HydrAIDE already powers platforms like [Trendizz.com](https://trendizz.com), indexing millions of websites and 
billions of structured relationships, with real-time search across hundreds of millions of words in under **1 seconds**, 
without preloading.

> In production for over 2 years.  
> Replaces Redis, MongoDB, Kafka, cron jobs, and their glue code.

---

## 🚀 Demo Applications

Explore ready-to-run demo applications built in Go to better understand the HydrAIDE Go SDK and its unique data modeling approach.

All demo apps are located in the [Example Applications in Go](https://github.com/hydraide/hydraide/tree/main/docs/sdk/go) folder.

> More demos coming soon: reactive dashboards, pub-sub listeners, real-time config editors...

### 📦 Available Demos

* **Queue** – A simple task queue system that manages scheduled jobs with future `expireAt` timestamps.

👉 [View Queue Demo Application](https://github.com/hydraide/hydraide/tree/main/docs/sdk/go/examples/applications/app-queue)

These examples are a great starting point to learn how to:

* Structure your HydrAIDE-powered services
* Use profile and catalog models 
* Handle real-time, reactive data flows efficiently

---

### ✅ Primary SDK: Go

HydrAIDE is written in Go — and `hydraidego` is the **official SDK powering production at scale**.

- Supports everything: save/read, locking, subscriptions, TTLs, indexes – all native
- Zero boilerplate: just write structs, and it *just works*
- Fast, typed, reactive – built to feel like part of Go itself

> 🧠 Designed for real-time systems.  
> 🔥 Used in live infrastructure today.  
> 📚 Comes with full docs, examples, and patterns.

* 👉 [`docs/sdk/go/go-sdk.md`](docs/sdk/go/go-sdk.md) – Full documentation with real-world examples

---

## 🤖 ChatGPT Support: Learn & Build with HydrAIDE Faster

The HydrAIDE documentation is purposefully structured to make it **fully compatible with LLM-based assistants like ChatGPT** — so you can focus on building instead of spending hours reading.

> ⚠️ HydrAIDE was **not created using ChatGPT or any LLM**.
> It is the result of years of real-world engineering experience.
> However, we believe in **leveraging AI tools wherever they can accelerate your work** — especially when learning new architectures or building production-grade systems.

### ✅ Turn ChatGPT into your personal HydrAIDE expert

To do that, simply create a **ChatGPT project**, and upload the following files:

| File Type             | Path                                                |
| --------------------- |-----------------------------------------------------|
| Installation Guide    |  [how-to-install-hydraide.md](docs/how-to-install-hydraide.md)                |
| LLM-Friendly Q\&A Set |  [hydraide-questions-answers-for-llm.md](docs/hydraide-questions-answers-for-llm.md)   |
| Go SDK Documentation  |  [go-sdk.md](docs/sdk/go/go-sdk.md)                            |
| Go Example Models     | All `.go` files from [models](docs/sdk/go/examples/models) |
| Go SDK Core Logic     |  [hydraidego.go](sdk/go/hydraidego/hydraidego.go)                  |

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

## 📚 Start Here: The HydrAIDE Documentation

To truly understand HydrAIDE, start with its **core philosophy and architecture**:

👉 [**Thinking in HydrAIDE – The Philosophy of Reactive Data**](docs/thinking-in-hydraide/thinking-in-hydraide.md)  
*Learn how HydrAIDE redefines structure, access, and system design from the ground up.*

### Then continue the 9-step journey:

| Step                                             | Section                                                                              | Description                                                     |
|--------------------------------------------------|--------------------------------------------------------------------------------------|-----------------------------------------------------------------|
| 1️⃣                                            | [📍 Naming Convention](docs/thinking-in-hydraide/naming-convention.md)               | Learn how data structure begins with naming. Not schemas.       |
| 2️⃣                                           | [🌿 Swamp Pattern](docs/thinking-in-hydraide/swamp-pattern.md)                       | Configure persistence, memory, and lifespan directly from code. |
| 3️⃣                                              | [💎 Treasures](docs/thinking-in-hydraide/treasures.md)                               | Understand the smallest, most powerful unit of data.            |
| 4️⃣                                              | [🧩 Indexing](docs/thinking-in-hydraide/indexing.md)                                 | Discover ephemeral, in-memory indexing that feels like magic.   |
| 5️⃣                                              | [🔄 Subscriptions](docs/thinking-in-hydraide/subscriptions.md)                       | Build reactive systems natively with HydrAIDE’s event engine.   |
| 6️⃣                                              | [🔐 Locking](docs/thinking-in-hydraide/locking.md)                                   | Achieve true concurrency without fear.                          |
| 7️⃣                                              | [🧹 Clean System](docs/thinking-in-hydraide/clean-system.md)                         | Never think about cleanup again, because HydrAIDE already did.  |
| 8️⃣                                              | [🧬 Migration](docs/thinking-in-hydraide/migration.md)                                | Struct Evolution Without Fear                                                                |
| 9️⃣                                              | [🌐 Distributed Architecture](docs/thinking-in-hydraide/distributed-architecture.md) | Scale horizontally without orchestration pain.                  |
| 🔟 | [🚀 Install & Update](docs/how-to-install-hydraide.md)                                        | Deploy HydrAIDE in minutes, not days.                           |

---

## 🚀 Quick Start – Install & Update HydrAIDE

**HydrAIDE** runs in a single Docker container. No database setup, No daemons, No surprises.

To get started:

1. Generate a valid **TLS certificate** (required for secure gRPC).
2. Create three folders for your data, certs, and settings.
3. Use the provided `docker-compose.yml` file and run:

```bash
docker-compose up -d
````

👉 [Full Installation Guide →](docs/how-to-install-hydraide.md)

---

## 💻 SDKs - Native Integration in Your Language

HydrAIDE communicates over **gRPC**, and all SDKs share a common `.proto` contract, ensuring cross-language consistency.

---

### 🛠️ Community SDKs - Looking for Contributors!

We're building native SDKs for more languages, and we're looking for contributors, early adopters, and curious 
minds to help shape them.

If you'd like to help bring HydrAIDE to your ecosystem, [open an issue or PR](https://github.com/hydraide/hydraide), 
or just come chat with us on Discord!

| 💻 Language   | SDK Name      | Status             | Goal                                        |
|--------------|---------------|--------------------|---------------------------------------------|
| 🐍 Python     | `hydraidepy`   | 🐣 In development   | ML-ready struct integration & event flows   |
| 🟡 Node.js    | `hydraidejs`   | 🧪 In planning      | Event-friendly reactive API                 |
| 🦀 Rust       | `hydraiders`   | 🧠 In design        | Zero-cost memory-safe abstractions          |
| ☕ Java       | `hydraidejv`   | 🧠 In design        | Enterprise-grade, service-oriented usage    |
| 🎯 C# / .NET  | `hydraidecs`   | 🧠 In design        | Async/await-friendly service layer          |
| 🧠 C++        | `hydraidecpp`  | 🧠 In design        | High-performance native integration         |
| 🌀 Kotlin     | `hydraidekt`   | 🧠 In design        | Android & backend client SDK                |
| 🍎 Swift      | `hydraidesw`   | 🧠 In design        | Index-aware mobile app logic for iOS/macOS  |

> ✨ Want to build with us?  
> [Contribute on GitHub](https://github.com/hydraide/hydraide) or join the [HydrAIDE Discord](https://discord.gg/Kbzs987d).

---

## 📊 Comparisons - HydrAIDE vs Other Systems

Want to see how HydrAIDE compares to the most popular databases and engines?  
We’re building a full series of deep comparisons, mindset-first, not config-first.

| 🔍 Comparison                    | 📄 Status           | Link                                                                    |
|----------------------------------|---------------------|-------------------------------------------------------------------------|
| HydrAIDE vs MongoDB              | ✅ Complete         | [hydraide-vs-mongodb.md](docs/comparisons/hydraide-vs-mongodb.md)       |
| HydrAIDE vs Redis                | ✅ Complete         | [hydraide-vs-redis.md](docs/comparisons/hydraide-vs-redis.md)           |
| HydrAIDE vs PostgreSQL           | ✅ Complete      | [hydraide-vs-postgresql.md](docs/comparisons/hydraide-vs-postgresql.md) |
| HydrAIDE vs MySQL                | ✅ Complete      | [hydraide-vs-mysql.md](docs/comparisons/hydraide-vs-mysql.md)           |
| HydrAIDE vs SQLite               | ✅ Complete     | [hydraide-vs-sqlite.md](docs/comparisons/hydraide-vs-sqlite.md)         |
| HydrAIDE vs Elasticsearch        | ✅ Complete     | [hydraide-vs-elasticsearch.md](docs/comparisons/hydraide-vs-elasticsearch.md)                                                           |
| HydrAIDE vs Firebase / Firestore | ✅ Complete       | [hydraide-vs-firebase.md](docs/comparisons/hydraide-vs-firebase.md)                                                           |
| HydrAIDE vs DynamoDB             | ✅ Complete     | [hydraide-vs-dynamodb.md](docs/comparisons/hydraide-vs-dynamodb.md)                                                          |
| HydrAIDE vs Cassandra            | ✅ Complete      | [hydraide-vs-cassandra.md](docs/comparisons/hydraide-vs-cassandra.md)                                                          |
| HydrAIDE vs ArangoDB             | ✅ Complete      | [hydraide-vs-arangodb.md](docs/comparisons/hydraide-vs-arangodb.md)                                                          |
| HydrAIDE vs InfluxDB             | ✅ Complete      | [hydraide-vs-influxdb.md](docs/comparisons/hydraide-vs-influxdb.md)                                                           |
| HydrAIDE vs ClickHouse           | ✅ Complete      | [hydraide-vs-clickhouse.md](docs/comparisons/hydraide-vs-clickhouse.md)                                                          |
| HydrAIDE vs Neo4j                | ✅ Complete      | [hydraide-vs-neo4j.md](docs/comparisons/hydraide-vs-neo4j.md)                                                           |
| HydrAIDE vs TimescaleDB          | ✅ Complete     |  [hydraide-vs-timescaledb.md](docs/comparisons/hydraide-vs-timescaledb.md)                                                          |
| HydrAIDE vs Apache Kafka         | ✅ Complete      | [hydraide-vs-apachekafka.md](docs/comparisons/hydraide-vs-apachekafka.md)                               |

---

## 🙌 Want to Contribute?

Start by reading the [Contributor Introduction](/CONTRIBUTORS.md), it explains why HydrAIDE exists, what kind of people 
we’re looking for, and how you can join.
Then check out our [Contribution Guide](/CONTRIBUTING.md), it walks you through the practical steps.

Once you're ready, open your first issue or pull request. We’ll be waiting! 🚀

---

## 📩 Contact & Enterprise

HydrAIDE is used in production at [Trendizz.com](https://trendizz.com). 
Interested in enterprise licensing, SDK development, or embedding HydrAIDE in your own platform?

📧 **Peter Gebri** – [peter.gebri@hydraide.io](mailto:peter.gebri@hydraide.io)
(Founder of HydrAIDE & Trendizz)
🌐 **Website** – [https://HydrAIDE.io ](https://hydraide.io) Currently in progress and directly linked to GitHub.

Join the movement. Build different.
