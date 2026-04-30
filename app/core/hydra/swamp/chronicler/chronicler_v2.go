// Package chronicler provides the V2 append-only chronicler implementation.
// This adapter implements the same Chronicler interface as V1 but uses
// the new single-file, append-only storage format for significantly
// improved performance (32-112x faster) and reduced storage (50% smaller).
package chronicler

import (
	"bytes"
	"encoding/gob"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"sync"

	"github.com/hydraide/hydraide/app/core/hydra/swamp/beacon"
	v2 "github.com/hydraide/hydraide/app/core/hydra/swamp/chronicler/v2"
	"github.com/hydraide/hydraide/app/core/hydra/swamp/treasure"
	"github.com/hydraide/hydraide/app/core/hydra/swamp/treasure/guard"
)

const (
	// defaultMinEntriesForCompact is the smallest total-entry count for which
	// inline compaction is worth running. Below this, the wasted-bytes savings
	// are negligible and not worth the rewrite cost.
	defaultMinEntriesForCompact = 100

	// defaultMaxFileSizeForLoadCompact caps the file size eligible for inline
	// self-heal during Load(). Anything above this is left to Write() / Close()
	// triggers (or a manual sweep) so summon latency stays bounded.
	// 256 MiB is large enough to cover almost all real swamps while still
	// finishing in well under a second on commodity disks.
	defaultMaxFileSizeForLoadCompact = int64(256 * 1024 * 1024)
)

// chroniclerV2 implements the Chronicler interface using the V2 append-only format.
// It stores all data in a single .hyd file instead of multiple chunk files.
//
// Key design decisions:
// - Persistent writer: The file writer stays open while the swamp is active
// - Lazy initialization: Writer is created on first write, not on chronicler creation
// - Proper cleanup: Close() flushes all pending data and closes the file handle
// - This approach minimizes file open/close overhead for frequently written swamps
type chroniclerV2 struct {
	mu sync.RWMutex

	// Path configuration
	swampDataFolderPath string // Parent folder path
	hydFilePath         string // Full path to .hyd file

	// Configuration
	maxBlockSize        int     // Block size for compression (default: 64KB)
	compactionThreshold float64 // Fragmentation threshold for compaction (default: 0.5)
	maxDepth            int     // Max depth for folder cleanup
	filesystemInitiated bool
	dontSendFilePointer bool
	compactionOnSave    bool // Whether to check and run compaction on save

	// minEntriesForCompact is the minimum total entry count below which inline
	// compaction is skipped. Avoids needless rewrites of tiny files even if
	// their fragmentation ratio is technically above the threshold.
	minEntriesForCompact int

	// maxFileSizeForLoadCompact caps the file size eligible for inline
	// self-heal during Load(). Larger files are left to the Write()/Close()
	// triggers (or a manual sweep) to avoid producing latency spikes on
	// summon. 0 = no cap.
	maxFileSizeForLoadCompact int64

	// Swamp metadata (stored in .hyd file, replaces separate meta file)
	swampName string // Full swamp name for reverse lookup

	// Callbacks
	swampSaveFunction           func(t treasure.Treasure, guardID guard.ID) treasure.TreasureStatus
	filePointerCallbackFunction func(event []*FileNameEvent) error

	// Runtime state
	lastFragmentation float64 // Last calculated fragmentation ratio

	// totalEntriesInFile tracks the total number of entries currently
	// persisted in the .hyd file (live + dead, every INSERT/UPDATE/DELETE
	// ever written and not yet compacted away). Initialized from the file
	// header on Load(), incremented on each successful Write(), and reset
	// to the live-count after a successful compaction.
	// Always accessed under c.mu.
	totalEntriesInFile int64

	// liveCountFunc returns the current number of live keys in the swamp.
	// Wired by the swamp via RegisterLiveCountFunction (typically backed by
	// beacon.Count). Used for O(1) fragmentation estimation during Write()
	// to decide whether inline compaction should run.
	// Always accessed under c.mu.
	liveCountFunc func() int

	// Persistent writer - stays open while swamp is active
	// This avoids repeated file open/close for each Write() call
	writer       *v2.FileWriter
	writerClosed bool

	// destroyed is set to true by Destroy() to prevent Write() from
	// recreating the .hyd file after it has been deleted.
	// Protected by mu.
	destroyed bool
}

