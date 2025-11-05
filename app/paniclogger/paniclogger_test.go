package paniclogger

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestInit(t *testing.T) {
	// Set up temporary directory for testing
	tmpDir := t.TempDir()
	os.Setenv("HYDRAIDE_ROOT_PATH", tmpDir)
	defer os.Unsetenv("HYDRAIDE_ROOT_PATH")

	// Reset the init state for testing
	Reset()

	err := Init()
	if err != nil {
		t.Fatalf("Init() failed: %v", err)
	}

	// Check if logs directory was created
	logsDir := filepath.Join(tmpDir, "logs")
	if _, err := os.Stat(logsDir); os.IsNotExist(err) {
		t.Errorf("logs directory was not created")
	}

	// Check if log file was created
	logPath := filepath.Join(logsDir, panicLogFile)
	if _, err := os.Stat(logPath); os.IsNotExist(err) {
		t.Errorf("panic.log file was not created")
	}

	// Cleanup
	Close()
}

func TestLogPanic(t *testing.T) {
	// Set up temporary directory for testing
	tmpDir := t.TempDir()
	os.Setenv("HYDRAIDE_ROOT_PATH", tmpDir)
	defer os.Unsetenv("HYDRAIDE_ROOT_PATH")

	// Reset the init state for testing
	Reset()

	err := Init()
	if err != nil {
		t.Fatalf("Init() failed: %v", err)
	}
	defer Close()

	// Log a test panic
	testContext := "test context"
	testError := "test panic error"
	testStack := "test stack trace"

	LogPanic(testContext, testError, testStack)

	// Give it a moment to write
	time.Sleep(100 * time.Millisecond)

	// Read the log file
	logPath := filepath.Join(tmpDir, "logs", panicLogFile)
	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}

	logContent := string(content)

	// Verify content
	if !strings.Contains(logContent, "PANIC DETECTED") {
		t.Errorf("Log file does not contain 'PANIC DETECTED'")
	}
	if !strings.Contains(logContent, testContext) {
		t.Errorf("Log file does not contain context: %s", testContext)
	}
	if !strings.Contains(logContent, testError) {
		t.Errorf("Log file does not contain error: %s", testError)
	}
	if !strings.Contains(logContent, testStack) {
		t.Errorf("Log file does not contain stack trace: %s", testStack)
	}
}

func TestLogPanicWithoutInit(t *testing.T) {
	// Reset the init state for testing
	Reset()

	// This should not panic, but write to stderr instead
	LogPanic("test", "error", "stack")
	// If we get here without panic, the test passes
}

func TestClose(t *testing.T) {
	// Set up temporary directory for testing
	tmpDir := t.TempDir()
	os.Setenv("HYDRAIDE_ROOT_PATH", tmpDir)
	defer os.Unsetenv("HYDRAIDE_ROOT_PATH")

	// Reset the init state for testing
	Reset()

	err := Init()
	if err != nil {
		t.Fatalf("Init() failed: %v", err)
	}

	err = Close()
	if err != nil {
		t.Errorf("Close() failed: %v", err)
	}
}

func TestConcurrentLogPanic(t *testing.T) {
	// Set up temporary directory for testing
	tmpDir := t.TempDir()
	os.Setenv("HYDRAIDE_ROOT_PATH", tmpDir)
	defer os.Unsetenv("HYDRAIDE_ROOT_PATH")

	// Reset the init state for testing
	Reset()

	err := Init()
	if err != nil {
		t.Fatalf("Init() failed: %v", err)
	}
	defer Close()

	// Test concurrent writes
	done := make(chan bool)
	numGoroutines := 10

	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			LogPanic("concurrent test", "test error", "stack trace")
			done <- true
		}(i)
	}

	// Wait for all goroutines to complete
	for i := 0; i < numGoroutines; i++ {
		<-done
	}

	// Give it a moment to write
	time.Sleep(100 * time.Millisecond)

	// Verify log file exists and has content
	logPath := filepath.Join(tmpDir, "logs", panicLogFile)
	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}

	// Count occurrences of "PANIC DETECTED"
	count := strings.Count(string(content), "PANIC DETECTED")
	if count != numGoroutines {
		t.Errorf("Expected %d panic entries, got %d", numGoroutines, count)
	}
}
