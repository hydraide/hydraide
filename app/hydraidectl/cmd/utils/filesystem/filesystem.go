package filesystem

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
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

	// RemoveDirIncremental recursively removes a directory, providing progress feedback.
	// It is resumable: if interrupted, running it again will delete remaining files.
	// It is cancellable via the context.
	RemoveDirIncremental(ctx context.Context, path string, progressCb func(path string)) error

	// ReadFile reads the content of a file at the specified path.
	// Parameters:
	//   - ctx: Context for cancellation and logging
	//   - path: The file path to read from
	// Returns:
	//   - []byte: The content of the file
	//   - error: Any error encountered during file reading
	ReadFile(ctx context.Context, path string) ([]byte, error)

	// than CheckIf...Exists as it provides full file info (size, permissions, etc.)
	// and is the idiomatic way to check for existence in Go via `os.IsNotExist(err)`.
	Stat(ctx context.Context, path string) (os.FileInfo, error)
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
	defer func() {
		if err := file.Close(); err != nil {
			fs.logger.ErrorContext(ctx, "Failed to close file after creation", "path", cleanPath, "error", err)
		}
	}()

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

// MoveFile moves a file from the given source path to the destination path,
// ensuring the target directory exists before attempting the move.
//
// Behavior:
//  1. Cleans and normalizes both source and destination paths.
//  2. Ensures the destination directory exists by creating it (with 0755 permissions) if missing.
//  3. Attempts a fast move via os.Rename() when possible (same filesystem).
//  4. If the move fails due to a cross-device error, falls back to a copy-then-delete strategy.
//  5. Logs detailed debug, info, and error messages for each step and outcome.
//  6. Returns a wrapped error if the operation fails at any stage.
func (fs *fileSystemImpl) MoveFile(ctx context.Context, src, dst string) error {
	cleanSrc := filepath.Clean(src)
	cleanDst := filepath.Clean(dst)
	fs.logger.DebugContext(ctx, "Moving file", "from", cleanSrc, "to", cleanDst)

	// Ensure target dir exists before any move attempt
	if err := os.MkdirAll(filepath.Dir(cleanDst), 0o755); err != nil {
		fs.logger.ErrorContext(ctx, "Failed to create target directory", "error", err)
		return fmt.Errorf("failed to create target directory %s: %w", filepath.Dir(cleanDst), err)
	}

	// Fast path
	if err := os.Rename(cleanSrc, cleanDst); err == nil {
		fs.logger.InfoContext(ctx, "File moved successfully (rename)")
		return nil
	} else if isCrossDevice(cleanSrc, cleanDst, err) {
		fs.logger.DebugContext(ctx, "Cross-device move detected, falling back to copy+delete")
		if err := fs.copyThenReplace(ctx, cleanSrc, cleanDst); err != nil {
			fs.logger.ErrorContext(ctx, "Copy+delete fallback failed", "error", err)
			return fmt.Errorf("failed to move file from %s to %s: %w", cleanSrc, cleanDst, err)
		}
		fs.logger.InfoContext(ctx, "File moved successfully (copy+delete)")
		return nil
	} else {
		fs.logger.ErrorContext(ctx, "Failed to move file", "error", err)
		return fmt.Errorf("failed to move file from %s to %s: %w", cleanSrc, cleanDst, err)
	}
}

// isCrossDevice detects cross-device/drive moves.
// On Unix it checks for EXDEV; on Windows it compares drive letters (C:, D:, G:...).
func isCrossDevice(src, dst string, renameErr error) bool {
	// Unix-like: EXDEV
	// (We avoid importing syscall/windows-specific constants; EXDEV is portable enough for Unix.)
	var exdev error
	// syscall.EXDEV is available on Unix builds; to keep this file portable without build tags,
	// we use a heuristic: if volumes differ on Windows, treat as cross-device; on others return false unless we can match EXDEV.
	// If you want exact EXDEV matching on Unix, gate a tiny build-tagged helper, but this is usually enough.

	if runtime.GOOS == "windows" {
		// Different drive letters => cross-device
		return !strings.EqualFold(filepath.VolumeName(src), filepath.VolumeName(dst))
	}

	// Best-effort: many Go runtimes wrap EXDEV; try string contains as a fallback hint.
	// (Optional: import "syscall" and check errors.Is(err, syscall.EXDEV) behind !windows build tag.)
	_ = exdev // placeholder to show intent
	// Loose heuristic for non-Windows: if rename failed and paths are on different mount roots,
	// callers will still hit fallback only when needed; otherwise the first rename would have succeeded.
	return strings.Contains(strings.ToLower(renameErr.Error()), "cross-device") ||
		strings.Contains(strings.ToLower(renameErr.Error()), "exdev")
}

