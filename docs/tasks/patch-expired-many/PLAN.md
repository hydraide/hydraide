# Plan — `PatchExpiredTreasures` + per-request Condition in `PatchTreasures`

**Status:** draft, awaiting review
**Author:** Claude (HydrAIDE side), 2026-05-07
**Driver:** Trendizz crawler-queue redesign (PLAN.md, Feature 2 + Feature 3)

---

## Goals

Two related changes, deliverable in one cycle:

1. **Feature 3 — per-request `Condition` in `PatchTreasuresRequest.Patches`** exposed via the Go SDK's `PatchManyRequest`. Wire-level `TreasurePatch.Condition` already exists; this is an SDK gap.
2. **Feature 2 — new `PatchExpiredTreasures` RPC** that atomically selects up-to-`HowMany` expired treasures from a swamp, applies a single shared op-set + meta to each one, and returns the patched bodies. Conceptual sibling of `ShiftExpiredTreasures`, but in-place rather than destructive.

Non-goals:

- Compound conditions in a single patch (still 1 condition per `TreasurePatch`).
- Changing the `ShiftExpiredTreasures` semantics.
- Cross-key atomicity within a `PatchExpiredTreasures` batch (each treasure is per-key atomic, like the existing patch model).

---

## Atomicity reference (why these primitives are race-free)

| Concern | Mechanism | Code |
|---|---|---|
| Per-key read-modify-write | `treasureObj.StartTreasureGuard(true)` held across `ApplyWithCondition` + `Save` | [`swamp_patch.go:167-218`](../../../app/core/hydra/swamp/swamp_patch.go) |
| Concurrent CAS on same key | Both callers serialize on the per-key guard; only one observes the pre-state matching the condition | same |
| Concurrent expired selection | `beacon.mu.Lock()` over the `expirationTimeBeaconASC` index + per-iteration removal | [`beacon.go:868`](../../../app/core/hydra/swamp/beacon/beacon.go) |

`PatchExpiredTreasures` reuses pattern (3) for selection + pattern (1) for per-treasure mutation. Two concurrent callers receive disjoint subsets, exactly like `ShiftExpiredTreasures`.

---

## Feature 3 — per-request `Condition` in `PatchManyRequest`

### Wire

No proto change. `TreasurePatch.Condition` is already defined ([`hydraide.proto:2779`](../../../proto/hydraide.proto)) and honoured by the gateway ([`gateway_patch.go:68`](../../../app/server/gateway/gateway_patch.go)).

### SDK (`sdk/go/hydraidego/`)

Extend [`hydraidego_patch.go`](../../../sdk/go/hydraidego/hydraidego_patch.go):

```go
// PatchCondOp mirrors hydraidepbgo.PatchCondition_Op without leaking the
// generated enum into the public SDK surface.
type PatchCondOp int

const (
    PatchCondEqual PatchCondOp = iota
    PatchCondNotEqual
    PatchCondGreaterThan
    PatchCondGreaterThanOrEqual
    PatchCondLessThan
    PatchCondLessThanOrEqual
    PatchCondExists
    PatchCondNotExists
)

// PatchCond is a single field-level pre-check evaluated under the per-key
// guard before any op runs. Failure short-circuits with
// PatchStatusConditionNotMet and applies no ops. Mirrors the existing
// PatchBuilder.IfField* surface, exposed for batch use.
type PatchCond struct {
    Op    PatchCondOp
    Path  string
    Value any // ignored for PatchCondExists / PatchCondNotExists
}

type PatchManyRequest struct {
    Key       string
    Fields    map[string]any
    Cond      *PatchCond // NEW — optional CAS gate
}
```

`CatalogPatchFieldsMany` translates `Cond` → `*hydraidepbgo.PatchCondition` via a small helper that reuses `encodePatchValue` for the threshold (only when the operator needs one). Result handling is unchanged; `CONDITION_NOT_MET` already round-trips.

