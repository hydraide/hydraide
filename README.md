![HydrAIDE – Adaptive Intelligent Data Engine](images/hydraide-banner.jpg)

# HydrAIDE - The Adaptive, Intelligent Data Engine

[![License](https://img.shields.io/badge/license-Apache--2.0-blue?style=for-the-badge)](http://www.apache.org/licenses/LICENSE-2.0)
![Version](https://img.shields.io/badge/version-2.0-informational?style=for-the-badge)
![Status](https://img.shields.io/badge/status-Production%20Ready-brightgreen?style=for-the-badge)
![Speed](https://img.shields.io/badge/Access-O(1)%20Always-ff69b4?style=for-the-badge)
![Go](https://img.shields.io/badge/built%20with-Go-00ADD8?style=for-the-badge&logo=go)
[![Join Discord](https://img.shields.io/discord/1355863821125681193?label=Join%20us%20on%20Discord&logo=discord&style=for-the-badge)](https://discord.gg/xE2YSkzFRm)

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

## 100+ Stars and Growing! (Star History)

[![Star History Chart](https://api.star-history.com/svg?repos=hydraide/hydraide&type=Date&theme=dark)](https://www.star-history.com/#hydraide/hydraide&Date")

### 💖 Thank You, Community!

When we envisioned HydrAIDE, our goal was to give developers a whole new way of thinking... A tool that doesn’t set limits, but unlocks possibilities.

Over the past weeks, it’s been truly incredible to see so many of you discover, understand, and embrace this philosophy. 
Every single star represents your trust, curiosity, and a shared desire to build something new and better.

This chart is not just numbers. Every point is someone’s decision to give HydrAIDE a chance. 
And to us, that’s the greatest recognition we could receive.

Thank you for being part of this journey!   
The best is yet to come…

— Peter & the HydRAIDER Team

---

### ⚙️ What HydrAIDE Does – In One Stack

| Feature                                         | Description                                                                                                                                                                                                                                                                         |
|-------------------------------------------------|-------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| 🗂️ **Database Engine**                         | A NoSQL-like, structure-first data store — no schemas, no query language. Just save your Go structs. [👉 read more](docs/features/database-engine.md)                                                                                                                               |
| 🔄 **Built-in Reactivity & Subscription logic** | Native real-time subscriptions on every write/update/delete. Like Redis Pub/Sub, but smarter. [👉 read more](docs/features/reactivity-and-subscription-logic.md)                                                                                                                    |
| ⚡️ **O(1) access**                              | Deterministic, constant-time O(1) access to data — every Swamp name maps directly to a fixed folder location, ensuring instant lookup without indexing or scanning. [👉 read more](docs/features/o1-access.md) |
| 🔐 **Concurrency-Safe**                         | Per-object locking with deadlock-free critical sections. Easy and safe for business rules. [👉 read more](docs/features/concurrency-safe.md)                                                                                                                                        |
| 🛡️ **Built-in business lock**                  | Per-key, distributed locking that works across services and servers — ideal for enforcing business-level rules without race conditions. HydrAIDE automatically queues lock requests (FIFO), applies a TTL to prevent deadlocks, and releases locks safely even if a service crashes [👉 read more](docs/features/built-in-busines-lock.md) |
| 🧠 **Memory-Efficient**                         | Swamps live in memory only when accessed. Instant hydration, automatic disposal when idle. [👉 read more](docs/features/memory-efficient.md)                                                                                                                                        |
| 🧹 **Zero Garbage**                             | No daemons. No cron jobs. No cleanup scripts. Swamps manage themselves via lifecycle logic. [👉 read more](docs/features/zero-garbage.md)                                                                                                                                           |
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

## 🔧 Maintainers & 💡 Contributors

HydrAIDE wouldn’t be where it is today without the brilliance, dedication, and vision of its early builders.
A heartfelt thank you to everyone who’s shaping this new paradigm of reactive, intention-driven data systems.

### 👑 Maintainers

* **Péter Gebri** – creator & lead architect – [peter.gebri@hydraide.io](mailto:peter.gebri@hydraide.io)
* **Ganesh Pawar** – [arch.gp@protonmail.com](mailto:arch.gp@protonmail.com)
* **Vinayak Mulgund** – [mulgundvinay@gmail.com](mailto:mulgundvinay@gmail.com)

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
