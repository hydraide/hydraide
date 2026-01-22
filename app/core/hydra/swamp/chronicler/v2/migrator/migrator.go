// Package migrator provides tools for migrating HydrAIDE data from V1 (multi-file chunks)
// to V2 (single-file append-only) format. This is a standalone migration tool designed
// to be run during a maintenance window.
//
// Usage:
//  1. Stop the HydrAIDE service
//  2. Create a backup (ZFS snapshot recommended)
//  3. Run: hydraidectl migrate --data-path=/var/hydraide/data --dry-run
//  4. If dry-run succeeds: hydraidectl migrate --data-path=/var/hydraide/data --verify --delete-old
//  5. Start the HydrAIDE service with V2 code
package migrator

import (
	"encoding/gob"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/hydraide/hydraide/app/core/compressor"
	"github.com/hydraide/hydraide/app/core/hydra/swamp/chronicler/v2"
	"github.com/hydraide/hydraide/app/core/hydra/swamp/metadata"
)

// Config holds the migration configuration
type Config struct {
	DataPath       string        // Root data path (e.g., /var/hydraide/data)
	DryRun         bool          // Only validate, don't write anything
	Verify         bool          // Verify after migration
	DeleteOld      bool          // Delete old V1 files after successful migration
	Parallel       int           // Number of parallel workers
	StopOnError    bool          // Stop at first error
	ProgressReport time.Duration // Progress report interval
}

// Result contains the migration results
type Result struct {
	StartTime        time.Time
	EndTime          time.Time
	Duration         time.Duration
	TotalSwamps      int64
	ProcessedSwamps  int64
	SuccessfulSwamps int64
	FailedSwamps     []FailedSwamp
	TotalEntries     int64
	OldSizeBytes     int64
	NewSizeBytes     int64
	DryRun           bool
	Errors           []string
}

// FailedSwamp contains information about a failed migration
type FailedSwamp struct {
	Path  string
	Error string
	Phase string // "load", "convert", "write", "verify"
}

// Migrator handles the migration process
type Migrator struct {
	config     Config
	result     Result
	mu         sync.Mutex
	compressor compressor.Compressor
	progressCh chan ProgressEvent
	stopCh     chan struct{}
}

// ProgressEvent is sent during migration to report progress
type ProgressEvent struct {
	ProcessedSwamps int64
	TotalSwamps     int64
	CurrentPath     string
	BytesProcessed  int64
}

// New creates a new Migrator with the given configuration
func New(config Config) (*Migrator, error) {
	if config.DataPath == "" {
		return nil, errors.New("data path is required")
	}

	if config.Parallel <= 0 {
		config.Parallel = 4 // Default parallelism
	}

	if config.ProgressReport <= 0 {
		config.ProgressReport = 5 * time.Second
	}

	return &Migrator{
		config:     config,
		compressor: compressor.New(compressor.Snappy),
		progressCh: make(chan ProgressEvent, 100),
		stopCh:     make(chan struct{}),
		result: Result{
			DryRun: config.DryRun,
		},
	}, nil
}

// Run executes the migration process
func (m *Migrator) Run() (*Result, error) {
	m.result.StartTime = time.Now()
	defer func() {
		m.result.EndTime = time.Now()
		m.result.Duration = m.result.EndTime.Sub(m.result.StartTime)
	}()

	// Find all V1 swamp folders
	swampFolders, err := m.findV1Swamps()
	if err != nil {
		return &m.result, fmt.Errorf("failed to find swamps: %w", err)
	}

	m.result.TotalSwamps = int64(len(swampFolders))

	if m.result.TotalSwamps == 0 {
		slog.Info("No V1 swamps found to migrate")
		return &m.result, nil
	}

	slog.Info("Found swamps to migrate",
		"count", m.result.TotalSwamps,
		"dry_run", m.config.DryRun)

	// Start progress reporter
	go m.progressReporter()

	// Process swamps with worker pool
	m.processSwamps(swampFolders)

	// Stop progress reporter
	close(m.stopCh)

	return &m.result, nil
}