Validation: ops list may not be empty (existing behaviour); when `Cond.Op` is `Exists` / `NotExists`, `Value` must be nil; otherwise `Value` is required.

### Why no new RPC

The existing `PatchTreasures` request is multi-patch and per-patch already supports `Condition` on the wire. This is purely an SDK API ergonomics fix.

---

## Feature 2 — `PatchExpiredTreasures` RPC

### Wire (proto/hydraide.proto)

New service method, alongside `ShiftExpiredTreasures` and `PatchTreasures`:

```proto
// PatchExpiredTreasures atomically selects up to HowMany expired treasures
// from the swamp and applies the same op-set + meta to each. Selection
// behaves exactly like ShiftExpiredTreasures (oldest-first, ExpireAt < now,
// disjoint subsets across concurrent callers); the patched bodies are
// returned in the response.
//
// Typical use: queue claim. Patch the ExpireAt forward via PatchMeta to
// "lease" a batch of items to a worker, with the new ExpireAt acting as
// both the lease deadline and the recovery trigger for crashed workers.
rpc PatchExpiredTreasures(PatchExpiredTreasuresRequest)
    returns (PatchExpiredTreasuresResponse) {}

message PatchExpiredTreasuresRequest {
  uint64 IslandID = 1;
  string SwampName = 2;

  // HowMany caps the result count. 0 means "all expired" (matches
  // ShiftExpiredTreasures).
  int32 HowMany = 3;

  // Ops is applied to every selected treasure. Empty Ops with non-nil Meta
  // is valid (meta-only patch, e.g. ExpireAt slide).
  repeated PatchOp Ops = 4;

  // Meta sets timestamp/identity metadata on every patched treasure.
  // SetExpiredAt is the typical knob — it doubles as the lease deadline.
  optional PatchMeta Meta = 5;

  // Condition is an optional secondary pre-check applied per-treasure
  // after the implicit "ExpireAt < now" selector. Useful for narrowing
  // the lease (e.g. "only claim entries where ClaimedBy == ''"). Treasures
  // failing the condition are skipped (NOT re-added with new ExpireAt) and
  // remain in the expired index for the next caller.
  optional PatchCondition Condition = 6;
}

message PatchExpiredTreasuresResponse {
  repeated PatchedExpiredTreasure Patched = 1;
}

message PatchedExpiredTreasure {
  string Key = 1;

  // Body is the post-patch msgpack payload (without the 2-byte HydrAIDE
  // msgpack magic prefix). Empty if the body was not msgpack.
  bytes Body = 2;

  // Status mirrors PatchResult.StatusCode for this treasure (PATCHED is
  // typical; CONDITION_NOT_MET / TYPE_MISMATCH possible when Condition is
  // set or the body is not msgpack).
  PatchResult.StatusCode Status = 3;

  // Error carries the server-side error string when Status indicates a
  // per-treasure failure.
  string Error = 4;

  // ExpiredAt is the new expiration time after the patch (post-Meta).
  // Zero if cleared.
  optional google.protobuf.Timestamp ExpiredAt = 5;
}
```

`make proto-go` regenerates `generated/hydraidepbgo/`.

### Server — Hydra/Swamp method

New method on the swamp interface, parallel to `CloneAndDeleteExpiredTreasures`:

```go
// app/core/hydra/swamp/swamp.go (interface)
PatchExpired(
    howMany int32,
    ops []msgpackpatch.Op,
    condition *msgpackpatch.Condition,
    meta *PatchMeta,
) ([]PatchedExpiredEntry, error)

type PatchedExpiredEntry struct {
    Key       string
    Body      []byte // post-patch, no magic prefix
    Status    PatchStatus
    Error     string
    ExpiredAt time.Time // zero == cleared
}
```

Implementation outline (in `swamp_patch.go` or a new `swamp_patch_expired.go`):

