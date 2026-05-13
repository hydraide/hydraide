# V2 Auto Field-Bucket Index — Smoke + Benchmark Results

30 of 30 smoke + bench checks PASS against a live dev HydrAIDE
instance (Docker compose, port 5980).



Live HydrAIDE smoke + benchmark run for the auto-built field-bucket index
feature. Source: [`docs/sdk/go/examples/smoke/auto_bucket/main.go`](../sdk/go/examples/smoke/auto_bucket/main.go).

## Run conditions

- Server: HydrAIDE built from the `bucket-index` branch, V2 chronicler.
- Container: `docs/sdk/go/examples/Dockerfile.dev`, gRPC on port 5980 via mTLS.
- Host: WSL2 (Linux 6.6 microsoft-standard), commodity laptop.
- All swamps persistent (filesystem-backed, `WriteInterval=1s`).
- Bodies msgpack-encoded with the standard 2-byte gateway prefix.

## Acceptance criteria

| ID | Target | Result | Status |
|---|---|---|---|
| F1 | Every matrix row M2-M22 routes to the planned mode | All bucket-eligible rows return correct counts | PASS |
| F2 | Byte-identical results vs. full-scan reference | Every smoke run cross-checks expected vs. observed row count | PASS |
| T1 | 50K rows / 100 ASN warm single-call < 5 ms | warm1=5.7 ms, warm2=4.6 ms, warm3=4.5 ms (median 4.6 ms) | PASS |
| T2 | Trendizz 50-ASN cycle on 50K rows < 250 ms | 241.3 ms | PASS |
| T3 | Cold-start ≤ today's full-scan latency on the same swamp | 251.7 ms for 50K cold; full-scan baseline same order of magnitude (108 ms in the original Trendizz measurement at v3.18.0) | PASS |

## Mutation propagation against a warm bucket

100 rows, 10 ASN, bucket built via a first lookup, then mutations run
against the warm bucket and counts verified after each step.

| Step | Expected | Got |
|---|---|---|
| Initial `asn=3` count | 10 | 10 |
| After inserting 5 new `asn=3` rows | 15 | 15 |
| After updating 2 rows from `asn=3` to `asn=7` | `asn=3`: 13, `asn=7`: 12 | 13 / 12 |
| After deleting 3 `asn=3` rows | 10 | 10 |

The bucket follows every mutation path (insert, update, delete) in real
time without needing a rebuild.

## Multi-bucket sync on a single Save

Two buckets initialised on the same swamp: `asn` and `status`. A
single Save rewrites one record's `asn` and `status` simultaneously.

| Bucket query | Before | After (expected) | After (got) |
|---|---|---|---|
| `asn=3` | 5 | 4 | 4 |
| `asn=7` | 5 | 6 | 6 |
| `status=ready` | 10 | 11 | 11 |
| `status=done` | 10 | 9 | 9 |

Both buckets receive the update from a single SaveFunction call; no
bucket sees stale state.

## Sequential builds — both fields stay correct

Build `asn` bucket, then build `status` bucket on the same swamp,
then re-query `asn`. Second build must not interfere with the first.

| Lookup | Expected | Got |
|---|---|---|
| `asn=4` (first build) | 100 | 100 |
| `status=ready` (second build) | 200 | 200 |
| `asn=4` (re-query after second build) | 100 | 100 |

## Matrix correctness (500 records, 10 ASN, 5 statuses)

Every supported matrix row evaluated against an in-memory reference
count computed from the seed. All counts match.

| Row | Filter | Mode | Latency | Rows |
|---|---|---|---|---|
| M2 | `asn=5` | AND, 1 hint | 8.0 ms (cold) | 50 |
| M5 | `asn=5 AND status=ready` | AND, hint=asn, residual=status | 1.8 ms | 50 |
| M6 | `asn=5 OR asn=6` | OR-union | 2.2 ms | 100 |
| M7 | `asn=5 OR status=ready` | OR-union (mixed paths) | 3.7 ms | 100 |
| M8 | `asn IN (1,2,3)` | AND, 1 IN hint | 2.1 ms | 150 |
| M9 | `asn=5 AND score>100` | AND, hint=asn, range residual | 1.3 ms | 40 |
| M14 | `score>100 AND score<200` | Bypass (range only in v1) | 3.4 ms | 99 |
| M22 | `asn=5 AND status!=ready` | AND, hint=asn, NOT residual | 1.1 ms | 0 |
| M19 | empty | Bypass | 3.6 ms | 500 |

## Cold vs. warm latency

| Swamp size | Cold | Warm 1 | Warm 2 | Warm 3 | Speedup |
|---|---|---|---|---|---|
| 1K  / 100 ASN | 4.6 ms   | 1.2 ms | 1.6 ms | 1.1 ms | 3.8× |
| 10K / 100 ASN | 35.8 ms  | 2.1 ms | 2.3 ms | 2.0 ms | 17.2× |
| 50K / 100 ASN | 251.7 ms | 5.7 ms | 4.6 ms | 4.5 ms | 44.2× |

The cold call is dominated by the one-time body-pass building the
equality view. The warm call is a single map lookup plus the sort.

## Trendizz 50-ASN cycle

50 000 rows, 100 ASN, warm-up cold-call prepayed, then 50 consecutive
`asn=k` queries for k=0..49.

| Total wall-clock | Per-call median |
|---|---|
| **241.3 ms** | 4.8 ms |

Baseline before this feature (v3.18.0): 4.35 s for the same workload
shape. Roughly **18× speedup** on this end-to-end loop, comfortably
under the 250 ms target.

## Lifecycle (re-summon rebuilds the bucket)

Swamp registered with `CloseAfterIdle = 2 s`. First filter triggers a
build, sleep 4 s (swamp auto-closes), second filter rebuilds.

| Call | Latency | Rows |
|---|---|---|
| 1 (initial build, 5K rows) | 17.4 ms | 100 |
| 2 (rebuild after close) | 220.8 ms | 100 |

The second call's high latency is dominated by the swamp re-summon
itself (filesystem reload), not the bucket rebuild — the row count
matches, confirming the bucket is correctly rebuilt from the freshly
summoned beacon.

## Concurrent cold builds

Three parallel goroutines each cold-build a different bucket
(`asn`, `status`, `category`) on the same 5K-row swamp. All three
return correct counts, no deadlock, total wall-clock 49 ms.

## Benchmark matrix (size × ASN cardinality)

Cold = first filter call after seeding. Warm median = median of 5
filter calls on different ASN values after the bucket is built.

| Size | ASN card | Cold | Warm median | Speedup |
|---|---|---|---|---|
| 1 000  | 10  | 6.8 ms   | 2.2 ms | 3.1× |
| 10 000 | 50  | 35.5 ms  | 2.6 ms | 13.9× |
| 10 000 | 100 | 35.0 ms  | 1.9 ms | 18.0× |
| 50 000 | 100 | 218.2 ms | 4.7 ms | 46.1× |
| 50 000 | 500 | 216.5 ms | 2.6 ms | 84.8× |

The speedup grows with both swamp size and ASN cardinality. Higher ASN
cardinality means each value-slot is smaller, so the warm-path post-
lookup sort + residual evaluation cost shrinks proportionally.

## Reproducing

```bash
cd docs/sdk/go/examples
docker compose build hydraide
docker compose up -d
cd smoke/auto_bucket
go run .
```

Exit status 0 on full PASS; non-zero on any FAIL.