// findV1Swamps walks the data path and finds all V1 swamp folders
func (m *Migrator) findV1Swamps() ([]string, error) {
	var swampFolders []string

	err := filepath.Walk(m.config.DataPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip if not a directory
		if !info.IsDir() {
			return nil
		}

		// Check if this is a V1 swamp folder (contains .dat files or meta.json)
		if m.isV1SwampFolder(path) {
			swampFolders = append(swampFolders, path)
			return filepath.SkipDir // Don't recurse into swamp folders
		}

		return nil
	})

	return swampFolders, err
}

// isV1SwampFolder checks if a folder is a V1 swamp (contains meta file and UUID data files)
func (m *Migrator) isV1SwampFolder(folderPath string) bool {
	entries, err := os.ReadDir(folderPath)
	if err != nil {
		return false
	}

	hasMeta := false
	hasDataFile := false

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()

		// V1 swamps have a 'meta' file
		if name == metadata.MetaFile {
			hasMeta = true
			continue
		}

		// V1 data files are UUID-named without extension
		if filepath.Ext(name) == "" && isV1DataFileName(name) {
			hasDataFile = true
		}
	}

	// A valid V1 swamp has either a meta file or data files (or both)
	return hasMeta || hasDataFile
}

// processSwamps migrates all swamps using a worker pool
func (m *Migrator) processSwamps(swampFolders []string) {
	var wg sync.WaitGroup
	workCh := make(chan string, len(swampFolders))

	// Start workers
	for i := 0; i < m.config.Parallel; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for folderPath := range workCh {
				m.migrateSwamp(folderPath)
			}
		}()
	}

	// Send work to workers
	for _, folder := range swampFolders {
		workCh <- folder
	}
	close(workCh)

	// Wait for all workers to finish
	wg.Wait()
}

// migrateSwamp migrates a single V1 swamp to V2 format
func (m *Migrator) migrateSwamp(folderPath string) {
	atomic.AddInt64(&m.result.ProcessedSwamps, 1)

	// Send progress event
	select {
	case m.progressCh <- ProgressEvent{
		ProcessedSwamps: atomic.LoadInt64(&m.result.ProcessedSwamps),
		TotalSwamps:     m.result.TotalSwamps,
		CurrentPath:     folderPath,
	}:
	default:
	}

	// Step 0: Load swamp name from meta file
	swampName, err := m.loadSwampNameFromMeta(folderPath)
	if err != nil {
		slog.Warn("Could not load swamp name from meta file",
			"path", folderPath,
			"error", err)
		// Continue anyway - swamp name is optional for basic functionality
	}

	// Step 1: Load V1 data
	entries, oldSize, err := m.loadV1Swamp(folderPath)
	if err != nil {
		m.recordFailure(folderPath, err.Error(), "load")
		return
	}

	atomic.AddInt64(&m.result.OldSizeBytes, oldSize)
	atomic.AddInt64(&m.result.TotalEntries, int64(len(entries)))

	// If dry-run, we're done after successful load
	if m.config.DryRun {
		atomic.AddInt64(&m.result.SuccessfulSwamps, 1)
		return
	}

	// Step 2: Write V2 file (including swamp name as metadata entry)
	hydFilePath := folderPath + ".hyd"
	newSize, err := m.writeV2File(hydFilePath, entries, swampName)
	if err != nil {
		m.recordFailure(folderPath, err.Error(), "write")
		return
	}

	atomic.AddInt64(&m.result.NewSizeBytes, newSize)

	// Step 3: Verify (if enabled)
	if m.config.Verify {
		if err := m.verifyMigration(hydFilePath, entries); err != nil {
			// Remove the new file on verify failure
			os.Remove(hydFilePath)
			m.recordFailure(folderPath, err.Error(), "verify")
			return
		}
	}

	// Step 4: Delete old files (if enabled)
	if m.config.DeleteOld {
		if err := m.deleteV1Files(folderPath); err != nil {
			slog.Warn("Failed to delete old V1 files",
				"path", folderPath,
				"error", err)
			// Don't fail the migration, just log warning
		}
	}

	atomic.AddInt64(&m.result.SuccessfulSwamps, 1)
}

