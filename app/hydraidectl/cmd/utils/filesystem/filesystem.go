package filesystem

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
)

// FileSystem defines an interface for common file and directory operations.
type FileSystem interface {
	// CreateDir creates a directory at the specified path with the given permissions.
	// If the directory already exists, it returns nil.
	// Parameters:
	//   - ctx: Context for cancellation and logging
	//   - path: The directory path to create
	//   - perm: File mode permissions (e.g., 0755)
	// Returns:
	//   - error: Any error encountered during directory creation
	CreateDir(ctx context.Context, path string, perm os.FileMode) error

	// CreateFileOnly creates an empty file at the specified path.
	// If the file already exists, it returns an error.
	// Parameters:
	//   - ctx: Context for cancellation and logging
	//   - path: The file path to create
	//   - perm: File mode permissions (e.g., 0644)
	// Returns:
	//   - error: Any error encountered during file creation
	CreateFileOnly(ctx context.Context, path string, perm os.FileMode) error

	// WriteFile writes content to a file at the specified path, creating the file if it doesn't exist.
	// If the file exists, it overwrites the content.
	// Parameters:
	//   - ctx: Context for cancellation and logging
	//   - path: The file path to write to
	//   - content: The content to write to the file
	//   - perm: File mode permissions (e.g., 0644) for new files
	// Returns:
	//   - error: Any error encountered during file writing
	WriteFile(ctx context.Context, path string, content []byte, perm os.FileMode) error

	// CheckIfFileExists checks if a file exists at the specified path.
	// Parameters:
	//   - ctx: Context for cancellation and logging
	//   - path: The file path to check
	// Returns:
	//   - bool: True if the file exists, false otherwise
	//   - error: Any error encountered during the check
	CheckIfFileExists(ctx context.Context, path string) (bool, error)

	// CheckIfDirExists checks if a directory exists at the specified path.
	// Parameters:
	//   - ctx: Context for cancellation and logging
	//   - path: The directory path to check
	// Returns:
	//   - bool: True if the directory exists, false otherwise
	//   - error: Any error encountered during the check
	CheckIfDirExists(ctx context.Context, path string) (bool, error)

	// RemoveFile removes a file at the specified path.
	// If the file does not exist, it returns nil.
	// Parameters:
	//   - ctx: Context for cancellation and logging
	//   - path: The file path to remove
	// Returns:
	//   - error: Any error encountered during file removal
	RemoveFile(ctx context.Context, path string) error

	// RemoveDir removes a directory at the specified path.
	// If the directory does not exist, it returns nil.
	// Parameters:
	//   - ctx: Context for cancellation and logging
	//   - path: The directory path to remove
	// Returns:
	//   - error: Any error encountered during directory removal
	RemoveDir(ctx context.Context, path string) error

	// MoveFile moves a file from the source path to the destination path.
	// If the source file does not exist or the destination cannot be created, it returns an error.
	// Parameters:
	//   - ctx: Context for cancellation and logging
	//   - src: The source file path
	//   - dst: The destination file path
	//
	// Returns:
	//   - error: Any error encountered during the file move operation
	MoveFile(ctx context.Context, src, dst string) error
}

// fileSystemImpl implements the FileSystem interface.
type fileSystemImpl struct {
	logger *slog.Logger
}

// NewFileSystem creates a new FileSystem instance with the provided logger.
// If no logger is provided, a default logger is created.
func New() FileSystem {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	return &fileSystemImpl{logger: logger}
}

// CreateDir implements the CreateDir method of the FileSystem interface.
func (fs *fileSystemImpl) CreateDir(ctx context.Context, path string, perm os.FileMode) error {
	cleanPath := filepath.Clean(path)
	fs.logger.DebugContext(ctx, "Creating directory", "path", cleanPath, "perm", perm)

	if err := os.MkdirAll(cleanPath, perm); err != nil {
		fs.logger.ErrorContext(ctx, "Failed to create directory", "path", cleanPath, "error", err)
		return fmt.Errorf("failed to create directory %s: %w", cleanPath, err)
	}

	fs.logger.InfoContext(ctx, "Directory created successfully", "path", cleanPath)
	return nil
}

// CreateFileOnly implements the CreateFileOnly method of the FileSystem interface.
func (fs *fileSystemImpl) CreateFileOnly(ctx context.Context, path string, perm os.FileMode) error {
	cleanPath := filepath.Clean(path)
	fs.logger.DebugContext(ctx, "Creating empty file", "path", cleanPath, "perm", perm)

	// Check if file already exists
	if _, err := os.Stat(cleanPath); err == nil {
		fs.logger.ErrorContext(ctx, "File already exists", "path", cleanPath)
		return fmt.Errorf("file %s already exists", cleanPath)
	}

	// Create an empty file
	file, err := os.Create(cleanPath)
	if err != nil {
		fs.logger.ErrorContext(ctx, "Failed to create empty file", "path", cleanPath, "error", err)
		return fmt.Errorf("failed to create empty file %s: %w", cleanPath, err)
	}
	defer file.Close()

	// Set file permissions
	if err := file.Chmod(perm); err != nil {
		fs.logger.ErrorContext(ctx, "Failed to set file permissions", "path", cleanPath, "error", err)
		return fmt.Errorf("failed to set permissions for file %s: %w", cleanPath, err)
	}

	fs.logger.InfoContext(ctx, "Empty file created successfully", "path", cleanPath)
	return nil
}

