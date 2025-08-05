## HydrAIDE vs InfluxDB

HydrAIDE is not a time-series database, but it often replaces InfluxDB in systems where real-time 
behavior, **structured intent**, and **memory-efficiency** matter more than raw timestamp-based metrics. 
While InfluxDB excels at **append-only, time-ordered data**, HydrAIDE introduces a **developer-centric**, 
**zero-bloat** engine that merges real-time event streaming, memory-sharded state, and 
safe mutation into a logic-first design.

InfluxDB is optimized for metrics — but HydrAIDE is optimized for **behavioral state** and **intent modeling**. 
When your system needs:

* fine-grained real-time mutation,
* zero-query reactive patterns,
* strongly typed logic in code,
* or atomic TTL-driven cleanup,

then HydrAIDE becomes a cleaner, leaner, and more **composable** solution — often **replacing both** 
the data store *and* the pub-sub layer.

HydrAIDE isn’t just an InfluxDB alternative, it’s an engine that **thinks like your system**, not just stores what happened.

---

## Feature Comparison

| Feature                    | HydrAIDE                                         | InfluxDB                                    |
| -------------------------- |--------------------------------------------------| ------------------------------------------- |
| 🔁 Reactive Subscriptions  | ✅ Native events, no brokers                      | ⚠️ Requires polling or Flux joins           |
| ⏱️ Time support            | ✅ Optional `expireAt`, `createdAt` metadata      | ✅ Built-in timestamp schema                 |
| 🧠 Logic model             | ✅ Developer intent (Go structs, naming)          | ❌ Query-first, schema-bound                 |
| 🔐 Concurrency             | ✅ Per-record locking, atomic slices              | ⚠️ Manual care needed for concurrent writes |
| 🧹 TTL / Auto-deletion     | ✅ Per-record `expireAt` + `ShiftExpired()`       | ⚠️ Retention policy per-bucket              |
| 📦 Data format             | ✅ Typed binary (`GOB`)                           | ❌ Text-based or compressed line protocol    |
| 💾 Write pattern           | ✅ Append, mutate, TTL-delete, overwrite          | ✅ Append-only (updates via retention)       |
| 📊 Indexing model          | ✅ In-memory Beacon per-Swamp (time indexed too)  | ⚠️ Time-indexed, limited field indexing     |
| 💬 Query complexity        | ❌ None: queries replaced by structure            | ⚠️ Flux/InfluxQL required                   |
| 🔌 Dependencies            | ✅ Single binary, zero infra                      | ❌ Requires InfluxD, Telegraf, UI etc.       |
| 🛠️ Learning curve         | 🟢 1-day onboarding                       | 🟡 Flux/Telegraf/Schema setup required      |
| ⚙️ Real-time logic support | ✅ `Subscribe()`, `ShiftExpired()`, `Increment()` | ⚠️ Manual joins / no native reactions       |
| 🧰 Tooling required?       | ✅ Optional only (CLI/SDK)                        | ❌ Admin UI and configuration needed         |
| 🧠 Thinking Model          | ✅ “Structure = Behavior”                         | ❌ “Schema = Queryability”                   |

---

## Terminology Comparison

| HydrAIDE Term    | InfluxDB Equivalent       | Explanation                                                                      | HydrAIDE-Native Improvement                                                |
| ---------------- | ------------------------- | -------------------------------------------------------------------------------- | -------------------------------------------------------------------------- |
| **Swamp**        | Bucket                    | Logical and physical container for data. Named deterministically.                | 🔹 Isolated, file-based, TTL-aware <br> 🔹 Zero query cost                 |
| **Treasure**     | Point (record)            | Single data unit. Can be binary struct, slice, or primitive.                     | 🔹 Type-safe <br> 🔹 Metadata-rich <br> 🔹 Directly tied to event triggers |
| **Key**          | Measurement/Tag Key       | Unique ID of each Treasure (e.g., `device-123`).                                 | 🔹 No tag hierarchy required <br> 🔹 Used for TTL, locking, indexing       |
| **ExpireAt**     | Retention policy          | Per-record expiration timestamp, not global bucket policy.                       | 🔹 Full control per entry <br> 🔹 `CatalogShiftExpired()` for deletion     |
| **Catalog**      | Time-series table (TSM)   | Grouped records under a Swamp, can be indexed, read, filtered, streamed.         | 🔹 Full slice ops, `CatalogReadMany()` with custom `Index` type            |
| **Beacon**       | Index (partial)           | Memory-resident index scoped per Swamp, created on-demand.                       | 🔹 Auto-destroyed, zero config <br> 🔹 Compatible with `Subscribe()`       |
| **Subscribe()**  | N/A (manual Flux polling) | Real-time event listener on data insert/update/delete.                           | 🔹 Push-model with no brokers <br> 🔹 Native pub/sub                       |
| **Increment()**  | N/A (manual logic)        | Atomic mutation of numeric values without loading.                               | 🔹 Zero-race condition logic <br> 🔹 Server-side guarantees                |
| **CatalogQueue** | Stream + TTL              | Queue logic with time-windowed activation (via `ExpireAt`) and safe consumption. | 🔹 `ShiftExpired()` triggers live cleanup and `StatusDeleted` events       |

---

## Summary

InfluxDB is powerful for **metric snapshots and analytics**, but HydrAIDE is designed for 
**logic, intent, and behavior-first systems**. When you need:

* Memory-tuned, reactive slices
* Per-record event streams
* TTL-driven task queues
* Safe concurrent mutations
* And code that expresses logic directly (no queries, no polling)

HydrAIDE becomes not just an alternative, but a **rethink** of how data should behave.