// loadV1Swamp loads all treasures from a V1 swamp folder
func (m *Migrator) loadV1Swamp(folderPath string) ([]v2.Entry, int64, error) {
	var entries []v2.Entry
	var totalSize int64

	files, err := os.ReadDir(folderPath)
	if err != nil {
		return nil, 0, err
	}

	for _, file := range files {
		if file.IsDir() {
			continue
		}

		name := file.Name()

		// Skip meta file
		if name == metadata.MetaFile {
			continue
		}

		// V1 data files are UUID-named without extension (e.g., "550e8400-e29b-41d4-a716...")
		// They have no extension and contain hex characters and dashes
		// Skip files with extensions (like .hyd)
		if filepath.Ext(name) != "" {
			continue
		}

		// Verify it looks like a UUID or hex string (V1 data file)
		if !isV1DataFileName(name) {
			continue
		}

		filePath := filepath.Join(folderPath, name)
		fileInfo, err := os.Stat(filePath)
		if err != nil {
			return nil, 0, fmt.Errorf("stat %s: %w", filePath, err)
		}
		totalSize += fileInfo.Size()

		// Read and decompress the file
		fileEntries, err := m.loadV1File(filePath)
		if err != nil {
			return nil, 0, fmt.Errorf("load %s: %w", filePath, err)
		}

		entries = append(entries, fileEntries...)
	}

	return entries, totalSize, nil
}

// loadV1File reads a single V1 .dat file and returns entries
func (m *Migrator) loadV1File(filePath string) ([]v2.Entry, error) {
	// Read file content
	compressedData, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	// Decompress
	decompressed, err := m.compressor.Decompress(compressedData)
	if err != nil {
		return nil, fmt.Errorf("decompress: %w", err)
	}

	// Parse binary segments (V1 format: length-prefixed segments)
	segments, err := m.parseV1Segments(decompressed)
	if err != nil {
		return nil, fmt.Errorf("parse segments: %w", err)
	}

	// Convert segments to entries
	var entries []v2.Entry
	for _, segment := range segments {
		// Each segment is a GOB-encoded treasure
		// We need to extract the key from the treasure data
		key, err := m.extractKeyFromTreasure(segment)
		if err != nil {
			return nil, fmt.Errorf("extract key: %w", err)
		}

		entries = append(entries, v2.Entry{
			Operation: v2.OpInsert,
			Key:       key,
			Data:      segment,
		})
	}

	return entries, nil
}

// parseV1Segments parses length-prefixed binary segments from V1 format
func (m *Migrator) parseV1Segments(data []byte) ([][]byte, error) {
	var segments [][]byte
	reader := NewByteReader(data)

	for reader.Remaining() > 0 {
		// Read segment length (4 bytes, big-endian)
		length, err := reader.ReadUint32()
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, err
		}

		if length == 0 {
			continue
		}

		// Read segment data
		segment, err := reader.ReadBytes(int(length))
		if err != nil {
			return nil, fmt.Errorf("read segment: %w", err)
		}

		segments = append(segments, segment)
	}

	return segments, nil
}

// extractKeyFromTreasure extracts the key from a GOB-encoded treasure
// This is a simplified extraction - we decode just enough to get the key
func (m *Migrator) extractKeyFromTreasure(data []byte) (string, error) {
	// The treasure is GOB-encoded. We need to decode it to get the key.
	// Since GOB is complex, we'll use a struct that matches the treasure.Model structure
	type TreasureModel struct {
		Key            string
		ExpirationTime int64
		CreatedAt      int64
		CreatedBy      string
		DeletedAt      int64
		DeletedBy      string
		ModifiedAt     int64
		ModifiedBy     string
		Content        interface{}
		FileName       *string
	}

	var model TreasureModel
	decoder := NewGobDecoder(data)
	if err := decoder.Decode(&model); err != nil {
		return "", fmt.Errorf("gob decode: %w", err)
	}

	if model.Key == "" {
		return "", errors.New("empty key in treasure")
	}

	return model.Key, nil
}