// copyThenReplace copies src -> dst atomically (temp file + rename), then removes src.
func (fs *fileSystemImpl) copyThenReplace(ctx context.Context, src, dst string) error {
	// Ensure target dir exists
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return fmt.Errorf("create target dir: %w", err)
	}

	// If destination exists, remove it to mimic "replace" semantics across platforms
	if _, err := os.Lstat(dst); err == nil {
		if err := os.Remove(dst); err != nil {
			return fmt.Errorf("remove existing destination: %w", err)
		}
	}

	// Open source
	sf, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open src: %w", err)
	}
	defer func() {
		if err := sf.Close(); err != nil {
			fs.logger.ErrorContext(ctx, "Failed to close source file", "path", src, "error", err)
		}
	}()

	// Stat source for mode
	st, err := sf.Stat()
	if err != nil {
		return fmt.Errorf("stat src: %w", err)
	}

	// Temp file in destination dir for atomic rename
	tmp := dst + ".tmp-copy"
	df, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, st.Mode())
	if err != nil {
		return fmt.Errorf("create temp dst: %w", err)
	}

	// Copy
	if _, err := io.Copy(df, sf); err != nil {
		if err := df.Close(); err != nil {
			fs.logger.ErrorContext(ctx, "Failed to close destination file", "path", tmp, "error", err)
			// Attempt to remove temp file even if close failed
			_ = os.Remove(tmp)
			return fmt.Errorf("close dst after copy error: %w", err)
		}
		if err := os.Remove(tmp); err != nil {
			fs.logger.ErrorContext(ctx, "Failed to remove temp file after copy error", "path", tmp, "error", err)
			// Log but do not return here; we want to return the original copy error
			return fmt.Errorf("remove temp file after copy error: %w", err)
		}
		return fmt.Errorf("copy data: %w", err)
	}

	// Flush to disk
	if err := df.Sync(); err != nil {
		_ = df.Close()
		_ = os.Remove(tmp)
		return fmt.Errorf("sync dst: %w", err)
	}
	if err := df.Close(); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("close dst: %w", err)
	}

	// Atomic rename temp -> final
	if err := os.Rename(tmp, dst); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("rename temp to final: %w", err)
	}

	// Remove source (best effort)
	if err := os.Remove(src); err != nil {
		return fmt.Errorf("remove src after copy: %w", err)
	}
	return nil
}

// RemoveDirIncremental implements the RemoveDirIncremental method of the FileSystem interface.
// This method recursively removes a directory, providing progress feedback and allowing for cancellation.
// It is resumable: if interrupted, running it again will delete remaining files.
// It is cancellable via the context.
// It uses a top-down approach to ensure that files are deleted before their parent directories.
func (fs *fileSystemImpl) RemoveDirIncremental(ctx context.Context, path string, progressCb func(path string)) error {
	cleanPath := filepath.Clean(path)

	// If the path doesn't exist, we're already done.
	if _, err := os.Stat(cleanPath); os.IsNotExist(err) {
		fs.logger.InfoContext(ctx, "Directory does not exist, nothing to remove", "path", cleanPath)
		return nil
	}

	fs.logger.DebugContext(ctx, "Starting incremental directory removal", "path", cleanPath)

	err := filepath.Walk(cleanPath, func(currentPath string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Check for cancellation signal
		select {
		case <-ctx.Done():
			fs.logger.InfoContext(ctx, "Directory removal cancelled by user", "path", cleanPath)
			return ctx.Err() // This will stop the Walk
		default:
			// Continue
		}

		// Report progress *before* deleting
		if progressCb != nil {
			progressCb(currentPath)
		}

		// Delete the file or empty directory
		if err := os.Remove(currentPath); err != nil {
			// If we get an error, it might be because the directory isn't empty yet,
			// which is normal for the top-down Walk. We'll get it on the way back up.
			// We only return a real error if something unexpected happens.
			if !os.IsExist(err) {
				// Return the error to stop the walk if it's not a simple "doesn't exist" error
				fs.logger.ErrorContext(ctx, "Failed to remove item", "path", currentPath, "error", err)
				return err
			}
		}
		return nil
	})

	// After the walk, the root directory might still exist if it wasn't empty initially.
	// One final remove call handles the root directory itself.
	if err == nil {
		if err := os.Remove(cleanPath); err != nil {
			// If it still fails, it means something inside couldn't be deleted.
			// The walk would have already returned an error in that case.
			// This is just a final check.
			if !os.IsNotExist(err) {
				fs.logger.ErrorContext(ctx, "Failed to remove final directory root", "path", cleanPath, "error", err)
				return err
			}
		}
	} else if err == context.Canceled || err == context.DeadlineExceeded {
		// Don't log cancellation as a failure.
		return err
	}

	fs.logger.InfoContext(ctx, "Directory removal process completed", "path", cleanPath)
	return err
}

// Add the implementation for ReadFile to fileSystemImpl
func (fs *fileSystemImpl) ReadFile(ctx context.Context, path string) ([]byte, error) {
	cleanPath := filepath.Clean(path)
	fs.logger.DebugContext(ctx, "Reading file", "path", cleanPath)

	fmt.Println("🔍 Reading file:", cleanPath)
	content, err := os.ReadFile(cleanPath)
	if err != nil {
		fs.logger.ErrorContext(ctx, "Failed to read file", "path", cleanPath, "error", err)
		return nil, fmt.Errorf("failed to read file %s: %w", cleanPath, err)
	}

	fs.logger.InfoContext(ctx, "File read successfully", "path", cleanPath)
	return content, nil
}

// Stat implements the FileSystem interface.
func (fs *fileSystemImpl) Stat(ctx context.Context, path string) (os.FileInfo, error) {
	cleanPath := filepath.Clean(path)
	fs.logger.DebugContext(ctx, "Statting path", "path", cleanPath)
	return os.Stat(cleanPath)
}
