# 🔒 Concurrency-Safe – Philosophy and Operation

## Philosophy

From the very beginning of HydrAIDE's design, a core principle was that a single **Swamp** should be able to handle millions of concurrent requests simultaneously, without blocking each other, while still operating with guaranteed safety.

Other systems (e.g., SQL databases, Redis `SETNX`-based locks) often rely on global or table-level locking, which reduces performance and can create race conditions. HydrAIDE, in contrast, uses per-object (Treasure) level deterministic locking, eliminating the need for external brokers and limiting write operations to the smallest possible unit.

The goal was to ensure that two different processes could never overwrite each other’s data, while reads and writes remain as fast as possible.

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

### 3. Exceptional speed

Even with write locking, performance is extremely high: on a single CPU core, a single Swamp can handle **600–700 thousand Treasure writes per second**.

### 4. Cluster and distributed operation

Based on the hash calculated from the Swamp’s name, the system always routes a given Treasure’s reads or writes to the same node. This ensures that FIFO locking remains **consistent in distributed environments**, with every key being handled in exactly one place within the cluster.

## Summary

This model enables developers to achieve:

* Maximum parallelism
* Data integrity even in race conditions
* **O(1)** access time for every key
* Simple, safe application logic — without external lock systems or brokers

HydrAIDE’s per-object locking strategy delivers not only technical safety but also business logic benefits.
