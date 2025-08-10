## HydrAIDE vs ArangoDB

HydrAIDE isn’t just an alternative to ArangoDB — it’s a **logic-native execution engine** with 
**real-time subscription support**, **zero-idle performance**, and **fully embedded coordination logic**. 
While ArangoDB positions itself as a multi-model database (document, graph, key-value), HydrAIDE simplifies 
everything into **Swamps**, **typed Treasures**, and **event-aware operations** that feel like 
writing regular code — not queries.

ArangoDB is powerful in hybrid use cases (e.g. documents + graphs), but if you’re building:

* logic-first systems,
* low-latency reactive apps,
* auto-expiring data without cleanup jobs,
* or edge-optimized infrastructure-free deployments,

HydrAIDE removes the database overhead entirely, giving you **code-native data orchestration**, 
per-Treasure locking, memory-first execution, and deterministic data routing without replicas or configuration servers.

HydrAIDE doesn’t aim to be “multi-model”. It aims to **eliminate the model** entirely, and just run your logic.

---

## Feature Comparison

| Feature                    | HydrAIDE                                     | ArangoDB                                     |
| -------------------------- |----------------------------------------------| -------------------------------------------- |
| 🔍 Querying model          | ✅ Structure-first (Swamps + Go logic)        | ⚠️ Query-heavy with AQL                      |
| 🔄 Built-in reactivity     | ✅ Native subscriptions, event streams        | ❌ Polling or custom change streams           |
| 🧠 Memory-first design     | ✅ Hydrates Swamps on access                  | ❌ Disk-centric with memory cache             |
| ⚙️ Indexing                | ✅ In-memory, automatic                       | ⚠️ Requires index declaration and tuning     |
| 🔐 Locking model           | ✅ Per-key FIFO distributed locks             | ❌ No built-in distributed lock               |
| ⏳ Data expiry              | ✅ Native TTL at Treasure level               | ⚠️ Needs TTL index or custom cleanup logic   |
| 🌐 Scaling                 | ✅ Client-side deterministic sharding         | ❌ Requires cluster coordinator & agents      |
| 📦 Storage format          | ✅ GOB-encoded Go structs (binary, compressed) | ❌ JSON-based documents                       |
| 👩‍💻 Developer experience | ✅ Zero YAML, native code                  | ⚠️ Requires query DSL, setup & schema        |
| ⚡ Speed focus              | ✅ Event-first, real-time, lock-free writes   | ⚠️ Mixed performance depending on query type |
| 🧰 CLI/UI required?        | ✅ Optional, not required                     | ⚠️ Web UI often needed for setup/debug       |
| 🐳 Install simplicity      | ✅ Single binary or Docker                    | ❌ Multiple binaries or coordination agents   |
| 🧗 Learning curve          | 🟢 Logic-based, 1-day onboarding             | 🟡 Requires AQL + model understanding        |

---

## Terminology Comparison

| HydrAIDE Term | ArangoDB Equivalent        | Explanation                                                                                                                       | HydrAIDE-Native Improvement                                              |
| ------------- | -------------------------- | --------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------ |
| **Swamp**     | Collection                 | Logical container for typed Treasures. Not schema-bound. Memory-hydrated on demand.                                               | 🔹 Zero setup <br> 🔹 Memory-hydrated <br> 🔹 Naming = behavior          |
| **Treasure**  | Document / Vertex / Edge   | A typed Go struct, stored natively as a binary unit. Includes metadata and lifecycle fields.                                      | 🔹 Native TTL <br> 🔹 Per-key event streaming <br> 🔹 Structs = behavior |
| **Hydra**     | arangod                    | Runtime engine managing Swamps. Unlike arangod, Hydra is embedded, single-binary, and event-driven with zero idle CPU.            | 🔹 Filesystem-based <br> 🔹 No idle <br> 🔹 Fully reactive               |
| **Zeus**      | N/A (or agent+coordinator) | Manages boot logic, health, and recovery. Not needed in most Arango setups unless clustered.                                      | 🔹 Auto-restart <br> 🔹 Deterministic Swamp hydration                    |
| **Island**    | Shard                      | Partition unit. In HydrAIDE, sharding is done client-side by name. No cluster config, no balancing needed.                        | 🔹 Stateless scaling <br> 🔹 Zero config sharding                        |
| **Beacon**    | Index                      | Memory-only scoped indexes per Swamp. Automatically discarded when memory is freed.                                               | 🔹 O(1) scans <br> 🔹 Subscribable <br> 🔹 Auto-cleanup                  |
| **Lock**      | Custom logic (external)    | HydrAIDE offers per-key distributed locks out of the box — no Redis or etcd required.                                             | 🔹 FIFO lock queues <br> 🔹 TTL-based <br> 🔹 Logic-first locking        |
| **Subscribe** | Change Streams (manual)    | In HydrAIDE, any write emits an event stream by default. Subscriptions are code-native, stream-compatible, and tied to Treasures. | 🔹 No brokers <br> 🔹 No polling <br> 🔹 Auto-callbacks in Go            |