// loadSwampNameFromMeta loads the swamp name from the V1 meta file.
// The meta file is GOB-encoded and contains SwampName field.
func (m *Migrator) loadSwampNameFromMeta(folderPath string) (string, error) {
	metaFilePath := filepath.Join(folderPath, metadata.MetaFile)

	file, err := os.Open(metaFilePath)
	if err != nil {
		return "", fmt.Errorf("open meta file: %w", err)
	}
	defer file.Close()

	// Decode the GOB-encoded meta file
	// We only need the SwampName field
	type MetaModel struct {
		SwampName string
	}

	var meta MetaModel
	gobDecoder := gob.NewDecoder(file)
	if err := gobDecoder.Decode(&meta); err != nil {
		return "", fmt.Errorf("decode meta file: %w", err)
	}

	return meta.SwampName, nil
}

// MetadataEntryKey is the special key used for storing swamp metadata in V2 files.
const MetadataEntryKey = "__swamp_meta__"

// writeV2File writes entries to a new V2 .hyd file
func (m *Migrator) writeV2File(filePath string, entries []v2.Entry, swampName string) (int64, error) {
	writer, err := v2.NewFileWriter(filePath, v2.DefaultMaxBlockSize)
	if err != nil {
		return 0, err
	}

	// First, write swamp metadata entry if we have a swamp name
	if swampName != "" {
		metaEntry := v2.Entry{
			Operation: v2.OpMetadata,
			Key:       MetadataEntryKey,
			Data:      []byte(swampName), // Simple encoding - just the swamp name string
		}
		if err := writer.WriteEntry(metaEntry); err != nil {
			writer.Close()
			os.Remove(filePath)
			return 0, fmt.Errorf("write metadata entry: %w", err)
		}
	}

	// Then write all data entries
	for _, entry := range entries {
		if err := writer.WriteEntry(entry); err != nil {
			writer.Close()
			os.Remove(filePath)
			return 0, err
		}
	}

	if err := writer.Close(); err != nil {
		os.Remove(filePath)
		return 0, err
	}

	info, err := os.Stat(filePath)
	if err != nil {
		return 0, err
	}

	return info.Size(), nil
}

// verifyMigration verifies that the V2 file contains all expected entries
func (m *Migrator) verifyMigration(hydFilePath string, originalEntries []v2.Entry) error {
	reader, err := v2.NewFileReader(hydFilePath)
	if err != nil {
		return err
	}
	defer reader.Close()

	index, _, err := reader.LoadIndex()
	if err != nil {
		return err
	}

	// Build expected keys map
	expectedKeys := make(map[string]bool)
	for _, entry := range originalEntries {
		expectedKeys[entry.Key] = true
	}

	// Verify all expected keys exist
	for key := range expectedKeys {
		if _, exists := index[key]; !exists {
			return fmt.Errorf("missing key after migration: %s", key)
		}
	}

	slog.Debug("Verification passed",
		"path", hydFilePath,
		"entries", len(originalEntries),
		"unique_keys", len(index))

	return nil
}

// deleteV1Files removes all V1 files from the folder
func (m *Migrator) deleteV1Files(folderPath string) error {
	// First, remove all files in the folder
	entries, err := os.ReadDir(folderPath)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		filePath := filepath.Join(folderPath, entry.Name())
		if err := os.Remove(filePath); err != nil {
			return err
		}
	}

	// Then try to remove the folder itself
	return os.Remove(folderPath)
}

// recordFailure records a failed migration
func (m *Migrator) recordFailure(path, errorMsg, phase string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.result.FailedSwamps = append(m.result.FailedSwamps, FailedSwamp{
		Path:  path,
		Error: errorMsg,
		Phase: phase,
	})

	slog.Error("Migration failed",
		"path", path,
		"phase", phase,
		"error", errorMsg)
}

// progressReporter periodically logs migration progress
func (m *Migrator) progressReporter() {
	ticker := time.NewTicker(m.config.ProgressReport)
	defer ticker.Stop()

	for {
		select {
		case <-m.stopCh:
			return
		case <-ticker.C:
			processed := atomic.LoadInt64(&m.result.ProcessedSwamps)
			total := m.result.TotalSwamps
			successful := atomic.LoadInt64(&m.result.SuccessfulSwamps)

			if total > 0 {
				percent := float64(processed) / float64(total) * 100
				slog.Info("Migration progress",
					"processed", processed,
					"total", total,
					"percent", fmt.Sprintf("%.1f%%", percent),
					"successful", successful,
					"failed", len(m.result.FailedSwamps))
			}
		}
	}
}

