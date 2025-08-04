## HydrAIDE vs Redis

HydrAIDE is not just a drop-in key-value store — it's a **logic-native**, **event-driven**, and **infrastructure-free**
data engine that matches Redis in raw speed, but goes far beyond in structure and behavior.
With **in-memory-only Swamps**, **O(1) lookups**, and **binary-encoded models**, HydrAIDE delivers **Redis-class performance** 
for caching scenarios **without sacrificing type safety, structure, or lifecycle logic**.

Redis is widely used for:

* caching,
* queues,
* simple data coordination.

And it's fast, no doubt.  But HydrAIDE is fast **and** structured. When your application needs:

* strongly typed, binary-safe structured data,
* logic-aware TTL and automatic cleanup,
* native pub/sub tied directly to data change,
* per-record locking or concurrent writes,

then HydrAIDE offers a **more aligned**, **developer-native**, and **infra-light** path.
It’s not “faster Redis”, it’s a new class: a **code-native runtime** that behaves like your system,
while still letting you build high-speed caches, queues, and coordination logic, just with a lot more intelligence.

## Feature comparison

| Feature                  | HydrAIDE                                  | Redis                                    |
| ------------------------ | ----------------------------------------- | ---------------------------------------- |
| 🔍 Querying model        | ✅ Structured Swamps + typed entries       | ⚠️ Key-oriented, manual namespacing      |
| 🧠 Memory-first design   | ✅ Hydrates only what's needed             | ✅ Everything lives in memory             |
| 🔄 Built-in reactivity   | ✅ Native subscriptions by key or pattern  | ⚠️ Requires Redis Streams or Pub/Sub     |
| ⚙️ Indexing              | ✅ Value + metadata, in-memory, TTL-aware  | ❌ Manual key scanning or external index  |
| 🔐 Locking model         | ✅ Per-record locking with TTL fallback    | ⚠️ Only primitive locks (SETNX + Lua)    |
| 🧹 Cleanup               | ✅ Automatic via TTL, hydration logic      | ⚠️ Manual EXPIRE or key naming patterns  |
| 📦 Data storage          | ✅ Typed, binary chunks (GOB)              | ❌ Strings, blobs, manual serialization   |
| 🌐 Scaling               | ✅ Client-side sharding, no config servers | ⚠️ Requires Redis Cluster setup          |
| 🤖 Copilot compatibility | ✅ Full type reflection                    | ⚠️ Poor insight, everything is `string`  |
| 🧗 Learning curve        | 🟢 Zero-to-Hero in 1 day              | 🟡 Simple to start, complex to scale     |
| ⚡ Developer Experience   | ✅ Code-first, type-safe SDK               | ⚠️ Scripting-based, verbose coordination |
| 🧰 CLI/UI required?      | ✅ Optional tooling only                   | ⚠️ CLI almost mandatory for ops/debug    |
| 🐳 Install simplicity    | ✅ Single binary or Docker                 | ⚠️ External dependencies for full stack  |

## Terminology Comparison

| HydrAIDE Term | Redis Equivalent      | Explanation                                                                                           | HydrAIDE-Native Improvement                                               |
| ------------- | --------------------- | ----------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------- |
| **Swamp**     | Redis Key Prefix      | A logical, hydrated data set — not just a name prefix. Stored as a physical folder, loaded on demand. | 🔹 Real structure <br> 🔹 TTL-aware <br> 🔹 Auto-partitioned by name      |
| **Treasure**  | Redis Key or Hash     | A single data unit with typed value and optional metadata like `createdAt`, `expireAt`.               | 🔹 Type-safe <br> 🔹 Binary model <br> 🔹 Triggers events natively        |
| **Key**       | Redis key string      | Used to identify a Treasure, but structured via Go tags — not dependent on string patterns.           | 🔹 Safer, deterministic <br> 🔹 No string parsing logic                   |
| **Content**   | Redis value/string    | In HydrAIDE, it’s a typed Go struct. In Redis, it's usually a string or blob.                         | 🔹 Compact GOB <br> 🔹 No need for JSON or custom parsing                 |
| **Hydra**     | Redis server process  | HydrAIDE runtime — memory-reactive, non-daemon, file-based.                                           | 🔹 No global state <br> 🔹 Zero idle CPU <br> 🔹 File-based streaming     |
| **Zeus**      | N/A                   | Boot coordinator and health manager. Redis doesn't have a separate boot-layer.                        | 🔹 Structured start flow <br> 🔹 Live recovery support                    |
| **Island**    | Redis Shard/Slot      | Physical host determined by folder hash. Stateless from client view.                                  | 🔹 No slots <br> 🔹 Deterministic hashing <br> 🔹 Serverless client logic |
| **Beacon**    | Redis Sorted Set/Scan | HydrAIDE's native, per-Swamp indexing mechanism (not global). Redis uses Sorted Sets or range scans.  | 🔹 Reactive <br> 🔹 TTL-aware <br> 🔹 Auto-destroyed with Swamp           |
