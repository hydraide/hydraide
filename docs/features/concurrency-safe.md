# Concurrency safety

A Swamp can serve many concurrent requests in parallel. Locking is per-key (per Treasure), not per Swamp or per table — so writes on different keys do not contend, and writes on the same key queue in arrival order. Reads do not take a lock at all.

The goal of this design is straightforward: two writers must not silently overwrite each other on the same key, and unrelated writers must not slow each other down.

## Operation

In HydrAIDE, every **Treasure** in every Swamp is independently accessible in a concurrent manner, with no waiting between calls if they target different keys.

This can be summarized in three key rules:

### 1. Parallel reads and writes at the key level

* Reads are entirely lock-free and non-blocking — reads never block each other or writes.
* Writing and reading different Treasures happens fully in parallel.

### 2. Treasure-level write lock

If the same Treasure is targeted by multiple simultaneous write operations, HydrAIDE applies deterministic, arrival-order **FIFO** locking.

* Only one write operation can run at a time for the same key.
* Incoming writes are placed into a true queue (not just between two actors, but across thousands of clients if necessary).
* If a client dies or its related context is closed, the lock is immediately released — preventing deadlock situations entirely.

This queuing offers business logic advantages, for example:

* Safely incrementing counters or stock values without conflicts
* Guaranteeing order in financial transactions
* Consistently updating game states or leaderboards

### 3. Throughput

Per-key FIFO locking does not become the bottleneck under typical workloads. For storage-engine measurements (insert/update/delete latencies, throughput per Swamp), see [V2 benchmark results](../benchmarks/V2_RESULTS_SUMMARY.md).

### 4. Cluster and distributed operation

Based on the hash calculated from the Swamp’s name, the system always routes a given Treasure’s reads or writes to the same node. This ensures that FIFO locking remains **consistent in distributed environments**, with every key being handled in exactly one place within the cluster.

## Summary

* Reads are lock-free.
* Writes on different keys run in parallel.
* Writes on the same key are serialised in FIFO order.
* Locks are released when the client context closes — no orphaned holds.
* Routing is deterministic across the cluster, so a given key is always handled in the same place.
