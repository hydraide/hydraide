package locker

import (
	"fmt"
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

// NewLocker creates a new file-based locker for a given instance name.
// It creates the lock file in a standard location (~/.hydraide/locks) and returns
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
