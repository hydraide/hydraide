# 🚧 Cap — Server-Enforced Quota Primitive

## Philosophy

Distributed work-queue, rate-limit, and quota patterns all collapse to the same problem:

> *N workers consume a shared queue. Each item is processed by exactly one worker. The number of in-flight items must never exceed a cap (per shard, per tenant, per resource, per rate-limit bucket).*

Before `Cap`, a HydrAIDE application could only express this with two manually-coordinated primitives:

1. `Count(filter)` — "how many items are claimed right now?"
2. `CatalogPatchExpired(HowMany = cap - count, …)` — "claim that many".

The two steps were two separate RPCs. Between them lay a race window — two workers could each read `count = 5`, each compute `slots = cap - 5 = 5`, each claim 5, and the swamp ends up with `count = 15`. The cap is broken.

Three workarounds existed, all bad:

* **Application-side counter** incremented on claim, decremented on finalize. The happy path works, but every code path that forgets to decrement (panic, shutdown mid-task, network timeout, alternative finalize) leaks +1. The drift is monotone — eventually every cap looks full while no records actually match. A reconciler script can smooth the drift, but the script is a symptom of the architectural mistake.
* **Distributed `Lock` around the claim path.** Correct, but it serialises the entire claim — at the typical "tens to hundreds of claims per second per shard" load, the lock becomes the throughput ceiling.
* **Soft cap with a small over-claim window.** Fine when the cap is advisory. Not fine when the cap protects an IP-ban window, a GPU OOM boundary, or a third-party rate-limit penalty.

`Cap` is a first-class HydrAIDE primitive that eliminates the race window by construction: the count of matching records and the claim mutation run under the same per-swamp guard. No application-side counter. No application-side lock. No drift.

## Operation

A request that opts into `Cap` carries:

* a **`Filter`** — a `FilterGroup` that defines which records count toward the quota (typically "claimed and lease still valid"),
* a **`MaxMatching`** — the hard post-operation upper bound on records matching `Filter`.

The server, under the same per-swamp guard that protects the mutation, counts records matching `Filter` and bounds the operation to respect the cap. The exact rule depends on the op shape:

### Selection-based ops (`CatalogShift`, `CatalogPatchExpired`)

