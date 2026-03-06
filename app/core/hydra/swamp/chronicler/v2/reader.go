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
	swampName string // V3: read from after header. V2: empty until LoadIndex is called.
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

// ReadAllEntries reads all entries from the file.
// For each entry, it calls the callback function with the entry.
// If callback returns false, reading stops.
// Returns: total entries read, error
func (fr *FileReader) ReadAllEntries(callback func(entry Entry) bool) (int, error) {
	// Seek to start of data (after header + swamp name for V3)
	if _, err := fr.file.Seek(fr.header.DataStartOffset(), io.SeekStart); err != nil {
		return 0, err
	}
	totalEntries := 0
	// Read blocks until EOF
	for {
		block, err := fr.readNextBlock()
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

// ReadAllBlocks reads all blocks from the file
func (fr *FileReader) ReadAllBlocks() ([]*Block, error) {
	// Seek to start of data (after header + swamp name for V3)
	if _, err := fr.file.Seek(fr.header.DataStartOffset(), io.SeekStart); err != nil {
		return nil, err
	}
	var blocks []*Block
	// Read blocks until EOF
	for {
		block, err := fr.readNextBlock()
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

// readNextBlock reads the next block from the current file position
func (fr *FileReader) readNextBlock() (*Block, error) {
	// Get current position (for block offset)
	offset, err := fr.file.Seek(0, io.SeekCurrent)
	if err != nil {
		return nil, err
	}
	// Read block header
	headerBuf := make([]byte, BlockHeaderSize)
	n, err := fr.file.Read(headerBuf)
	if err != nil {
		return nil, err
	}
	if n < BlockHeaderSize {
		return nil, io.EOF
	}
	blockHeader := &BlockHeader{}
	if err := blockHeader.Deserialize(headerBuf); err != nil {
		return nil, err
	}
	// Read compressed data
	compressedData := make([]byte, blockHeader.CompressedSize)
	if _, err := io.ReadFull(fr.file, compressedData); err != nil {
		return nil, err
	}
	// Parse block
	block, err := ParseBlock(blockHeader, compressedData)
	if err != nil {
		return nil, err
	}
	block.Offset = offset
	return block, nil
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
// Fragmentation = (total entries - live entries) / total entries
// Returns: fragmentation ratio (0.0 to 1.0), live count, total count, error
func (fr *FileReader) CalculateFragmentation() (float64, int, int, error) {
	liveKeys := make(map[string]bool)
	totalEntries := 0
	_, err := fr.ReadAllEntries(func(entry Entry) bool {
		totalEntries++
		switch entry.Operation {
		case OpDelete:
			delete(liveKeys, entry.Key)
		case OpInsert, OpUpdate:
			liveKeys[entry.Key] = true
		}
		return true
	})
	if err != nil {
		return 0, 0, 0, err
	}
	liveCount := len(liveKeys)
	if totalEntries == 0 {
		return 0, 0, 0, nil
	}
	fragmentation := float64(totalEntries-liveCount) / float64(totalEntries)
	return fragmentation, liveCount, totalEntries, nil
}
