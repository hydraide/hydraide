# 🌀 Catalog Shift — Parametric Atomic Claim by Index + Filter

## Philosophy

`ShiftExpiredTreasures` solved one shape of the queue-claim problem: select the expired set under the beacon lock, return entries to one caller, delete them from the swamp. Concurrent callers get disjoint subsets, no read-then-shift race, no lock dance at the application layer. But it was hard-coded to one ordering (oldest expiration first) and one filter (`ExpiredAt < now`). Plenty of real-world patterns have the same shape — atomically pop a bounded set matching predicate P, ordered by index I — but a different I or a different P:

- **FIFO scan-claim queue:** N workers consume a backlog ordered by `CreatedAt`, optionally filtered by `Status == "pending"`.
- **Priority-queue pop:** consume highest-score items first via `IndexValueInt32 + IndexOrderDesc`.
- **Top-K consumer:** atomically claim the K leaders of a scoreboard.
- **Alphabetical drain:** process keys in alphabetical order under `IndexKey + ASC`.

`CatalogShift` is the parametric generalisation: one primitive that covers every "atomically pop a bounded set matching predicate P, ordered by index I" use case. `ShiftExpiredTreasures` is now one instance of it (`IndexType = ExpirationTime`, `Filters = ExpiredAt < ServerNow`).

## Operation

A client issues a `ShiftMatchingTreasures` RPC carrying:

* the **swamp** to operate on (one swamp per call),
* an **`IndexType`** — `Key`, `CreationTime`, `UpdateTime`, `ExpirationTime`, or any `ValueX` (string / int / uint / float),
* an **`OrderType`** — `ASC` or `DESC`,
* a **`HowMany`** cap on the result count (`0` means "all matching", still bounded by `MaxResults` and `Cap`),
* a **`MaxResults`** hard cap defended at the engine layer (production safety net against runaway callers),
* an optional **`Filters`** FilterGroup applied server-side (AND/OR, BytesField paths, ranges) — same machinery used by `CatalogReadManyStream`,
* optional **`FromTime` / `ToTime`** range bounds for time-based indexes,
* an optional **`Cap`** — see [cap-quota.md](cap-quota.md).

The server walks the selected index under the beacon lock, applies the filter predicate per treasure, removes up to `HowMany` matching treasures from every sibling index, and returns them to the caller. Selection and deletion happen under the same lock acquisition — two concurrent callers cannot observe the same treasure.

### Atomic Disjoint Subsets

The selection step holds the chosen beacon's mutex for the entire pass. Concurrent `CatalogShift` callers on the same swamp (whether they use the same index or different indexes) serialise on selection, exactly as `ShiftExpiredTreasures` did. After selection releases, the auto-destroy check fires if the swamp is empty.

### Filter Semantics

`Filters` runs under the beacon lock against the live treasure, using the same `evaluateNativeFilterGroup` evaluator as `CatalogReadManyStream`. Any filter expressible on the read path is also expressible on Shift: `BytesField` paths, `FilterAND` / `FilterOR` trees, value comparisons, timestamps, geo, nested-slice — all supported. No new filter machinery for callers to learn.

### MaxResults vs HowMany

`HowMany` is the soft cap a caller requests. `MaxResults` is the hard cap the engine enforces. When `HowMany == 0` (meaning "drain"), the engine substitutes a billion-element sentinel; `MaxResults` is the real bound. Set `MaxResults` in production to protect a swamp from a runaway caller — a misbehaving worker cannot accidentally drain a 100M-entry swamp in one RPC.

### Legacy compatibility

`ShiftExpiredTreasures` and `CatalogShiftExpired` stay in place and continue to work. They are semantically equivalent to:

```go
h.CatalogShift(ctx, swamp, &hydraidego.ShiftRequest{
    IndexType:  hydraidego.IndexExpirationTime,
    IndexOrder: hydraidego.IndexOrderAsc,
    HowMany:    howMany,
    Filters: hydraidego.FilterAND(
        hydraidego.FilterExpiredAt(hydraidego.LessThan, hydraidego.ServerNow()),
    ),
}, model, iter)
```

New code should prefer `CatalogShift` for clarity; existing code does not need to change.

## When to use `Shift` vs `PatchExpired`

| Pattern | Use |
|---|---|
| Pop N records, process them externally, never put them back | `CatalogShift` |
| Pop N records, do some work, and either re-arm them or drop them | `CatalogShift` for the pop, separate `CatalogSave` for the re-arm |
| Claim N records by flipping a status flag, keep them in the swamp | `CatalogPatchExpired` (preserves the record, slides the TTL) |
| Claim a specific known key by flipping a status flag | `CatalogPatch` builder with `WithCap` |

Rule of thumb: if the lifecycle of the claimed record continues in the same swamp (lease extensions, re-checks), use `PatchExpired`. If the record's life ends or moves elsewhere on claim, use `Shift`.

## What `CatalogShift` is not

- **Not a multi-swamp atomic transaction.** Each `CatalogShift` RPC targets exactly one swamp. The multi-swamp variant `CatalogShiftManyFromMany` runs each swamp independently (per-swamp atomicity, no cross-swamp guarantees).
- **Not a subscription stream.** Use `SubscribeToEvents` for live event delivery; `CatalogShift` is a one-shot atomic claim.
- **Not a replacement for `Lock` when you need cross-record atomicity.** For "read these three records, decide, write some of them back", use a distributed lock.

## See also

- [`cap-quota.md`](cap-quota.md) — how `Cap` bounds the result of `CatalogShift` (and every other state-mutating op).
- [`patch-expired-treasures.md`](patch-expired-treasures.md) — claim-in-place pattern.
- [`structural-msgpack-patch.md`](structural-msgpack-patch.md) — Patch op semantics.
- [`.claude/skills/hydraidego/SKILL.md`](../../.claude/skills/hydraidego/SKILL.md) §14a — Go SDK usage with full examples.
