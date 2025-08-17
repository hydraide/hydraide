package locker

import (
	"os"
	"path/filepath"
	"testing"
)

// Test suite for the deleteLockFile function.
func Test_deleteLockFile(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "hydraide-test-locks-")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	originalGetLockDirFunc := getLockDirFunc
	getLockDirFunc = func() (string, error) {
		return tempDir, nil
	}
	defer func() {
		getLockDirFunc = originalGetLockDirFunc
	}()

	t.Run("should delete an existing lock file successfully", func(t *testing.T) {
		instanceName := "test-instance-1"
		lockFilePath := filepath.Join(tempDir, instanceName+".lock")

		// Create a dummy lock file to be deleted.
		if _, err := os.Create(lockFilePath); err != nil {
			t.Fatalf("failed to create dummy file: %v", err)
		}

		// Call the function under test.
		err := DeleteLockFile(instanceName)
		if err != nil {
			t.Errorf("expected no error, but got: %v", err)
		}

		// Verify the file no longer exists.
		if _, err := os.Stat(lockFilePath); !os.IsNotExist(err) {
			t.Errorf("expected file to be deleted, but it still exists or another error occurred: %v", err)
		}
	})

	t.Run("should return error if lock file doesn't exist", func(t *testing.T) {
		instanceName := "non-existent-file"

		err := DeleteLockFile(instanceName)
		if err == nil {
			t.Errorf("expected error but did not get any.")
		}
	})

	t.Run("should return an error if it fails to delete due to permissions", func(t *testing.T) {
		instanceName := "test-instance-2"
		lockFilePath := filepath.Join(tempDir, instanceName+".lock")

		if _, err := os.Create(lockFilePath); err != nil {
			t.Fatalf("failed to create dummy file: %v", err)
		}

		if err := os.Chmod(tempDir, 0o444); err != nil {
			t.Fatalf("failed to change directory permissions: %v", err)
		}

		err := DeleteLockFile(instanceName)
		if err == nil {
			t.Errorf("expected an error due to permissions, but got nil")
		}

		os.Chmod(tempDir, 0o777)
	})
}
