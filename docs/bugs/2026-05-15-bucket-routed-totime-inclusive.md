# Bug — `Index.ToTime` is inclusive on the bucket-routed path

**Found:** 2026-05-15
**Reporter:** Peter Gebri (Trendizz)
**Affected versions:** server `v3.19.0`, SDK `v3.6.0` (reproduced); likely older too
**Severity:** Medium — silent off-by-one. Returns one extra record at the boundary; breaks cursor-style pagination by re-emitting the cursor record on the next page.

---

## Summary

`Index.ToTime` is documented as an **exclusive** upper bound on time-based reads
(`CatalogReadManyStream`, `CatalogReadMany`, …). On the **bucket-routed** filter
path (any query that hits an auto-built field-bucket index together with a time
range), the bound is applied **inclusively** instead. The legacy beacon-walk path
applies it exclusively as documented. The two paths disagree.

For cursor-paginated reads this means: when the client sets `ToTime = lastCreatedAt`
on the next page request, the record exactly at `lastCreatedAt` is returned a
second time. With second-resolution `createdAt` storage and a high-resolution
client cursor, the boundary record is the only collision — but it is a real
duplicate row across consecutive pages.

## Repro

A real-world reproducer from the Trendizz monorepo. Setup: a Catalog with a
flat `Direction int8` body field and a time-ordered `createdAt`. Seed 10 records
across one swamp at 1-hour spacings. Query with `IndexCreationTime` DESC and a
filter on `Direction IN [1, 3]`:

```go
idx := &hydraidego.Index{
    IndexType:  hydraidego.IndexCreationTime,
    IndexOrder: hydraidego.IndexOrderDesc,
    MaxResults: 6,                 // limit+1
    ToTime:     nil,               // page 1
    // ToTime:  &cursor,           // page 2 (cursor = page1.last.CreatedAt)
}

filters := hydraidego.FilterOR(
    hydraidego.FilterBytesFieldInt8(hydraidego.Equal, "Direction", 1),
    hydraidego.FilterBytesFieldInt8(hydraidego.Equal, "Direction", 3),
)
```

Observed output on page 2 (where `ToTime = page1.last.CreatedAt`):

```
page1: cursor=2026-05-15 02:09:28
  p1[0] createdAt=06:09:28
  p1[1] createdAt=05:09:28
  p1[2] createdAt=04:09:28
  p1[3] createdAt=03:09:28
  p1[4] createdAt=02:09:28   ← page1 last record (= cursor)

page2 (with ToTime=cursor):
  p2[0] createdAt=02:09:28   ← BUG: same record as p1[4]
  p2[1] createdAt=01:09:28
  p2[2] createdAt=00:09:28
  p2[3] createdAt=2026-05-14 23:09:28
  p2[4] createdAt=2026-05-14 22:09:28
```

Expected per the SDK doc (`ToTime` exclusive): page 2 starts at `01:09:28`, no
overlap with page 1.

## Root cause

Two code paths in the gateway evaluate `ToTime` with different semantics.

**Bucket-routed path** (when a filter has at least one indexable leg, beacon
type is time-based, and `bucketExecPreconditions` holds):
[`app/server/gateway/gateway.go:663-670`](../../app/server/gateway/gateway.go)

```go
if plan.Mode != PlanModeBypass && bucketExecPreconditions(beaconType) {
    candidates := collectBucketCandidates(swampInterface, plan.Hints)
    candidates = applyTimeRange(candidates, beaconType, fromTime, toTime)
    sortCandidates(candidates, beaconType, order)
    treasures = applyFromLimit(candidates, in.GetFrom(), in.GetLimit())
    residualFilters = plan.Residual
}
```

The filter at [`app/server/gateway/bucket_exec.go:88-99`](../../app/server/gateway/bucket_exec.go):

```go
out := candidates[:0]
for _, t := range candidates {
    ts := beaconTimeOf(t, beaconType)
    if fromTime != nil && ts < fromNs {
        continue
    }
    if toTime != nil && ts > toNs {   // ← INCLUSIVE: keeps ts == toNs
        continue
    }
    out = append(out, t)
}
```

`ts > toNs` keeps every record with `ts <= toNs`. That is **inclusive** on the
upper bound.

**Beacon-walk path** (filter is empty, or no indexable leg / not time-beacon):
[`app/core/hydra/swamp/beacon/beacon.go:1491-1503`](../../app/core/hydra/swamp/beacon/beacon.go) and the descending mirror below it:

```go
// Asc: end = last idx with ts < toNano (exclusive upper bound)
if toTime != nil {
    l, r := 0, n
    for l < r {
        m := l + (r-l)/2
        if b.getTimestampFromTreasure(b.treasuresByOrder[m]) < toNano {
            l = m + 1
        } else {
            r = m
        }
    }
    endIdx = l - 1
}
```

