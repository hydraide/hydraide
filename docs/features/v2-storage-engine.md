## V2 Storage Engine – Architecture and Operation

HydrAIDE 3.0 introduces the **V2 Storage Engine**, a completely redesigned append-only storage format that delivers **32-112x faster writes**, **50% smaller storage**, and **95% fewer files** compared to the legacy V1 engine.

This document explains how the V2 engine works under the hood, why it's so fast, and how data flows from memory to disk.

---

### 🎯 Design Goals

The V2 engine was designed with these priorities:

1. **Minimize file I/O** – Reduce the number of file operations to maximize SSD lifespan
2. **Optimize for ZFS** – Align block sizes with ZFS record sizes (16KB default)
3. **Append-only writes** – Never modify existing data, only append new entries
4. **Single file per Swamp** – Replace hundreds of chunk files with one `.hyd` file
5. **Lazy persistence** – Buffer writes in memory and flush efficiently

---

### 📁 File Format

Each Swamp is stored in a single `.hyd` file with this structure:

```
┌─────────────────────────────────────────────────────────────┐
│                    FILE HEADER (64 bytes)                    │
│  Magic: "HYDR" | Version: 2 | Created | Modified | Stats    │
├─────────────────────────────────────────────────────────────┤
│                         BLOCK 1                              │
│  ┌─────────────────────────────────────────────────────────┐│
│  │ Block Header (16 bytes)                                 ││
│  │ Compressed Size | Uncompressed Size | Entry Count | CRC ││
│  ├─────────────────────────────────────────────────────────┤│
│  │ Compressed Entry Data (Snappy)                          ││
│  │ [Entry1][Entry2][Entry3]...                             ││
│  └─────────────────────────────────────────────────────────┘│
├─────────────────────────────────────────────────────────────┤
│                         BLOCK 2                              │
│                          ...                                 │
├─────────────────────────────────────────────────────────────┤
│                         BLOCK N                              │
└─────────────────────────────────────────────────────────────┘
```

Each **entry** contains:
- **Operation**: INSERT (1), UPDATE (2), DELETE (3), or METADATA (4)
- **Key**: The treasure's unique identifier
- **Data**: GOB-encoded treasure data (or empty for DELETE)

---

### 🧠 Memory and Write Buffer

The V2 engine uses a **persistent writer** that stays open while the Swamp is active. This eliminates the overhead of repeatedly opening and closing files.

#### Write Buffer Architecture

```
┌──────────────────────────────────────────────────────────────────┐
│                         MEMORY                                    │
│  ┌────────────────────────────────────────────────────────────┐  │
│  │                    Write Buffer                             │  │
│  │  ┌─────────┐ ┌─────────┐ ┌─────────┐ ┌─────────┐           │  │
│  │  │ Entry 1 │ │ Entry 2 │ │ Entry 3 │ │ Entry 4 │ ...       │  │
│  │  └─────────┘ └─────────┘ └─────────┘ └─────────┘           │  │
│  │                                                             │  │
│  │  Current Size: 8KB          Max Size: 16KB                  │  │
│  └────────────────────────────────────────────────────────────┘  │
│                              │                                    │
│                              ▼ (when buffer full OR Close())      │
│                     ┌────────────────┐                            │
│                     │ Snappy Compress │                           │
│                     └────────────────┘                            │
│                              │                                    │
└──────────────────────────────┼────────────────────────────────────┘
                               ▼
┌──────────────────────────────────────────────────────────────────┐
│                          DISK                                     │
│                     ┌────────────────┐                            │
│                     │  .hyd file     │                            │
│                     │  [Block N+1]   │  ← Append only             │
│                     └────────────────┘                            │
└──────────────────────────────────────────────────────────────────┘
```

**Key behaviors:**

1. **Buffered writes**: Entries accumulate in the write buffer (up to 16KB uncompressed)
2. **Automatic flush**: When the buffer reaches 16KB, it's compressed and written as a new block
3. **Close flush**: When the Swamp closes, any remaining buffer data is flushed
4. **Metadata immediate flush**: Swamp name metadata is flushed immediately to ensure consistency

---

### ⚡ Write Flow

When you save a Treasure, here's what happens:

```
1. Save(treasure)
       │
       ▼
2. Swamp adds to "waiting for write" queue
       │
       ▼
3. Write Ticker fires (default: every 10 seconds)
   OR immediate flush if WriteInterval = 0
       │
       ▼
4. Chronicler.Write(treasures[])
       │
       ▼
5. ensureWriter() - Opens file if needed
   └── First write? → Write metadata entry (swamp name) + FLUSH
       │
       ▼
6. For each treasure:
   └── Encode to GOB → Create Entry → Add to WriteBuffer
       │
       ▼
7. Buffer full? → Compress with Snappy → Append block to file
       │
       ▼
8. Swamp.Close() → Flush remaining buffer → Update header → Close file
```

