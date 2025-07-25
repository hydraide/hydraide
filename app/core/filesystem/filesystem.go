// Package filesystem provides thread-safe operations for low-level file and folder handling,
// including compression, atomic writes, and binary-slice storage.
//
// This package is designed to support high-performance embedded storage systems,
// where data is stored as compressed binary segments in single files.
//
// Key features:
//   - Safe concurrent access using mutexes per file/folder
//   - Transparent compression (e.g. Snappy) via the Compressor interface
//   - Append and overwrite modes for structured binary data
//   - Recursive deletion with depth limit
//   - Metadata-friendly structure for integration with higher-level swamp logic
//
// All read/write operations use lock granularity (per file or folder), enabling controlled parallelism.
// Intended use cases include embedded databases, in-process file systems, and edge-node persistence.
package filesystem

import (
	"bytes"
	"encoding/binary"
	"errors"
	"github.com/hydraide/hydraide/app/core/compressor"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
)

// Filesystem defines thread-safe file and folder operations with support for
// compression, binary-encoded content, and controlled deletion.
//
// All methods are designed for use in embedded systems, persistent layers,
// and modular storage engines.

type Filesystem interface {
	// CreateFolder creates the given folder path if it doesn't already exist.
	CreateFolder(folderPath string) error

	// DeleteFolder attempts to delete the folder if it's empty, then recursively
	// checks and deletes its parent folders up to maxDepth levels.
	DeleteFolder(folderPath string, maxDepth int) error

	// SaveFile stores or updates the given file with binary-encoded and compressed content.
	// If appendFile is true, the new content is appended to the existing file content.
	// If false, the file is overwritten with the new content.
	SaveFile(filePath string, content [][]byte, appendFile bool) error

	// DeleteFile removes the specified file if it exists.
	DeleteFile(filePath string) error

	// DeleteAllFiles removes all files within the given folder. Subfolders are not deleted.
	DeleteAllFiles(folderPath string) error

	// GetFile reads and decompresses the specified file, returning its content
	// as a slice of binary segments ([][]byte).
	GetFile(filePath string) ([][]byte, error)

	// GetAllFileContents reads all files in the given folder (excluding listed ones),
	// and returns a map of filename to binary content segments.
	GetAllFileContents(folderPath string, excludedFiles ...string) (map[string][][]byte, error)

	// GetFileSize returns the size of the file in bytes.
	GetFileSize(filePath string) (int64, error)

	// IsFolderExists checks whether the given folder path exists.
	IsFolderExists(folderPath string) bool
}

type filesystem struct {
	folderLocks         sync.Map              // Mappa zárolások kezelése
	compressorInterface compressor.Compressor // compressorInterface a fájlok be és -kitömörítését kezeli
}

func New() Filesystem {
	fs := &filesystem{
		compressorInterface: compressor.New(compressor.Snappy),
	}
	return fs
}

// CreateFolder creates the specified absolute folder path if it does not already exist.
func (fs *filesystem) CreateFolder(folderPath string) error {

	// Validate the folder path
	if folderPath == "" {
		return errors.New("invalid folder path")
	}

	// Acquire a dedicated lock for the folder (folder-level lock)
	folderLock := fs.getFolderLock(folderPath)
	folderLock.Lock()
	defer folderLock.Unlock()

	// Create the folder if it doesn't exist
	return os.MkdirAll(folderPath, os.ModePerm)

}

// DeleteFolder deletes the specified absolute folder path if it's empty,
// and recursively checks and removes parent folders up to maxDepth levels.
func (fs *filesystem) DeleteFolder(folderPath string, maxDepth int) error {

	// Normalize the folder path to ensure consistent comparison
	folderPath = filepath.Clean(folderPath)

	// Check if the folder exists
	if _, err := os.Stat(folderPath); os.IsNotExist(err) {
		return nil // Ha a mappa már nem létezik, nincs mit törölni
	}

	// Acquire lock for folder deletion (folder-level lock)
	folderLock := fs.getFolderLock(folderPath)
	folderLock.Lock()

	// Check if the folder is empty and delete it if so
	err := fs.deleteIfEmpty(folderPath)

	// Release the lock after deletion
	folderLock.Unlock()

	if err != nil {
		return err // Ha a mappa nem üres, térjünk vissza hibával
	}

	// If folder is not empty, return with error

	// Check and delete parent folders up to maxDepth
	for i := 0; i < maxDepth; i++ {
		parentPath := filepath.Dir(folderPath)
		// Lock the parent folder
		parentLock := fs.getFolderLock(parentPath)
		parentLock.Lock()
		// Check if the parent is empty and delete it if so
		err := fs.deleteIfEmpty(parentPath)
		// Unlock the parent after deletion
		parentLock.Unlock()
		if err != nil {
			break // Ha egy szülő nem üres, kilépünk
		}
		// Stop if a parent is not empty
		folderPath = parentPath
	}

	return nil

}

