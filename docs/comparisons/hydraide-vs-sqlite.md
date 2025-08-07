## HydrAIDE vs SQLite

HydrAIDE isn’t just a more modern engine than SQLite, it’s a different **thinking model** entirely.
Where SQLite focuses on **file-based SQL databases** and transaction safety, HydrAIDE brings 
a **logic-native**, **event-driven**, and **zero-coordination** runtime, 
optimized not just for reads/writes, but for **intent**.

SQLite is powerful for embedded use cases, but when your system needs:

* live subscriptions and real-time updates,
* file-free distributed sharding,
* per-record event streaming,
* or strongly typed structures with binary storage,

then **HydrAIDE offers a reactive, zero-admin, low-latency** alternative, **even for local apps**.

> ✅ HydrAIDE runs equally well in SQLite’s target environments (single-process, embedded systems, Dockerized services),
> but adds powerful features like TTL, reactive logic, and distributed Swamp hydration, without requiring SQL, 
> schemas, or migrations.

---

## Feature comparison

| Feature                 | HydrAIDE                              | SQLite                                      |
| ----------------------- | ------------------------------------- | ------------------------------------------- |
| 📦 Storage model        | ✅ Swamps with typed binary Treasures  | ❌ Table-based, SQL-driven B-tree files      |
| ⚡ Query mechanism       | ✅ Zero-query: structure = access      | ❌ Query-first (SQL with indexes)            |
| 🔄 Reactivity           | ✅ Native `Subscribe()` support        | ❌ No built-in event or pub-sub mechanism    |
| 🔐 Locking              | ✅ Per-Treasure, deadlock-free logic   | ❌ Table/file-level locking with contention  |
| 🧠 Code integration     | ✅ Struct-first, type-safe Go code     | ❌ Requires SQL + field mapping manually     |
| 🧹 Auto-cleanup (TTL)   | ✅ Built-in `expireAt` + auto-delete   | ❌ Manual deletes or cron needed             |
| 🧬 Data structure model | ✅ No schema, just Go types            | ❌ Schema must be defined manually           |
| 🪶 Binary performance   | ✅ GOB-encoded, zero-marshaling        | ⚠️ Text/SQL parsing overhead                |
| 🚦 Concurrency          | ✅ Safe by design, per-Swamp hydration | ⚠️ Write locks needed, especially on disk   |
| 🔎 Indexing             | ✅ In-memory, ephemeral Beacons        | ❌ Manual index creation required            |
| 🧰 CLI/UI Required      | ✅ Optional tooling only        | ❌ CLI/tools needed for schema/debug         |
| 🧗 Learning curve       | 🟢 Zero-to-Hero in 1 day              | 🟡 Requires SQL fluency and migration logic |
| 📊 Analytics model      | ✅ Catalogs + reactive indexing        | ❌ Manual queries and joins required         |
| 🐳 Embedded ready       | ✅ Single binary, fileless if needed   | ✅ Lightweight .db file                      |

---

## Terminology Comparison

| HydrAIDE Term | SQLite Equivalent    | Explanation                                                                                                    | HydrAIDE-Native Improvement                                                               |
| ------------- | -------------------- | -------------------------------------------------------------------------------------------------------------- | ----------------------------------------------------------------------------------------- |
| **Swamp**     | Table/File           | Logical container of Treasures. Similar to a table or a file, but not schema-bound. Created instantly by name. | 🔹 Auto-created <br> 🔹 Memory-hydrated only when used <br> 🔹 Zero-file mode possible    |
| **Treasure**  | Row                  | A key–value data unit. Strongly typed, binary, and optional metadata (createdAt, expireAt, etc.).              | 🔹 No parsing <br> 🔹 Event-emitting <br> 🔹 Used in queues, reverse indexes, rate limits |
| **Key**       | Primary key          | Defined in Go via struct tag `hydraide:"key"`. Type-safe, not tied to SQL definition.                          | 🔹 Used in TTL, locking, reverse slices <br> 🔹 Auto-indexed                              |
| **Content**   | Column(s)            | The `hydraide:"value"` part – a full struct or primitive. Only one field per Treasure – but can be nested.     | 🔹 Full structs in binary <br> 🔹 Compact GOB format <br> 🔹 No JSON parsing needed       |
| **Subscribe** | Triggers (roughly)   | Allows you to listen to changes in real time, like `INSERT`, `UPDATE`, `DELETE`. But without SQL or polling.   | 🔹 Reactive-first <br> 🔹 Native streaming logic                                          |
| **TTL**       | No direct equivalent | Built-in support for auto-expiry per Treasure using `expireAt`.                                                | 🔹 Auto cleanup <br> 🔹 Can be shifted, indexed, filtered in memory                       |
| **Hydra**     | SQLite engine        | The runtime logic that stores, indexes, and manages Swamps. No shell, no daemon.                               | 🔹 Folder-mapped logic <br> 🔹 Deterministic hashing, reactive design                     |
| **Beacon**    | Index                | A memory-only, Swamp-local index that enables fast scans, reverse lookups and reactive filtering.              | 🔹 Auto-expiring <br> 🔹 Used in `CatalogReadMany()` and stream filters                   |