// NewV2 creates a new V2 chronicler that uses append-only storage.
// The swampDataFolderPath is used as the parent folder, and data is stored
// in a single .hyd file (swampDataFolderPath + ".hyd").
//
// Configuration:
//   - maxBlockSize: Size of compressed blocks (default: 64KB)
//   - compactionThreshold: Fragmentation ratio to trigger compaction (default: 30%)
func NewV2(swampDataFolderPath string, maxDepth int) Chronicler {
	// The .hyd file is placed at the same level as the folder would be
	// e.g., /data/words/ap/apple -> /data/words/ap/apple.hyd
	hydFilePath := swampDataFolderPath + ".hyd"

	return &chroniclerV2{
		swampDataFolderPath:       swampDataFolderPath,
		hydFilePath:               hydFilePath,
		maxBlockSize:              v2.DefaultMaxBlockSize,
		compactionThreshold:       0.3,
		maxDepth:                  maxDepth,
		compactionOnSave:          true,
		minEntriesForCompact:      defaultMinEntriesForCompact,
		maxFileSizeForLoadCompact: defaultMaxFileSizeForLoadCompact,
	}
}

// NewV2WithName creates a new V2 chronicler with the swamp name for metadata storage.
// The swamp name is stored in the .hyd file and can be used for reverse lookup
// when iterating over hashed folder names.
func NewV2WithName(swampDataFolderPath string, maxDepth int, swampName string) Chronicler {
	hydFilePath := swampDataFolderPath + ".hyd"

	return &chroniclerV2{
		swampDataFolderPath:       swampDataFolderPath,
		hydFilePath:               hydFilePath,
		maxBlockSize:              v2.DefaultMaxBlockSize,
		compactionThreshold:       0.3,
		maxDepth:                  maxDepth,
		compactionOnSave:          true,
		swampName:                 swampName,
		minEntriesForCompact:      defaultMinEntriesForCompact,
		maxFileSizeForLoadCompact: defaultMaxFileSizeForLoadCompact,
	}
}

// NewV2WithConfig creates a V2 chronicler with custom configuration.
func NewV2WithConfig(swampDataFolderPath string, maxDepth int, maxBlockSize int, compactionThreshold float64) Chronicler {
	hydFilePath := swampDataFolderPath + ".hyd"

	return &chroniclerV2{
		swampDataFolderPath:       swampDataFolderPath,
		hydFilePath:               hydFilePath,
		maxBlockSize:              maxBlockSize,
		compactionThreshold:       compactionThreshold,
		maxDepth:                  maxDepth,
		compactionOnSave:          true,
		minEntriesForCompact:      defaultMinEntriesForCompact,
		maxFileSizeForLoadCompact: defaultMaxFileSizeForLoadCompact,
	}
}

func (c *chroniclerV2) DontSendFilePointer() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.dontSendFilePointer = true
}

func (c *chroniclerV2) RegisterFilePointerFunction(filePointerFunction func(event []*FileNameEvent) error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.filePointerCallbackFunction = filePointerFunction
}

func (c *chroniclerV2) RegisterSaveFunction(swampSaveFunction func(t treasure.Treasure, guardID guard.ID) treasure.TreasureStatus) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.swampSaveFunction = swampSaveFunction
}

// RegisterLiveCountFunction wires a callback (typically beacon.Count) used by
// the Write()/Close()/Load() paths to estimate fragmentation in O(1) and
// decide whether inline compaction should run.
func (c *chroniclerV2) RegisterLiveCountFunction(liveCountFunction func() int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.liveCountFunc = liveCountFunction
}

func (c *chroniclerV2) GetSwampAbsPath() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.swampDataFolderPath
}

func (c *chroniclerV2) IsFilesystemInitiated() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.filesystemInitiated
}

// CreateDirectoryIfNotExists ensures the parent folder exists for the .hyd file.
// Note: In V2, we don't create the swamp folder itself, only the parent folders.
func (c *chroniclerV2) CreateDirectoryIfNotExists() {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Ensure parent directory exists
	parentDir := filepath.Dir(c.hydFilePath)
	if err := os.MkdirAll(parentDir, os.ModePerm); err != nil {
		slog.Error("cannot create parent directory for swamp",
			"path", parentDir,
			"error", err)
		return
	}

	c.filesystemInitiated = true
}