// SaveFile creates or updates a file with the given binary content.
// If appendFile is true, the new content is appended to the existing file content.
// If appendFile is false, the file is fully overwritten with the new content.
func (fs *filesystem) SaveFile(filePath string, content [][]byte, appendFile bool) error {
	// Validate the file path
	if filePath == "" {
		return errors.New("invalid file path")
	}

	// Acquire a lock for file-level operations
	fileLock := fs.getFolderLock(filePath)
	fileLock.Lock()
	defer fileLock.Unlock()

	// Check if the file exists
	if _, err := os.Stat(filePath); err != nil {
		if errors.Is(err, os.ErrNotExist) {

			// If the file doesn't exist locally, optionally check a remote backup (e.g., aegisInterface)
			// TODO: implement remote check and download logic
			// Example:
			// if fs.aegisInterface.Exists(filePath) {
			//     return fs.aegisInterface.Download(filePath)
			// }

			// Create necessary folders if the file and its path do not exist
			if err := os.MkdirAll(filepath.Dir(filePath), os.ModePerm); err != nil {
				return err
			}
		} else {
			return err // Return other stat errors
		}
	}

	var finalContent []byte

	if appendFile {
		// Read existing file content if appending
		existingContent, err := os.ReadFile(filePath)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}

		// Decompress existing content (if any)
		if len(existingContent) > 0 {
			decompressedContent, err := fs.compressorInterface.Decompress(existingContent)
			if err != nil {
				return err
			}
			finalContent = decompressedContent
		}

		// Append new binary parts to the decompressed content
		for _, part := range content {
			finalContent = append(finalContent, encodeBinaryLength(part)...)
			finalContent = append(finalContent, part...)
		}

	} else {
		// If not appending, build content from scratch
		for _, part := range content {
			finalContent = append(finalContent, encodeBinaryLength(part)...)
			finalContent = append(finalContent, part...)
		}
	}

	// Compress the final binary content
	compressedContent, err := fs.compressorInterface.Compress(finalContent)
	if err != nil {
		return err
	}

	// Write the compressed content to the file
	return os.WriteFile(filePath, compressedContent, os.ModePerm)
}

// DeleteFile removes the specified file if it exists.
func (fs *filesystem) DeleteFile(filePath string) error {
	// Validate the file path
	if filePath == "" {
		return errors.New("invalid file path")
	}

	// Acquire a lock for the file (file-level lock)
	fileLock := fs.getFolderLock(filePath)
	fileLock.Lock()
	defer fileLock.Unlock()

	// Check if the file exists
	if _, err := os.Stat(filePath); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			// If the file doesn't exist locally, optionally delete it from remote storage (e.g., aegisInterface)
			// TODO: implement deletion from aegis
		}
		return err // Return other stat errors
	}

	// Delete the file from the local filesystem
	err := os.Remove(filePath)
	if err != nil {
		return err // Return error if deletion failed
	}

	// Delete the file from remote storage (e.g., aegisInterface)
	// TODO: implement remote file deletion
	return nil
}

