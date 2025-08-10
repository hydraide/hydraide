## HydrAIDE vs MongoDB

HydrAIDE is not just a modern alternative to MongoDB, it’s a full-fledged data engine designed around **logic-first development**, **real-time behavior**, and **zero-idle performance**. While MongoDB focuses on flexible JSON storage, HydrAIDE brings something fundamentally different: a **code-native**, **subscription-aware**, and **infrastructure-free** data layer that thinks like your application does.

MongoDB is often used as a general-purpose document store and in many use cases, it works well. But when your system needs:

* low-latency reactivity,
* zero-maintenance index logic,
* safe concurrent writes,
* or full control without shell/admin UI overhead,

then HydrAIDE offers a simpler, safer, and significantly more **developer-aligned** path. It replaces *both* 
the storage layer and the coordination logic (like pub-sub or cron-driven batch systems), in a single, composable runtime.

HydrAIDE isn't a NoSQL alternative. It's a logic-native engine that eliminates the need for one.

## Feature comparison

| Feature                  | HydrAIDE                                        | MongoDB                                 |
| ------------------------ |-------------------------------------------------| --------------------------------------- |
| 🔍 Querying model        | ✅ Structure-first (Swamps + set logic)          | ❌ Query-heavy, needs index planning     |
| 🧠 Memory-first design   | ✅ Swamps hydrate on demand                      | ❌ Primarily disk-based                  |
| 🔄 Built-in reactivity   | ✅ Native subscriptions, no brokers              | ❌ Requires Change Streams or polling    |
| ⚙️ Indexing              | ✅ In-memory, ephemeral, no config               | ❌ Static, disk-based, managed manually  |
| 🔐 Locking model         | ✅ Per-Treasure, deadlock-free                   | ❌ Global/collection locks possible      |
| 🧹 Cleanup               | ✅ Automatic, zero-waste architecture            | ❌ Requires TTL indexes, manual scripts  |
| 📦 Data storage          | ✅ Typed binary chunks, compressed and minimal   | ❌ JSON/BSON with serialization overhead |
| 🌐 Scaling               | ✅ No replica sets, client-side sharding by name | ❌ Requires replica sets and config srv  |
| 🤖 Copilot compatibility | ✅ Fully AI-readable docs and code               | ⚠️ Partial, limited type insight        |
| 🧗 Learning curve        | 🟢 Zero-to-Hero in 1 day                        | 🟡 Medium – needs schema, drivers setup |
| ⚡ Developer Experience   | ✅ Code-native, zero YAML, logic-first           | ⚠️ Setup-heavy, verbose patterns        |
| 🧰 CLI/UI required?      | ✅ Optional tools only                           | ❌ Required tooling   |
| 🐳 Install simplicity    | ✅ Single binary or Docker     | ⚠️ Multiple services, configs, shell    |

## Terminology Comparison

| HydrAIDE Term | MongoDB Equivalent     | Explanation                                                                                                                                                           | HydrAIDE-Native Improvement                                                                               |
| ------------- | ---------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------- |
| **Swamp**     | Collection             | A **Swamp** is like a MongoDB collection (e.g., `users`, `orders`), but not schema-bound. It’s lightweight, created on demand, and structured by name.                | 🔹 No schema to manage <br> 🔹 Memory is hydrated only when accessed <br> 🔹 Naming = behavior            |
| **Treasure**  | Document               | A **Treasure** is the equivalent of a document in MongoDB, representing a single unit of data. It’s a Go struct stored in binary form — no BSON or JSON needed.       | 🔹 Strongly typed Go structs <br> 🔹 O(1) access <br> 🔹 Native TTL, metadata, event streaming            |
| **Key**       | `_id` field            | Every Treasure has a key tagged via `hydraide:"key"`. Similar to MongoDB's `_id` field, but you define it through struct tags — field name is irrelevant.             | 🔹 Deterministic indexing <br> 🔹 Type-safe <br> 🔹 Used in TTL, distributed locking, reverse indexes     |
| **Content**   | Fields inside document | In HydrAIDE, the value of a Treasure is a single field tagged with `hydraide:"value"`. Can be any Go struct. Think of it as the payload part of the MongoDB document. | 🔹 Fully binary <br> 🔹 Structs = logic <br> 🔹 Compact and fast (GOB format)                             |
| **Hydra**     | mongod process         | The **Hydra** engine is the runtime that manages Swamps. Comparable to the `mongod` process, but optimized for event-driven, zero-idle, reactive execution.           | 🔹 Zero idle CPU <br> 🔹 Filesystem-based persistence <br> 🔹 No global server daemon                     |
| **Zeus**      | mongos (partially)     | **Zeus** controls the Hydra engine — like `mongos` managing mongod nodes. It handles startup, health checks, boot logic, and recovery.                                | 🔹 Auto-restart <br> 🔹 Coordinated hydration <br> 🔹 Swamp-first startup logic                           |
| **Island**    | Shard                  | **Island** is a physical/memory shard. Swamps are mapped to Islands deterministically. Similar to MongoDB sharding by shard key.                                      | 🔹 Built-in partitioning <br> 🔹 Stateless client logic <br> 🔹 Ideal for edge / ephemeral deployments    |
| **Beacon**    | Index                  | A **Beacon** is like a per-collection index. It allows sorting/filtering inside a Swamp. However, it’s memory-only and scoped per Swamp — no global indexes.          | 🔹 O(1) reverse scan <br> 🔹 Event-aware (Beacon + Subscribe) <br> 🔹 Auto-discarded when Swamp is closed |
