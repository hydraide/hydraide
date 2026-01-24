package v2

import (
	"bytes"
	"sync"

	"github.com/hydraide/hydraide/app/core/compressor"
)

// snappyCompressor is a shared compressor instance for Snappy compression
var snappyCompressor = compressor.New(compressor.Snappy)

// WriteBuffer collects entries before flushing them as a compressed block.
// It provides efficient batching of writes to minimize I/O operations.
type WriteBuffer struct {
	mu          sync.Mutex
	entries     []Entry
	currentSize int
	maxSize     int
}

// NewWriteBuffer creates a new write buffer with the specified maximum size
func NewWriteBuffer(maxSize int) *WriteBuffer {
	if maxSize <= 0 {
		maxSize = DefaultMaxBlockSize
	}
	return &WriteBuffer{
		entries: make([]Entry, 0, 64), // Pre-allocate for typical usage
		maxSize: maxSize,
	}
}

// Add appends an entry to the buffer and returns true if the buffer should be flushed
func (wb *WriteBuffer) Add(entry Entry) bool {
	wb.mu.Lock()
	defer wb.mu.Unlock()

	wb.entries = append(wb.entries, entry)
	wb.currentSize += entry.Size()

	return wb.currentSize >= wb.maxSize
}

// ShouldFlush returns true if the buffer has reached its maximum size
func (wb *WriteBuffer) ShouldFlush() bool {
	wb.mu.Lock()
	defer wb.mu.Unlock()
	return wb.currentSize >= wb.maxSize
}

// IsEmpty returns true if the buffer has no entries
func (wb *WriteBuffer) IsEmpty() bool {
	wb.mu.Lock()
	defer wb.mu.Unlock()
	return len(wb.entries) == 0
}

// Size returns the current uncompressed size of the buffer
func (wb *WriteBuffer) Size() int {
	wb.mu.Lock()
	defer wb.mu.Unlock()
	return wb.currentSize
}

// Count returns the number of entries in the buffer
func (wb *WriteBuffer) Count() int {
	wb.mu.Lock()
	defer wb.mu.Unlock()
	return len(wb.entries)
}

// Flush serializes all entries, compresses them, and returns the block data.
// The buffer is cleared after flushing.
// Returns: blockHeader, compressedData, error
func (wb *WriteBuffer) Flush() (*BlockHeader, []byte, error) {
	wb.mu.Lock()
	defer wb.mu.Unlock()

	if len(wb.entries) == 0 {
		return nil, nil, nil
	}

	// Serialize all entries
	uncompressed := wb.serializeEntries()

	// Compress with Snappy using the shared compressor
	compressed, err := snappyCompressor.Compress(uncompressed)
	if err != nil {
		return nil, nil, err
	}

	// Create block header
	header := &BlockHeader{
		CompressedSize:   uint32(len(compressed)),
		UncompressedSize: uint32(len(uncompressed)),
		EntryCount:       uint16(len(wb.entries)),
		Checksum:         CalculateChecksum(compressed),
		Flags:            0,
	}

	// Clear buffer
	wb.entries = wb.entries[:0]
	wb.currentSize = 0

	return header, compressed, nil
}

// GetEntriesAndClear returns all entries and clears the buffer.
// This is useful when we need entries without compression (e.g., for compaction).
func (wb *WriteBuffer) GetEntriesAndClear() []Entry {
	wb.mu.Lock()
	defer wb.mu.Unlock()

	entries := make([]Entry, len(wb.entries))
	copy(entries, wb.entries)

	wb.entries = wb.entries[:0]
	wb.currentSize = 0

	return entries
}

// serializeEntries converts all entries to a single byte slice
func (wb *WriteBuffer) serializeEntries() []byte {
	var buf bytes.Buffer
	buf.Grow(wb.currentSize)

	for _, entry := range wb.entries {
		buf.Write(entry.Serialize())
	}

	return buf.Bytes()
}

// Block represents a decompressed block with its entries
type Block struct {
	Header  BlockHeader
	Entries []Entry
	Offset  int64 // Position in file where this block starts
}

// ParseBlock decompresses and parses a block from compressed data
func ParseBlock(header *BlockHeader, compressedData []byte) (*Block, error) {
	// Validate checksum
	if !ValidateChecksum(compressedData, header.Checksum) {
		return nil, ErrCorruptedBlock
	}

	// Decompress using the shared compressor
	uncompressed, err := snappyCompressor.Decompress(compressedData)
	if err != nil {
		return nil, err
	}

	// Validate uncompressed size
	if uint32(len(uncompressed)) != header.UncompressedSize {
		return nil, ErrCorruptedBlock
	}

	// Parse entries
	entries := make([]Entry, 0, header.EntryCount)
	offset := 0

	for i := uint16(0); i < header.EntryCount; i++ {
		entry := &Entry{}
		consumed, err := entry.Deserialize(uncompressed[offset:])
		if err != nil {
			return nil, err
		}
		entries = append(entries, *entry)
		offset += consumed
	}

	return &Block{
		Header:  *header,
		Entries: entries,
	}, nil
}

// CompressEntries takes a slice of entries and returns compressed block data
func CompressEntries(entries []Entry) (*BlockHeader, []byte, error) {
	if len(entries) == 0 {
		return nil, nil, nil
	}

	// Calculate total size and serialize
	var buf bytes.Buffer
	for _, entry := range entries {
		buf.Write(entry.Serialize())
	}
	uncompressed := buf.Bytes()

	// Compress using the shared compressor
	compressed, err := snappyCompressor.Compress(uncompressed)
	if err != nil {
		return nil, nil, err
	}

	// Create header
	header := &BlockHeader{
		CompressedSize:   uint32(len(compressed)),
		UncompressedSize: uint32(len(uncompressed)),
		EntryCount:       uint16(len(entries)),
		Checksum:         CalculateChecksum(compressed),
		Flags:            0,
	}

	return header, compressed, nil
}

// DecompressBlock decompresses raw block data without parsing entries
func DecompressBlock(compressedData []byte) ([]byte, error) {
	return snappyCompressor.Decompress(compressedData)
}

// CompressBlock compresses raw data
func CompressBlock(data []byte) []byte {
	compressed, err := snappyCompressor.Compress(data)
	if err != nil {
		return nil
	}
	return compressed
}