// DeleteAllFiles removes all files within the specified folder.
// Only regular files are deleted — subdirectories are ignored.
// Each file is locked individually to ensure safe concurrent access.
func (fs *filesystem) DeleteAllFiles(folderPath string) error {
	// Validate the folder path
	if folderPath == "" {
		return errors.New("invalid folder path")
	}

	// Check if the folder exists
	dirEntries, err := os.ReadDir(folderPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // If the folder doesn't exist, there's nothing to delete
		}
		return err // Return error if folder check failed
	}

	// Iterate over each entry in the folder
	for _, entry := range dirEntries {
		// Skip subdirectories — we only delete files
		if entry.IsDir() {
			continue
		}

		// Full path to the file
		filePath := filepath.Join(folderPath, entry.Name())

		// Acquire lock for the file (file-level lock)
		fileLock := fs.getFolderLock(filePath)
		fileLock.Lock()

		// Delete the file from the local filesystem
		err := os.Remove(filePath)
		if err != nil {
			fileLock.Unlock() // Always release the lock before returning
			return err
		}

		// TODO: Implement deletion from remote Aegis storage
		// Example:
		// err = fs.aegisInterface.Delete(filePath)
		// if err != nil {
		//     fileLock.Unlock()
		//     return err
		// }

		// Release the file lock
		fileLock.Unlock()
	}

	return nil
}

// GetFile returns the contents of the specified file as a slice of binary segments.
// The file is expected to be compressed and structured as length-prefixed binary blocks.
func (fs *filesystem) GetFile(filePath string) ([][]byte, error) {
	// Validate the file path
	if filePath == "" {
		return nil, errors.New("invalid file path")
	}

	// Acquire file-level lock
	fileLock := fs.getFolderLock(filePath)
	fileLock.Lock()
	defer fileLock.Unlock()

	// Open the file for reading
	file, err := os.Open(filePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			// If the file doesn't exist, attempt to download it from remote storage (e.g., aegisInterface)
			// TODO: implement download from aegis
			return nil, err
		}
		return nil, err // Return other file open errors
	}

	// Read the full file content
	fileContent, err := io.ReadAll(file)
	if err != nil {
		if err := file.Close(); err != nil {
			slog.Error("Error closing file after read failure", "file", filePath, "error", err.Error())
		}
		return nil, err
	}

	// Close the file
	if err := file.Close(); err != nil {
		return nil, err
	}

	// Decompress the content
	decompressedContent, err := fs.compressorInterface.Decompress(fileContent)
	if err != nil {
		return nil, err // Return if decompression failed
	}

	// Parse the binary content into individual byte slices
	fileParts, err := parseBinaryData(decompressedContent)
	if err != nil {
		return nil, err
	}

	return fileParts, nil
}

// GetAllFileContents reads the contents of all files in the specified folder,
// excluding any files listed in excludedFiles.
// Returns a map where each filename maps to a slice of binary segments ([]byte).
func (fs *filesystem) GetAllFileContents(folderPath string, excludedFiles ...string) (map[string][][]byte, error) {

	// Validate the folder path
	if folderPath == "" {
		return nil, errors.New("invalid folder path")
	}

	// Result container for all file contents
	allFileContents := make(map[string][][]byte)

	// Build a fast lookup set for excluded file names
	excluded := make(map[string]struct{}, len(excludedFiles))
	for _, file := range excludedFiles {
		excluded[file] = struct{}{}
	}

	// Read all entries in the folder
	files, err := os.ReadDir(folderPath)
	if err != nil {
		return nil, err
	}

	// Lock the folder to ensure safe access
	folderLock := fs.getFolderLock(folderPath)
	folderLock.Lock()
	defer folderLock.Unlock()

	// Iterate over each file in the folder
	for _, fileInfo := range files {

		// Skip excluded files
		if _, skip := excluded[fileInfo.Name()]; skip {
			continue
		}

		// Full path to the file
		filePath := filepath.Join(folderPath, fileInfo.Name())

		// Lock the file for reading
		fileLock := fs.getFolderLock(filePath)
		fileLock.Lock()

		// Open the file for reading
		file, err := os.Open(filePath)
		if err != nil {
			fileLock.Unlock()
			continue // Skip this file if it can't be opened
		}

		// Read file content
		fileContent, err := io.ReadAll(file)
		if err != nil {
			func() {
				if closeErr := file.Close(); closeErr != nil {
					slog.Error("Error closing file after read failure", "file", filePath, "error", closeErr.Error())
				}
			}()

			fileLock.Unlock()
			continue // Skip on read failure
		}

		// Close the file
		err = file.Close()
		if err != nil {
			fileLock.Unlock()
			continue
		}

		// Release the file lock
		fileLock.Unlock()

		// Decompress file content
		decompressedContent, err := fs.compressorInterface.Decompress(fileContent)
		if err != nil {
			continue // Skip on decompression failure
		}

		// Parse binary data segments
		fileParts, err := parseBinaryData(decompressedContent)
		if err != nil {
			continue // Skip on parse failure
		}

		// Store the parsed content under the filename
		allFileContents[fileInfo.Name()] = fileParts
	}

	return allFileContents, nil
}