// WriteFile implements the WriteFile method of the FileSystem interface.
func (fs *fileSystemImpl) WriteFile(ctx context.Context, path string, content []byte, perm os.FileMode) error {
	cleanPath := filepath.Clean(path)
	fs.logger.DebugContext(ctx, "Writing to file", "path", cleanPath, "perm", perm)

	// Write content to the file (creates file if it doesn't exist, overwrites if it does)
	if err := os.WriteFile(cleanPath, content, perm); err != nil {
		fs.logger.ErrorContext(ctx, "Failed to write to file", "path", cleanPath, "error", err)
		return fmt.Errorf("failed to write to file %s: %w", cleanPath, err)
	}

	fs.logger.InfoContext(ctx, "File written successfully", "path", cleanPath)
	return nil
}

// CheckIfFileExists implements the CheckIfFileExists method of the FileSystem interface.
func (fs *fileSystemImpl) CheckIfFileExists(ctx context.Context, path string) (bool, error) {
	cleanPath := filepath.Clean(path)
	fs.logger.DebugContext(ctx, "Checking if file exists", "path", cleanPath)

	info, err := os.Stat(cleanPath)
	if err != nil {
		if os.IsNotExist(err) {
			fs.logger.DebugContext(ctx, "File does not exist", "path", cleanPath)
			return false, nil
		}
		fs.logger.ErrorContext(ctx, "Failed to check file existence", "path", cleanPath, "error", err)
		return false, fmt.Errorf("failed to check file existence %s: %w", cleanPath, err)
	}

	if info.IsDir() {
		fs.logger.ErrorContext(ctx, "Path is a directory, not a file", "path", cleanPath)
		return false, fmt.Errorf("path %s is a directory, not a file", cleanPath)
	}

	fs.logger.InfoContext(ctx, "File exists", "path", cleanPath)
	return true, nil
}

// CheckIfDirExists implements the CheckIfDirExists method of the FileSystem interface.
func (fs *fileSystemImpl) CheckIfDirExists(ctx context.Context, path string) (bool, error) {
	cleanPath := filepath.Clean(path)
	fs.logger.DebugContext(ctx, "Checking if directory exists", "path", cleanPath)

	info, err := os.Stat(cleanPath)
	if err != nil {
		if os.IsNotExist(err) {
			fs.logger.DebugContext(ctx, "Directory does not exist", "path", cleanPath)
			return false, nil
		}
		fs.logger.ErrorContext(ctx, "Failed to check directory existence", "path", cleanPath, "error", err)
		return false, fmt.Errorf("failed to check directory existence %s: %w", cleanPath, err)
	}

	if !info.IsDir() {
		fs.logger.ErrorContext(ctx, "Path is a file, not a directory", "path", cleanPath)
		return false, fmt.Errorf("path %s is a file, not a directory", cleanPath)
	}

	fs.logger.InfoContext(ctx, "Directory exists", "path", cleanPath)
	return true, nil
}

// RemoveFile implements the RemoveFile method of the FileSystem interface.
func (fs *fileSystemImpl) RemoveFile(ctx context.Context, path string) error {
	cleanPath := filepath.Clean(path)
	fs.logger.DebugContext(ctx, "Removing file", "path", cleanPath)

	if err := os.Remove(cleanPath); err != nil {
		if os.IsNotExist(err) {
			fs.logger.DebugContext(ctx, "File does not exist, nothing to remove", "path", cleanPath)
			return nil
		}
		fs.logger.ErrorContext(ctx, "Failed to remove file", "path", cleanPath, "error", err)
		return fmt.Errorf("failed to remove file %s: %w", cleanPath, err)
	}

	fs.logger.InfoContext(ctx, "File removed successfully", "path", cleanPath)
	return nil
}

// RemoveDir implements the RemoveDir method of the FileSystem interface.
func (fs *fileSystemImpl) RemoveDir(ctx context.Context, path string) error {
	cleanPath := filepath.Clean(path)
	fs.logger.DebugContext(ctx, "Removing directory", "path", cleanPath)

	if err := os.RemoveAll(cleanPath); err != nil {
		if os.IsNotExist(err) {
			fs.logger.DebugContext(ctx, "Directory does not exist, nothing to remove", "path", cleanPath)
			return nil
		}
		fs.logger.ErrorContext(ctx, "Failed to remove directory", "path", cleanPath, "error", err)
		return fmt.Errorf("failed to remove directory %s: %w", cleanPath, err)
	}

	fs.logger.InfoContext(ctx, "Directory removed successfully", "path", cleanPath)
	return nil
}

// RemoveDir implements the RemoveDir method of the FileSystem interface.
func (fs *fileSystemImpl) MoveFile(ctx context.Context, src, dst string) error {
	cleanSrc := filepath.Clean(src)
	cleanDst := filepath.Clean(dst)
	fs.logger.DebugContext(ctx, "Moving file", "from", cleanSrc, "to", cleanDst)
	if err := os.Rename(cleanSrc, cleanDst); err != nil {
		fs.logger.ErrorContext(ctx, "Failed to move file", "error", err)
		return fmt.Errorf("failed to move file from %s to %s: %w", cleanSrc, cleanDst, err)
	}
	fs.logger.InfoContext(ctx, "File moved successfully")
	return nil
}