// Destroy removes the .hyd file and cleans up empty parent folders.
// After Destroy, Write() will be a no-op to prevent file recreation.
func (c *chroniclerV2) Destroy() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.destroyed = true

	// Remove the .hyd file
	if err := os.Remove(c.hydFilePath); err != nil && !os.IsNotExist(err) {
		slog.Error("cannot delete swamp file",
			"path", c.hydFilePath,
			"error", err)
		return
	}

	// Clean up empty parent folders
	c.cleanupEmptyFolders(filepath.Dir(c.hydFilePath), c.maxDepth)
}

// Load reads all treasures from the .hyd file and populates the beacon index.
// It automatically handles the replay of INSERT/UPDATE/DELETE entries to
// build the final state with only live entries.
//
// Crash recovery: any leftover ".hyd.compact" temp file from a previous
// interrupted compaction is removed before reading. The atomic-rename
// guarantee in Compactor.Compact ensures the .hyd file is always either
// the full pre- or full post-compaction state, never mid-flight.
//
// Self-heal: after a successful index load, if the file is large enough,
// fragmented above the threshold, and not larger than maxFileSizeForLoadCompact,
// an inline compaction runs immediately so the swamp starts its session
// with a clean file. The lock is held the entire time, so this can never
// race with concurrent Write()/Close() on the same chronicler.
func (c *chroniclerV2) Load(indexObj beacon.Beacon) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Crash recovery — drop any leftover temp from a previously crashed
	// compaction. The .hyd file itself is always intact thanks to atomic
	// rename, but the temp file would otherwise sit around forever.
	if err := v2.CleanupCompactionTemp(c.hydFilePath); err != nil {
		slog.Warn("could not remove leftover compaction temp",
			"path", c.hydFilePath,
			"error", err)
	}

	// Check if file exists
	if _, err := os.Stat(c.hydFilePath); os.IsNotExist(err) {
		// No data file yet - empty swamp
		return
	}

	// Open the reader
	reader, err := v2.NewFileReader(c.hydFilePath)
	if err != nil {
		slog.Error("cannot open swamp file for reading",
			"path", c.hydFilePath,
			"error", err)
		return
	}

	// Load index and get swamp metadata
	index, swampNameFromFile, err := reader.LoadIndex()
	if err != nil {
		reader.Close()
		slog.Error("cannot load index from swamp file",
			"path", c.hydFilePath,
			"error", err)
		return
	}

	// Capture header stats while the reader is still open. The header's
	// EntryCount reflects the total entries persisted across all writes
	// (live + dead) — exactly the value we need for inline-compaction
	// fragmentation estimates.
	header := reader.GetHeader()
	totalFromHeader := int64(0)
	if header != nil {
		totalFromHeader = int64(header.EntryCount)
	}
	reader.Close()

	// Update swamp name from file if not set
	if swampNameFromFile != "" && c.swampName == "" {
		c.swampName = swampNameFromFile
	}

	// Calculate fragmentation for later compaction decision
	liveEntries := len(index)

	// Initialize the persistent total-entries counter from the header.
	// If the header is stale (e.g., crash before final close updated it),
	// fall back to live count — Write() will append from there.
	c.totalEntriesInFile = totalFromHeader
	if c.totalEntriesInFile < int64(liveEntries) {
		c.totalEntriesInFile = int64(liveEntries)
	}

	// Self-heal: rewrite the file in place if it is heavily fragmented and
	// small enough that the rewrite cost is bounded. Skipping the size check
	// for very large files keeps summon latency predictable; those will be
	// compacted incrementally by the Write() trigger or a manual sweep.
	if c.compactionOnSave && totalFromHeader >= int64(c.minEntriesForCompact) && liveEntries < int(totalFromHeader) {
		dead := totalFromHeader - int64(liveEntries)
		frag := float64(dead) / float64(totalFromHeader)
		c.lastFragmentation = frag
		if frag > c.compactionThreshold {
			fileSize := int64(0)
			if fi, err := os.Stat(c.hydFilePath); err == nil {
				fileSize = fi.Size()
			}
			if c.maxFileSizeForLoadCompact <= 0 || fileSize <= c.maxFileSizeForLoadCompact {
				slog.Info("load self-heal compaction triggered",
					"path", c.hydFilePath,
					"fragmentation", frag,
					"live_entries", liveEntries,
					"total_entries", totalFromHeader,
					"file_size", fileSize)
				result, cerr := v2.CompactFromIndex(c.hydFilePath, c.maxBlockSize, c.swampName, index, int(totalFromHeader))
				if cerr != nil {
					slog.Error("load self-heal compaction failed",
						"path", c.hydFilePath,
						"error", cerr)
					// Original file is intact; counters keep header-derived value.
				} else if result != nil && result.Compacted {
					c.totalEntriesInFile = int64(result.LiveEntries)
					c.lastFragmentation = 0
					slog.Info("load self-heal compaction completed",
						"path", c.hydFilePath,
						"old_size", result.OldFileSize,
						"new_size", result.NewFileSize,
						"saved_bytes", result.OldFileSize-result.NewFileSize)
				}
			} else {
				slog.Debug("load self-heal skipped due to file size cap",
					"path", c.hydFilePath,
					"file_size", fileSize,
					"cap", c.maxFileSizeForLoadCompact)
			}
		}
	}

	// Convert entries to treasures
	treasures := make(map[string]treasure.Treasure)
	for key, entryData := range index {
		// Decode treasure from data
		treasureObj, err := c.decodeTreasure(entryData)
		if err != nil {
			slog.Error("cannot decode treasure",
				"key", key,
				"error", err)
			continue
		}

		treasures[key] = treasureObj
	}

	// Push all treasures to the beacon index
	indexObj.PushManyFromMap(treasures)

	slog.Debug("loaded swamp from V2 file",
		"path", c.hydFilePath,
		"live_entries", liveEntries,
		"fragmentation", c.lastFragmentation)
}

