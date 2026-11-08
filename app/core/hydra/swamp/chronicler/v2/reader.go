package v2

import (
	"errors"
	"io"
	"os"
)

// FileReader handles reading from a .hyd file (V2 or V3).
// For V3 files, the swamp name is read from the header area (no block decompression needed).
type FileReader struct {
	file      *os.File
	filePath  string
	header    *FileHeader
	swampName string        // V3: read from after header. V2: empty until LoadIndex is called.
	recovery  RecoveryStats // damage skipped by the most recent block walk
}

// NewFileReader opens a .hyd file for reading.
// Supports both V2 and V3 formats. For V3, the swamp name is read immediately.
func NewFileReader(filePath string) (*FileReader, error) {
	if filePath == "" {
		return nil, errors.New("file path cannot be empty")
	}
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	// Read header
	headerBuf := make([]byte, FileHeaderSize)
	if _, err := io.ReadFull(file, headerBuf); err != nil {
		file.Close()
		return nil, err
	}
	header := &FileHeader{}
	if err := header.Deserialize(headerBuf); err != nil {
		file.Close()
		return nil, err
	}

	fr := &FileReader{
		file:     file,
		filePath: filePath,
		header:   header,
	}

	// V3: read swamp name from after header
	if header.IsV3() && header.NameLength > 0 {
		nameBuf := make([]byte, header.NameLength)
		if _, err := io.ReadFull(file, nameBuf); err != nil {
			file.Close()
			return nil, err
		}
		fr.swampName = string(nameBuf)
	}

	return fr, nil
}

// GetSwampName returns the swamp name.
// For V3 files, this is read from the header area (always available).
// For V2 files, this returns empty string — use LoadIndex to get the name from blocks.
func (fr *FileReader) GetSwampName() string {
	return fr.swampName
}

// GetHeader returns the file header
func (fr *FileReader) GetHeader() *FileHeader {
	return fr.header
}

// RecoveryStats describes the damage the most recent block walk skipped over.
//
// A block is invalid when its header points past the end of the file, its
// size fields are zero, its checksum does not match, or its data does not
// decompress and parse. The usual cause is a torn write: the process died
// while a block was being appended, so part of a block is left on disk and
// later sessions appended new, valid blocks after it. The walk then resyncs
// byte by byte to the next position where a full block validates.
type RecoveryStats struct {
	// SkippedRegions is the number of invalid regions that were followed by
	// a valid block (damage in the middle of the file).
	SkippedRegions int
	// SkippedBytes is the total size of those middle regions.
	SkippedBytes int64
	// TailBytes is the number of bytes after the last valid block that do not
	// form a valid block (a torn tail at the end of the file).
	TailBytes int64
	// ValidEnd is the file offset right after the last valid block, or the
	// data start offset when the file has no valid block.
	ValidEnd int64
}

// Recovered reports whether the walk had to skip any invalid bytes.
func (s RecoveryStats) Recovered() bool {
	return s.SkippedRegions > 0 || s.SkippedBytes > 0 || s.TailBytes > 0
}

// RecoveryStats returns the recovery statistics of the most recent full walk
// (ReadAllEntries, ReadAllBlocks, LoadIndex, CalculateFragmentation or
// ScanBlockHeaders). The zero value means the file was clean.
func (fr *FileReader) RecoveryStats() RecoveryStats {
	return fr.recovery
}

// ReadAllEntries reads all entries from the file.
// For each entry, it calls the callback function with the entry.
// If callback returns false, reading stops.
// Invalid blocks are skipped (see RecoveryStats), they never abort the walk.
// Returns: total entries read, error (only for I/O errors)
func (fr *FileReader) ReadAllEntries(callback func(entry Entry) bool) (int, error) {
	w, err := fr.newBlockWalker()
	if err != nil {
		return 0, err
	}
	totalEntries := 0
	for {
		block, err := w.next()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return totalEntries, err
		}
		// Process entries in block
		for _, entry := range block.Entries {
			totalEntries++
			if !callback(entry) {
				return totalEntries, nil
			}
		}
	}
	return totalEntries, nil
}

