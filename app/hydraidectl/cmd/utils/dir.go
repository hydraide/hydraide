package utils

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func CreateFolders(basePath string, folders []string) error {

	// Create each folder
	for _, folder := range folders {
		fullPath := filepath.Join(basePath, folder)

		// Create the directory with 0755 permissions (rwxr-xr-x)
		err := os.MkdirAll(fullPath, 0755)
		if err != nil {
			return fmt.Errorf("failed to create directory %s: %v", fullPath, err)
		}

		fmt.Printf("Created directory: %s\n", fullPath)
	}

	return nil
}

// CheckDirectoryExists verifies if all required subdirectories exist in the base path
// Returns:
//   - string: detailed log message about directory status
//   - error: nil if all directories exist, or error describing missing directories
func CheckDirectoryExists(basePath string, folders []string) (string, error) {
	var existingDirs []string
	var missingDirs []string
	var logBuilder strings.Builder

	// Check each directory
	for _, folder := range folders {
		fullPath := filepath.Join(basePath, folder)
		_, err := os.Stat(fullPath)

		if os.IsNotExist(err) {
			missingDirs = append(missingDirs, fullPath)
			logBuilder.WriteString(fmt.Sprintf("[MISSING] %s\n", fullPath))
		} else if err != nil {
			// Handle other potential errors (e.g., permission issues)
			missingDirs = append(missingDirs, fullPath)
			logBuilder.WriteString(fmt.Sprintf("[ERROR ACCESSING] %s: %v\n", fullPath, err))
		} else {
			existingDirs = append(existingDirs, fullPath)
			logBuilder.WriteString(fmt.Sprintf("[EXISTS] %s\n", fullPath))
		}
	}

	// Prepare final output
	logMsg := logBuilder.String()

	if len(missingDirs) > 0 {
		return logMsg, fmt.Errorf("missing required directories: %v", missingDirs)
	}

	return logMsg, nil
}

// MoveFile moves a file from source to destination with proper checks
// Returns nil on success, or error if any operation fails
func MoveFile(src, dst string) error {
	// Check if source file exists
	srcInfo, err := os.Stat(src)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("source file does not exist: %s error : %v", src, err)
		}
		return fmt.Errorf("error checking source file: %w  ", err)
	}

	// Verify source is a regular file (not directory)
	if !srcInfo.Mode().IsRegular() {
		return errors.New("source is not a regular file")
	}

	// Check if destination directory exists
	dstDir := filepath.Dir(dst)
	if _, err := os.Stat(dstDir); os.IsNotExist(err) {
		return fmt.Errorf("destination directory does not exist: %s   error : %v", dstDir, err)
	}

	// Check if destination file already exists
	if _, err := os.Stat(dst); err == nil {
		return fmt.Errorf("destination file already exists: %s   error : %v", dst, err)
	}

	// First try atomic rename (works within same filesystem)
	err = os.Rename(src, dst)
	if err != nil {
		return err
	}

	return nil
}
