## HydrAIDE vs Elasticsearch

Elasticsearch is a **document-first**, full-text search engine designed for indexing and querying JSON payloads.
HydrAIDE, on the other hand, is a **logic-native**, **event-driven**, and **intent-aware** runtime designed 
for structured, type-safe, low-latency storage, with **zero orchestration**.

While Elasticsearch shines in log aggregation and fuzzy search, HydrAIDE is built for:

* reactive stateful systems,
* binary storage of strongly typed values,
* low-latency event subscriptions,
* and seamless, file-free distributed sharding.

> ✅ Unlike Elasticsearch, **HydrAIDE does not need a coordinator or master node**. Sharding is deterministic, 
> based on the Swamp name hash.  This allows embedded or Dockerized deployments without orchestration, 
> and ensures **predictable horizontal scaling**, even across volatile nodes.

---

## Feature comparison

| Feature                | HydrAIDE                               | Elasticsearch                             |
| ---------------------- |----------------------------------------| ----------------------------------------- |
| 📦 Storage model       | ✅ Typed Treasures inside Swamps        | ❌ JSON document-based inverted indexes    |
| ⚡ Query mechanism      | ✅ Zero-query: `structure = access`     | ❌ Requires Lucene DSL or Kibana queries   |
| 🔄 Reactivity          | ✅ Built-in `Subscribe()` streams       | ⚠️ Requires polling or custom hooks       |
| 🔐 Locking             | ✅ Per-key, cross-service lock API      | ❌ No native lock or atomic lock support   |
| 🧠 Code integration    | ✅ Native structs (type-safe)           | ⚠️ Mapping logic for JSON/doc required    |
| 🧹 Auto-cleanup (TTL)  | ✅ Native `expireAt` + `ShiftExpired()` | ⚠️ Background merge policy (slow cleanup) |
| 🧬 Data model          | ✅ Structs with binary encoding         | ❌ Schema-less, text-optimized documents   |
| 🪶 Binary performance  | ✅ Zero-marshaling binary                  | ❌ Text-based parsing and compression      |
| 🚦 Sharding            | ✅ Hash-based Swamp routing             | ⚠️ Manual config, shard balancing needed  |
| 🧰 Orchestrator needed | ✅ Optional tooling only         | ❌ Requires master nodes, replicas, Kibana |
| 🧗 Learning curve      | 🟢 Zero-to-Hero in 1 day               | 🟡 Elastic DSL, analyzers, mappings       |
| 📊 Analytics model     | ✅ `CatalogReadMany()` + live filtering | ✅ Rich dashboards, but higher overhead    |
| 🐳 Embedded ready      | ✅ Single binary, fileless if needed    | ❌ JVM-based, heavy setup                  |

---

## Use case comparison

| Use Case                      | HydrAIDE                                      | Elasticsearch                         |
| ----------------------------- | --------------------------------------------- | ------------------------------------- |
| ✅ Reactive pub/sub system     | Native `Subscribe()` + TTL + event types      | ⚠️ Needs Webhook + sidecar            |
| ✅ Embedded queue or stream    | `CatalogShiftExpired()` + event callbacks     | ❌ Not designed for real-time queues   |
| ✅ Type-safe state storage     | GOB-encoded structs, no parsing               | ❌ JSON only, typed logic externalized |
| ✅ Lock-based coordination     | `Lock()/Unlock()` API                         | ❌ Not supported                       |
| ✅ Distributed write by key    | Swamp-hash-based server targeting (no config) | ⚠️ Requires routing rules + replicas  |
| ✅ Domain event hydration      | Live reverse lookup + TTL auto-delete         | ⚠️ Needs join/aggregation logic       |
| ✅ Fileless embedded operation | No index files, no daemons, runs in-memory    | ❌ Disk + JVM + index management       |

---

## Terminology comparison

| HydrAIDE Term   | Elasticsearch Equivalent | Explanation                                                                            | HydrAIDE-Native Improvement                                                                      |
| --------------- | ------------------------ | -------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------ |
| **Swamp**       | Index + Type             | Logical container for Treasures. Created instantly by name, no schema or provisioning. | 🔹 Auto-created <br> 🔹 In-memory hydration <br> 🔹 Shard-free and zero-file mode support        |
| **Treasure**    | Document                 | A binary, typed data record. One key–value per unit, optionally with metadata.         | 🔹 GOB-encoded <br> 🔹 Strongly typed <br> 🔹 Emits events <br> 🔹 TTL, locking, streaming ready |
| **Key**         | Document ID              | Unique identifier, defined via `hydraide:"key"` tag in Go struct.                      | 🔹 Used in sharding, TTL, reverse indexing, locking <br> 🔹 Auto-indexed                         |
| **Content**     | `_source`                | The `hydraide:"value"` field — the payload of a Treasure. Full struct or primitive.    | 🔹 Binary storage <br> 🔹 No parsing needed <br> 🔹 Can be nested Go structs                     |
| **Hydra**       | Elasticsearch Engine     | Runtime logic behind Swamp hydration, event processing, and routing.                   | 🔹 Stateless client <br> 🔹 Deterministic name-based sharding <br> 🔹 No config, no daemon       |
| **Beacon**      | Inverted Index           | Ephemeral in-memory filter created via code, not schema. Used in `CatalogReadMany()`.  | 🔹 No index config <br> 🔹 Auto-expires <br> 🔹 Used in analytics and filtered reads             |
| **Subscribe**   | Watcher / Webhook        | Reactive event stream for `INSERT`, `UPDATE`, `DELETE` — no polling or DSL.            | 🔹 Real-time <br> 🔹 Built-in <br> 🔹 No Kibana, no Logstash needed                              |
| **Catalog**     | Analytics index          | A streamable Swamp for grouped records with filtering and TTL.                         | 🔹 Auto-sharded by name <br> 🔹 Reverse slices, TTL shift, Beacon-ready                          |
| **Profile**     | User document / Profile  | A key–value state that’s updated and overwritten. Often one-per-user logic.            | 🔹 No upsert logic needed <br> 🔹 Save, read, TTL, lock natively supported                       |
| **Lock()**      | External locking system  | Distributed business logic lock, cross-service, TTL-based.                             | 🔹 Built-in <br> 🔹 Auto-expiring <br> 🔹 Works across Swamps and services                       |
| **expireAt**    | `ttl` / Lifecycle policy | Metadata field that marks auto-deletion time.                                          | 🔹 Native TTL enforcement <br> 🔹 Can be indexed, filtered, shifted                              |
| **Increment()** | Counter document         | Atomic arithmetic on keys — no read-modify-write needed.                               | 🔹 Safe-by-default <br> 🔹 Works on int, float, uint                                             |
| **Uint32Slice** | Nested object or array   | A list of values under one key — supports push, delete, check.                         | 🔹 No script needed <br> 🔹 Structured in-memory mutation                                        |
| **Shard logic** | Routing rules + shards   | Data placement is resolved via hash of Swamp name, not config.                         | 🔹 No orchestrator <br> 🔹 Zero-config scaling <br> 🔹 Always consistent routing                 |
| **Island**      | Node / Shard group       | Logical abstraction of a cluster partition, auto-discovered.                           | 🔹 Hidden from user <br> 🔹 Used for routing only                                                |