// ReadAllBlocks reads all valid blocks from the file.
// Invalid blocks are skipped (see RecoveryStats).
func (fr *FileReader) ReadAllBlocks() ([]*Block, error) {
	w, err := fr.newBlockWalker()
	if err != nil {
		return nil, err
	}
	var blocks []*Block
	for {
		block, err := w.next()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, err
		}
		blocks = append(blocks, block)
	}
	return blocks, nil
}

// blockWalker walks the blocks of a file from the data start offset.
//
// Happy path: each block is read with two positional reads (header, data),
// the same I/O as before. When the first invalid block is hit, the rest of
// the file is read into memory once and the walk continues from that buffer,
// so the byte-by-byte resync never issues one syscall per byte.
type blockWalker struct {
	fr      *FileReader
	offset  int64  // offset of the next block to read
	size    int64  // file size at walk start
	mem     []byte // file content from memBase to size, loaded at the first gap
	memBase int64
}

func (fr *FileReader) newBlockWalker() (*blockWalker, error) {
	info, err := fr.file.Stat()
	if err != nil {
		return nil, err
	}
	start := fr.header.DataStartOffset()
	fr.recovery = RecoveryStats{ValidEnd: start}
	return &blockWalker{
		fr:     fr,
		offset: start,
		size:   info.Size(),
	}, nil
}

// next returns the next valid block, io.EOF at the end of the file, or an
// I/O error. Invalid regions are skipped and recorded in fr.recovery.
func (w *blockWalker) next() (*Block, error) {
	if w.offset >= w.size {
		return nil, io.EOF
	}

	if w.mem == nil {
		block, end, err := w.readFromFile(w.offset)
		if err != nil {
			return nil, err
		}
		if block != nil {
			w.advance(block, w.offset, end)
			return block, nil
		}
		// First invalid block: load the rest of the file once for the resync.
		w.mem = make([]byte, w.size-w.offset)
		if _, err := w.fr.file.ReadAt(w.mem, w.offset); err != nil && !errors.Is(err, io.EOF) {
			return nil, err
		}
		w.memBase = w.offset
	}

	gapStart := w.offset
	for pos := gapStart; pos+BlockHeaderSize <= w.size; pos++ {
		// At the expected offset the block is validated as on the happy path;
		// while scanning, cheap header plausibility checks run first so the
		// checksum is computed only for real candidates.
		block, end := w.parseFromMem(pos, pos != gapStart)
		if block == nil {
			continue
		}
		if pos > gapStart {
			w.fr.recovery.SkippedRegions++
			w.fr.recovery.SkippedBytes += pos - gapStart
		}
		w.advance(block, pos, end)
		return block, nil
	}

	// No valid block until EOF: torn tail.
	w.fr.recovery.TailBytes += w.size - gapStart
	w.offset = w.size
	return nil, io.EOF
}

func (w *blockWalker) advance(block *Block, start, end int64) {
	block.Offset = start
	w.offset = end
	w.fr.recovery.ValidEnd = end
}

// readFromFile reads and parses the block at offset with positional reads.
// It returns (nil, 0, nil) when the block is invalid, and an error only for
// I/O failures.
func (w *blockWalker) readFromFile(offset int64) (*Block, int64, error) {
	if offset+BlockHeaderSize > w.size {
		return nil, 0, nil
	}
	headerBuf := make([]byte, BlockHeaderSize)
	if _, err := w.fr.file.ReadAt(headerBuf, offset); err != nil {
		if errors.Is(err, io.EOF) {
			return nil, 0, nil
		}
		return nil, 0, err
	}
	bh := &BlockHeader{}
	if err := bh.Deserialize(headerBuf); err != nil {
		return nil, 0, nil
	}
	end := offset + BlockHeaderSize + int64(bh.CompressedSize)
	if bh.CompressedSize == 0 || end > w.size {
		return nil, 0, nil
	}
	compressedData := make([]byte, bh.CompressedSize)
	if _, err := w.fr.file.ReadAt(compressedData, offset+BlockHeaderSize); err != nil {
		if errors.Is(err, io.EOF) {
			return nil, 0, nil
		}
		return nil, 0, err
	}
	block, err := ParseBlock(bh, compressedData)
	if err != nil {
		return nil, 0, nil
	}
	return block, end, nil
}

