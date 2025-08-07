## HydrAIDE vs PostgreSQL

HydrAIDE is not a SQL alternative, it’s a **logic-native runtime** that fundamentally rethinks data interaction. 
While PostgreSQL excels at **relational consistency**, **ACID transactions**, and **powerful queries**, 
HydrAIDE delivers **real-time**, **code-native**, and **infrastructure-free** performance, optimized for developers 
building distributed, reactive systems.

PostgreSQL is an excellent choice for many traditional use cases. But when your system demands:

* O(1) access to specific entries without SQL overhead
* subscription-based reactivity instead of polling
* zero-index configuration and memory-driven sorting
* or filesystem-native persistence for edge and embedded environments

HydrAIDE offers a dramatically faster, leaner, and developer-aligned approach. no schema, no queries, no daemons, no replication setup.

HydrAIDE doesn’t replace SQL. It eliminates the need for it, by making **logic the schema**.

---

## Feature comparison

| Feature                  | HydrAIDE                                 | PostgreSQL                                        |
| ------------------------ |------------------------------------------| ------------------------------------------------- |
| 🔍 Querying model        | ✅ Structure-first (Swamps + event logic) | ❌ Query-based (SQL required)                      |
| 🧠 Memory-first design   | ✅ Hydrates only on access                | ❌ Disk-bound with shared buffer logic             |
| 🔄 Built-in reactivity   | ✅ Native `Subscribe()` logic             | ❌ Requires triggers + NOTIFY + polling            |
| ⚙️ Indexing              | ✅ Memory-only, ephemeral, zero-config    | ❌ Manual index creation + query planner           |
| 🔐 Locking model         | ✅ Per-Treasure, O(1) scoped              | ❌ Row/table/page-level locks, possible contention |
| 🧹 Cleanup               | ✅ Zero-waste (auto unload, TTL, GC)      | ❌ Requires cron jobs, VACUUM, triggers            |
| 📦 Data storage          | ✅ Binary GOB + optional metadata         | ❌ Typed rows stored as disk blocks                |
| 🌐 Scaling               | ✅ Deterministic sharding, no coordinator | ❌ Requires pgpool / logical replication           |
| 🤖 Copilot compatibility | ✅ Fully code-driven, type-safe structs   | ⚠️ Partial: SQL metadata needs reverse parsing    |
| 🧗 Learning curve        | 🟢 Zero-to-Hero in 1 day                 | 🟡 Requires SQL, indexes, relational models       |
| ⚡ Developer Experience   | ✅ Code = behavior, no schema             | ⚠️ Separate schema, config, queries               |
| 🧰 CLI/UI required?      | ✅ Optional CLI / zero shell needed       | ❌ psql, pgAdmin, SQL tools required               |
| 🐳 Install simplicity    | ✅ Single binary or Docker                | ⚠️ Needs Postgres + extensions + psql setup       |

---

## Terminology Comparison

| HydrAIDE Term | PostgreSQL Equivalent      | Explanation                                                                                       | HydrAIDE-Native Improvement                                                         |
| ------------- | -------------------------- | ------------------------------------------------------------------------------------------------- | ----------------------------------------------------------------------------------- |
| **Swamp**     | Table                      | A lightweight, memory-hydrated data unit; replaces both table and index scope                     | 🔹 No DDL needed<br>🔹 O(1) folder access<br>🔹 Subscription-aware by default       |
| **Treasure**  | Row                        | A Go struct stored as binary under a key; all logic happens per Treasure                          | 🔹 Binary storage<br>🔹 Metadata support<br>🔹 No parsing needed                    |
| **Key**       | Primary Key                | A string or typed key defined via tag — not enforced via constraints                              | 🔹 Fully developer-controlled<br>🔹 Used for locking, TTL, indexing                 |
| **Content**   | Row fields / columns       | A `hydraide:"value"` field: any Go struct, slice, or primitive                                    | 🔹 Compact serialization<br>🔹 Omit empty fields<br>🔹 Struct = business logic      |
| **Hydra**     | PostgreSQL server          | The HydrAIDE runtime that maps Swamps → memory → filesystem, and executes logic deterministically | 🔹 Zero-idle CPU<br>🔹 No query engine<br>🔹 No active background process           |
| **Zeus**      | `pg_ctl`, Postgres manager | Handles startup, folder mapping, and server hydration                                             | 🔹 Auto recovery<br>🔹 Hash-based routing<br>🔹 Works even on embedded edge devices |
| **Island**    | Partition / tablespace     | A memory- and disk-isolated unit — maps Swamps to folders or disks                                | 🔹 Predictable scaling<br>🔹 Stateless clients<br>🔹 No query planner needed        |
| **Beacon**    | Index                      | In-memory index per Swamp, created dynamically when needed (e.g., reverse scan by `createdAt`)    | 🔹 Auto-discarded<br>🔹 Event-aware<br>🔹 Memory only                               |
