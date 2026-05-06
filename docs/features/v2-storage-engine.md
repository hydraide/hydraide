## Storage engine — architecture and operation

The storage engine is append-only. Each Swamp is one `.hyd` file. Writes are buffered in memory, flushed in compressed blocks, and never modify existing bytes. Compaction runs automatically when fragmentation crosses a threshold.

For measured insert/update/delete/read latencies and on-disk sizes on a Threadripper 2950X + Samsung 990 PRO, see [V2 benchmark results](../benchmarks/V2_RESULTS_SUMMARY.md). To reproduce, see [run instructions](../benchmarks/CHRONICLER_BENCHMARKS.md).

---

### Design goals

1. **Minimise file I/O.** Fewer file operations, less SSD wear.
2. **Block-aligned writes.** Default 16 KB block flush threshold aligns well with common filesystem record sizes.
3. **Append-only writes.** Existing bytes are never modified; updates and deletes are new entries.
4. **One file per Swamp.** A single `.hyd` file per Swamp replaces the legacy multi-chunk folder layout.
5. **Lazy persistence.** Writes accumulate in a buffer and flush as full compressed blocks.

---

### 📁 File Format

Each Swamp is stored in a **single `.hyd` file**. The file is created at the same directory level where the V1 chunk folder would have been, with a `.hyd` extension appended:

```
V1 (legacy):  /data/sanctuary/realm/ab/swampname/   ← folder with many chunk files
V2:           /data/sanctuary/realm/ab/swampname.hyd ← one file
```

The `.hyd` file has a fixed structure. Starting with server 3.3.0+, the swamp name is stored as plain text immediately after the header, enabling fast metadata scanning without decompressing any blocks:

```
V2 format (legacy):
┌─────────────────────────────────────────────────────────────┐
│                    FILE HEADER (64 bytes)                    │
│  Magic: "HYDR" (4B) | Version: 2 (2B) | Flags (2B)          │
│  CreatedAt (8B) | ModifiedAt (8B) | BlockSize (4B)           │
│  EntryCount (8B) | BlockCount (8B) | Reserved (16B)          │
├─────────────────────────────────────────────────────────────┤
│                     COMPRESSED BLOCKS ...                     │
└─────────────────────────────────────────────────────────────┘

V2 optimized format (current, with embedded swamp name):
┌─────────────────────────────────────────────────────────────┐
│                    FILE HEADER (64 bytes)                    │
│  Magic: "HYDR" (4B) | Version: 3 (2B) | Flags (2B)          │
│  CreatedAt (8B) | ModifiedAt (8B) | BlockSize (4B)           │
│  EntryCount (8B) | BlockCount (8B)                           │
│  NameLength (2B) | Reserved (14B)                            │
├─────────────────────────────────────────────────────────────┤
│              SWAMP NAME (NameLength bytes, plain UTF-8)      │
├─────────────────────────────────────────────────────────────┤
│                     COMPRESSED BLOCKS ...                     │
└─────────────────────────────────────────────────────────────┘
```

The optimized format stores the swamp name (e.g., `users/profiles/alice`) in plain text right after the 64-byte header. This allows tools like `hydraidectl explore` to read only ~100 bytes per file to discover swamp names, instead of decompressing the first block. Older files are automatically upgraded during compaction. The reader is fully backward-compatible with files that don't have the embedded name.

#### Entry binary format (variable size)

Each entry inside a block has the following binary layout:

```
Operation  (1 byte)  – 1=INSERT, 2=UPDATE, 3=DELETE, 4=METADATA
KeyLen     (2 bytes) – length of the key string
Key        (N bytes) – the Treasure's unique key (UTF-8 string)
DataLen    (4 bytes) – length of the data payload
Data       (M bytes) – encoded Treasure bytes (msgpack by default; GOB for legacy data) — empty for DELETE
```

Minimum entry size: **7 bytes** (1+2+4 with empty key and empty data — key cannot actually be empty).

#### Metadata entry (legacy fallback)