// parseFromMem tries to parse a full valid block at file offset pos from the
// in-memory buffer. strict adds plausibility checks used while scanning.
func (w *blockWalker) parseFromMem(pos int64, strict bool) (*Block, int64) {
	rel := pos - w.memBase
	bh := &BlockHeader{}
	if err := bh.Deserialize(w.mem[rel : rel+BlockHeaderSize]); err != nil {
		return nil, 0
	}
	if bh.CompressedSize == 0 {
		return nil, 0
	}
	if strict && (bh.Flags != 0 || bh.EntryCount == 0 || bh.UncompressedSize == 0) {
		return nil, 0
	}
	end := pos + BlockHeaderSize + int64(bh.CompressedSize)
	if end > w.size {
		return nil, 0
	}
	block, err := ParseBlock(bh, w.mem[rel+BlockHeaderSize:end-w.memBase])
	if err != nil {
		return nil, 0
	}
	return block, end
}

// BlockScanResult contains aggregated statistics from scanning block headers.
// No decompression is performed — only the 16-byte block headers are read.
type BlockScanResult struct {
	BlockCount            uint64
	TotalEntryCount       uint64
	TotalUncompressedSize uint64
}

// ScanBlockHeaders reads all block headers without decompressing block data.
// This is used to get accurate block/entry counts and estimated memory size
// when the file header has stale values (e.g., writer not yet closed).
//
// If the header chain is inconsistent (a zero size, or a block that would end
// past EOF or leaves a partial header at EOF), the file is damaged and the scan
// falls back to a full validated walk, so the counts cover only valid blocks
// and RecoveryStats reports what was skipped. A header-only scan cannot verify
// checksums, so damage that keeps the chain consistent goes undetected here.
func (fr *FileReader) ScanBlockHeaders() (*BlockScanResult, error) {
	info, err := fr.file.Stat()
	if err != nil {
		return nil, err
	}
	size := info.Size()
	offset := fr.header.DataStartOffset()
	fr.recovery = RecoveryStats{ValidEnd: offset}

	result := &BlockScanResult{}
	headerBuf := make([]byte, BlockHeaderSize)

	for offset < size {
		if offset+BlockHeaderSize > size {
			return fr.scanBlockHeadersValidated()
		}
		if _, err := fr.file.ReadAt(headerBuf, offset); err != nil {
			return nil, err
		}

		var bh BlockHeader
		if err := bh.Deserialize(headerBuf); err != nil {
			return nil, err
		}
		end := offset + BlockHeaderSize + int64(bh.CompressedSize)
		if bh.CompressedSize == 0 || bh.EntryCount == 0 || end > size {
			return fr.scanBlockHeadersValidated()
		}

		result.BlockCount++
		result.TotalEntryCount += uint64(bh.EntryCount)
		result.TotalUncompressedSize += uint64(bh.UncompressedSize)
		offset = end
	}
	fr.recovery.ValidEnd = offset

	return result, nil
}

// scanBlockHeadersValidated computes the scan statistics from a full,
// validated block walk. Used only when the header chain is damaged.
func (fr *FileReader) scanBlockHeadersValidated() (*BlockScanResult, error) {
	w, err := fr.newBlockWalker()
	if err != nil {
		return nil, err
	}
	result := &BlockScanResult{}
	for {
		block, err := w.next()
		if err != nil {
			if errors.Is(err, io.EOF) {
				return result, nil
			}
			return nil, err
		}
		result.BlockCount++
		result.TotalEntryCount += uint64(block.Header.EntryCount)
		result.TotalUncompressedSize += uint64(block.Header.UncompressedSize)
	}
}

