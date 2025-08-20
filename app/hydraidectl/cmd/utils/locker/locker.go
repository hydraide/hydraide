package locker

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

// Locker manages a file-based lock for a single HydrAIDE instance.
// This provides an inter-process locking mechanism to ensure only one lifecycle
// command can be executed for a given instance at a time, even across separate CLI processes.
type Locker interface {
	// Lock acquires a file lock for the instance.
	// It will block until the lock is available.
	Lock() error

	// Unlock releases the file lock.
	Unlock() error
}

// getLockDirFunc is a package-level variable that holds a function to get the lock directory.
// It is primarily used to allow for easy mocking in unit tests without
// exposing the internal `getLockDir` function.
var getLockDirFunc = getLockDir

// DeleteLockFile removes the lock file for a given instance name from the
// system-wide lock directory. It is a public function in the locker package,
// providing a clean way for other packages to perform cleanup without
// needing to know the low-level locking details.
func DeleteLockFile(instanceName string) error {
	dir, err := getLockDirFunc()
	if err != nil {
		return fmt.Errorf("failed to get system lock directory: %w", err)
	}

	path := filepath.Join(dir, instanceName+".lock")

	if err := os.Remove(path); err != nil {
		return fmt.Errorf("failed to delete lock file at '%s': %w", path, err)
	}

	return nil
}

// NewLocker creates a new file-based locker for a given instance name.
// It creates the lock file in a standard location and returns
// an error if it cannot be created or opened.
func NewLocker(instanceName string) (Locker, error) {
	switch runtime.GOOS {
	case "windows":
		return newWindowsLocker(instanceName)
	case "linux":
		return newPosixLocker(instanceName)
	default:
		return nil, fmt.Errorf("locker: unsupported platform")
	}
}
