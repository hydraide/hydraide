## Auto field-bucket indexes: zero-declaration server-side filter acceleration

HydrAIDE evaluates `FilterGroup` filters server-side. A filter like `asn = 42` walks the swamp's primary beacon and msgpack-decodes every Treasure's body to check the field. On a 50 000-row swamp that means 50 000 body decodes per call. Fine once, expensive in a tight loop.

The auto field-bucket index sits between the swamp and the filter evaluator. The first filter that picks a single body field with an equality or `IN` operator triggers an **in-memory map** keyed by that field's canonical value. Subsequent filters on the same field skip the body-decode pass entirely.

Two things make the feature easy to live with:

- **No declaration.** No `IndexedFields` list, no migration, no schema change. The bucket appears the first time you filter on a field.
- **No persistence.** The bucket is derived state, kept only while the swamp is summoned. Closing the swamp drops every bucket, and the next filter rebuilds.

### What it does, in one paragraph

When you query `asn = 42` for the first time, the swamp takes a snapshot of its in-memory key index, body-decodes every Treasure once, and stores a `map[canonicalValue]map[treasureKey]Treasure` in memory. That first call pays the full body-pass cost (the same cost a non-bucket filter would have paid). Every subsequent call for any value on the same field is then a map lookup plus a stable sort of the small result set. Mutations (`Save`, `Patch`, delete) refresh every initialised bucket through the same `SaveFunction` hook the existing beacons use, so the index stays consistent with the data without a rebuild.

### When the planner uses a bucket

The server-side planner inspects every `FilterGroup` before evaluation. It picks a route per group:

| Filter shape | Route |
|---|---|
| Single `Equal` or `IN` on a body field | Bucket lookup, then optional residual filter |
| AND of one indexable leg plus other legs | Bucket lookup on the indexable leg, residual is the rest |
| OR where every leg is `Equal` or `IN` on a body field | Bucket lookup per leg, deduplicated union |
| `(asn = 1 OR asn = 2) AND status = ready` | The inner OR becomes the candidate set, the AND's other legs become residual |
| AND containing a range (`>`, `<`, BETWEEN) plus an indexable leg | Bucket on the indexable leg, range stays as residual |
| Only range, `NOT_EQUAL`, `CONTAINS`, vector, geo, phrase, nested-slice | Bypass. The query falls back to the legacy beacon walk. |

In v1 the planner picks the first indexable leg it finds in a group; it does not yet score selectivity. Selectivity-aware leg selection via per-value counts is a v1.1 enhancement.

The bucket route is **byte-identical** to the bypass route. The feature is a pure optimisation: no filter shape changes its result.

### Decision: sharding vs. auto-bucket

The bucket index trades memory for lookup speed. Before reaching for it, ask whether the workload would be better served by **sharding**, meaning splitting the data across multiple swamps so the filter axis becomes the swamp name.

| Situation | Use | Why |
|---|---|---|
| One main filter axis (`ASN`, `tenantID`, `region`), high cardinality | **Sharding** (`crawl-queue/<asn>/...`, one swamp per ASN) | Zero index work, `CloseAfterIdle` distributes memory across the live set, axis-level isolation |
| Several filter fields mixed (`asn` + `status` + `category`), and filters combine them | **Auto-bucket** | Sharding by N axes gives N×M×K swamps and pushes set logic to the client |
| One axis, low cardinality (3 statuses, 5 tenants) | **Auto-bucket** | Sharding into 3 huge swamps still has the per-shard size problem |
| Many fields, but a query touches only one at a time | **Sharding** on the most common field, auto-bucket on the rest | Per-shard size stays small, auto-bucket handles the long tail cheaply on the smaller candidate set |
| Hot multi-tenant data | **Sharding** by tenant plus auto-bucket inside each tenant swamp | Isolation and cache locality first, then index acceleration on the secondary fields |

Rule of thumb: if a single field path drives most of the query volume **and** the cardinality is high enough that per-shard size stays sane, sharding wins. If queries are compositional (multiple fields in a single `AND`) or low-cardinality, the bucket index is the simpler design.

### What makes a bucket build (and what doesn't)

For a bucket to build, the data and the filter both need to hold up their end.

**The data side: what to design for.**