1. `s.buildBeacon(s.expirationTimeBeaconASC, s.expirationTimeBeaconDESC, BeaconTypeExpirationTime)` — same warm-up as `CloneAndDeleteExpiredTreasures`.
2. New beacon method `SelectExpiredForPatch(howMany int) []*treasure.Treasure` — analogous to `ShiftExpired`, but **does not delete from `treasuresByKeys`**. Holds `b.mu.Lock()`, scans `treasuresByOrder`, picks the first `howMany` whose `ExpirationTime != 0 && ExpirationTime < now`, and returns the live `*treasure.Treasure` pointers (no clone). The selected treasures are removed from `treasuresByOrder` (so a second concurrent caller will not see them as expired) **but kept in `treasuresByKeys`** so subsequent operations on the key still resolve.
3. For each selected treasure, `StartTreasureGuard(true)` and replay the per-key flow from `PatchFields` (extract msgpack body → `ApplyWithCondition` → `SetContentByteArray` → `applyPatchMeta` → `Save`). Collect the post-patch body + meta into the response.
4. Re-insert each successfully-patched treasure into the expirationTime indexes with its **new** `ExpireAt` (the meta-applied value). This is the analog of "the next caller eventually sees it expired again, once the new ExpireAt elapses". This step requires a beacon helper `ReindexExpirationTime(t *treasure.Treasure)` — currently the indexes are rebuilt lazily; we add an explicit upsert. (If we skip this and rely on lazy rebuild, the next `PatchExpiredTreasures` call may miss the entry until the index is rebuilt; explicit upsert is correct.)
5. CONDITION_NOT_MET / TYPE_MISMATCH treasures are also re-inserted (with their **unchanged** `ExpireAt`, so they remain expired and the next caller can retry).
6. Auto-destroy the swamp if `beaconKey.Count() == 0` (matches `CloneAndDeleteExpiredTreasures`); should not happen since we did not delete keys, but keep the assertion symmetric.

Beacon changes needed:

- New `SelectExpiredForPatch(howMany int) []*treasure.Treasure` — derived from `ShiftExpired` minus the delete-from-keys step, plus pulling treasures out of `treasuresByOrder` so concurrent selectors get disjoint sets.
- New `ReindexExpiration(t *treasure.Treasure)` (or a more general `Upsert`) — used by step 4 to put the patched treasure back into the order with its new ExpireAt.

### Server — gateway

New `gateway_patch_expired.go`, mirroring [`gateway_patch.go`](../../../app/server/gateway/gateway_patch.go):

```go
func (g Gateway) PatchExpiredTreasures(ctx context.Context,
    in *hydrapb.PatchExpiredTreasuresRequest,
) (*hydrapb.PatchExpiredTreasuresResponse, error) {
    g.ZeusInterface.GetSafeops().LockSystem()
    defer g.ZeusInterface.GetSafeops().UnlockSystem()
    defer handlePanic()

    if in.GetSwampName() == "" { ... }

    swampName, err := checkSwampName(g.ZeusInterface, in.GetIslandID(), in.GetSwampName(), true)
    if err != nil { ... } // missing swamp → empty result, not error
        // (mirrors ShiftExpired which returns empty on missing swamp)

    swampObj, err := hydraInterface.SummonSwamp(...)
    swampObj.BeginVigil(); defer swampObj.CeaseVigil()

    ops, _ := protoOpsToMsgpackpatchOps(in.GetOps())
    cond := protoCondToMsgpackpatchCond(in.GetCondition())
    meta := protoMetaToSwampMeta(in.GetMeta())

    entries, err := swampObj.PatchExpired(in.GetHowMany(), ops, cond, meta)
    // marshal to PatchedExpiredTreasure[]
}
```

Reuses the same proto→engine helpers as `PatchTreasures`. Empty `Ops` is allowed when `Meta` is non-nil (meta-only patch — typical for "slide ExpireAt forward").

### SDK (`sdk/go/hydraidego/`)

```go
// hydraidego.go (interface)
CatalogPatchExpired(
    ctx context.Context,
    swampName name.Name,
    howMany int32,
    model any,
    iterator CatalogPatchExpiredIteratorFunc,
    builder *PatchExpiredOps, // ops + cond + meta in one bag
) error

// New iterator type that surfaces the post-patch model + the new ExpireAt
type CatalogPatchExpiredIteratorFunc func(model any, expiredAt time.Time) error
```