Selected records always go from "not matching" to "matching" (they weren't claimed before, they are now). The cap reduces to budget arithmetic:

```
budget       = MaxMatching - currentMatching
claim_count  = min(HowMany, budget)
```

If `budget <= 0`, zero records are claimed and `CapReached = true`. If the budget shrinks the result below the requested `HowMany`, `CapReached = true` so the caller knows to back off rather than poll.

`Cap.Filter` *defines the match-set*; it does **not** scope the selection. With the "all selected records enter `Cap.Filter`" assumption, the budget arithmetic above is exact — but the assumption breaks when `Cap.Filter` carries scope predicates (e.g. `ASN == "AS-X"`) that the patched record does not enter. Use **`Filters`** (a separate `FilterGroup` field on `CatalogShift` and `CatalogPatchExpired`) to narrow the candidate set *before* selection. The rule of thumb:

- **Selection scope** (which records are candidates) → `Filters`.
- **Match-set definition** (what counts as "claimed" for `MaxMatching`) → `Cap.Filter`.

The two often overlap (e.g. both reference `ASN == "AS-X"` in a per-ASN claim queue). For per-key claim patterns sharing a single swamp across many tenants/ASNs, both fields are required:

```go
// Per-ASN bounded claim. The Filters narrows selection so ASN==Y records
// are skipped entirely; Cap.Filter defines the per-ASN match-set so the
// budget is computed against that subset.
hydraidego.NewPatchExpiredOps().
    Set("ClaimedBy", workerID).
    WithExpiredAt(now.Add(24 * time.Hour)).
    WithFilters(hydraidego.FilterBytesFieldString(hydraidego.Equal, "ASN", asn)).
    WithCap(&hydraidego.Cap{
        Filter: hydraidego.FilterAND(
            hydraidego.FilterBytesFieldString(hydraidego.Equal, "ASN", asn),
            hydraidego.FilterBytesFieldString(hydraidego.NotEqual, "ClaimedBy", ""),
            hydraidego.FilterExpiredAt(hydraidego.GreaterThan, now),
        ),
        MaxMatching: asn.MaxParallel,
    })
```

Without `Filters`, the selection walks every expired record in the swamp and the cap budget can be consumed by records that do not enter `Cap.Filter` post-patch — making per-scope claim flows unviable on mixed-population swamps.

### Explicit-key ops (`CatalogPatch`, `CatalogPatchFields`, `CatalogPatchFieldsMany`, `CatalogPatchManyToMany`)

The mutation does not necessarily push a record into `Filter`'s match set. The server evaluates `Filter` on both the pre-mutation body and the simulated post-mutation body and applies the four-cell rule:

| pre matches | post matches | Δ count | Action |
|---|---|---|---|
| no  | no  | 0  | proceed (untouched) |
| yes | yes | 0  | proceed (idempotent re-mutation) |
| yes | no  | -1 | proceed (count shrinks) |
| no  | yes | +1 | proceed only if `currentMatching + accepted_so_far < MaxMatching`, otherwise `PatchStatusCapExceeded` |

The (no → yes) transition is the only one that consumes budget. The other three are always allowed.

### Atomicity model

The cap check + the mutation run under one swamp guard. Concurrent Cap-bearing flows on the same swamp serialise on a swamp-level mutex (`capMu`). No cross-RPC race window exists.

Non-Cap operations on the same swamp do **not** acquire `capMu`. They run in parallel with each other and with reads, as before. A non-Cap `CatalogPatch` that moves a record into `Cap.Filter` between two Cap-bearing flows is observed by the next flow's count step — the budget arithmetic stays correct.

## Restrictions on Patch surfaces

`Cap.Filter` on explicit-key Patch surfaces (`CatalogPatch`, `CatalogPatchFields*`) is restricted to **BytesField filters** — i.e. filters that read a path inside the msgpack body (`Status`, `ClaimedBy`, nested struct fields). Metadata filters (`FilterCreatedAt`, `FilterUpdatedAt`, `FilterExpiredAt`, typed-value filters) are rejected with `InvalidArgument`. The reason: simulating arbitrary post-mutation metadata for a patch op-set is out of scope. `CatalogShift` and `CatalogPatchExpired` have no such restriction.

## What `Cap` is not

- **Not a cross-swamp quota.** Each `Cap` applies to exactly one swamp. Cross-server / cross-swamp quotas need application-level coordination (or a different design — e.g. a single "shard" swamp that owns the quota state).
- **Not a rate limiter over time.** `Cap` bounds the count of records matching `Filter` at any given moment. To express "10 claims per minute", use `Cap` for concurrency (claims-in-flight) plus a separate rate-limiter (token bucket) on top.
- **Not a substitute for `Condition`.** `Cap` enforces a swamp-wide aggregate invariant. `PatchCondition` enforces a per-key precondition. Use both together when both are relevant.

## Empty result vs cap reached

When a Cap-bearing call returns zero results, two cases are possible:

* `CapReached = false` — the caller should retry: there's room under the cap but no candidates matched the selection / mutation.
* `CapReached = true` — the quota is exhausted. Back off; do not retry until the matching count drops (typically when a leased record expires or a worker explicitly releases).

Most callers can ignore `CapReached` and just retry on a timer — the loop terminates eventually either way. The signal exists for callers that want to back off intelligently and surface "system at capacity" to upstream monitoring.

## Common pitfall: keeping the app-side counter

When migrating to Cap, **delete the application-side claim counter**. The Cap-bearing call is the only source of truth. A counter alongside Cap will drift over hours (every code path that forgets to decrement leaks +1), and the cap will look full while no records actually match. The reconciler that "smooths the drift" treats a symptom — the counter never had to exist.

## See also

- [`catalog-shift.md`](catalog-shift.md) — parametric atomic shift, the primary Cap-bearing selection-based op.
- [`patch-expired-treasures.md`](patch-expired-treasures.md) — claim-in-place with Cap.
- [`structural-msgpack-patch.md`](structural-msgpack-patch.md) — Patch op semantics.
- [`.claude/skills/hydraidego/SKILL.md`](../../.claude/skills/hydraidego/SKILL.md) §14b — Go SDK usage with full examples per surface.
- [`proto/hydraide.proto`](../../proto/hydraide.proto) — wire-level `Cap` message and `CAP_EXCEEDED` status.
