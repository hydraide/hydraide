## HydrAIDE vs TimescaleDB

HydrAIDE isn’t just an event-driven alternative to TimescaleDB — it’s a **logic-native, time-optimized data engine** 
that treats time not as a column, but as a **first-class behavior**. Unlike TimescaleDB, which extends 
PostgreSQL for time-series workloads, HydrAIDE was **designed from scratch** for **event-streaming, real-time analytics, 
and automatic memory lifecycle**, without SQL, schemas, or background workers.

TimescaleDB is great when you want:

* Structured, SQL-based analytics over long timelines
* Retention policies and continuous aggregates
* Tight integration with PostgreSQL tools

…but when you need:

* Zero-latency real-time reactivity,
* Built-in TTL logic without cron jobs,
* Automatic cleanup and memory flushing,
* Or fully developer-controlled event semantics without triggers or schemas,

then **HydrAIDE gives you everything Timescale does, just with less setup, more control, and blazing-fast performance**.

HydrAIDE replaces your:

* time-series table
* retention script
* trigger logic
* pub-sub infra
* connection poolers

…with a **single reactive runtime**.

---

## Feature comparison

| Feature                 | HydrAIDE                                 | TimescaleDB (Postgres)                  |
| ----------------------- | ---------------------------------------- | --------------------------------------- |
| ⌛ Time modeling         | ✅ Native TTL & lifecycle tags            | ⚠️ Timestamp columns + custom policies  |
| 🧠 Memory-first design  | ✅ Swamps hydrate on access               | ❌ Disk-first, Postgres buffer pool      |
| 🔄 Built-in reactivity  | ✅ Subscriptions out of the box           | ⚠️ Requires triggers or NOTIFY/LISTEN   |
| 🔥 Real-time ingestion  | ✅ Constant time writes, auto-indexed     | ⚠️ Depends on partitioning/index tuning |
| 📉 Downsampling support | ✅ Via Swamp naming, no background jobs   | ⚠️ Needs continuous aggregates          |
| 🧹 Cleanup & retention  | ✅ TTL-based memory + disk flush          | ⚠️ Manual or policy-based (DROP chunks) |
| 🕰️ Backfill safety     | ✅ Timestamp fields with expiration logic | ⚠️ Requires conflict handling, triggers |
| 🚀 Query performance    | ✅ O(1) reverse scan via Beacons          | ⚠️ Depends on B-tree indexes            |
| 🧰 Tooling required?    | ✅ Optional only                          | ❌ Requires psql, extensions, config     |
| 🧗 Learning curve       | 🟢 Zero-to-Hero in 1 day                 | 🟡 Medium — SQL + partitioning          |
| 🧬 Data type safety     | ✅ Strong Go typing, GOB-encoded          | ⚠️ SQL types only, conversion needed    |
| ⚡ Developer experience  | ✅ Code-native, no SQL                    | ⚠️ Schema-heavy, SQL-first logic        |
| 🧠 Event reasoning      | ✅ Every write is an event                | ❌ Must implement via triggers/functions |
| 📦 Deployment           | ✅ Single binary or Docker                | ❌ Requires PostgreSQL + Timescale ext   |

---

## Terminology Comparison

| HydrAIDE Term | TimescaleDB Equivalent | Explanation                                                                | HydrAIDE-Native Improvement                                      |
| ------------- | ---------------------- | -------------------------------------------------------------------------- | ---------------------------------------------------------------- |
| **Swamp**     | Hypertable             | Logically scoped, hydrated on access, memory-aware.                        | 🔹 Auto-flushing<br>🔹 No table planning<br>🔹 Naming = behavior |
| **Treasure**  | Row                    | Each Treasure is a typed Go struct — not a row of SQL fields               | 🔹 O(1) read/write<br>🔹 No need for ORM<br>🔹 Binary format     |
| **Beacon**    | Index                  | Index scoped to Swamp, event-aware, and auto-discarded                     | 🔹 Auto cleanup<br>🔹 No bloat<br>🔹 Subscribable index          |
| **TTL**       | Retention Policy       | TTL is built-in to any Treasure via struct field                           | 🔹 No config needed<br>🔹 Auto-expiry memory/disk                |
| **Hydrate**   | Read/Query             | When a Swamp is accessed, memory is loaded — with expiry and discard logic | 🔹 Real-time hot data<br>🔹 No query cache needed                |
| **Subscribe** | LISTEN / NOTIFY        | True real-time streaming of change events from any Swamp                   | 🔹 No Postgres required<br>🔹 One-line subscription              |
| **Island**    | TimescaleDB Node       | Stateless memory or disk segment — no replication config                   | 🔹 Client-side deterministic mapping<br>🔹 Edge-optimized        |

---

## Summary

While **TimescaleDB** brought time-series into the relational world, **HydrAIDE** was built for a world 
**beyond tables**, where **every write is an event**, **every record has a lifecycle**, and **logic drives data**, 
not the other way around.

You don't need `psql`, `DROP CHUNK`, `mat views`, or `job schedulers`.

Just code.

Just logic.

Just HydrAIDE.
