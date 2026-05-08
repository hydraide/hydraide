---
name: hydraide
description: Conceptual and educational explanations of HydrAIDE — how the engine works internally, why it is designed the way it is, what Swamp lifecycle, addressing, query engine, msgpack patch, subscriptions, locking, and the storage engine actually do under the hood. Use when the user asks "how does X work", "why does HydrAIDE do Y", "explain the architecture of Z", or wants to understand concepts (not write implementation code). For Go SDK code, use the `hydraidego` skill. For server operations, use the `hydraidectl` skill.
---

# HydrAIDE — Concepts and Internals

This skill is a router. It does not contain the explanations itself — it points to the focused concept docs in `docs/features/`. Each doc is short and self-contained; read only the one(s) the user is asking about.

When the user asks an educational or "how does it work" question, pick the matching topic from the table below and read that file. Quote and paraphrase from it; do not fabricate internals from memory.

## When to use this skill vs. its siblings

| User's question shape | Skill |
|---|---|
| "Explain how X works", "Why does HydrAIDE do Y", "What is a Swamp/Treasure/Beacon", "How does the storage engine handle Z" | **`hydraide`** (this skill) |
| "Write Go code that…", "How do I model X in `hydraidego`", "What filter do I use for…" | **`hydraidego`** |
| "Install / upgrade / backup / restore / migrate the server", "Why is my instance doing X" | **`hydraidectl`** |

Conceptual questions sometimes blur into implementation. If the user asks "how does X work *and* show me the code", read the relevant concept doc here first, then hand off to `hydraidego` for the SDK call.

## Topic index

| Topic | When the user is asking about… | Read |
|---|---|---|
| Database engine — overview | What HydrAIDE is, struct-as-schema philosophy, where SQL is and isn't | [`docs/features/database-engine.md`](../../../docs/features/database-engine.md) |
| Struct-first data model | Why your Go struct *is* the schema, how it maps to Treasures, why msgpack | [`docs/features/struct-first-data-model.md`](../../../docs/features/struct-first-data-model.md) |
| Deterministic addressing | How `Sanctuary/Realm/Swamp` hashes to a folder and to a server, why there's no metadata service | [`docs/features/deterministic-addressing.md`](../../../docs/features/deterministic-addressing.md) |
| Swamp lifecycle | How Swamps are summoned, idle-evicted from memory, zero-garbage cleanup, `CloseAfterIdle` semantics | [`docs/features/swamp-lifecycle.md`](../../../docs/features/swamp-lifecycle.md) |
| V2 storage engine | `.hyd` file format, append-only writes, compressed blocks, compaction, header layout | [`docs/features/v2-storage-engine.md`](../../../docs/features/v2-storage-engine.md) |
| Query engine | Server-side filters, AND/OR, vector, geo, nested-slice, phrase, IN — internals and design intent | [`docs/features/query-engine.md`](../../../docs/features/query-engine.md) |
| Concurrency safety | Per-Treasure locking, lock-free reads, write queueing, why Swamps don't deadlock | [`docs/features/concurrency-safe.md`](../../../docs/features/concurrency-safe.md) |
| Built-in business locks | Cross-service distributed locks, FIFO queue, TTL semantics, when to use them | [`docs/features/built-in-business-lock.md`](../../../docs/features/built-in-business-lock.md) |
| Reactivity & subscriptions | How writes emit events, why there's no separate broker, FIFO ordering, what Subscribe is *not* | [`docs/features/reactivity-and-subscription-logic.md`](../../../docs/features/reactivity-and-subscription-logic.md) |
| Structural msgpack patch | Atomic field-level mutations on msgpack Treasures, conditions, ops, when not to use | [`docs/features/structural-msgpack-patch.md`](../../../docs/features/structural-msgpack-patch.md) |
| Map-body Catalogs | Single-value vs map-body shape, wire format, Save/Read/Patch symmetry, version compatibility | [`docs/features/map-body-catalog.md`](../../../docs/features/map-body-catalog.md) |
| Pure gRPC control | Why the proto is the contract, why there is no REST gateway or SDK-only API, polyglot story | [`docs/features/pure-grpc-control.md`](../../../docs/features/pure-grpc-control.md) |

## How to answer

1. Pick the matching file from the table.
2. Read it (use the `Read` tool on the absolute path under `docs/features/`).
3. Answer the user's actual question, grounded in the file. Quote sparingly; explain in your own words.
4. If a concept spans multiple files (e.g. "how does HydrAIDE handle concurrent writes" touches both `concurrency-safe.md` and `swamp-lifecycle.md`), read both before answering.
5. If the user wants code after the explanation, hand off to the `hydraidego` skill.

## What this skill is not

- **Not the API reference.** API surface lives in `hydraidego` (Go SDK) and `proto/hydraide.proto` (wire-level).
- **Not an ops guide.** Install, upgrade, backup, migrate → `hydraidectl`.
- **Not a marketing pitch.** For positioning and "why HydrAIDE", the user can read [`docs/why-hydraide.md`](../../../docs/why-hydraide.md) directly.
