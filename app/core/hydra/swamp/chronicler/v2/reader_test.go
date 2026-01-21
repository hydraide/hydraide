package v2

import (
	"os"
	"path/filepath"
	"testing"
)

func createTestFile(t *testing.T, entries []Entry) string {
	t.Helper()
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "test.hyd")

	writer, err := NewFileWriter(filePath, DefaultMaxBlockSize)
	if err != nil {
		t.Fatalf("failed to create writer: %v", err)
	}

	for _, e := range entries {
		if err := writer.WriteEntry(e); err != nil {
			t.Fatalf("failed to write entry: %v", err)
		}
	}

	if err := writer.Close(); err != nil {
		t.Fatalf("failed to close writer: %v", err)
	}

	return filePath
}

func TestFileReader_ReadAllEntries(t *testing.T) {
	entries := []Entry{
		{Operation: OpInsert, Key: "key1", Data: []byte("data1")},
		{Operation: OpInsert, Key: "key2", Data: []byte("data2")},
		{Operation: OpUpdate, Key: "key1", Data: []byte("data1-updated")},
		{Operation: OpDelete, Key: "key2", Data: nil},
	}

	filePath := createTestFile(t, entries)

	reader, err := NewFileReader(filePath)
	if err != nil {
		t.Fatalf("failed to create reader: %v", err)
	}
	defer reader.Close()

	var readEntries []Entry
	count, err := reader.ReadAllEntries(func(entry Entry) bool {
		readEntries = append(readEntries, entry)
		return true
	})

	if err != nil {
		t.Fatalf("failed to read entries: %v", err)
	}

	if count != 4 {
		t.Errorf("expected 4 entries, got %d", count)
	}

	if len(readEntries) != 4 {
		t.Errorf("expected 4 entries in slice, got %d", len(readEntries))
	}

	// Verify order and content
	if readEntries[0].Key != "key1" || readEntries[0].Operation != OpInsert {
		t.Error("first entry mismatch")
	}
	if readEntries[2].Key != "key1" || readEntries[2].Operation != OpUpdate {
		t.Error("third entry mismatch")
	}
	if readEntries[3].Key != "key2" || readEntries[3].Operation != OpDelete {
		t.Error("fourth entry mismatch")
	}
}

func TestFileReader_LoadIndex(t *testing.T) {
	entries := []Entry{
		{Operation: OpInsert, Key: "key1", Data: []byte("data1")},
		{Operation: OpInsert, Key: "key2", Data: []byte("data2")},
		{Operation: OpUpdate, Key: "key1", Data: []byte("data1-updated")},
		{Operation: OpDelete, Key: "key2", Data: nil},
	}

	filePath := createTestFile(t, entries)

	reader, err := NewFileReader(filePath)
	if err != nil {
		t.Fatalf("failed to create reader: %v", err)
	}
	defer reader.Close()

	index, total, err := reader.LoadIndex()
	if err != nil {
		t.Fatalf("failed to load index: %v", err)
	}

	if total != 4 {
		t.Errorf("expected 4 total entries, got %d", total)
	}

	// Only key1 should remain (key2 was deleted)
	if len(index) != 1 {
		t.Errorf("expected 1 live entry, got %d", len(index))
	}

	// key1 should have updated data
	data, exists := index["key1"]
	if !exists {
		t.Error("key1 should exist")
	}
	if string(data) != "data1-updated" {
		t.Errorf("expected 'data1-updated', got '%s'", string(data))
	}

	// key2 should not exist
	if _, exists := index["key2"]; exists {
		t.Error("key2 should not exist (was deleted)")
	}
}