In older files, a special `METADATA` entry is written as the first entry in the first block, with key `__swamp_meta__` and the swamp name as data. This enables tools to reverse-map hashed folder names to human-readable swamp names.

**In the optimized format, this entry is no longer needed** because the swamp name is stored as plain text after the header. Older files with metadata entries are still fully supported — the name is read from the compressed block as a fallback. During compaction, files are automatically upgraded to the optimized format.

---

### 🧠 Memory and Write Buffer

The V2 engine uses a **persistent writer** (`FileWriter`) that stays open while the Swamp is active in memory. This eliminates the overhead of repeatedly opening and closing the file on every write cycle.

#### Write Buffer Architecture

```
┌──────────────────────────────────────────────────────────────────┐
│                         MEMORY                                    │
│  ┌────────────────────────────────────────────────────────────┐  │
│  │                    WriteBuffer                              │  │
│  │  ┌─────────┐ ┌─────────┐ ┌─────────┐ ┌─────────┐          │  │
│  │  │ Entry 1 │ │ Entry 2 │ │ Entry 3 │ │ Entry N │ ...       │  │
│  │  └─────────┘ └─────────┘ └─────────┘ └─────────┘          │  │
│  │                                                             │  │
│  │  Accumulated uncompressed size: tracked per-entry          │  │
│  │  Flush threshold: 16 KB (DefaultMaxBlockSize)              │  │
│  └────────────────────────────────────────────────────────────┘  │
│                              │                                    │
│          Buffer reaches 16KB │  OR  Flush() called               │
│                              ▼                                    │
│                     ┌────────────────┐                            │
│                     │ Snappy Compress │ ← single block            │
│                     └────────────────┘                            │
│                              │                                    │
└──────────────────────────────┼────────────────────────────────────┘
                               ▼
┌──────────────────────────────────────────────────────────────────┐
│                          DISK (.hyd file)                         │
│   [FILE HEADER] [BLOCK 1] [BLOCK 2] ... [BLOCK N] [BLOCK N+1]   │
│                                                    ↑ appended    │
└──────────────────────────────────────────────────────────────────┘
```

**The block flush threshold is 16 KB of uncompressed entry data** (the `DefaultMaxBlockSize` constant in the source). This value is **not configurable per Swamp** — it is a fixed server-side internal constant. After Snappy compression, the actual bytes written per block are typically 40–60% of the original size.

**Block flush is triggered by exactly two conditions:**
1. The accumulated uncompressed size in the `WriteBuffer` reaches or exceeds **16 KB** → automatic flush during `WriteEntry()`
2. An explicit `Flush()` call is made (e.g. immediately after writing the METADATA entry on new file creation)

A flush always produces **exactly one block**: all buffered entries are serialized, Snappy-compressed together, a 16-byte `BlockHeader` is prepended (with `CRC32` checksum of the compressed payload), and the result is appended to the file.

---

### ⚡ Write Flow

When a client saves a Treasure, here is the exact sequence:

```
1. Client calls CatalogSave / ProfileSave / etc.
       │
       ▼
2. Swamp marks Treasure as "waiting for write" (in-memory queue)
       │
       ▼
3. WriteInterval ticker fires  ← controlled by client's RegisterSwamp setting
   (OR WriteInterval = 0 → immediate flush on every change)
       │
       ▼
4. swamp.fileWriterHandler() is called
       │
       ▼
5. chroniclerV2.Write([]treasure) is called
       │
       ▼
6. ensureWriter():
   └── Writer already open? → reuse it (no file re-open overhead)
   └── Writer nil or closed? → open file (create or append)
       └── New file? → write METADATA entry + immediate Flush()
       └── Existing file missing metadata? → write METADATA + Flush() (repair)
       │
       ▼
7. For each Treasure in batch:
   ├── Deleted (DeletedAt > 0)?  → Entry{Op: DELETE, Key, Data: nil}
   ├── Has no FileName yet?      → Entry{Op: INSERT, Key, Data: encoded bytes (msgpack/GOB)}
   └── Has FileName already?     → Entry{Op: UPDATE, Key, Data: encoded bytes (msgpack/GOB)}
       │
       ▼
8. WriteBuffer.Add(entry):
   └── Accumulated size < 16KB → stays in buffer, no disk I/O
   └── Accumulated size >= 16KB → automatic Flush() → 1 block appended to file
       │
       ▼
9. FilePointerCallback fired for each Treasure
       (tells the Swamp which file the Treasure lives in → always the .hyd file)
```

