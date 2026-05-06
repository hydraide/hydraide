# HydrAIDE Chronicler V2 Benchmarks

This directory contains the benchmark suite for the V2 chronicler — the append-only single-file storage engine that backs every Swamp on disk.

For measured results on a reference machine, see [V2_RESULTS_SUMMARY.md](V2_RESULTS_SUMMARY.md).

## Quick start

Run the full V2 benchmark set:

```bash
go test -bench=BenchmarkV2 -benchmem -benchtime=3x ./app/core/hydra/swamp/chronicler/v2/
```

A wrapper script that captures the output to a timestamped directory:

```bash
./scripts/benchmark-chronicler.sh
```

Results are written to `benchmark-results-<timestamp>/`.

## Benchmark scenarios

| Benchmark | Description |
|---|---|
| `Insert100K` | Insert 100,000 new entries into a fresh Swamp |
| `Update10K` | Update 10,000 entries inside a 100K-entry Swamp |
| `Delete10K` | Delete 10,000 entries |
| `Read100K` | Cold-start hydration: load and index 100,000 entries |
| `MixedWorkload` | 10K ops — 50% updates, 30% inserts, 20% deletes |
| `CompactionNeeded` | Compaction of a heavily fragmented Swamp |
| `BlockSizes` | Sweep across block sizes from 8 KB to 128 KB |

## Reading the output

```
BenchmarkV2_Insert100K-32    1    46422159 ns/op    1538623 bytes    15.39 bytes/entry
```

- `46422159 ns/op` — time for the full insert of 100K entries (~46 ms)
- `1538623 bytes` — final file size (~1.54 MB)
- `15.39 bytes/entry` — average bytes per entry on disk

## Metrics reported

- `ns/op` — nanoseconds per operation
- `B/op` — bytes allocated per operation
- `allocs/op` — allocation count per operation
- `bytes` — total file size
- `bytes/entry` — average size per stored entry
- `bytes_before`, `bytes_after`, `bytes_growth` — file size around the measured operation
- `fragmentation_%` — fraction of dead entries (compaction scenario)

## Targeted runs

```bash
# Sweep block sizes
go test -bench=BenchmarkV2_BlockSizes -benchtime=5x ./app/core/hydra/swamp/chronicler/v2/

# Compaction under fragmentation
go test -bench=BenchmarkV2_CompactionNeeded -benchtime=1x ./app/core/hydra/swamp/chronicler/v2/

# Memory profile
go test -bench=BenchmarkV2_MixedWorkload -benchmem -memprofile=mem.out ./app/core/hydra/swamp/chronicler/v2/
go tool pprof mem.out

# CPU profile
go test -bench=BenchmarkV2_MixedWorkload -cpuprofile=cpu.out ./app/core/hydra/swamp/chronicler/v2/
go tool pprof cpu.out
```

## Hardware notes

Numbers vary with disk and CPU. Recommended for reproducible comparisons:

- Fast NVMe SSD (e.g. Samsung 990 PRO)
- 16+ GB RAM
- Linux

The reference results in [V2_RESULTS_SUMMARY.md](V2_RESULTS_SUMMARY.md) were produced on a Threadripper 2950X with a Samsung 990 PRO.

Benchmarks use temporary directories that are cleaned up automatically.

---

Last updated: 2026-01-21
