// Package paniclogger provides panic logging functionality to a local file.
// This package ensures that panic events are always logged to panic.log,
// regardless of other logging configurations.
package paniclogger

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const (
	panicLogFile = "panic.log"
	maxFileSize  = 50 * 1024 * 1024 // 50MB max size for panic log
)

var (
	logFile  *os.File
	fileLock sync.Mutex
	rootPath string
	initOnce sync.Once
	initErr  error
)

// Init initializes the panic logger. Should be called at application startup.
// The panic log will be stored in HYDRAIDE_ROOT_PATH/logs/panic.log
func Init() error {
	initOnce.Do(func() {
		rootPath = os.Getenv("HYDRAIDE_ROOT_PATH")
		if rootPath == "" {
			rootPath = "/hydraide"
		}

		logDir := filepath.Join(rootPath, "logs")
		if err := os.MkdirAll(logDir, 0755); err != nil {
			initErr = fmt.Errorf("failed to create logs directory: %w", err)
			return
		}

		logPath := filepath.Join(logDir, panicLogFile)
		var err error
		logFile, err = os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			initErr = fmt.Errorf("failed to open panic log file: %w", err)
			return
		}
	})
	return initErr
}

// LogPanic logs a panic event to the panic.log file
func LogPanic(context string, panicError any, stackTrace string) {
	fileLock.Lock()
	defer fileLock.Unlock()

	// If Init hasn't been called or failed, try to write to stderr as fallback
	if logFile == nil {
		_, _ = fmt.Fprintf(os.Stderr, "[PANIC] Failed to write to panic.log - logging to stderr instead\n")
		_, _ = fmt.Fprintf(os.Stderr, "[PANIC] Context: %s\n", context)
		_, _ = fmt.Fprintf(os.Stderr, "[PANIC] Error: %v\n", panicError)
		_, _ = fmt.Fprintf(os.Stderr, "[PANIC] Stack trace:\n%s\n", stackTrace)
		return
	}

	// Check file size and rotate if needed
	if err := rotateIfNeeded(); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "Failed to rotate panic log: %v\n", err)
	}

	// Format the panic log entry
	timestamp := time.Now().Format("2006-01-02T15:04:05.000Z07:00")
	entry := fmt.Sprintf(
		"\n================================================================================\n"+
			"PANIC DETECTED\n"+
			"================================================================================\n"+
			"Timestamp: %s\n"+
			"Context:   %s\n"+
			"Error:     %v\n"+
			"\nStack Trace:\n%s\n"+
			"================================================================================\n\n",
		timestamp, context, panicError, stackTrace,
	)

	// Write to file
	if _, err := logFile.WriteString(entry); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "Failed to write panic log: %v\n", err)
	}

	// Ensure it's written to disk immediately
	_ = logFile.Sync()
}

// rotateIfNeeded checks if the log file exceeds maxFileSize and rotates it
func rotateIfNeeded() error {
	if logFile == nil {
		return nil
	}

	stat, err := logFile.Stat()
	if err != nil {
		return err
	}

	if stat.Size() < maxFileSize {
		return nil
	}

	// Close current file
	_ = logFile.Close()

	// Rotate the file
	logDir := filepath.Join(rootPath, "logs")
	logPath := filepath.Join(logDir, panicLogFile)
	backupPath := filepath.Join(logDir, panicLogFile+".old")

	// Remove old backup if exists
	_ = os.Remove(backupPath)

	// Rename current to backup
	if err := os.Rename(logPath, backupPath); err != nil {
		return err
	}

	// Open new file
	logFile, err = os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	return err
}

// Close closes the panic log file. Should be called during application shutdown.
func Close() error {
	fileLock.Lock()
	defer fileLock.Unlock()

	if logFile != nil {
		err := logFile.Close()
		logFile = nil
		return err
	}
	return nil
}

// Reset resets the panic logger state. FOR TESTING ONLY.
func Reset() {
	fileLock.Lock()
	defer fileLock.Unlock()

	if logFile != nil {
		_ = logFile.Close()
	}
	logFile = nil
	initOnce = sync.Once{}
	initErr = nil
}
