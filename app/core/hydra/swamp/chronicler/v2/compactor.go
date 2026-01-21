package v2

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// Compactor handles file compaction to remove fragmentation.
// It rewrites the file with only live entries.
type Compactor struct {
	mu           sync.Mutex
	filePath     string
	maxBlockSize int
	isRunning    bool
	threshold    float64 // Fragmentation threshold (0.0-1.0)
}

// CompactionResult contains statistics about the compaction operation
type CompactionResult struct {
	OldFileSize    int64
	NewFileSize    int64
	TotalEntries   int
	LiveEntries    int
	RemovedEntries int
	Fragmentation  float64
	Compacted      bool
	Error          error
}

// NewCompactor creates a new compactor for the given file
func NewCompactor(filePath string, maxBlockSize int, threshold float64) *Compactor {
	if threshold <= 0 || threshold > 1 {
		threshold = 0.5 // Default 50% fragmentation threshold
	}
	if maxBlockSize <= 0 {
		maxBlockSize = DefaultMaxBlockSize
	}
	return &Compactor{
		filePath:     filePath,
		maxBlockSize: maxBlockSize,
		threshold:    threshold,
	}
}

// ShouldCompact checks if the file needs compaction based on fragmentation
func (c *Compactor) ShouldCompact() (bool, float64, error) {
	reader, err := NewFileReader(c.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return false, 0, nil
		}
		return false, 0, err
	}
	defer reader.Close()

	fragmentation, _, _, err := reader.CalculateFragmentation()
	if err != nil {
		return false, 0, err
	}

	return fragmentation >= c.threshold, fragmentation, nil
}

// Compact performs the compaction operation.
// It reads all live entries and writes them to a new file, then atomically replaces the old file.
func (c *Compactor) Compact() (*CompactionResult, error) {
	c.mu.Lock()
	if c.isRunning {
		c.mu.Unlock()
		return nil, ErrCompactionRunning
	}
	c.isRunning = true
	c.mu.Unlock()

	defer func() {
		c.mu.Lock()
		c.isRunning = false
		c.mu.Unlock()
	}()

	result := &CompactionResult{}

	// Get old file size
	oldInfo, err := os.Stat(c.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			result.Compacted = false
			return result, nil
		}
		result.Error = err
		return result, err
	}
	result.OldFileSize = oldInfo.Size()

	// Read and calculate fragmentation
	reader, err := NewFileReader(c.filePath)
	if err != nil {
		result.Error = err
		return result, err
	}

	fragmentation, liveCount, totalCount, err := reader.CalculateFragmentation()
	if err != nil {
		reader.Close()
		result.Error = err
		return result, err
	}

	result.Fragmentation = fragmentation
	result.TotalEntries = totalCount
	result.LiveEntries = liveCount
	result.RemovedEntries = totalCount - liveCount

	// Check if compaction is needed
	if fragmentation < c.threshold {
		reader.Close()
		result.Compacted = false
		return result, nil
	}

	// Load live entries index
	index, _, err := reader.LoadIndex()
	if err != nil {
		reader.Close()
		result.Error = err
		return result, err
	}
	reader.Close()

	// Create temp file for new data
	tempPath := c.filePath + ".compact"
	writer, err := NewFileWriter(tempPath, c.maxBlockSize)
	if err != nil {
		result.Error = err
		return result, err
	}

	// Write all live entries to new file
	for key, data := range index {
		entry := Entry{
			Operation: OpInsert,
			Key:       key,
			Data:      data,
		}
		if err := writer.WriteEntry(entry); err != nil {
			writer.Close()
			os.Remove(tempPath)
			result.Error = err
			return result, err
		}
	}

	// Close and sync the new file
	if err := writer.Close(); err != nil {
		os.Remove(tempPath)
		result.Error = err
		return result, err
	}

	// Atomic rename: replace old file with new
	if err := os.Rename(tempPath, c.filePath); err != nil {
		os.Remove(tempPath)
		result.Error = err
		return result, err
	}

	// Get new file size
	newInfo, err := os.Stat(c.filePath)
	if err != nil {
		result.Error = err
		return result, err
	}
	result.NewFileSize = newInfo.Size()
	result.Compacted = true

	return result, nil
}

// CompactIfNeeded checks fragmentation and compacts only if threshold is exceeded
func (c *Compactor) CompactIfNeeded() (*CompactionResult, error) {
	shouldCompact, fragmentation, err := c.ShouldCompact()
	if err != nil {
		return nil, err
	}

	if !shouldCompact {
		return &CompactionResult{
			Fragmentation: fragmentation,
			Compacted:     false,
		}, nil
	}

	return c.Compact()
}

// ForceCompact runs compaction regardless of fragmentation level
func (c *Compactor) ForceCompact() (*CompactionResult, error) {
	// Temporarily set threshold to 0 to force compaction
	originalThreshold := c.threshold
	c.threshold = 0
	defer func() { c.threshold = originalThreshold }()

	return c.Compact()
}

// String returns a human-readable summary of the compaction result
func (r *CompactionResult) String() string {
	if !r.Compacted {
		return fmt.Sprintf("Compaction skipped (fragmentation: %.1f%%)", r.Fragmentation*100)
	}
	savedBytes := r.OldFileSize - r.NewFileSize
	savedPercent := float64(savedBytes) / float64(r.OldFileSize) * 100
	return fmt.Sprintf("Compaction complete: %d entries (%d removed), %d → %d bytes (%.1f%% saved)",
		r.LiveEntries, r.RemovedEntries, r.OldFileSize, r.NewFileSize, savedPercent)
}

// GetCompactionTempPath returns the path of the temporary compaction file
func GetCompactionTempPath(filePath string) string {
	return filePath + ".compact"
}

// CleanupCompactionTemp removes any leftover compaction temp files
func CleanupCompactionTemp(filePath string) error {
	tempPath := GetCompactionTempPath(filePath)
	if _, err := os.Stat(tempPath); err == nil {
		return os.Remove(tempPath)
	}
	return nil
}

// IsCompacting returns true if compaction is currently running
func (c *Compactor) IsCompacting() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.isRunning
}

// GetThreshold returns the current fragmentation threshold
func (c *Compactor) GetThreshold() float64 {
	return c.threshold
}

// SetThreshold updates the fragmentation threshold
func (c *Compactor) SetThreshold(threshold float64) {
	if threshold > 0 && threshold <= 1 {
		c.threshold = threshold
	}
}

// CompactDirectory compacts all .hyd files in a directory
func CompactDirectory(dirPath string, maxBlockSize int, threshold float64) (map[string]*CompactionResult, error) {
	results := make(map[string]*CompactionResult)

	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return nil, err
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		if filepath.Ext(entry.Name()) != ".hyd" {
			continue
		}

		filePath := filepath.Join(dirPath, entry.Name())
		compactor := NewCompactor(filePath, maxBlockSize, threshold)
		result, err := compactor.CompactIfNeeded()
		if err != nil {
			results[filePath] = &CompactionResult{Error: err}
			continue
		}
		results[filePath] = result
	}

	return results, nil
}
