## HydrAIDE vs ClickHouse

HydrAIDE isn’t built to compete with ClickHouse, it’s built to replace **the need for it** in most modern application stacks.

ClickHouse is optimized for analytical workloads, especially OLAP-style aggregations over columnar data. 
It’s incredibly fast, when you know what you’re doing, when your schema is prepared, and when your data 
volumes justify its complexity.

But HydrAIDE offers something fundamentally different: a **real-time, event-native**, 
**logic-first data engine** that aligns directly with application behavior, without any infrastructure, 
schema planning, or index orchestration.

If your system needs:

* low-latency, subscription-driven processing,
* per-record TTL and real-time cleanup,
* safe concurrent writes with native event tracking,
* or multi-tenant sharding without replica sets,

then HydrAIDE becomes the **simpler, more reactive, and code-native** choice. It serves as both a storage and a 
behavior layer, fusing pub/sub, state logic, and reactivity into a single composable runtime.

ClickHouse is an analytical beast. HydrAIDE is a **developer-aligned engine** that makes one irrelevant.

---

## Feature Comparison

| Feature                 | HydrAIDE                                       | ClickHouse                                   |
| ----------------------- |------------------------------------------------| -------------------------------------------- |
| 🧠 Model                | ✅ Code-first (Swamps + struct logic)           | ❌ Schema-first (DDL, optimized column types) |
| 🟢 Memory Hydration     | ✅ On-demand (event-triggered)                  | ⚠️ Needs memory tuning, preload configs      |
| 🔄 Built-in reactivity  | ✅ Native Subscribe + TTL                       | ❌ External event tools needed                |
| ⏳ Cleanup & Expiry      | ✅ Auto-expire with CatalogShiftExpired         | ❌ TTL needs manual setup and is non-reactive |
| ⚙️ Indexing             | ✅ Ephemeral, memory-local, zero config         | ⚠️ Heavy planning, persistent indices        |
| 🧩 Storage Model        | ✅ binary encoded structs, typed + compact      | ❌ Columnar storage, compressed               |
| 🛡️ Concurrency Model   | ✅ Per-Treasure, isolated + lock-free           | ⚠️ Block-level contention                    |
| 🔍 Lookups & Filters    | ✅ O(1) via Key/Swamp, no query engine          | ✅ Fast when indexed                          |
| ⚡ Real-Time Streams     | ✅ Subscriptions + Reverse Index out of box     | ❌ External sinks (Kafka, RabbitMQ, etc.)     |
| 🔐 Permissions          | ✅ File-based isolation, TLS auth               | ❌ SQL + RBAC (manual config)                 |
| 🌍 Scaling              | ✅ Folder-level sharding, stateless client      | ⚠️ Manual clusters, config-heavy             |
| 🧰 Tooling              | ✅ CLI optional, no shell needed                | ❌ Requires CLI/SQL shell                     |
| 🚀 Developer Experience | ✅ 1-day onboarding, config-free, deterministic | ⚠️ Steep setup for non-analytical cases      |

---

## Terminology Comparison

| HydrAIDE Term | ClickHouse Equivalent    | Explanation                                                                                          | HydrAIDE-Native Advantage                                             |
| ------------- | ------------------------ | ---------------------------------------------------------------------------------------------------- | --------------------------------------------------------------------- |
| **Swamp**     | Table (logical or shard) | A Swamp is a logical group of Treasures. Unlike tables, it doesn’t require schema or DDL.            | 🔹 Auto-hydrated 🔹 No migrations 🔹 Behavior = Name-based            |
| **Treasure**  | Row / Record             | A single typed struct stored as a binary blob. No columnar separation.                               | 🔹 Full struct stored as-is 🔹 Real-time mutations / subscriptions    |
| **Catalog**   | Indexed table            | A key-value list with typed metadata and per-record TTL, filtering, and streaming.                   | 🔹 Reactive lists 🔹 TTL-native 🔹 Filter/sort/index without SQL      |
| **Profile**   | Single entity object     | A Profile stores all fields of a given object (e.g. user) in a single place — like denormalized row. | 🔹 Struct = state 🔹 O(1) hydration 🔹 Fully overwriteable / lockable |
| **Beacon**    | Index                    | Memory-only indexing per Swamp, used for sorting/filtering.                                          | 🔹 Auto-discarded 🔹 No global config 🔹 Reactive filtering           |
| **Hydra**     | ClickHouse Server        | The core engine. No config, no daemons, just runs.                                                   | 🔹 Zero idle 🔹 Named folder = location 🔹 File-based isolation       |
| **Zeus**      | Supervisor tools         | Controls startup, health checks, Swamp hydration.                                                    | 🔹 Auto-restart 🔹 Pattern-based boot logic 🔹 Recovery aware         |
| **Island**    | Node / Shard             | Each Island represents a distributed storage space, mapped from name hash.                           | 🔹 No metadata needed 🔹 Client-side routing 🔹 Stateless scaling     |
| **Subscribe** | Kafka/Materialize/Sinks  | Native reactive pub-sub layer — no brokers needed.                                                   | 🔹 No middleware 🔹 Typed events 🔹 TTL-driven event triggering       |

---

## Summary

ClickHouse is great when you need to scan billions of rows with millisecond aggregation over pre-modeled datasets. 
But when your use case is **application behavior**, **user actions**, **real-time state**, or **per-record logic**, 
ClickHouse becomes overkill.

HydrAIDE handles:

* time-based queues,
* chat message streams,
* reverse indexes,
* per-user rate limits,
* slice mutations,
* and real-time filtering

in a **single binary runtime**, without any SQL, brokers, or configuration files.

**ClickHouse is analytics.**
**HydrAIDE is behavior.**

Choose your engine based on what your system *does*, not just what it *stores*.