`PatchExpiredOps` is a thin builder mirroring `PatchBuilder`'s ops/meta surface but without a key (the key set is determined server-side):

```go
b := hydraidego.NewPatchExpiredOps().
    Set("ClaimedBy", crawlerID).
    Set("ClaimedAt", time.Now().UTC()).
    WithUpdatedAt().
    WithExpiredAt(time.Now().UTC().Add(24 * time.Hour)).
    IfFieldEquals("ClaimedBy", "")          // optional CAS — skip already-claimed entries

err := h.CatalogPatchExpired(ctx, swamp, 50, MyCatalog{}, func(m any, exp time.Time) error {
    claimed = append(claimed, m.(*MyCatalog))
    return nil
}, b)
```

Why a separate builder rather than reusing `PatchBuilder`: `PatchBuilder` is bound to a single key and exposes `Exec()`. A keyless variant is clearer than overloading the existing builder.

Implementation: marshals `b` to `PatchExpiredTreasuresRequest`, dispatches the RPC, decodes the per-treasure msgpack body into `model` clones using the same `decode` path as `CatalogShiftExpired`.

---

## Documentation

1. **`proto/hydraide.proto`** — full doc comments on the new messages and RPC, in the existing voice.
2. **`docs/features/`** — add or extend the patch / queue-pattern feature doc:
   - Update [`docs/features/patch-treasures.md`](../../features/patch-treasures.md) (if it exists; otherwise extend the relevant page) with the per-request `Condition` story for `CatalogPatchFieldsMany`.
   - New page `docs/features/patch-expired-treasures.md` — primitive overview, atomicity contract, queue-claim recipe, recovery story, comparison table with `ShiftExpiredTreasures`.
3. **`.claude/skills/hydraidego/`** — extend the Go SDK skill:
   - `PatchManyRequest.Cond` example.
   - `CatalogPatchExpired` example (queue-claim recipe).
   - Common pitfalls: empty Ops + Meta is valid; Condition skips don't reset the ExpireAt.
4. **`.claude/skills/hydraide/`** — concept-level: how the new RPC composes ShiftExpired's atomic selection with per-key patch atomicity.

No CLAUDE.md update needed unless we restructure the operation model.

---

## Tests

### Unit tests

#### `app/core/hydra/swamp/beacon/beacon_test.go`

- `SelectExpiredForPatch` returns oldest-first up to N
- `SelectExpiredForPatch` skips `ExpirationTime == 0`
- `SelectExpiredForPatch` removes from `treasuresByOrder` but keeps `treasuresByKeys`
- `ReindexExpiration` re-inserts in correct order
- Concurrent `SelectExpiredForPatch` x 5 goroutines, 100 expired entries, `howMany=20` each → exactly 100 unique keys returned, no overlap
- `ReindexExpiration` after `SelectExpiredForPatch` → next `SelectExpiredForPatch` honours the new `ExpireAt`

#### `app/core/hydra/swamp/swamp_patch_expired_test.go` (new)

- Plain expired-find + Set: 100 entries, 50 expired → `PatchExpired(50)` patches all 50, returns post-patch bodies
- ExpireAt slide via `PatchMeta` only (empty Ops): old expired entries get new ExpireAt; subsequent `PatchExpired` call returns 0 (none expired now)
- Condition gate: only entries with `ClaimedBy == ""` are patched; gated entries remain expired
- Type mismatch: non-msgpack treasure → `TYPE_MISMATCH` per-entry, not a server error
- `howMany=0` → no-op, empty result
- Empty swamp → empty result
- Concurrent (same swamp, 5 goroutines, `howMany=20`, 100 expired entries) → 100 patches total, each key seen exactly once across all callers
- Crash recovery: stuck-claimed entry (ExpireAt set 24h ago by a "previous" call) is re-claimed by next call
- Mixed: 30 expired + 30 fresh, `howMany=100` → exactly 30 patched