**The writer stays open** (`FileWriter` holds an open `*os.File`) for the entire duration that the Swamp is loaded in memory. It is only closed when the Swamp closes.

---

### 🔒 Swamp Close Flow

When `CloseAfterIdle` expires (no reads or writes for the configured duration) or a graceful shutdown is triggered:

```
1. swamp.Close() is called
       │
       ▼
2. All pending Treasures in "waiting for write" queue are flushed:
   └── chroniclerV2.Write() called with remaining batch
       └── Remaining buffer entries → Flush() → last block appended
       │
       ▼
3. chroniclerV2.Close() is called
   └── flushLocked() → flush any remaining buffer → last block(s) written
   └── Header updated (BlockCount, EntryCount written back to byte 0-63)
   └── file.Close() → OS file handle released
       │
       ▼
4. Compaction check (maybeCompactUnlocked):
   └── Open file with FileReader
   └── Calculate fragmentation = (total entries - live entries) / total entries
   └── Fragmentation < 50%?  → skip compaction
   └── Fragmentation >= 50%? → run Compact()
       │
       ▼
5. Swamp removed from in-memory map → GC frees memory
```

---

### 🔄 Read Flow (Hydration)

When a Swamp is accessed after being idle (not in memory):

```
1. SummonSwamp(name) called
       │
       ▼
2. Swamp not in memory → create new Swamp object
       │
       ▼
3. chroniclerV2.Load(beacon) called
       │
       ▼
4. FileReader opens .hyd file, reads 64-byte FILE HEADER
       │
       ▼
5. Sequential read of all blocks from byte 64 onward:
   └── Read BlockHeader (16 bytes) → decompress payload → parse Entries
       │
       ▼
6. BuildIndex (replay log):
   ├── METADATA entry → extract swamp name (ignored in data index)
   ├── INSERT / UPDATE → index[key] = latestData  (overwrites previous)
   └── DELETE        → delete(index, key)
       │
       ▼
7. All live entries pushed into the in-memory Beacon index
       │
       ▼
8. Writer is NOT opened yet (lazy initialization — opened on first write)
       │
       ▼
9. Swamp is ready in memory with full Treasure state
```

The replay mechanism means the engine never needs random-access reads. It reads the file sequentially from start to finish exactly once, building the final live state by applying all logged operations in order.

---

### 🗜️ Compaction

Over time, the `.hyd` file accumulates **dead entries**: every UPDATE writes a new entry without removing the old one, and every DELETE writes a tombstone without physically removing the original INSERT. This is the append-only trade-off.

**Fragmentation formula:**
```
fragmentation = (total_entry_count - live_entry_count) / total_entry_count
```

**When compaction is triggered:**
- Automatically on every `swamp.Close()`, if `fragmentation >= 0.5` (50%)
- Manually at any time via `hydraidectl compact`

**Compaction process (atomic):**
```
1. Open .hyd file with FileReader
2. Calculate fragmentation → skip if below threshold
3. LoadIndex() → build map of only live entries (key → latest data)
4. Write all live entries to a NEW temp file: swampname.hyd.compact
   └── Optimized header with swamp name in plain text (auto-upgrade)
   └── Each live key written as OpInsert
5. writer.Close() on temp file → header finalized, file synced
6. os.Rename(swampname.hyd.compact, swampname.hyd) → atomic replacement
7. Old file is gone; new compact file takes its place
```

