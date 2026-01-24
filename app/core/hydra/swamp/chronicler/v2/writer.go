package v2

import (
	"errors"
	"io"
	"os"
	"sync"
)

// FileWriter handles append-only writes to a .hyd file.
// It manages the write buffer and flushes blocks to disk.
type FileWriter struct {
	mu         sync.Mutex
	file       *os.File
	filePath   string
	header     *FileHeader
	buffer     *WriteBuffer
	blockCount uint64
	entryCount uint64
	closed     bool
}

// NewFileWriter creates a new file writer for the given path.
// If the file exists, it opens it for appending. Otherwise, it creates a new file.
func NewFileWriter(filePath string, maxBlockSize int) (*FileWriter, error) {
	if filePath == "" {
		return nil, errors.New("file path cannot be empty")
	}

	fw := &FileWriter{
		filePath: filePath,
		buffer:   NewWriteBuffer(maxBlockSize),
	}

	// Check if file exists
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		// Create new file
		if err := fw.createNewFile(); err != nil {
			return nil, err
		}
	} else {
		// Open existing file for appending
		if err := fw.openExistingFile(); err != nil {
			return nil, err
		}
	}

	return fw, nil
}

// createNewFile creates a new .hyd file with header
func (fw *FileWriter) createNewFile() error {
	file, err := os.Create(fw.filePath)
	if err != nil {
		return err
	}

	fw.file = file
	fw.header = NewFileHeader()

	// Write header
	if _, err := file.Write(fw.header.Serialize()); err != nil {
		file.Close()
		return err
	}

	return nil
}

// openExistingFile opens an existing .hyd file and reads its header
func (fw *FileWriter) openExistingFile() error {
	file, err := os.OpenFile(fw.filePath, os.O_RDWR, 0644)
	if err != nil {
		return err
	}

	// Read header
	headerBuf := make([]byte, FileHeaderSize)
	if _, err := io.ReadFull(file, headerBuf); err != nil {
		file.Close()
		return err
	}

	fw.header = &FileHeader{}
	if err := fw.header.Deserialize(headerBuf); err != nil {
		file.Close()
		return err
	}

	fw.file = file
	fw.blockCount = fw.header.BlockCount
	fw.entryCount = fw.header.EntryCount

	// Seek to end for appending
	if _, err := file.Seek(0, io.SeekEnd); err != nil {
		file.Close()
		return err
	}

	return nil
}

// WriteEntry adds an entry to the buffer and flushes if necessary
func (fw *FileWriter) WriteEntry(entry Entry) error {
	fw.mu.Lock()
	defer fw.mu.Unlock()

	if fw.closed {
		return ErrFileClosed
	}

	shouldFlush := fw.buffer.Add(entry)
	if shouldFlush {
		return fw.flushLocked()
	}

	return nil
}

// WriteEntries adds multiple entries to the buffer
func (fw *FileWriter) WriteEntries(entries []Entry) error {
	fw.mu.Lock()
	defer fw.mu.Unlock()

	if fw.closed {
		return ErrFileClosed
	}

	for _, entry := range entries {
		shouldFlush := fw.buffer.Add(entry)
		if shouldFlush {
			if err := fw.flushLocked(); err != nil {
				return err
			}
		}
	}

	return nil
}

// Flush forces a flush of the buffer to disk
func (fw *FileWriter) Flush() error {
	fw.mu.Lock()
	defer fw.mu.Unlock()

	if fw.closed {
		return ErrFileClosed
	}

	return fw.flushLocked()
}

// flushLocked writes the buffer to disk (must be called with lock held)
func (fw *FileWriter) flushLocked() error {
	header, compressed, err := fw.buffer.Flush()
	if err != nil {
		return err
	}

	if header == nil {
		return nil // Nothing to flush
	}

	// Write block header
	if _, err := fw.file.Write(header.Serialize()); err != nil {
		return err
	}

	// Write compressed data
	if _, err := fw.file.Write(compressed); err != nil {
		return err
	}

	// Update counts
	fw.blockCount++
	fw.entryCount += uint64(header.EntryCount)

	return nil
}

// Sync flushes the buffer and syncs to disk
func (fw *FileWriter) Sync() error {
	fw.mu.Lock()
	defer fw.mu.Unlock()

	if fw.closed {
		return ErrFileClosed
	}

	// Flush any remaining buffer
	if err := fw.flushLocked(); err != nil {
		return err
	}

	// Update header with current counts
	fw.header.BlockCount = fw.blockCount
	fw.header.EntryCount = fw.entryCount

	// Seek to beginning and update header
	if _, err := fw.file.Seek(0, io.SeekStart); err != nil {
		return err
	}

	if _, err := fw.file.Write(fw.header.Serialize()); err != nil {
		return err
	}

	// Seek back to end
	if _, err := fw.file.Seek(0, io.SeekEnd); err != nil {
		return err
	}

	// Sync to disk
	return fw.file.Sync()
}

// Close flushes, syncs, and closes the file
func (fw *FileWriter) Close() error {
	fw.mu.Lock()
	defer fw.mu.Unlock()

	if fw.closed {
		return nil
	}

	// Flush remaining buffer
	if err := fw.flushLocked(); err != nil {
		fw.file.Close()
		return err
	}

	// Update header
	fw.header.BlockCount = fw.blockCount
	fw.header.EntryCount = fw.entryCount

	// Seek to beginning and update header
	if _, err := fw.file.Seek(0, io.SeekStart); err != nil {
		fw.file.Close()
		return err
	}

	if _, err := fw.file.Write(fw.header.Serialize()); err != nil {
		fw.file.Close()
		return err
	}

	fw.closed = true
	return fw.file.Close()
}

// GetStats returns current file statistics
func (fw *FileWriter) GetStats() (blockCount, entryCount uint64) {
	fw.mu.Lock()
	defer fw.mu.Unlock()
	return fw.blockCount, fw.entryCount
}

// BufferCount returns the number of entries waiting in the buffer
func (fw *FileWriter) BufferCount() int {
	return fw.buffer.Count()
}