#### `app/server/gateway/gateway_patch_expired_test.go` (new)

- Empty `SwampName` → InvalidArgument
- Missing swamp → empty `Patched`, no error
- Empty `Ops` + nil `Meta` → InvalidArgument (must do *something*)
- Empty `Ops` + Meta with `SetExpiredAt` → meta-only patch succeeds
- Condition + `Ops` round-trip: per-treasure `CONDITION_NOT_MET` is reported in `Status`, not as RPC error
- Wire-level: response order is selection order (oldest expired first)

#### SDK unit tests `sdk/go/hydraidego/hydraidego_patch_test.go`

- `PatchManyRequest.Cond` translation: each `PatchCondOp` maps to the right wire op
- `PatchManyRequest.Cond` with nil `Value` for non-Exists ops → SDK validation error
- `PatchManyRequest.Cond` with non-nil `Value` for Exists ops → validation error
- `CatalogPatchExpired` builder marshals ops + cond + meta correctly (golden test against the wire request)
- `CatalogPatchExpired` decodes response bodies into the model template
- `CatalogPatchExpired` with empty result → iterator not invoked, no error

### E2E tests

`app/server/e2etests/sdk_patch_expired_e2e_test.go` (new), wired into the same harness as [`sdk_patch_e2e_test.go`](../../../app/server/e2etests/sdk_patch_e2e_test.go):

- End-to-end queue claim:
  - Seed 100 catalog entries, half expired
  - 5 goroutines call `CatalogPatchExpired(howMany=20)`
  - Verify: total patched = 100, no key claimed twice, every entry has new ExpireAt = now+24h, every entry has `ClaimedBy == crawlerID_of_its_caller`
- Recovery flow:
  - Seed 10 entries, claim them (ExpireAt = now+1s)
  - Wait 2s, call `CatalogPatchExpired` again → all 10 re-claimed
- Per-request CAS in `CatalogPatchFieldsMany`:
  - 10 keys, half with `Counter=0`, half with `Counter=1`
  - Patch all 10 with `Cond=IfFieldLessThan("Counter", 1)` and `Inc("Counter", 1)`
  - Verify: 5 patched, 5 CONDITION_NOT_MET; final `Counter` distribution = [1,1,1,1,1, 1,1,1,1,1] (the half that started at 0 incremented to 1; the half that started at 1 unchanged)

### Smoke tests under Docker (`docs/tasks/patch-expired-many/smoke/`)

Self-contained Go programs that exercise the new primitives against a real `hydraide` server in a docker container. Pattern follows `docs/install/docker-*` examples.

`smoke/run.sh`:

1. Build the server image from the current branch (`docker build -t hydraide:smoke .`).
2. Start a fresh container with a tmp data volume.
3. `cd smoke && go run ./claim_smoke` — seeds 1000 entries, 5 concurrent claimers, asserts disjoint claims + new ExpireAt.
4. `cd smoke && go run ./recovery_smoke` — claim, wait, re-claim, assert.
5. `cd smoke && go run ./cond_many_smoke` — exercises `PatchManyRequest.Cond`.
6. `cd smoke && go run ./meta_only_smoke` — empty Ops + ExpireAt-only meta on expired entries, verify the slide.
7. Tear down the container.

Each smoke binary prints a short PASS/FAIL line and exits non-zero on failure so `run.sh` can chain them.

---

## Rollout

1. Branch: `feat/patch-expired-many` off `main`.
2. Commits (Conventional Commits, no AI co-author lines per [CLAUDE.md](../../../CLAUDE.md)):
   - `feat(sdk-go): add Cond field to PatchManyRequest`
   - `feat(proto): PatchExpiredTreasures RPC + messages`
   - `feat(beacon): SelectExpiredForPatch + ReindexExpiration`
   - `feat(swamp): PatchExpired method`
   - `feat(gateway): PatchExpiredTreasures handler`
   - `feat(sdk-go): CatalogPatchExpired + PatchExpiredOps builder`
   - `test: unit + e2e coverage for patch-expired and PatchManyRequest.Cond`
   - `docs(features): patch-expired-treasures + per-request Cond on PatchManyRequest`
   - `docs(claude): hydraidego skill — PatchManyRequest.Cond + CatalogPatchExpired`