// GetProgressChannel returns the progress event channel
func (m *Migrator) GetProgressChannel() <-chan ProgressEvent {
	return m.progressCh
}

// Summary returns a human-readable summary of the migration
func (r *Result) Summary() string {
	var sb strings.Builder

	sb.WriteString("\n")
	sb.WriteString("================================================================================\n")
	if r.DryRun {
		sb.WriteString("HydrAIDE Migration DRY-RUN Report\n")
	} else {
		sb.WriteString("HydrAIDE Migration Report\n")
	}
	sb.WriteString(fmt.Sprintf("Date: %s\n", r.StartTime.Format("2006-01-02 15:04:05")))
	sb.WriteString("================================================================================\n\n")

	sb.WriteString("SUMMARY:\n")
	sb.WriteString(fmt.Sprintf("  Total swamps found:     %d\n", r.TotalSwamps))
	sb.WriteString(fmt.Sprintf("  Successfully processed: %d\n", r.SuccessfulSwamps))
	sb.WriteString(fmt.Sprintf("  Failed:                 %d\n", len(r.FailedSwamps)))
	sb.WriteString(fmt.Sprintf("  Duration:               %s\n", r.Duration.Round(time.Second)))
	sb.WriteString("\n")

	if !r.DryRun {
		sb.WriteString("SIZE:\n")
		sb.WriteString(fmt.Sprintf("  Old size (V1):          %s\n", formatBytes(r.OldSizeBytes)))
		sb.WriteString(fmt.Sprintf("  New size (V2):          %s\n", formatBytes(r.NewSizeBytes)))
		if r.OldSizeBytes > 0 {
			savings := float64(r.OldSizeBytes-r.NewSizeBytes) / float64(r.OldSizeBytes) * 100
			sb.WriteString(fmt.Sprintf("  Savings:                %.1f%%\n", savings))
		}
		sb.WriteString("\n")
	}

	sb.WriteString("ENTRIES:\n")
	sb.WriteString(fmt.Sprintf("  Total entries:          %d\n", r.TotalEntries))
	if r.TotalSwamps > 0 {
		sb.WriteString(fmt.Sprintf("  Average per swamp:      %d\n", r.TotalEntries/r.TotalSwamps))
	}
	sb.WriteString("\n")

	if len(r.FailedSwamps) > 0 {
		sb.WriteString("FAILED SWAMPS:\n")
		for i, failed := range r.FailedSwamps {
			sb.WriteString(fmt.Sprintf("  %d. %s\n", i+1, failed.Path))
			sb.WriteString(fmt.Sprintf("     Phase: %s\n", failed.Phase))
			sb.WriteString(fmt.Sprintf("     Error: %s\n", failed.Error))
			sb.WriteString("\n")
		}
	}

	sb.WriteString("================================================================================\n")

	if len(r.FailedSwamps) > 0 {
		sb.WriteString("RECOMMENDATION:\n")
		sb.WriteString(fmt.Sprintf("  ❌ %d swamps need manual inspection before live migration.\n", len(r.FailedSwamps)))
		sb.WriteString("  Fix the issues and re-run.\n")
	} else if r.DryRun {
		sb.WriteString("RECOMMENDATION:\n")
		sb.WriteString("  ✅ All swamps validated successfully.\n")
		sb.WriteString("  You can proceed with live migration.\n")
	} else {
		sb.WriteString("RESULT:\n")
		sb.WriteString("  ✅ Migration completed successfully.\n")
	}

	sb.WriteString("================================================================================\n")

	return sb.String()
}

// ToJSON returns the result as JSON
func (r *Result) ToJSON() ([]byte, error) {
	return json.MarshalIndent(r, "", "  ")
}

// formatBytes formats bytes to human-readable string
func formatBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

// isV1DataFileName checks if a filename looks like a V1 data file.
// V1 data files are UUID-like names containing only hex characters and dashes,
// with no file extension.
func isV1DataFileName(name string) bool {
	if len(name) == 0 {
		return false
	}
	// V1 data files are typically UUIDs: 550e8400-e29b-41d4-a716-446655440000
	// They contain only hex characters (0-9, a-f, A-F) and dashes
	for _, c := range name {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F') || c == '-') {
			return false
		}
	}
	return true
}