func TestFileReader_CalculateFragmentation(t *testing.T) {
	entries := []Entry{
		{Operation: OpInsert, Key: "key1", Data: []byte("data1")},
		{Operation: OpInsert, Key: "key2", Data: []byte("data2")},
		{Operation: OpUpdate, Key: "key1", Data: []byte("data1-v2")},
		{Operation: OpUpdate, Key: "key1", Data: []byte("data1-v3")},
		{Operation: OpDelete, Key: "key2", Data: nil},
	}

	filePath := createTestFile(t, entries)

	reader, err := NewFileReader(filePath)
	if err != nil {
		t.Fatalf("failed to create reader: %v", err)
	}
	defer reader.Close()

	fragmentation, liveCount, totalCount, err := reader.CalculateFragmentation()
	if err != nil {
		t.Fatalf("failed to calculate fragmentation: %v", err)
	}

	// 5 total entries, 1 live (key1 only, key2 deleted)
	if totalCount != 5 {
		t.Errorf("expected 5 total entries, got %d", totalCount)
	}
	if liveCount != 1 {
		t.Errorf("expected 1 live entry, got %d", liveCount)
	}

	// Fragmentation = (5-1)/5 = 0.8
	expectedFrag := 0.8
	if fragmentation < expectedFrag-0.01 || fragmentation > expectedFrag+0.01 {
		t.Errorf("expected fragmentation ~%.2f, got %.2f", expectedFrag, fragmentation)
	}
}

func TestFileReader_EmptyFile(t *testing.T) {
	filePath := createTestFile(t, []Entry{})

	reader, err := NewFileReader(filePath)
	if err != nil {
		t.Fatalf("failed to create reader: %v", err)
	}
	defer reader.Close()

	index, total, err := reader.LoadIndex()
	if err != nil {
		t.Fatalf("failed to load index: %v", err)
	}

	if total != 0 {
		t.Errorf("expected 0 entries, got %d", total)
	}
	if len(index) != 0 {
		t.Errorf("expected empty index, got %d entries", len(index))
	}
}

func TestFileReader_NonExistentFile(t *testing.T) {
	_, err := NewFileReader("/non/existent/path.hyd")
	if err == nil {
		t.Error("expected error for non-existent file")
	}
}

func TestFileReader_GetHeader(t *testing.T) {
	entries := []Entry{
		{Operation: OpInsert, Key: "key1", Data: []byte("data1")},
	}

	filePath := createTestFile(t, entries)

	reader, err := NewFileReader(filePath)
	if err != nil {
		t.Fatalf("failed to create reader: %v", err)
	}
	defer reader.Close()

	header := reader.GetHeader()
	if header == nil {
		t.Fatal("header should not be nil")
	}

	if string(header.Magic[:]) != MagicBytes {
		t.Errorf("expected magic %s, got %s", MagicBytes, string(header.Magic[:]))
	}

	if header.Version != CurrentVersion {
		t.Errorf("expected version %d, got %d", CurrentVersion, header.Version)
	}
}

func TestFileReader_ReadAllBlocks(t *testing.T) {
	// Create enough entries for multiple blocks
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "test.hyd")

	// Use small block size to force multiple blocks
	writer, err := NewFileWriter(filePath, 500) // 500 bytes max
	if err != nil {
		t.Fatalf("failed to create writer: %v", err)
	}

	// Write 20 entries, each ~50 bytes
	for i := 0; i < 20; i++ {
		entry := Entry{
			Operation: OpInsert,
			Key:       "key-with-some-length-" + string(rune('a'+i)),
			Data:      []byte("some data content here"),
		}
		writer.WriteEntry(entry)
	}
	writer.Close()

	// Read back
	reader, err := NewFileReader(filePath)
	if err != nil {
		t.Fatalf("failed to create reader: %v", err)
	}
	defer reader.Close()

	blocks, err := reader.ReadAllBlocks()
	if err != nil {
		t.Fatalf("failed to read blocks: %v", err)
	}

	if len(blocks) < 2 {
		t.Errorf("expected at least 2 blocks, got %d", len(blocks))
	}

	// Count total entries across blocks
	totalEntries := 0
	for _, block := range blocks {
		totalEntries += len(block.Entries)
	}

	if totalEntries != 20 {
		t.Errorf("expected 20 total entries, got %d", totalEntries)
	}
}

func TestFileReader_InvalidFile(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "invalid.hyd")

	// Create invalid file
	if err := os.WriteFile(filePath, []byte("not a valid hyd file"), 0644); err != nil {
		t.Fatalf("failed to create invalid file: %v", err)
	}

	_, err := NewFileReader(filePath)
	if err == nil {
		t.Error("expected error for invalid file")
	}
}