3. CI must pass before push.
4. Push only after explicit Péter approval (per repeated user feedback).
5. Tag a release using the `hydraide-release` skill — server, hydraidectl, Go SDK, plugin all bump together. Release note in Péter's voice. **Plugin will change** (skill content updated), so the release note flags the plugin update.
6. Trendizz redesign Plan B can switch to the new RPC the moment the SDK tag is published.

---

## Decisions (confirmed in review with Péter, 2026-05-07)

1. **Naming:** `PatchExpiredTreasures` (mirrors `ShiftExpiredTreasures`).
2. **Condition policy:** secondary per-treasure filter under the guard; `CONDITION_NOT_MET` treasures stay in the expired index unchanged so the next caller can retry.
3. **Empty `Ops`:** allowed on the wire when `Meta` is non-nil (meta-only patch is the typical "slide ExpireAt" use case). Both empty → `InvalidArgument`.
4. **Re-indexing strategy:** **eager** — every patched treasure is re-inserted into `expirationTimeBeaconASC` with its new `ExpireAt` before the RPC returns. Consistent with `CloneAndDeleteExpiredTreasures`. Lazy is a future optimization only if measurements demand it.
5. **Doc placement:** `docs/features/patch-expired-treasures.md` for the primitive + queue-claim recipe; brief pointer from the `hydraide` concept skill. No separate how-to until more queue-style patterns accumulate.

---

## Benchmarks

Run before merge, results checked into [`docs/benchmarks/`](../../benchmarks/). Goal: confirm eager re-indexing is acceptable at realistic scale, and document the curve so we know when lazy becomes worth it.

### Bench matrix

| Dimension | Values |
|---|---|
| Swamp size (K) | 10k, 100k, 1M |
| Expired fraction | 1%, 10%, 50% |
| Batch size (`HowMany`) | 50, 500, 5000 |
| Concurrent callers | 1, 5, 20 |
| Ops per patch | 0 (meta-only), 1, 5 |

Cross-product is too large; pick representative slices:

- **Throughput sweep:** fix swamp 100k / 10% expired, vary batch size and concurrency. Reports patches/sec, p50/p95/p99 per-call latency.
- **Scale sweep:** fix batch=500, concurrency=5, vary swamp size 10k → 1M. Reports per-call latency to surface the O(N log K) curve.
- **Re-indexing isolation:** same as scale sweep, but with a build-tagged "skip re-indexing" variant of `PatchExpired` for comparison. The delta is the eager re-indexing cost specifically.
- **Meta-only vs ops:** fix swamp 100k / 10% expired / batch=500, compare 0 / 1 / 5 ops per patch. Reports the per-op cost so callers can size their batches.

### Bench harness

`app/core/hydra/swamp/swamp_patch_expired_bench_test.go` — `go test -bench` style, using the existing in-process swamp harness (no gRPC overhead, isolates the engine cost). Plus one end-to-end bench under `app/server/e2etests/` that goes through the gateway + SDK round-trip so we have a realistic number for callers.

### Acceptance thresholds (sanity, not hard gates)

- 100k swamp, batch=500, concurrency=5: ≥ 5k patches/sec, p99 per-call < 200ms.
- 1M swamp, batch=500, concurrency=5: ≥ 1k patches/sec, p99 per-call < 1s.
- Re-indexing overhead < 30% of total per-call latency at 1M swamp size.

If a threshold is missed, the plan is to land the eager version anyway (correctness first), open a tracking issue, and revisit lazy re-indexing as a follow-up.

---

**End of plan.** Implementation begins after Péter signs off.
