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
	workers := runtime.NumCPU()
	if workers > 16 {
		workers = 16
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
		// V2 fallback: need to read blocks for OpMetadata
		_, name, err := reader.LoadIndex()
		if err != nil {
			return nil, err
		}
		swampName = name
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

	return &SwampDetail{
		Sanctuary:  parts[0],
		Realm:      parts[1],
		Swamp:      parts[2],
		FilePath:   filePath,
		FileSize:   info.Size(),
		CreatedAt:  time.Unix(0, header.CreatedAt),
		ModifiedAt: time.Unix(0, header.ModifiedAt),
		EntryCount: header.EntryCount,
		BlockCount: header.BlockCount,
		IslandID:   islandID,
		Version:    header.Version,
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
