//go:build windows
// +build windows

package locker

import (
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/sys/windows"
)

// windowsLocker uses Win32 LockFileEx/UnlockFileEx on a small file.
type windowsLocker struct {
	handle windows.Handle
}

func newWindowsLocker(instanceName string) (Locker, error) {
	dir, err := getLockDir()
	if err != nil {
		return nil, err
	}
	path := filepath.Join(dir, instanceName+".lock")

	// OPEN_ALWAYS, GENERIC_READ|GENERIC_WRITE, share read/write
	h, err := windows.CreateFile(
		windows.StringToUTF16Ptr(path),
		windows.GENERIC_READ|windows.GENERIC_WRITE,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE,
		nil,
		windows.OPEN_ALWAYS,
		windows.FILE_ATTRIBUTE_NORMAL,
		0,
	)
	if err != nil {
		return nil, fmt.Errorf("windowsLocker: CreateFile: %w", err)
	}
	return &windowsLocker{handle: h}, nil
}

func (l *windowsLocker) Lock() error {
	ol := new(windows.Overlapped)
	// LOCKFILE_EXCLUSIVE_LOCK = 2
	return windows.LockFileEx(
		l.handle,
		windows.LOCKFILE_EXCLUSIVE_LOCK,
		0,
		1, 0, // lock 1 byte
		ol,
	)
}

func (l *windowsLocker) Unlock() error {
	ol := new(windows.Overlapped)
	if err := windows.UnlockFileEx(l.handle, 0, 1, 0, ol); err != nil {
		return fmt.Errorf("windowsLocker: UnlockFileEx: %w", err)
	}
	return windows.CloseHandle(l.handle)
}

// getLockDirectory returns the path to the directory where lock files are stored.
// It creates the directory if it does not exist.
func getLockDir() (string, error) {
	programData := os.Getenv("ProgramData")
	if programData == "" {
		return "", fmt.Errorf("%%ProgramData%% environment variable not set")
	}
	dir := filepath.Join(programData, "HydrAIDE", "locks")
	if err := os.MkdirAll(dir, 0o777); err != nil {
		return "", fmt.Errorf("failed to create system lock directory '%s': %w", dir, err)
	}
	return dir, nil
}

// Stub function for linux: Used when compiled/compiling in windows
func newPosixLocker(_ string) (Locker, error) {
	return nil, fmt.Errorf("not compiled in linux")
}
