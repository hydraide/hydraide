// Package v2 implements the append-only block storage format for HydrAIDE swamps.
// This package provides a single-file-per-swamp storage solution with block-based
// compression, reducing file count by ~100x and improving write performance significantly.
//
// File format:
//   - Single .hyd file per swamp
//   - Append-only writes (no random I/O)
//   - 16KB block size (ZFS optimized)
//   - Snappy compression per block
//   - Automatic compaction when fragmentation exceeds threshold
package v2

import (
	"encoding/binary"
	"errors"
	"hash/crc32"
	"time"
)

// File format constants
const (
	// MagicBytes identifies a valid HydrAIDE V2 file
	MagicBytes = "HYDR"

	// CurrentVersion is the current file format version
	CurrentVersion uint16 = 2

	// DefaultMaxBlockSize is the maximum uncompressed block size (16KB for ZFS optimization)
	DefaultMaxBlockSize = 16 * 1024

	// FileHeaderSize is the fixed size of the file header
	FileHeaderSize = 64

	// BlockHeaderSize is the fixed size of each block header
	BlockHeaderSize = 16
)

// Operation types for entries
const (
	OpInsert uint8 = 1
	OpUpdate uint8 = 2
	OpDelete uint8 = 3
)

// Errors
var (
	ErrInvalidMagic      = errors.New("invalid magic bytes: not a HydrAIDE V2 file")
	ErrUnsupportedVer    = errors.New("unsupported file version")
	ErrCorruptedBlock    = errors.New("block checksum mismatch")
	ErrCorruptedEntry    = errors.New("entry data corrupted")
	ErrEmptyKey          = errors.New("entry key cannot be empty")
	ErrFileClosed        = errors.New("file is closed")
	ErrCompactionRunning = errors.New("compaction is already running")
)

// FileHeader represents the header at the beginning of each .hyd file.
// Total size: 64 bytes (fixed)
type FileHeader struct {
	Magic      [4]byte // "HYDR"
	Version    uint16  // File format version (currently 2)
	Flags      uint16  // Reserved for future use
	CreatedAt  int64   // Unix nano timestamp when file was created
	ModifiedAt int64   // Unix nano timestamp of last modification
	BlockSize  uint32  // Maximum block size (default 16KB)
	EntryCount uint64  // Total number of live entries (updated after compaction)
	BlockCount uint64  // Total number of blocks in file
	Reserved   [16]byte
}

// Serialize converts the header to bytes
func (h *FileHeader) Serialize() []byte {
	buf := make([]byte, FileHeaderSize)
	copy(buf[0:4], h.Magic[:])
	binary.LittleEndian.PutUint16(buf[4:6], h.Version)
	binary.LittleEndian.PutUint16(buf[6:8], h.Flags)
	binary.LittleEndian.PutUint64(buf[8:16], uint64(h.CreatedAt))
	binary.LittleEndian.PutUint64(buf[16:24], uint64(h.ModifiedAt))
	binary.LittleEndian.PutUint32(buf[24:28], h.BlockSize)
	binary.LittleEndian.PutUint64(buf[28:36], h.EntryCount)
	binary.LittleEndian.PutUint64(buf[36:44], h.BlockCount)
	copy(buf[44:60], h.Reserved[:])
	return buf
}

// Deserialize parses bytes into the header
func (h *FileHeader) Deserialize(buf []byte) error {
	if len(buf) < FileHeaderSize {
		return errors.New("buffer too small for file header")
	}

	copy(h.Magic[:], buf[0:4])
	if string(h.Magic[:]) != MagicBytes {
		return ErrInvalidMagic
	}

	h.Version = binary.LittleEndian.Uint16(buf[4:6])
	if h.Version != CurrentVersion {
		return ErrUnsupportedVer
	}

	h.Flags = binary.LittleEndian.Uint16(buf[6:8])
	h.CreatedAt = int64(binary.LittleEndian.Uint64(buf[8:16]))
	h.ModifiedAt = int64(binary.LittleEndian.Uint64(buf[16:24]))
	h.BlockSize = binary.LittleEndian.Uint32(buf[24:28])
	h.EntryCount = binary.LittleEndian.Uint64(buf[28:36])
	h.BlockCount = binary.LittleEndian.Uint64(buf[36:44])
	copy(h.Reserved[:], buf[44:60])

	return nil
}

// NewFileHeader creates a new file header with default values
func NewFileHeader() *FileHeader {
	now := time.Now().UnixNano()
	return &FileHeader{
		Magic:      [4]byte{'H', 'Y', 'D', 'R'},
		Version:    CurrentVersion,
		CreatedAt:  now,
		ModifiedAt: now,
		BlockSize:  DefaultMaxBlockSize,
	}
}

