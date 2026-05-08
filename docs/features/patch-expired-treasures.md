# ⏰ Patch Expired Treasures – Atomic In-Place Patch of the Expired Set

## Philosophy

You have a swamp full of records that go stale on a schedule. Crawl jobs that need to re-run after 24 hours. Rate-limit windows that reset every minute. Queue items that a worker needs to claim and process. The shape of the problem is always the same: **find everything that has expired, do something to it, and arrange for the next round of expirations.**

The naive approach is a three-step dance: list the expired keys, mutate each one, hope nothing else got there first. With concurrent workers, that hope becomes a race. Someone reads, you read, you both write, one write wins, the other vanishes. The standard fix is a distributed lock — but a lock around the whole expired-set kills throughput, and a lock per key still leaves the *fetch* step racy.

`ShiftExpiredTreasures` solved the queue-claim version of this with one atomic primitive: select the expired set under the beacon lock, return the entries to one caller, delete them from the swamp. Two callers cannot both observe the same item — the one who wins the lock takes it, the other moves to the next. **But "delete" is too strong** when you want to keep the record in place: claim it, lease it, slide its TTL, recheck it later.

`PatchExpiredTreasures` is the in-place version. **One atomic RPC** that selects the expired set, applies a typed patch (ops + condition + metadata) to each one under its per-key guard, **re-indexes them with their new ExpiredAt**, and returns the post-patch bodies. Concurrent callers receive disjoint subsets — same guarantee as `ShiftExpired`, same lock primitive — but the swamp keeps the data.

## Operation

A client issues a `PatchExpiredTreasures` RPC carrying:

* the **swamp** to operate on (one swamp per call, like `PatchTreasures`),
* a **HowMany** cap on the result count (`0` means "all currently-expired"),
* an ordered list of **typed ops** applied to every selected treasure (`SET`, `INC`, `APPEND`, `MERGE`, etc.) — see [structural-msgpack-patch.md](structural-msgpack-patch.md) for the op semantics,
* an optional **PatchCondition** evaluated per-treasure under its guard,
* an optional **PatchMeta** stamped on every patched treasure (typically `SetExpiredAt` to slide the TTL forward, the same field used by `PatchTreasures`).

The server walks the expiration-time index under the beacon lock, pulls up to `HowMany` treasures whose `ExpiredAt < now()` out of the ordered slice, and for each one runs the same per-key flow that powers `PatchTreasures`: take the per-treasure guard, evaluate the condition, splice the ops into the msgpack body, stamp the metadata, save. Then the patched treasures are re-inserted into the expiration index with their new `ExpiredAt` so a subsequent caller picks them up again on the next cycle.

### Atomic Disjoint Subsets

The selection step holds the same beacon mutex that `ShiftExpired` uses. Concurrent callers serialize on selection — caller A removes its 50 entries from the ordered slice, then caller B sees only the remaining set. Two callers **cannot** both observe the same treasure as expired. After selection releases, the per-treasure mutations run in parallel under independent per-key guards. Callers see exactly the throughput of the per-key Patch primitive, with one round-trip instead of `2N` (a fetch + a write per claimed item).

### Condition Semantics

If you supply a `PatchCondition`, the selection still happens by `ExpiredAt < now`. The condition is then evaluated **per-treasure under its guard**, after selection. Treasures that fail the condition are **not patched**, are reported as `CONDITION_NOT_MET`, and are re-inserted into the expired index with their **unchanged** `ExpiredAt` — the next caller sees them again. This makes "claim only the entries where ClaimedBy is empty" a one-line addition to the call:

```go
NewPatchExpiredOps().
    Set("ClaimedBy", workerID).
    WithExpiredAt(now.Add(24 * time.Hour)).
    IfFieldEquals("ClaimedBy", "")
```

### Meta-Only Patches

Empty ops + non-nil meta is allowed and is the typical "slide ExpiredAt forward without changing the body" form. Use it when you need to defer a recheck, or to reset a leak of stuck entries without rewriting their state:

```go
NewPatchExpiredOps().WithExpiredAt(now.Add(7 * 24 * time.Hour))
```