// GetFileSize returns the size of the specified file in bytes.
func (fs *filesystem) GetFileSize(filePath string) (int64, error) {
	// Validate the file path
	if filePath == "" {
		return 0, errors.New("invalid file path")
	}

	// Acquire file-level lock
	fileLock := fs.getFolderLock(filePath)
	fileLock.Lock()
	defer fileLock.Unlock()

	// Retrieve file info
	fileInfo, err := os.Stat(filePath)
	if err != nil {
		return 0, err // Return error if file does not exist or can't be accessed
	}

	// Return the file size in bytes
	return fileInfo.Size(), nil
}

// parseBinaryData iterates over a decompressed binary stream and splits it into
// separate byte slices based on length-prefixed blocks.
// Each block is prefixed with a 4-byte little-endian length header.
func parseBinaryData(data []byte) ([][]byte, error) {
	var result [][]byte
	reader := bytes.NewReader(data)

	// Read through the binary stream
	for {
		// Read the next 4-byte length header (uint32)
		var length uint32
		err := binary.Read(reader, binary.LittleEndian, &length)
		if err != nil {
			if errors.Is(err, io.EOF) {
				break // End of file reached
			}
			return nil, err // Failed to read length
		}

		// Read the data block of the given length
		dataBlock := make([]byte, length)
		_, err = reader.Read(dataBlock)
		if err != nil {
			return nil, err // Failed to read block
		}

		// Append to the result set
		result = append(result, dataBlock)
	}
	return result, nil
}

// encodeBinaryLength encodes the length of a binary slice
// as a 4-byte little-endian value (used as a prefix).
func encodeBinaryLength(data []byte) []byte {
	length := uint32(len(data)) // Determine length of data
	buf := new(bytes.Buffer)
	// Write length in binary format
	if err := binary.Write(buf, binary.LittleEndian, length); err != nil {
		slog.Error("Failed to encode binary length, returning nil", "error", err.Error(), "data_length", len(data))
		return nil // Return nil if encoding fails
	}
	return buf.Bytes() // Return length bytes
}

// getFolderLock returns a mutex for the given folder path.
// If no lock exists yet, it creates and stores one.
func (fs *filesystem) getFolderLock(folder string) *sync.Mutex {
	actual, _ := fs.folderLocks.LoadOrStore(folder, &sync.Mutex{})
	return actual.(*sync.Mutex)
}

// deleteIfEmpty removes the given folder if it is empty.
// If the folder does not exist, it returns nil.
func (fs *filesystem) deleteIfEmpty(folderPath string) error {
	// Attempt to open the directory
	dir, err := os.Open(folderPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			// Nothing to delete if directory doesn't exist
			return nil
		}
		return err // Failed to open directory
	}
	defer func() {
		if closeErr := dir.Close(); closeErr != nil {
			slog.Error("Error closing directory after checking emptiness", "folder", folderPath, "error", closeErr.Error())
		}
	}()

	// Try reading one entry to check if it's empty
	files, readErr := dir.Readdirnames(1)
	if readErr != nil && !errors.Is(readErr, io.EOF) {
		return readErr // Error while reading directory
	}

	// If EOF is returned and no files were read, it's empty
	if len(files) == 0 && errors.Is(readErr, io.EOF) {
		return os.Remove(folderPath) // Safe to remove
	}

	// Directory is not empty
	return errors.New("directory not empty")
}

func (fs *filesystem) IsFolderExists(folderPath string) bool {

	folderLock := fs.getFolderLock(folderPath)
	folderLock.Lock()
	defer folderLock.Unlock()

	_, err := os.Stat(folderPath)
	if err != nil {
		if os.IsNotExist(err) {
			return false
		}
	}
	return true

}
