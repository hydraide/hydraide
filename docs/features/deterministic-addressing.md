# Deterministic addressing — from filesystem to cluster

Every Swamp is reached by deterministic hashing, not by index lookup or scan. The same hash function maps a Swamp name to a folder on disk and, in a multi-server deployment, to a specific server. The client computes both — there is no metadata service to consult, no shard map fetched at startup, no query planner deciding which index to use.

This page covers the addressing model at two scales: a single host, and a cluster of HydrAIDE servers.

---

## Single host — Swamp name to folder

A Swamp's name (`Sanctuary/Realm/Swamp`) is hashed deterministically to a specific folder on the data volume. The folder is opened, the `.hyd` file is loaded, the Swamp is hydrated. There is no global index, no B-tree walk over millions of records.

### Why this works in practice

* **NVMe SSDs make small-file access cheap.** Loading a Swamp's `.hyd` file is a small, predictable I/O — well within the budget for a real-time read.
* **Each Swamp is physically isolated.** Lifecycle, lock domain, and disk file are per Swamp, so growing the dataset by adding more Swamps does not slow down the existing ones.
* **Folder lookups are bounded by design.** The directory tree is one level deep with up to 1000 sub-folders, so `open()` cost on the data root does not grow with the number of Swamps in the system.

This is what "O(1) access" means here: the cost of reaching a specific Swamp is bounded by a constant — not by the total size of the dataset or the number of other Swamps that exist.

### The save path

When you save data:

1. The client hashes the Swamp's name.
2. The hash maps to a specific folder on the SSD (and, in a multi-server deployment, to a specific server).
3. The Swamp is loaded or created at that location.
4. The data is written, and any subscribers receive a real-time event.

Steps 1 and 2 are constant time, regardless of how much data the system holds in total.

---

## Cluster — Swamp name to server

The same hash extends across machines. Each Swamp belongs to an **island** (a deterministic number between 1 and `AllIslands`), and each server in the cluster owns a contiguous range of islands.

### How the island model works

The client SDK hashes the full Swamp name, then calculates the island ID. Island ranges are pre-assigned to servers:

* Server 1: islands 1–100
* Server 2: islands 101–200

This mapping **never changes** unless the total number of islands (`AllIslands`) changes. That's why it's best to start with a generous value (e.g., 1000) and leave room for future scale-out — changing `AllIslands` later requires data migration.

### Multi-server connection in the SDK

In `repo.New()`, you list each server's address, TLS certificate, and island range. The client then knows automatically, on every operation, which server to talk to:

```go
repoInterface := repo.New([]*client.Server{
    {
        Host:         os.Getenv("HYDRA_HOST_1"),
        FromIsland:   1,
        ToIsland:     100,
        CertFilePath: os.Getenv("HYDRA_CERT_1"),
    },
    {
        Host:         os.Getenv("HYDRA_HOST_2"),
        FromIsland:   101,
        ToIsland:     200,
        CertFilePath: os.Getenv("HYDRA_CERT_2"),
    },
}, 200, 10485760, true)
```

Two servers, 200 islands, exact range boundaries.

### Why this is fast

* The Swamp → island calculation is constant time.
* There is no intermediate layer, no query parser, no metadata lookup on the read path.
* The SDK connects directly to the right server.

---

## How this differs from sharded engines

Engines that ship with consistent-hashed sharding (Cassandra, ScyllaDB, Couchbase, and others) achieve deterministic routing at the storage layer too. What's specific to HydrAIDE is *where* the routing happens and what comes around it:

* **The client does the computation directly from the Swamp name.** There is no per-cluster metadata exchange to find a key — the routing is pure function of the Swamp name and the local island-range configuration.
* **Single binary, no cluster machinery.** No gossip protocol, no repair scheduler, no quorum tuning, no replication-factor configuration to maintain. A single HydrAIDE process is a complete deployment.
* **No second language.** Cassandra and Scylla expose CQL; HydrAIDE is reachable directly from Go structs (or any language with a protoc-generated client), with no separate query dialect to learn or review.
* **Reactivity in the same engine.** Subscriptions on writes are a first-class API, not a CDC pipeline bolted onto the side.
* **Per-Swamp memory budget set by the developer.** Caching in those engines is configured globally; in HydrAIDE every Swamp pattern gets its own `CloseAfterIdle` window.
* **Schemaless, code-first.** No `CREATE TABLE` step, no schema-evolution choreography for additive changes — the Go struct on disk is the schema.

If you already run Cassandra or Scylla and the routing-only side is the only thing you need from HydrAIDE, those engines do that part well. The reason to reach for HydrAIDE is the rest of the package: the single-binary deployment, the schemaless Go-first surface, the built-in subscriptions, and the per-Swamp memory control.

---

## Result

* No central coordinator is needed for read or write routing.
* Adding a server is a configuration change: pick an island range, point the new server at it.
* Routing is deterministic — the same Swamp name always lands on the same server.

A note on rebalancing: changing the total `AllIslands` count *does* require migrating data, which is why we recommend choosing a generous starting value (e.g. 1000) rather than reshuffling later.