// Write saves treasures to the .hyd file using append-only operations.
// Deleted treasures are written as DELETE entries.
// Modified/new treasures are written as UPDATE/INSERT entries.
//
// Performance optimization: Uses a persistent writer that stays open
// while the swamp is active. The writer is lazily initialized on first
// write and closed only when Close() is called on the chronicler.
func (c *chroniclerV2) Write(treasures []treasure.Treasure) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.destroyed {
		return
	}

	if len(treasures) == 0 {
		return
	}

	// Ensure we have a writer (lazy initialization)
	if err := c.ensureWriter(); err != nil {
		slog.Error("cannot initialize swamp file writer",
			"path", c.hydFilePath,
			"error", err)
		return
	}

	// Track file pointer events for callback
	var filePointerEvents []*FileNameEvent
	// Count successfully written entries so the in-memory total counter stays
	// in lockstep with what is actually persisted to the .hyd file.
	writtenCount := 0

	// Write each treasure as an entry
	for _, t := range treasures {
		guardID := t.StartTreasureGuard(true, guard.BodyAuthID)

		key := t.GetKey()
		// Check if treasure is deleted (DeletedAt > 0 means it's marked for deletion)
		// This works for both shadow delete and real delete
		isDeleted := t.GetDeletedAt() > 0

		var entry v2.Entry

		if isDeleted {
			// Write DELETE entry
			entry = v2.Entry{
				Operation: v2.OpDelete,
				Key:       key,
				Data:      nil,
			}
		} else {
			// Encode treasure to bytes
			data, err := c.encodeTreasure(t, guardID)
			if err != nil {
				slog.Error("cannot encode treasure",
					"key", key,
					"error", err)
				t.ReleaseTreasureGuard(guardID)
				continue
			}

			// Determine operation type
			op := v2.OpUpdate
			if t.GetFileName() == nil {
				op = v2.OpInsert
			}

			entry = v2.Entry{
				Operation: op,
				Key:       key,
				Data:      data,
			}
		}

		// Write the entry to persistent writer
		if err := c.writer.WriteEntry(entry); err != nil {
			slog.Error("cannot write entry to swamp file",
				"key", key,
				"error", err)
			t.ReleaseTreasureGuard(guardID)
			continue
		}
		writtenCount++

		// Track for file pointer callback
		if !c.dontSendFilePointer {
			filePointerEvents = append(filePointerEvents, &FileNameEvent{
				TreasureKey: key,
				FileName:    c.hydFilePath,
			})
		}

		t.ReleaseTreasureGuard(guardID)
	}

	// Send file pointer callback if registered
	if len(filePointerEvents) > 0 && c.filePointerCallbackFunction != nil && !c.dontSendFilePointer {
		if err := c.filePointerCallbackFunction(filePointerEvents); err != nil {
			slog.Error("file pointer callback failed",
				"error", err)
		}
	}

	// Update the persistent total-entries counter with however many entries
	// actually made it into the writer's buffer (errored entries are skipped
	// above with `continue`).
	c.totalEntriesInFile += int64(writtenCount)

	// Note: We don't close the writer here.
	// The writer stays open and is closed when Close() is called.
	//
	// Inline compaction trigger: now that the counters are up to date, do an
	// O(1) fragmentation check against the live-count callback. If the file
	// is heavily fragmented we run compaction synchronously, still under
	// c.mu.Lock(), so no concurrent Write() can race us.
	c.maybeCompactInline()
}