// BlockHeader represents the header of each compressed block.
// Total size: 16 bytes (fixed)
type BlockHeader struct {
	CompressedSize   uint32 // Size of compressed data
	UncompressedSize uint32 // Original size before compression
	EntryCount       uint16 // Number of entries in this block
	Checksum         uint32 // CRC32 of compressed data
	Flags            uint16 // Reserved for future use
}

// Serialize converts the block header to bytes
func (b *BlockHeader) Serialize() []byte {
	buf := make([]byte, BlockHeaderSize)
	binary.LittleEndian.PutUint32(buf[0:4], b.CompressedSize)
	binary.LittleEndian.PutUint32(buf[4:8], b.UncompressedSize)
	binary.LittleEndian.PutUint16(buf[8:10], b.EntryCount)
	binary.LittleEndian.PutUint32(buf[10:14], b.Checksum)
	binary.LittleEndian.PutUint16(buf[14:16], b.Flags)
	return buf
}

// Deserialize parses bytes into the block header
func (b *BlockHeader) Deserialize(buf []byte) error {
	if len(buf) < BlockHeaderSize {
		return errors.New("buffer too small for block header")
	}

	b.CompressedSize = binary.LittleEndian.Uint32(buf[0:4])
	b.UncompressedSize = binary.LittleEndian.Uint32(buf[4:8])
	b.EntryCount = binary.LittleEndian.Uint16(buf[8:10])
	b.Checksum = binary.LittleEndian.Uint32(buf[10:14])
	b.Flags = binary.LittleEndian.Uint16(buf[14:16])

	return nil
}

// Entry represents a single key-value entry in a block.
// Variable size: 1 + 2 + keyLen + 4 + dataLen bytes
type Entry struct {
	Operation uint8  // OpInsert, OpUpdate, or OpDelete
	Key       string // Unique key for this entry
	Data      []byte // Serialized treasure data (empty for delete)
}

// Serialize converts the entry to bytes
func (e *Entry) Serialize() []byte {
	keyBytes := []byte(e.Key)
	keyLen := len(keyBytes)
	dataLen := len(e.Data)

	// Calculate total size: op(1) + keyLen(2) + key(N) + dataLen(4) + data(M)
	totalSize := 1 + 2 + keyLen + 4 + dataLen
	buf := make([]byte, totalSize)

	offset := 0

	// Operation (1 byte)
	buf[offset] = e.Operation
	offset++

	// Key length (2 bytes)
	binary.LittleEndian.PutUint16(buf[offset:offset+2], uint16(keyLen))
	offset += 2

	// Key (N bytes)
	copy(buf[offset:offset+keyLen], keyBytes)
	offset += keyLen

	// Data length (4 bytes)
	binary.LittleEndian.PutUint32(buf[offset:offset+4], uint32(dataLen))
	offset += 4

	// Data (M bytes)
	if dataLen > 0 {
		copy(buf[offset:], e.Data)
	}

	return buf
}

// Deserialize parses bytes into the entry, returns bytes consumed
func (e *Entry) Deserialize(buf []byte) (int, error) {
	if len(buf) < 7 { // minimum: op(1) + keyLen(2) + dataLen(4)
		return 0, ErrCorruptedEntry
	}

	offset := 0

	// Operation (1 byte)
	e.Operation = buf[offset]
	offset++

	// Key length (2 bytes)
	keyLen := int(binary.LittleEndian.Uint16(buf[offset : offset+2]))
	offset += 2

	if len(buf) < offset+keyLen+4 {
		return 0, ErrCorruptedEntry
	}

	// Key (N bytes)
	e.Key = string(buf[offset : offset+keyLen])
	offset += keyLen

	if e.Key == "" {
		return 0, ErrEmptyKey
	}

	// Data length (4 bytes)
	dataLen := int(binary.LittleEndian.Uint32(buf[offset : offset+4]))
	offset += 4

	if len(buf) < offset+dataLen {
		return 0, ErrCorruptedEntry
	}

	// Data (M bytes)
	if dataLen > 0 {
		e.Data = make([]byte, dataLen)
		copy(e.Data, buf[offset:offset+dataLen])
	} else {
		e.Data = nil
	}
	offset += dataLen

	return offset, nil
}

// Size returns the serialized size of the entry
func (e *Entry) Size() int {
	return 1 + 2 + len(e.Key) + 4 + len(e.Data)
}

// CalculateChecksum computes CRC32 checksum for data
func CalculateChecksum(data []byte) uint32 {
	return crc32.ChecksumIEEE(data)
}

// ValidateChecksum verifies if the checksum matches
func ValidateChecksum(data []byte, expected uint32) bool {
	return CalculateChecksum(data) == expected
}
