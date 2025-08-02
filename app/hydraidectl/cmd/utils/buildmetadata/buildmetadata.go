package buildmeta

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/hydraide/hydraide/app/hydraidectl/cmd/utils/filesystem"
)

// TEMP_FILENAME defines the name of the temporary directory for build metadata.
const (
	TEMP_FILENAME = "hydraide-cache"
	META_DATA     = "metadata.json"
)

// BuildMetadataStore defines the interface for metadata operations.
type BuildMetadataStore interface {
	// Get retrieves the value associated with the given key.
	// Returns an error if the key does not exist.
	Get(key string) (string, error)
	// Update sets or updates the value for the given key and persists it to the file.
	// Returns an error if the save operation fails.
	Update(key, value string) error
	// Delete removes the key-value pair for the given key and persists the change.
	// Returns an error if the save operation fails.
	Delete(key string) error
}

// BuildMetadata manages build metadata stored in a JSON file.
type BuildMetadata struct {
	cacheFile string                // Path to the metadata JSON file
	logger    *slog.Logger          // Logger for recording operations and errors
	fs        filesystem.FileSystem // FileSystem interface for file operations
}

// New creates a new instance of BuildMetadata.
// It sets up the cache file path in the system's temporary directory,
// creates the directory and file if they don't exist.
func New() (BuildMetadataStore, error) {
	// Initialize the logger with default text handler
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	// Initialize the FileSystem
	fs := filesystem.New()

	// Define the cache directory and file path
	tempDir := filepath.Join(os.TempDir(), TEMP_FILENAME)
	buildCacheFile := filepath.Join(tempDir, META_DATA)

	ctx := context.Background()

	// Create the cache directory if it doesn't exist
	if err := fs.CreateDir(ctx, tempDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create cache directory: %w", err)
	}

	// Create file if it doesn't exist
	exists, err := fs.CheckIfFileExists(ctx, buildCacheFile)
	if err != nil {
		return nil, fmt.Errorf("failed to check metadata file existence: %w", err)
	}
	if !exists {
		if err := fs.CreateFileOnly(ctx, buildCacheFile, 0644); err != nil {
			return nil, fmt.Errorf("failed to create metadata file: %w", err)
		}
	}

	// Create the BuildMetadata instance
	bm := &BuildMetadata{
		cacheFile: buildCacheFile,
		logger:    logger,
		fs:        fs,
	}

	return bm, nil
}

// load reads metadata from the JSON file.
// Returns a map of the metadata or an empty map if the file is empty or doesn't exist.
func (bm *BuildMetadata) load() (map[string]string, error) {
	ctx := context.Background()

	// Check if the file exists
	exists, err := bm.fs.CheckIfFileExists(ctx, bm.cacheFile)
	if err != nil {
		return nil, fmt.Errorf("failed to check metadata file existence: %w", err)
	}
	if !exists {
		bm.logger.Info("Metadata file does not exist, returning empty data", "file", bm.cacheFile)
		return make(map[string]string), nil
	}

	// Read the file contents
	data, err := os.ReadFile(bm.cacheFile) // Note: FileSystem interface doesn't have ReadFile
	if err != nil {
		return nil, fmt.Errorf("failed to read metadata file: %w", err)
	}

	// Handle empty file
	if len(data) == 0 {
		bm.logger.Info("Metadata file is empty, returning empty data", "file", bm.cacheFile)
		return make(map[string]string), nil
	}

	// Unmarshal JSON into a map
	var metadata map[string]string
	if err := json.Unmarshal(data, &metadata); err != nil {
		return nil, fmt.Errorf("failed to unmarshal metadata: %w, data: %v", err, string(data))
	}

	bm.logger.Info("Metadata loaded successfully", "file", bm.cacheFile)
	return metadata, nil
}

// Get retrieves the value associated with the given key by reading the JSON file.
// Returns the value and nil error if found, or an empty string and an error if not.
func (bm *BuildMetadata) Get(key string) (string, error) {
	metadata, err := bm.load()
	if err != nil {
		return "", err
	}

	value, exists := metadata[key]
	if !exists {
		bm.logger.Warn("Key not found", "key", key)
		return "", fmt.Errorf("key not found: %s", key)
	}

	bm.logger.Info("Retrieved value", "key", key)
	return value, nil
}

// Update sets or updates the value for the given key and saves it to the file.
// Returns an error if reading or writing fails.
func (bm *BuildMetadata) Update(key, value string) error {
	ctx := context.Background()

	// Load current metadata
	metadata, err := bm.load()
	if err != nil {
		return err
	}

	// Update the key-value pair
	metadata[key] = value

	// Marshal the updated metadata to JSON
	data, err := json.Marshal(metadata)
	if err != nil {
		bm.logger.Error("Failed to marshal metadata", "error", err)
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}

	// Write the JSON data to the file
	if err := bm.fs.WriteFile(ctx, bm.cacheFile, data, 0644); err != nil {
		bm.logger.Error("Failed to write metadata file", "error", err)
		return fmt.Errorf("failed to write metadata file: %w", err)
	}

	bm.logger.Info("Metadata updated and saved successfully", "key", key)
	return nil
}

// Delete removes the key-value pair for the given key and saves the updated metadata.
// Returns an error if reading or writing fails; does not error if the key doesn't exist.
func (bm *BuildMetadata) Delete(key string) error {
	ctx := context.Background()

	// Load current metadata
	metadata, err := bm.load()
	if err != nil {
		return err
	}

	// Remove the key (no-op if key doesn't exist)
	delete(metadata, key)

	// Marshal the updated metadata to JSON
	data, err := json.Marshal(metadata)
	if err != nil {
		bm.logger.Error("Failed to marshal metadata", "error", err)
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}

	// Write the JSON data to the file
	if err := bm.fs.WriteFile(ctx, bm.cacheFile, data, 0644); err != nil {
		bm.logger.Error("Failed to write metadata file", "error", err)
		return fmt.Errorf("failed to write metadata file: %w", err)
	}

	bm.logger.Info("Metadata deleted and saved successfully", "key", key)
	return nil
}