// Close closes the file
func (fr *FileReader) Close() error {
	if fr.file != nil {
		return fr.file.Close()
	}
	return nil
}

// MetadataEntryKey is the special key used for storing swamp metadata in V2 files.
const MetadataEntryKey = "__swamp_meta__"

// LoadIndex reads the file and builds an in-memory index.
// The index maps key -> latest entry data.
// DELETE entries remove keys from the index.
// Metadata entries (OpMetadata) store the swamp name and are not included in the index.
// Returns: map of key to entry data, swamp name (if found), error
func (fr *FileReader) LoadIndex() (map[string][]byte, string, error) {
	index := make(map[string][]byte)
	swampName := fr.swampName // V3: already read from header area
	_, err := fr.ReadAllEntries(func(entry Entry) bool {
		switch entry.Operation {
		case OpDelete:
			delete(index, entry.Key)
		case OpInsert, OpUpdate:
			// Make a copy of the data
			dataCopy := make([]byte, len(entry.Data))
			copy(dataCopy, entry.Data)
			index[entry.Key] = dataCopy
		case OpMetadata:
			// V2 fallback: extract swamp name from metadata entry
			if swampName == "" && entry.Key == MetadataEntryKey && len(entry.Data) > 0 {
				swampName = string(entry.Data)
			}
		}
		return true // Continue reading
	})
	if err != nil {
		return nil, swampName, err
	}
	return index, swampName, nil
}

// ReadSwampName reads only the swamp name from a .hyd file without loading blocks.
// For V3 files, reads ~100 bytes (header + name). For V2 files, must decompress blocks.
// This is the fast-path function for the Swamp Explorer scanner.
func ReadSwampName(filePath string) (string, error) {
	fr, err := NewFileReader(filePath)
	if err != nil {
		return "", err
	}
	defer fr.Close()

	// V3: name already read from header area
	if fr.header.IsV3() {
		return fr.swampName, nil
	}

	// V2 fallback: must read blocks to find OpMetadata entry
	_, swampName, err := fr.LoadIndex()
	if err != nil {
		return "", err
	}
	return swampName, nil
}

// CalculateFragmentation reads the file and calculates fragmentation.
// It computes both entry-based and byte-based fragmentation and returns
// the higher of the two. This prevents large dead entries from going
// undetected when entry counts alone show low fragmentation.
//
// Entry-based: (total entries - live entries) / total entries
// Byte-based:  (total data bytes - live data bytes) / total data bytes
//
// Returns: fragmentation ratio (0.0 to 1.0), live count, total count, error
func (fr *FileReader) CalculateFragmentation() (float64, int, int, error) {
	liveKeys := make(map[string]int) // key → latest data size
	totalEntries := 0
	totalBytes := 0

	_, err := fr.ReadAllEntries(func(entry Entry) bool {
		totalEntries++
		switch entry.Operation {
		case OpDelete:
			delete(liveKeys, entry.Key)
		case OpInsert, OpUpdate:
			liveKeys[entry.Key] = len(entry.Data)
		}
		totalBytes += len(entry.Data)
		return true
	})
	if err != nil {
		return 0, 0, 0, err
	}

	liveCount := len(liveKeys)
	if totalEntries == 0 {
		return 0, 0, 0, nil
	}

	// Entry-based fragmentation
	entryFrag := float64(totalEntries-liveCount) / float64(totalEntries)

	// Byte-based fragmentation
	liveBytes := 0
	for _, size := range liveKeys {
		liveBytes += size
	}
	byteFrag := 0.0
	if totalBytes > 0 {
		byteFrag = float64(totalBytes-liveBytes) / float64(totalBytes)
	}

	// Use the higher of the two — catches both many-small-dead-entries
	// and few-but-large-dead-entries scenarios
	fragmentation := entryFrag
	if byteFrag > fragmentation {
		fragmentation = byteFrag
	}

	return fragmentation, liveCount, totalEntries, nil
}