---

### 🔄 Read Flow (Hydration)

When a Swamp is accessed after being idle:

```
1. SummonSwamp(name)
       │
       ▼
2. Swamp not in memory → Create new Swamp
       │
       ▼
3. Chronicler.Load(beacon)
       │
       ▼
4. Open .hyd file → Read all blocks
       │
       ▼
5. For each block:
   └── Decompress → Parse entries → Apply to index
       │
       ▼
6. Build final state:
   - INSERT/UPDATE → Add/update in index
   - DELETE → Remove from index
   - METADATA → Extract swamp name
       │
       ▼
7. Swamp ready in memory with all Treasures
```

The **replay** mechanism means we never need to compact during reads – we simply replay the log and keep only the latest state of each key.

---

### 🗜️ Compaction

Over time, the `.hyd` file accumulates dead entries (updates and deletes). Compaction rewrites the file with only live entries.

**When compaction runs:**
- On `Close()` if fragmentation exceeds threshold (default: 50%)
- Manually via `hydraidectl compact`

**Compaction process:**
1. Read all entries and build live index
2. Write new file with only live entries (including metadata)
3. Atomic rename to replace old file

```
Before Compaction:
┌─────────────────────────────────────────┐
│ INSERT key1 │ UPDATE key1 │ DELETE key2 │  ← 3 entries
│ INSERT key2 │ UPDATE key1 │ INSERT key3 │  ← 6 total
└─────────────────────────────────────────┘
Fragmentation: 66% (4 dead, 2 live)

After Compaction:
┌─────────────────────────────────────────┐
│ METADATA │ INSERT key1 │ INSERT key3   │  ← 3 entries (2 live + meta)
└─────────────────────────────────────────┘
Fragmentation: 0%
```

---

### 📊 Performance Characteristics

| Operation | V1 Engine | V2 Engine | Improvement |
|-----------|-----------|-----------|-------------|
| Write 1K treasures | 45ms | 1.4ms | **32x faster** |
| Write 10K treasures | 450ms | 4ms | **112x faster** |
| File count (10K treasures) | ~200 files | 1 file | **99.5% fewer** |
| Storage size | 100% | ~50% | **50% smaller** |
| SSD write amplification | High | Minimal | **100x less wear** |

---

### 🛡️ Reliability Features

1. **Atomic block writes**: Each block is written atomically with CRC32 checksum
2. **Append-only**: Existing data is never modified, preventing corruption
3. **Graceful recovery**: Corrupted trailing blocks can be truncated without data loss
4. **Metadata persistence**: Swamp name is written and flushed immediately on file creation

---

### 🔧 Configuration

V2 engine settings can be configured per-Swamp:

```go
h.RegisterSwamp(ctx, &hydraidego.RegisterSwampRequest{
    SwampPattern:   "users/profiles/*",

    // CloseAfterIdle controls how long the Swamp stays loaded in memory after the last access.
    // Once this idle period expires with no active reads or writes, the Swamp is automatically
    // closed and any remaining in-memory changes are flushed to disk.
    // This setting is fully active in V2 — the client controls memory lifetime per Swamp.
    CloseAfterIdle: 6 * time.Hour,

    FilesystemSettings: &hydraidego.SwampFilesystemSettings{
        // WriteInterval controls how often the V2 engine flushes in-memory changes to the .hyd file.
        // This setting is fully active in V2 — the client controls flush frequency per Swamp.
        // Lower values = more durable (more frequent disk writes).
        // Higher values = better throughput (fewer disk writes, but more data at risk on crash).
        WriteInterval: 10 * time.Second,

        // MaxFileSize is a V1-only field. Do NOT set this for V2 Swamps — it is ignored.
        // The V2 engine uses a single append-only .hyd file with automatic internal block
        // management. There is no concept of a configurable max file size in V2.
    },
})
```

**Global settings** (in HydrAIDE server config):
- `UseV2Engine: true` – Enable V2 for all new Swamps (default: true)
- `MaxBlockSize: 16384` – Block size in bytes (default: 16KB, optimized for ZFS)
- `CompactionThreshold: 0.5` – Fragmentation ratio to trigger auto-compaction

---

### 🔄 Migration from V1

Existing V1 data can be migrated to V2 format using `hydraidectl`:

```bash
# Dry-run first
hydraidectl migrate --instance myinstance --dry-run

# Full migration with automatic cleanup
hydraidectl migrate --instance myinstance --full
```

See [Migration Guide](../hydraidectl/hydraidectl-migration.md) for detailed instructions.

---

### 📈 When to Use V2

**Always use V2** for new deployments. It's faster, smaller, and more reliable.

The only reason to stay on V1 is if you have specific tooling that depends on the multi-file chunk format – but even then, migration is straightforward.

---

This architecture makes HydrAIDE one of the fastest embedded data engines available, while maintaining simplicity and reliability.
