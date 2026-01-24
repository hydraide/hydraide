# HydrAIDE Chronicler V1 vs V2 Performance Benchmarks

This directory contains comprehensive benchmarks comparing the V1 (filesystem-based, multi-file) and V2 (append-only, single-file) chronicler implementations.

## Quick Start

Run all benchmarks and generate a comparison report:

```bash
./scripts/benchmark-chronicler.sh
```

Results will be saved to `benchmark-results-<timestamp>/`

## Individual Benchmarks

### Run V2 Benchmarks Only

```bash
go test -bench=BenchmarkV2 -benchmem -benchtime=3x ./app/core/hydra/swamp/chronicler/v2/
```

###Run V1 Benchmarks Only

```bash
go test -bench=BenchmarkV1 -benchmem -benchtime=3x ./app/core/hydra/swamp/chronicler/
```

## Benchmark Scenarios

| Benchmark | Description |
|-----------|-------------|
| **Insert100K** | Insert 100,000 new entries/treasures |
| **Update10K** | Update 10,000 existing entries (from 100K dataset) |
| **Delete10K** | Delete 10,000 entries |
| **Read100K** | Load and index 100,000 entries |
| **MixedWorkload** | 50% updates, 30% inserts, 20% deletes (10K ops) |
| **CompactionNeeded** (V2 only) | Test compaction on highly fragmented data |
| **BlockSizes** (V2 only) | Compare different block sizes (8KB - 128KB) |

## Expected Results

Based on design analysis:

### Write Performance
- **V2 Update**: ~250x faster (append vs read-modify-write)
- **V2 Insert**: Similar or slightly faster
- **V2 Delete**: Much faster (just append DELETE entry)

### Storage Efficiency  
- **File Count**: V1: ~400-1000 files, V2: 1 file (400-1000x reduction)
- **Space Usage**: V2 uses ~30-40% less space (after compaction)
- **ZFS Metadata**: ~99% reduction

### Memory Usage
- **V2**: Lower allocation count (fewer file operations)
- **V2**: More predictable memory patterns

## Understanding the Output

```
BenchmarkV2_Insert100K-32    1    29861982 ns/op    1538614 bytes    15.39 bytes/entry
```

- `29861982 ns/op`: Time to insert 100K entries (≈30ms)
- `1538614 bytes`: Total file size
- `15.39 bytes/entry`: Average bytes per entry

## Metrics Reported

- `ns/op`: Nanoseconds per operation
- `B/op`: Bytes allocated per operation
- `allocs/op`: Number of allocations per operation
- `bytes`: Total file/directory size
- `bytes/entry` or `bytes/treasure`: Size per stored item
- `bytes_before`: Size before operation
- `bytes_after`: Size after operation
- `bytes_growth`: Size increase
- `files_before`, `files_after`: File count (V1 only)
- `fragmentation_%`: Percentage of dead entries (V2 compaction)

## Running Specific Tests

```bash
# Test different block sizes
go test -bench=BenchmarkV2_BlockSizes -benchtime=5x ./app/core/hydra/swamp/chronicler/v2/

# Test compaction performance
go test -bench=BenchmarkV2_CompactionNeeded -benchtime=1x ./app/core/hydra/swamp/chronicler/v2/

# Profile memory usage
go test -bench=BenchmarkV2_MixedWorkload -benchmem -memprofile=mem.out ./app/core/hydra/swamp/chronicler/v2/
go tool pprof mem.out

# Profile CPU usage
go test -bench=BenchmarkV2_MixedWorkload -cpuprofile=cpu.out ./app/core/hydra/swamp/chronicler/v2/
go tool pprof cpu.out
```

## Interpreting Results for Migration Decision

### ✅ Proceed with V2 if:
- Write operations are >10x faster
- Storage reduction is >50%
- File count reduced by >100x
- Memory usage is stable or improved

### ⚠️ Review if:
- V2 is slower than V1 for writes
- Storage increase or no significant reduction
- Memory usage significantly higher

### ❌ Block if:
- V2 consistently fails tests
- Data corruption detected
- Unpredictable performance

## Notes

- Benchmarks use temporary directories (automatically cleaned)
- V1 benchmarks require the full treasure/beacon infrastructure
- V2 benchmarks are more isolated and faster to run
- Times will vary based on hardware (especially SSD speed)
- Results on Samsung 990 PRO should show V2's best performance

## Hardware Requirements

Recommended for accurate benchmarks:
- Fast NVMe SSD (e.g., Samsung 990 PRO)
- 16+ GB RAM
- Linux with ZFS (for V1 production comparison)

## Next Steps

After reviewing benchmark results:

1. If results are favorable: Run `hydraidectl migrate --dry-run`
2. Schedule migration window
3. Execute full migration
4. Deploy V2-enabled code
5. Monitor production metrics

---

Last Updated: 2026-01-21