### Recovery Without Extra Code

Because the selection criterion is `ExpiredAt < now`, a worker that crashes after claiming an entry simply lets its lease expire — `ExpiredAt = claim_time + 24h`. After 24 hours, the next `PatchExpiredTreasures` call sees the entry as expired again and a different worker re-claims it. **The recovery mechanism is the primitive itself** — no separate "stuck-claim cleanup" job, no zombie hunter, no operator intervention.

## Advantages

* **One atomic RPC instead of `Fetch + Lock + Save`** — selection, condition, mutation, and re-indexing all happen server-side under the beacon lock + per-key guard.
* **Concurrent-safe by construction** — N workers calling `PatchExpiredTreasures` produce a fair, disjoint partition of the expired set with no client coordination.
* **Recovery for free** — crashed workers' claims expire on their own; the next call re-claims them automatically.
* **Type-preserving** — every guarantee from [structural-msgpack-patch.md](structural-msgpack-patch.md) carries over (untouched leaves keep their exact wire encoding).
* **Generalizes beyond queues** — bulk TTL slides, periodic recheck scheduling, lease extensions, rate-limit window resets all use the same primitive.

## Comparable Primitives Elsewhere

* **Redis** has `BLPOP` for queue claims with a timeout, but not in-place TTL slides on a sorted set with per-item conditions. `ZRANGEBYSCORE … LIMIT` + per-key Lua scripts get partway there at the cost of scripting and partial atomicity.
* **PostgreSQL** with `SELECT … FOR UPDATE SKIP LOCKED + UPDATE` does the queue-claim form well, but every "claim" is one round-trip per item; bulk meta-only TTL slides need a separate transaction pattern.
* **MongoDB** has `findAndModify` with TTL indexes, which solves the "find one expired and patch it" case but not batched in-place updates with disjoint-subset guarantees across concurrent callers.
* **DynamoDB** has TTL-driven background deletion, not patch-in-place — the recovery story (re-add the item with a new TTL) requires application-level retries.

`PatchExpiredTreasures` fits naturally inside HydrAIDE because the expiration index, the per-key guard, the structural msgpack patch, and the disjoint-subset selection primitive are already in the engine — the new RPC just composes them.

## When to Reach For It

Use `PatchExpiredTreasures` when:

* You have a swamp where every record carries an `ExpiredAt`-style TTL.
* Multiple workers need to consume entries that expire on a schedule.
* You want crash-safe queue claims, lease-with-extension flows, or bulk TTL slides.
* You'd otherwise build a `ShiftExpired + SaveMany + retry-on-CAS-fail` loop with a microsecond-wide race window.

Stick with `ShiftExpiredTreasures` when:

* You want to **drain** the expired set into a downstream system and remove it from HydrAIDE entirely.
* You don't need the recovery-via-re-expiration mechanism (e.g. a one-shot batch).

Stick with `PatchTreasures` (per-key) when:

* You know the exact key set up front (no expiration-driven selection needed).
* You want the patch to apply regardless of whether the key is expired.

## API Surface

* Wire-level: `rpc PatchExpiredTreasures(PatchExpiredTreasuresRequest) returns (PatchExpiredTreasuresResponse)` in [`proto/hydraide.proto`](../../proto/hydraide.proto).
* Go SDK: `CatalogPatchExpired(ctx, swamp, howMany, model, iterator, builder)` — see [`sdk/go/hydraidego/hydraidego_patch_expired.go`](../../sdk/go/hydraidego/hydraidego_patch_expired.go).
* Builder: `hydraidego.NewPatchExpiredOps()` — same fluent shape as `PatchBuilder` minus the per-key `Exec`. Includes `Set` / `Inc` / `Append` / `Prepend` / `Delete` / `RemoveAt` / `RemoveVal` / `Merge`, every `IfField*` condition, and `WithUpdatedAt` / `WithUpdatedBy` / `WithExpiredAt` / `WithoutExpiredAt`.

For the design rationale, atomicity proofs against the engine code, and the bench methodology, see [`docs/tasks/patch-expired-many/PLAN.md`](../tasks/patch-expired-many/PLAN.md).