// ensureWriter creates the persistent writer if it doesn't exist.
// For new files, uses V3 format (swamp name stored in header area).
// For existing files, preserves the existing format.
// Must be called with lock held.
func (c *chroniclerV2) ensureWriter() error {
	if c.writer != nil && !c.writerClosed {
		return nil
	}

	// Check if this is a new file (doesn't exist yet)
	isNewFile := false
	if _, err := os.Stat(c.hydFilePath); os.IsNotExist(err) {
		isNewFile = true
	}

	var writer *v2.FileWriter
	var err error

	if isNewFile && c.swampName != "" {
		// New file: use V3 format with name in header area
		writer, err = v2.NewFileWriterWithName(c.hydFilePath, c.maxBlockSize, c.swampName)
	} else {
		// Existing file: preserve format (V2 or V3)
		writer, err = v2.NewFileWriter(c.hydFilePath, c.maxBlockSize)
	}
	if err != nil {
		return err
	}

	c.writer = writer
	c.writerClosed = false

	return nil
}

// maybeCompactInline checks the in-memory counters and triggers an inline
// compaction if fragmentation exceeds the threshold.
//
// Must be called with c.mu held. The compaction itself runs synchronously
// under the same lock — this is what guarantees no concurrent Write() can
// race with the temp-file build and atomic rename.
//
// Hysteresis: compaction is only considered when total >= 2 × live. This
// prevents an oscillating workload (rewrite the same N keys repeatedly)
// from compacting on every batch — it amortizes the rewrite cost so that
// each compaction does at least live× useful work, bounding the total
// rewrite cost to O(N log N) for N writes.
func (c *chroniclerV2) maybeCompactInline() {
	if !c.compactionOnSave {
		return
	}
	if c.liveCountFunc == nil {
		return
	}
	total := c.totalEntriesInFile
	if total < int64(c.minEntriesForCompact) {
		return
	}

	live := int64(c.liveCountFunc())
	if live < 0 {
		live = 0
	}
	if live > total {
		// Should not happen, but guard against counter drift.
		return
	}
	// Hysteresis guard — refuse to compact unless the file has roughly
	// doubled in size since the last compaction (or initial load).
	if total < 2*live {
		dead := total - live
		if dead <= 0 {
			c.lastFragmentation = 0
		} else {
			c.lastFragmentation = float64(dead) / float64(total)
		}
		return
	}
	dead := total - live
	if dead <= 0 {
		c.lastFragmentation = 0
		return
	}

	frag := float64(dead) / float64(total)
	c.lastFragmentation = frag
	if frag <= c.compactionThreshold {
		return
	}

	if err := c.runCompactionLocked(); err != nil {
		slog.Error("inline compaction failed",
			"path", c.hydFilePath,
			"error", err)
		// Counters left as-is — next trigger will retry.
	}
}

// runCompactionLocked closes the open writer, runs a full file compaction,
// and updates counters. Must be called with c.mu held.
//
// Safety invariants:
//   - All writes go through c.mu, so no concurrent appender can race the
//     compaction's read/temp-write/rename sequence.
//   - The Compactor uses atomic os.Rename, so the .hyd file is always either
//     the full pre-compaction or the full post-compaction state — never mid.
//   - On any error path inside Compactor.Compact, the original file is left
//     intact and the temp file is removed.
func (c *chroniclerV2) runCompactionLocked() error {
	// Close the writer so its file handle is released and all buffered data
	// is flushed before the compactor reads the file.
	if c.writer != nil && !c.writerClosed {
		if err := c.writer.Close(); err != nil {
			return err
		}
		c.writerClosed = true
		c.writer = nil
	}

	// Defensively wipe any leftover temp from a previously crashed run before
	// the compactor creates a fresh one (it would also overwrite, but explicit
	// cleanup keeps logs honest).
	_ = v2.CleanupCompactionTemp(c.hydFilePath)

	compactor := v2.NewCompactor(c.hydFilePath, c.maxBlockSize, 0)
	result, err := compactor.Compact()
	if err != nil {
		return err
	}
	if result == nil || !result.Compacted {
		return nil
	}

	// After compaction the file contains exactly the live entries.
	c.totalEntriesInFile = int64(result.LiveEntries)
	c.lastFragmentation = 0

	slog.Info("compaction completed",
		"path", c.hydFilePath,
		"old_size", result.OldFileSize,
		"new_size", result.NewFileSize,
		"live_entries", result.LiveEntries,
		"removed_entries", result.RemovedEntries,
		"saved_bytes", result.OldFileSize-result.NewFileSize)
	return nil
}

