package v2

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFileWriter_CreateNew(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "test.hyd")

	writer, err := NewFileWriter(filePath, DefaultMaxBlockSize)
	if err != nil {
		t.Fatalf("failed to create writer: %v", err)
	}
	defer writer.Close()

	// Check file was created
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		t.Error("file was not created")
	}

	// Check initial stats
	blocks, entries := writer.GetStats()
	if blocks != 0 {
		t.Errorf("expected 0 blocks, got %d", blocks)
	}
	if entries != 0 {
		t.Errorf("expected 0 entries, got %d", entries)
	}
}

func TestFileWriter_WriteAndFlush(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "test.hyd")

	writer, err := NewFileWriter(filePath, DefaultMaxBlockSize)
	if err != nil {
		t.Fatalf("failed to create writer: %v", err)
	}

	// Write some entries
	entries := []Entry{
		{Operation: OpInsert, Key: "key1", Data: []byte("data1")},
		{Operation: OpInsert, Key: "key2", Data: []byte("data2")},
		{Operation: OpUpdate, Key: "key1", Data: []byte("data1-updated")},
	}

	for _, e := range entries {
		if err := writer.WriteEntry(e); err != nil {
			t.Fatalf("failed to write entry: %v", err)
		}
	}

	// Flush
	if err := writer.Flush(); err != nil {
		t.Fatalf("failed to flush: %v", err)
	}

	// Check stats
	blocks, entryCount := writer.GetStats()
	if blocks != 1 {
		t.Errorf("expected 1 block, got %d", blocks)
	}
	if entryCount != 3 {
		t.Errorf("expected 3 entries, got %d", entryCount)
	}

	// Close
	if err := writer.Close(); err != nil {
		t.Fatalf("failed to close: %v", err)
	}
}

func TestFileWriter_Reopen(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "test.hyd")

	// Create and write
	writer, err := NewFileWriter(filePath, DefaultMaxBlockSize)
	if err != nil {
		t.Fatalf("failed to create writer: %v", err)
	}

	writer.WriteEntry(Entry{Operation: OpInsert, Key: "key1", Data: []byte("data1")})
	writer.Flush()
	writer.Close()

	// Reopen
	writer2, err := NewFileWriter(filePath, DefaultMaxBlockSize)
	if err != nil {
		t.Fatalf("failed to reopen writer: %v", err)
	}

	// Check stats from header
	blocks, entries := writer2.GetStats()
	if blocks != 1 {
		t.Errorf("expected 1 block after reopen, got %d", blocks)
	}
	if entries != 1 {
		t.Errorf("expected 1 entry after reopen, got %d", entries)
	}

	// Write more
	writer2.WriteEntry(Entry{Operation: OpInsert, Key: "key2", Data: []byte("data2")})
	writer2.Flush()
	writer2.Close()

	// Reopen again and check
	writer3, err := NewFileWriter(filePath, DefaultMaxBlockSize)
	if err != nil {
		t.Fatalf("failed to reopen writer again: %v", err)
	}
	defer writer3.Close()

	blocks, entries = writer3.GetStats()
	if blocks != 2 {
		t.Errorf("expected 2 blocks, got %d", blocks)
	}
	if entries != 2 {
		t.Errorf("expected 2 entries, got %d", entries)
	}
}

func TestFileWriter_WriteEntries(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "test.hyd")

	writer, err := NewFileWriter(filePath, DefaultMaxBlockSize)
	if err != nil {
		t.Fatalf("failed to create writer: %v", err)
	}

	entries := []Entry{
		{Operation: OpInsert, Key: "key1", Data: []byte("data1")},
		{Operation: OpInsert, Key: "key2", Data: []byte("data2")},
		{Operation: OpInsert, Key: "key3", Data: []byte("data3")},
	}

	if err := writer.WriteEntries(entries); err != nil {
		t.Fatalf("failed to write entries: %v", err)
	}

	if err := writer.Sync(); err != nil {
		t.Fatalf("failed to sync: %v", err)
	}

	writer.Close()
}

func TestFileWriter_ClosedError(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "test.hyd")

	writer, err := NewFileWriter(filePath, DefaultMaxBlockSize)
	if err != nil {
		t.Fatalf("failed to create writer: %v", err)
	}

	writer.Close()

	// Try to write after close
	err = writer.WriteEntry(Entry{Operation: OpInsert, Key: "key", Data: []byte("data")})
	if err != ErrFileClosed {
		t.Errorf("expected ErrFileClosed, got %v", err)
	}
}
