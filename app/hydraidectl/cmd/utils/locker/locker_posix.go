//go:build !windows
// +build !windows

package locker

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"syscall"
)

type posixLocker struct {
	file *os.File
}

func newPosixLocker(instanceName string) (Locker, error) {
	dir, err := getLockDir()
	if err != nil {
		return nil, err
	}
	path := filepath.Join(dir, instanceName+".lock")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("posixLocker: open lock file: %w", err)
	}
	return &posixLocker{file: f}, nil
}

func (l *posixLocker) Lock() error {
	if err := syscall.Flock(int(l.file.Fd()), syscall.LOCK_EX); err != nil {
		return fmt.Errorf("failed to acquire file lock: %w", err)
	}
	return nil
}

func (l *posixLocker) Unlock() error {
	if err := syscall.Flock(int(l.file.Fd()), syscall.LOCK_UN); err != nil {
		return fmt.Errorf("failed to release file lock: %w", err)
	}
	l.file.Close()
	return nil
}

// getLockDir returns the path to the directory where lock files are stored.
// It creates the directory if it does not exist.
func getLockDir() (string, error) {
	var dir string
	switch runtime.GOOS {
	case "darwin":
		// On macOS, use /tmp/hydraide for lock files
		dir = "/tmp/hydraide"
	case "linux":
		// On Linux, use /var/lock/hydraide
		dir = "/var/lock/hydraide"
	default:
		// Fallback for other POSIX systems
		dir = "/tmp/hydraide"
	}

	if err := os.MkdirAll(dir, 0o777); err != nil {
		return "", fmt.Errorf("failed to create system lock directory '%s': %w", dir, err)
	}

	return dir, nil
}

// Stub function for non-Windows: Used when compiled on POSIX systems
func newWindowsLocker(_ string) (Locker, error) {
	return nil, fmt.Errorf("Windows locker not available on POSIX systems")
}
