## HydrAIDE vs Neo4J

HydrAIDE is not a graph database. It’s a **logic-native**, **real-time**, and **zero-infrastructure** data engine 
built for **code-first developers** who want full control over structure, performance, and reactivity. 
While Neo4J shines at graph traversal and relationship-based modeling (Cypher, nodes, edges), HydrAIDE takes a 
fundamentally different approach: **structured behavior through naming**, **native Go types**, and **event-based operations**.

Neo4J is excellent for query-heavy, relationship-centric data. But when your system needs:

* native code-bound structure,
* real-time mutation streams,
* distributed stateless logic,
* or predictable performance without tuning,

HydrAIDE provides a radically simpler and **event-oriented** path, where the logic **is** the structure,
and the structure **is** the runtime.

It’s not just "faster than Neo4J", it’s a completely different model: **intent > graph**, **structure > traversal**.

---

## Feature Comparison

| Feature                  | HydrAIDE                                   | Neo4J                                     |
| ------------------------ |--------------------------------------------| ----------------------------------------- |
| 🧠 Data Model            | ✅ Named Swamps + Go structs (typed logic)  | ❌ Graph nodes/edges require schema config |
| 🔄 Built-in reactivity   | ✅ Native event emitters (Subscribe)        | ⚠️ Requires triggers or polling           |
| 🧱 Relationship modeling | ✅ Pattern-based with deterministic mapping | ✅ Native via edges and Cypher             |
| ⚡ Read performance       | ✅ O(1) access per key                      | ❌ Depends on traversal/query cost         |
| 📦 Storage format        | ✅ GOB-binary Go types                      | ❌ Serialized as property graph structures |
| 🔐 Concurrency           | ✅ Lock-free write, per-Treasure mutex      | ⚠️ Can require transaction retries        |
| 🔍 Indexing              | ✅ Ephemeral, scoped, memory-only           | ❌ Manual config or schema-based indexes   |
| 🧹 Cleanup / TTL         | ✅ Built-in per-record TTL, auto-deletion   | ⚠️ Requires manual cleanup or procedures  |
| 📡 Subscriptions         | ✅ Native, per-key or per-Swamp             | ⚠️ Not native; needs APOC or bolt hooks   |
| 🗺️ Distribution logic   | ✅ Stateless, name-hashed Swamp mapping     | ❌ Enterprise features only (Fabric)       |
| 🧗 Learning Curve        | 🟢 1 day zero to hero                     | 🟡 Medium – needs Cypher, model thinking  |
| 🔧 Schema Migration      | ✅ None needed — logic defines structure    | ❌ Requires schema/versioning if strict    |
| 🧰 CLI / UI              | ✅ Optional tools only                      | ❌ Desktop UI often needed (Neo4J Browser) |
| 🧬 Data Inspection       | ✅ In-code via structs, events, TTL         | ⚠️ Needs UI or Cypher                     |
| ⚙️ Developer Flow        | ✅ Logic-native, zero-query mindset         | ⚠️ Graph-first, needs query translations  |

---

## Terminology Comparison

| HydrAIDE Term | Neo4J Equivalent    | Explanation                                                                             | HydrAIDE-Native Improvement                                          |
| ------------- | ------------------- | --------------------------------------------------------------------------------------- | -------------------------------------------------------------------- |
| **Swamp**     | Node set or label   | Logical + physical group of Treasures — named by structure and behavior                 | 🔹 Hydrated only when needed <br> 🔹 File-mapped + deterministic     |
| **Treasure**  | Node / Edge         | Binary-encoded record, always typed (Go struct), addressable by key                     | 🔹 No serialization overhead <br> 🔹 TTL, locking, indexing built-in |
| **Key**       | Internal ID / `id`  | Defined manually via struct tag — not auto-generated, not hidden                        | 🔹 Fully deterministic <br> 🔹 Can be used for locking, TTL          |
| **Hydration** | Cache / materialize | Swamp loaded into memory based on access, not kept open permanently                     | 🔹 Predictable memory footprint <br> 🔹 Auto-close + garbage collect |
| **Subscribe** | Trigger/listener    | Built-in mechanism to listen to changes (create, update, delete) in Swamps or Treasures | 🔹 Real-time streaming <br> 🔹 No brokers or polling needed          |
| **Beacon**    | Index               | Memory-only sorted index per Swamp (not persisted)                                      | 🔹 Reverse scan + discard on unload <br> 🔹 No global coordination   |
| **Island**    | Cluster shard       | Folder-based physical partition — Swamps are hashed and routed per folder               | 🔹 Stateless clients <br> 🔹 No Fabric, no config overhead           |
| **Zeus**      | Query router        | Controls HydrAIDE server startup, hydration, fault recovery                             | 🔹 No config, no shell, just Go code                                 |
