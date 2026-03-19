package explorer

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	v2 "github.com/hydraide/hydraide/app/core/hydra/swamp/chronicler/v2"
)

// scanDirectory walks the data directory, finds all .hyd files,
// and populates the index with swamp metadata.
func (e *Explorer) scanDirectory(ctx context.Context, status *ScanStatus) error {
	// First pass: collect all .hyd file paths
	var hydFiles []string
	err := filepath.WalkDir(e.dataPath, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // Skip inaccessible directories
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if d.IsDir() {
			return nil
		}
		if filepath.Ext(path) == ".hyd" {
			hydFiles = append(hydFiles, path)
		}
		return nil
	})
	if err != nil {
		return err
	}

	atomic.StoreInt64(&status.TotalFiles, int64(len(hydFiles)))

	if len(hydFiles) == 0 {
		return nil
	}

	// Process files in parallel
	workers := runtime.NumCPU() * 4
	if workers > 64 {
		workers = 64
	}
	if workers > len(hydFiles) {
		workers = len(hydFiles)
	}

	workCh := make(chan string, len(hydFiles))
	for _, f := range hydFiles {
		workCh <- f
	}
	close(workCh)

	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for filePath := range workCh {
				if ctx.Err() != nil {
					return
				}
				detail, err := e.scanFile(filePath)
				if err != nil {
					atomic.AddInt64(&status.ErrorCount, 1)
					slog.Debug("failed to scan .hyd file",
						"path", filePath,
						"error", err)
					continue
				}
				if detail != nil {
					e.idx.add(detail)
				}
				atomic.AddInt64(&status.ScannedFiles, 1)
			}
		}()
	}

	wg.Wait()
	return ctx.Err()
}

// scanFile reads metadata from a single .hyd file without reading block data.
func (e *Explorer) scanFile(filePath string) (*SwampDetail, error) {
	// Get file size
	info, err := os.Stat(filePath)
	if err != nil {
		return nil, err
	}

	// Open reader (reads header + V3 name)
	reader, err := v2.NewFileReader(filePath)
	if err != nil {
		return nil, err
	}
	defer reader.Close()

	header := reader.GetHeader()

	// Get swamp name
	swampName := reader.GetSwampName()
	if swampName == "" {
		// V2 fallback: read only until we find the OpMetadata entry
		// (typically the very first entry in the first block)
		reader.ReadAllEntries(func(entry v2.Entry) bool {
			if entry.Operation == v2.OpMetadata && entry.Key == v2.MetadataEntryKey {
				swampName = string(entry.Data)
				return false // stop reading
			}
			return true
		})
	}

	if swampName == "" {
		// No name found — skip this file
		slog.Debug("skipping .hyd file without swamp name", "path", filePath)
		return nil, nil
	}

	// Parse sanctuary/realm/swamp from full name
	parts := strings.SplitN(swampName, "/", 3)
	if len(parts) != 3 {
		slog.Debug("skipping .hyd file with invalid name format",
			"path", filePath,
			"name", swampName)
		return nil, nil
	}

	// Extract island ID from path
	islandID := extractIslandID(e.dataPath, filePath)

	entryCount := header.EntryCount
	blockCount := header.BlockCount
	var estimatedMemorySize uint64

	// Memory estimation formula (calculated purely from file block headers, no decompression needed):
	//
	//   EstimatedMemory = UncompressedDataSize + (EntryCount × GoOverheadPerEntry)
	//
	// Component 1 — UncompressedDataSize:
	//   Sum of BlockHeader.UncompressedSize across all blocks. On disk the data is Snappy-compressed;
	//   this value represents the actual decompressed payload (GOB-encoded treasures + entry headers).
	//   When loaded, the GOB data is decoded into Go structs whose size is roughly similar to the
	//   encoded form, so the uncompressed size is a good approximation of the data portion in RAM.
	//
	// Component 2 — Go object overhead per entry (512 bytes):
	//   Each treasure loaded into memory carries fixed struct overhead that is NOT part of the
	//   serialized data. Breakdown:
	//     treasure struct:
	//       sync.RWMutex            24 B
	//       Model (strings/int64s) 120 B   (string headers 16B each, int64s 8B each)
	//       guard.Guard             80 B   (RWMutex + Cond ptr + slice header + fields)
	//       saveMethod func         16 B
	//       10 bool flags + padding 10 B
	//     Content struct           144 B   (14 pointer fields × 8B + []byte slice 24B + padding)
	//     Go map entry overhead     50 B   (hash bucket, key/value pointers, tophash)
	//     Safety margin / alignment 68 B
	//                             ------
	//     Total                    512 B per entry
	//
	// Example: 15 GB compressed file, ~2x Snappy ratio, 1M entries
	//   Decompressed data:  ~30 GB
	//   Go overhead:        1M × 512 B = 0.5 GB
	//   Estimated RAM:      ~30.5 GB
	const goOverheadPerEntry uint64 = 512

	// Scan block headers to get accurate entry/block counts and memory estimate.
	// This also handles stale file headers (writer not yet closed → 0/0 in header).
	// Only 16 bytes per block are read + a seek over the compressed data — no decompression.
	if info.Size() > header.DataStartOffset() {
		scanResult, err := reader.ScanBlockHeaders()
		if err == nil && scanResult != nil {
			if entryCount == 0 && blockCount == 0 {
				entryCount = scanResult.TotalEntryCount
				blockCount = scanResult.BlockCount
			}
			estimatedMemorySize = scanResult.TotalUncompressedSize + entryCount*goOverheadPerEntry
		}
	}

	return &SwampDetail{
		Sanctuary:           parts[0],
		Realm:               parts[1],
		Swamp:               parts[2],
		FilePath:            filePath,
		FileSize:            info.Size(),
		CreatedAt:           time.Unix(0, header.CreatedAt),
		ModifiedAt:          time.Unix(0, header.ModifiedAt),
		EntryCount:          entryCount,
		BlockCount:          blockCount,
		IslandID:            islandID,
		Version:             header.Version,
		EstimatedMemorySize: estimatedMemorySize,
	}, nil
}

// extractIslandID extracts the island ID from the file path.
// Given dataPath="/data" and filePath="/data/600/ab/cd/hash.hyd",
// returns "600".
func extractIslandID(dataPath, filePath string) string {
	rel, err := filepath.Rel(dataPath, filePath)
	if err != nil {
		return ""
	}
	parts := strings.SplitN(rel, string(filepath.Separator), 2)
	if len(parts) == 0 {
		return ""
	}
	return parts[0]
}