// encodeTreasure serializes a treasure to bytes using GOB encoding.
func (c *chroniclerV2) encodeTreasure(t treasure.Treasure, guardID guard.ID) ([]byte, error) {
	// Get the raw bytes from treasure
	data, err := t.ConvertToByte(guardID)
	if err != nil {
		return nil, err
	}
	return data, nil
}

// decodeTreasure deserializes bytes back to a treasure object.
func (c *chroniclerV2) decodeTreasure(data []byte) (treasure.Treasure, error) {
	if len(data) == 0 {
		return nil, errors.New("empty treasure data")
	}

	t := treasure.New(c.swampSaveFunction)
	guardID := t.StartTreasureGuard(true, guard.BodyAuthID)
	defer t.ReleaseTreasureGuard(guardID)

	// Load from bytes - note: fileName is the .hyd file for V2
	if err := t.LoadFromByte(guardID, data, c.hydFilePath); err != nil {
		return nil, err
	}

	return t, nil
}

// cleanupEmptyFolders removes empty parent folders up to maxDepth levels.
func (c *chroniclerV2) cleanupEmptyFolders(folderPath string, maxDepth int) {
	for i := 0; i < maxDepth; i++ {
		// Try to remove the folder
		err := os.Remove(folderPath)
		if err != nil {
			// Folder is not empty or doesn't exist
			break
		}

		// Move to parent
		folderPath = filepath.Dir(folderPath)
	}
}

// GetFragmentation returns the last calculated fragmentation ratio.
func (c *chroniclerV2) GetFragmentation() float64 {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.lastFragmentation
}

// ForceCompaction runs compaction regardless of fragmentation threshold.
// Shares the locked critical-section pattern with the inline trigger so it
// is safe to call concurrently with normal swamp activity.
func (c *chroniclerV2) ForceCompaction() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.runCompactionLocked()
}

// Close flushes all pending writes and closes the file handle.
// This method MUST be called when the swamp is closing to ensure
// all data is written to disk. After Close(), the chronicler
// can be reopened by calling Write() again (lazy reinitialization).
//
// The Close() method also checks if compaction is needed and runs it
// if the fragmentation threshold is exceeded — this runs even when no
// writer was opened in this session, so a swamp that was only loaded
// (read-only) can still self-heal a previously-fragmented file.
func (c *chroniclerV2) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Close the writer if currently open. Skip cleanly if already closed
	// or never opened — we still want the compaction check below to run.
	if c.writer != nil && !c.writerClosed {
		if err := c.writer.Close(); err != nil {
			slog.Error("failed to close V2 chronicler writer",
				"path", c.hydFilePath,
				"error", err)
			return err
		}
		c.writerClosed = true
		c.writer = nil
		slog.Debug("V2 chronicler closed",
			"path", c.hydFilePath)
	}

	// Compaction check runs unconditionally on Close — this is the safety net
	// for swamps that idle out without enough writes to hit the inline trigger,
	// or that were only ever loaded in this session.
	c.maybeCompactInline()
	return nil
}

// Sync forces a sync of pending data to disk without closing the writer.
// Use this for periodic syncs when you want to ensure data durability
// but keep the writer open for more writes.
func (c *chroniclerV2) Sync() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.writer == nil || c.writerClosed {
		return nil
	}

	return c.writer.Sync()
}

// gobEncode is a helper for GOB encoding
func gobEncode(v interface{}) ([]byte, error) {
	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// gobDecode is a helper for GOB decoding
func gobDecode(data []byte, v interface{}) error {
	buf := bytes.NewReader(data)
	dec := gob.NewDecoder(buf)
	return dec.Decode(v)
}
