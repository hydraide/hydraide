# HydrAIDE Chronicler V2 — Benchmark Results

**Date:** 2026-01-21
**Hardware:** AMD Ryzen Threadripper 2950X (32 threads), Samsung 990 PRO NVMe

These are raw measurements taken from the Go benchmark suite under `app/core/hydra/swamp/chronicler/v2/`. To reproduce, see [CHRONICLER_BENCHMARKS.md](CHRONICLER_BENCHMARKS.md).

---

## Summary

| Operation | Time | Throughput |
|---|---|---|
| Insert 100,000 entries | ~46 ms | ~2.15 M inserts/sec |
| Update 10,000 entries (in a 100K Swamp) | ~3.75 ms | ~2.67 M updates/sec |
| Delete 10,000 entries | ~1.66 ms | ~6 M deletes/sec |
| Read 100,000 entries (cold start, full index rebuild) | ~81 ms | ~1.23 M entries/sec |
| Mixed workload, 10K ops (50% update / 30% insert / 20% delete) | ~3.33 ms | — |
| Compaction of a 90% fragmented 100K Swamp | ~1.07 s | reclaims ~90% of file size |

| Storage metric | Value |
|---|---|
| Bytes per entry (average) | ~15.4 bytes |
| File count per Swamp | 1 (`<swamp>.hyd`) |
| File size for 100K entries | ~1.54 MB |

---

## Detailed results

### 1. Insert 100,000 entries

```
BenchmarkV2_Insert100K-32    1    46422159 ns/op    1538623 bytes    15.39 bytes/entry
```

- Time: 46.4 ms → ~2.15 M inserts/sec
- File size after insert: 1.54 MB
- Average entry size: 15.39 bytes
- Write throughput: ~33 MB/sec

### 2. Update 10,000 entries inside a 100K Swamp

```
BenchmarkV2_Update10K-32    1    3752886 ns/op
  1504213 bytes_before
  1658316 bytes_after
   154103 bytes_growth
```

- Time: 3.75 ms → ~2.67 M updates/sec
- File growth from 10K updates: 154 KB
- Append throughput: ~41 MB/sec

The append-only design means an update is a single new entry written at the tail; the previous version becomes garbage that is reclaimed by compaction.

### 3. Delete 10,000 entries

```
BenchmarkV2_Delete10K-32    1    1664445 ns/op
```

- Time: 1.66 ms → ~6 M deletes/sec
- Mechanism: a DELETE entry is appended; the actual data is reclaimed at compaction time.

### 4. Read 100,000 entries (cold start, full index rebuild)

```
BenchmarkV2_Read100K-32    1    81390403 ns/op
```

- Time: 81.4 ms
- Throughput: ~1.23 M entries/sec
- File scanned: ~1.5 MB → ~18.4 MB/sec including decompression and index rebuild

This is the cold-start hydration cost. Once the Swamp is loaded, subsequent reads serve from memory.

### 5. Mixed workload (10,000 ops)

50% updates, 30% inserts, 20% deletes.

```
BenchmarkV2_MixedWorkload-32    1    3332036 ns/op
  1009350 bytes_before
  1159837 bytes_after
        1 files
```

- Time: 3.33 ms for 10K mixed operations
- File growth: 150 KB
- File count remains 1.

### 6. Compaction of a 90% fragmented Swamp

```
BenchmarkV2_CompactionNeeded-32    1    1072090316 ns/op
   90.91% fragmentation
  100000   live_entries
 1100000   total_entries
 11079440  bytes_before  (10.6 MB)
  1108991  bytes_after   (1.08 MB)
  9970449  bytes_saved   (9.5 MB, 90%)
```

- Time: 1.07 s for 100K live entries inside a heavily fragmented Swamp.
- Size reduction: 10.6 MB → 1.08 MB (~90% reclaimed).
- Live entry ratio: 100K live / 1.1M total = 9.1%.

In practice this level of fragmentation is unusual; compaction is triggered far earlier under the engine's default thresholds.

### 7. Block size comparison (10K entries)

| Block size | Time | File size |
|---|---|---|
| 8 KB  | 7.7 ms | 101 KB |
| 16 KB | 7.4 ms | 100 KB |
| 32 KB | 7.4 ms | 100 KB |
| **64 KB** | **7.3 ms** | **94 KB** |
| 128 KB | 7.3 ms | 94 KB |

64 KB is the default — a balance between compression ratio and write latency.

---

## Projection for the Trendizz workload

Assuming roughly 1,000,000 Swamps with ~100M words indexed and ~10 saves per Swamp per day, the V2 engine produces:

| Metric | V2 value |
|---|---|
| File count | ~1 M files (one per Swamp) |
| Storage footprint | ~300–400 GB |
| Daily write volume | ~40 GB/day |

These are projections based on the per-Swamp measurements above, not direct cluster measurements.

---

## Reproducing these numbers

See [CHRONICLER_BENCHMARKS.md](CHRONICLER_BENCHMARKS.md) for the run scripts and per-scenario commands. Numbers will vary with hardware; the Threadripper 2950X + Samsung 990 PRO results above set the reference baseline.