- **Body must be msgpack-encoded.** Buckets read the body via `msgpack.Unmarshal`. GOB-encoded bodies cannot be bucket-indexed. Always set `EncodingFormat: hydraidego.EncodingMsgPack` on the swamp pattern.
- **Indexed field must live inside the body.** The `BytesFieldPath` in the filter points into the msgpack map. Treasure metadata (`CreatedAt`, `UpdatedAt`, `ExpiredAt`, the Treasure key itself) is never bucket-indexed. Those are sorted by their own beacons and the filter routes through the legacy beacon walk.
- **Field path is case-sensitive.** `metadata.asn` and `metadata.ASN` are two separate buckets. Stay consistent between writers and readers.
- **Field values must be one of the supported canonical kinds:** `bool`, `int8..int64`, `uint8..uint64`, `float32` / `float64`, `string`, or `nil`. Anything else (slices, maps, `time.Time` embedded in a struct) collapses to the null bucket, and queries on real values return no rows.
- **The field must actually exist in the body.** If half the rows omit the field, those rows go into the "null" slot. A filter like `asn = 42` matches the value slot, not the null slot (which is what you want), but a filter like `asn IS_EMPTY` is not bucket-indexed at all and stays on the bypass route.

**The filter side: what triggers a build.**

- `Equal` on a `BytesFieldPath` with a scalar `CompareValue` builds the bucket.
- `STRING_IN` / `INT32_IN` / `INT64_IN` on a `BytesFieldPath` builds the bucket.
- Anything else (`NOT_EQUAL`, range, `CONTAINS`, `STARTS_WITH`, `IS_EMPTY`, `HAS_KEY`, vector, geo, phrase, nested-slice, NOT-wrapped groups) does **not** build a bucket. The filter still runs correctly; it just goes through the legacy beacon walk.

**Things that look indexable but aren't:**

- `Equal` without `BytesFieldPath`. The proto allows comparing against Treasure-level scalar fields (the typed value field of a Treasure). Those are not body fields, and the planner skips the bucket route.
- `Equal` against a `CreatedAt` / `UpdatedAt` / `ExpiredAt` timestamp value. Same reason: it's metadata, not body.
- `Equal` across a type boundary, for example a body that stores `"42"` as a string while the filter compares against int 42. The canonical normalisation collapses numeric kinds across signedness and lossless float conversion, but it never crosses the string/number boundary.

### Lifecycle and cost

- **Cold start.** First filter on a field path does one body pass. Cost is proportional to the swamp size, identical to today's filter latency on the same swamp. There is no separate "warm-up" step: the first query pays for the build.
- **Steady state.** Subsequent filters on any value of the same field are a map lookup plus a sort of the result set. On a 50K-row swamp the warm latency drops from ~250 ms (cold) to ~5 ms (warm).
- **Mutation cost.** Every `Save` decodes the body once per initialised bucket on that swamp to refresh the index. One bucket per swamp adds roughly a microsecond to a Save. Five buckets, five body decodes per Save. Plan accordingly when designing many fields' worth of buckets.
- **Eviction.** Closing the swamp drops every bucket. The next filter after a re-summon rebuilds.
- **No persistence.** Buckets are not written to disk. There is no recovery cost on swamp summon beyond loading the data itself.

### Concurrency model

A bucket is safe to query while mutations land. The build runs on a snapshot of the swamp's primary key index; mutations that arrive during the build are buffered and replayed in FIFO order at the end. Two callers that race on the same first lookup cooperate via a `sync.Once` inside the bucket, so there is only one snapshot and one build. Two callers building different field paths run in parallel without blocking each other.

The internal lock order is `bucketsMu` then `bucket.mu` then `bucket.pendingMu`. A lookup acquires `bucket.mu.RLock`, copies the matching slice, releases the lock, and works on the copy, so a `LookupEqual` never blocks a concurrent `Save`.

### Verification

Live results on a dev compose instance are in [`docs/benchmarks/V2_BUCKET_RESULTS.md`](../benchmarks/V2_BUCKET_RESULTS.md). 30 of 30 smoke and bench checks pass, including matrix correctness, mutation propagation, multi-bucket sync, sequential builds, lifecycle (re-summon rebuild), and concurrent cold builds.

### Where to go next

- Filter syntax and planner-eligible operators live in [`query-engine.md`](query-engine.md).
- The Go SDK section on filters with bucket-aware pitfalls is in [`.claude/skills/hydraidego/SKILL.md`](../../.claude/skills/hydraidego/SKILL.md) under "Server-side filters" and "Indexing and pagination".
- The proto definitions for `TreasureFilter`, `FilterGroup`, and the `Relational.Operator` enum live in [`proto/hydraide.proto`](../../proto/hydraide.proto).