**Note:** Compaction always outputs the optimized format with embedded swamp name in the header. This means older files are automatically upgraded over time as they get compacted.

Example:
```
Before compaction (66% fragmentation):
┌──────────────────────────────────────────────────────┐
│ META:meta │ INSERT:key1 │ UPDATE:key1 │ DELETE:key2   │ ← block 1
│ INSERT:key2 │ UPDATE:key1 │ INSERT:key3               │ ← block 2
└──────────────────────────────────────────────────────┘
total=6 entries, live=2 (key1, key3)

After compaction (0% fragmentation):
┌──────────────────────────────────────────────────────┐
│ META:meta │ INSERT:key1 │ INSERT:key3                 │ ← block 1
└──────────────────────────────────────────────────────┘
total=3 entries (2 live + 1 meta), 1 block
```

---

### Performance characteristics

Measured on a Threadripper 2950X + Samsung 990 PRO. See [V2 benchmark results](../benchmarks/V2_RESULTS_SUMMARY.md) for raw output and methodology, and [CHRONICLER_BENCHMARKS](../benchmarks/CHRONICLER_BENCHMARKS.md) for the run scripts.

| Operation | Measurement |
|---|---|
| Insert 100,000 entries | ~46 ms (~2.15 M inserts/sec) |
| Update 10,000 entries (in a 100K Swamp) | ~3.75 ms (~2.67 M updates/sec) |
| Delete 10,000 entries | ~1.66 ms (~6 M deletes/sec) |
| Cold read of 100,000 entries (full index rebuild) | ~81 ms |
| Average bytes per entry on disk | ~15.4 bytes |
| File count per Swamp | 1 (`<swamp>.hyd`) |

---

### 🛡️ Reliability Features

1. **CRC32 per block**: Every block carries a checksum of its compressed payload. A mismatch on read signals corruption.
2. **Append-only**: Existing bytes in the file are never overwritten during normal operation — only the 64-byte header is rewritten on `Close()` to update `BlockCount` and `EntryCount`.
3. **Atomic compaction**: The compacted file is built in a temp path and only swapped in via `os.Rename` (atomic on all POSIX systems). A crash during compaction leaves the original file intact.
4. **Metadata flush**: The METADATA entry (swamp name) is always flushed immediately to disk after being written, ensuring it is visible even if the process crashes before the next write-interval.
5. **Graceful recovery**: If the last block is partially written (e.g., crash mid-write), the reader will hit an `io.EOF` or checksum mismatch on that block and stop reading. All previously valid blocks remain intact.

---

### 🔧 Configuration

The following settings are **client-controlled** via `RegisterSwamp` and remain fully active in V2:

```go
h.RegisterSwamp(ctx, &hydraidego.RegisterSwampRequest{
    SwampPattern:   "users/profiles/*",

    // CloseAfterIdle: how long the Swamp stays loaded in memory after the last
    // read or write. When this expires, the Swamp is closed, all remaining
    // buffered writes are flushed to disk, and memory is released.
    // Fully active in V2 — the client controls memory lifetime per Swamp.
    CloseAfterIdle: 6 * time.Hour,

    FilesystemSettings: &hydraidego.SwampFilesystemSettings{
        // WriteInterval: how often the write-ticker fires to flush in-memory
        // changes (Treasures in the "waiting for write" queue) to the .hyd file.
        // Fully active in V2 — the client controls flush frequency per Swamp.
        // 0 = flush immediately on every change (highest durability, more I/O).
        WriteInterval: 10 * time.Second,

        // MaxFileSize: V1-only field, completely ignored by V2. Do not set.
    },
})
```

**Internal server-side constants** (not configurable per Swamp):
- `DefaultMaxBlockSize = 16384` – block flush threshold in bytes (16 KB uncompressed)
- `CompactionThreshold = 0.5` – fragmentation ratio above which compaction runs on Close()
- Block compression: **Snappy** (always, not configurable)
- File format version: **3** (magic bytes: `HYDR`, backward-compatible with V2)

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