This is **exclusive** on the upper bound — keeps only `ts < toNano`.

## SDK documentation

Both the SDK doc comments and the protobuf comment say **exclusive**:

[`sdk/go/hydraidego/hydraidego.go:142-143`](../../sdk/go/hydraidego/hydraidego.go):

```
//   - FromTime:      inclusive lower bound (records with time >= FromTime are included)
//   - ToTime:        exclusive upper bound (records with time < ToTime are included)
```

[`sdk/go/hydraidego/hydraidego.go:170`](../../sdk/go/hydraidego/hydraidego.go):

```go
ToTime *time.Time // Exclusive upper bound for time-based filtering ...
```

`hydraidepbgo/hydraide.pb.go:3186`:

```
// FromTime / ToTime are inclusive lower / exclusive upper bounds for ...
```

The beacon-walk path matches the doc. The bucket-routed path does not.

## Why nobody hit it earlier (probably)

- Without a filter that the planner can route through a bucket, every query
  falls back to the beacon-walk path, which honors the documented semantics.
- The bucket-routing path (`bucketExecPreconditions` + non-bypass filter plan)
  was added with the auto field-bucket index work (`feat(swamp): wire auto-built field buckets into SaveFunction` and friends), and its time-range filter was written fresh in `bucket_exec.go` without realising it had to mirror the beacon-walk semantics exactly.
- The collision only shows up when `ToTime` lands on the exact timestamp of an
  existing record. Cursor pagination is the natural way to trigger it; one-shot
  range queries usually pick a bound that no record sits on.

## Proposed fix

Make the bucket-routed path match the documented (and beacon-walk) behavior.
One-line change in [`app/server/gateway/bucket_exec.go:94`](../../app/server/gateway/bucket_exec.go):

```diff
- if toTime != nil && ts > toNs {
+ if toTime != nil && ts >= toNs {
     continue
 }
```

`FromTime` is already correct (`ts < fromNs → drop`, equivalent to "keep
ts >= fromNs", which is the documented inclusive lower bound).

A regression test belongs next to the existing bucket smoke tests
(`test(bucket): live smoke covers mutation, multi-bucket sync, sequential builds`),
seeding records at known timestamps and asserting the boundary record is
excluded by `ToTime`.

## Client-side mitigation (Trendizz)

Documented for our own future-self. We worked around the bug in our cursor
pagination code without depending on a server fix — when emitting `nextCursor`,
subtract one second from the last record's `CreatedAt`. With second-resolution
`createdAt` storage, this skips exactly the boundary record and nothing else.
Commit `97acf52` on `feature/email-model-redesign` in `trendizz-monorepo`:

```go
// HydrAIDE stores CreatedAt at second resolution and its ToTime upper
// bound on IndexCreationTime is inclusive in practice (the boundary
// record is returned). To make the next page strictly older than the
// current last record, subtract one second from the cursor.
t := emails[len(emails)-1].CreatedAt.Add(-time.Second)
lastCreatedAt = &t
```

We will remove this `-1s` shim once the server fix ships and our SDK pins to
the fixed version.

## Decision needed: which semantics is canonical?

If the documented "exclusive ToTime" is intentional, the bucket path is the bug
(one-line fix as above). If the inclusive behaviour is intentional — for
example, to make `[FromTime, ToTime]` a closed range — then both the SDK doc
comments and the `beacon.findTimeRangeBounds` implementation need to change,
which is a larger ripple. Either is fine for downstream code; the only
non-negotiable is that the two paths agree.

The current Trendizz mitigation works under either decision, so resolution
priority is normal, not urgent.

---

## Resolution

**Decision:** exclusive is canonical. Both paths must apply `ToTime` as an
exclusive upper bound, matching the SDK doc, the protobuf comment, and the
beacon-walk implementation.

**Fix:** [`app/server/gateway/bucket_exec.go`](../../app/server/gateway/bucket_exec.go) `applyTimeRange` — flipped
the upper-bound check from `ts > toNs` to `ts >= toNs`. The function comment now
states the `[FromTime, ToTime)` half-open contract explicitly so future edits
do not regress it. No SDK or proto changes were required.

**Regression test:** [`app/server/gateway/bucket_exec_test.go`](../../app/server/gateway/bucket_exec_test.go) —
`TestApplyTimeRange_ToTimeExclusive` asserts that a record whose CreatedAt
exactly equals `ToTime` is dropped from the bucket-routed result set;
`TestApplyTimeRange_FromTimeInclusive` pins the lower bound to inclusive.
These are unit-level tests against the gateway helper, so they run without
a live HydrAIDE instance.

**Trendizz `-1s` shim:** still safe to keep until the Trendizz SDK pin moves to
a HydrAIDE release that includes this fix. The shim is correct under both the
old and new semantics; removing it is a follow-up, not a precondition.
