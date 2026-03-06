package v2

import (
	"fmt"
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

	index, swampName, err := reader.LoadIndex()
	if err != nil {
		t.Fatalf("failed to load index: %v", err)
	}

	// swampName should be empty in this test (no metadata entry)
	_ = swampName

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

	index, _, err := reader.LoadIndex()
	if err != nil {
		t.Fatalf("failed to load index: %v", err)
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

// 8.3. Reader V3 — Unit tesztek

func createV3TestFile(t *testing.T, swampName string, entries []Entry) string {
	t.Helper()
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "v3test.hyd")

	writer, err := NewFileWriterWithName(filePath, DefaultMaxBlockSize, swampName)
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

func TestV3Reader_ReadsNameFromHeader(t *testing.T) {
	filePath := createV3TestFile(t, "sanctuary/realm/myswamp", []Entry{
		{Operation: OpInsert, Key: "k", Data: []byte("v")},
	})

	reader, err := NewFileReader(filePath)
	if err != nil {
		t.Fatalf("failed to open: %v", err)
	}
	defer reader.Close()

	if reader.GetSwampName() != "sanctuary/realm/myswamp" {
		t.Errorf("expected %q, got %q", "sanctuary/realm/myswamp", reader.GetSwampName())
	}
	if !reader.GetHeader().IsV3() {
		t.Error("expected V3 header")
	}
}

func TestV3Reader_ReadsEntriesCorrectly(t *testing.T) {
	var entries []Entry
	for i := 0; i < 50; i++ {
		entries = append(entries, Entry{
			Operation: OpInsert,
			Key:       fmt.Sprintf("key-%03d", i),
			Data:      []byte(fmt.Sprintf("data-%03d", i)),
		})
	}

	filePath := createV3TestFile(t, "test/50entries", entries)

	reader, err := NewFileReader(filePath)
	if err != nil {
		t.Fatalf("failed to open: %v", err)
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

	if count != 50 {
		t.Errorf("expected 50 entries, got %d", count)
	}

	for i, entry := range readEntries {
		expectedKey := fmt.Sprintf("key-%03d", i)
		expectedData := fmt.Sprintf("data-%03d", i)
		if entry.Key != expectedKey {
			t.Errorf("entry %d: expected key %q, got %q", i, expectedKey, entry.Key)
		}
		if string(entry.Data) != expectedData {
			t.Errorf("entry %d: expected data %q, got %q", i, expectedData, string(entry.Data))
		}
	}
}

func TestV3Reader_LoadIndex_V3(t *testing.T) {
	entries := []Entry{
		{Operation: OpInsert, Key: "a", Data: []byte("1")},
		{Operation: OpInsert, Key: "b", Data: []byte("2")},
		{Operation: OpUpdate, Key: "a", Data: []byte("3")},
		{Operation: OpDelete, Key: "b"},
	}

	filePath := createV3TestFile(t, "test/loadindex", entries)

	reader, err := NewFileReader(filePath)
	if err != nil {
		t.Fatalf("failed to open: %v", err)
	}
	defer reader.Close()

	index, swampName, err := reader.LoadIndex()
	if err != nil {
		t.Fatalf("LoadIndex failed: %v", err)
	}

	if swampName != "test/loadindex" {
		t.Errorf("swamp name: expected %q, got %q", "test/loadindex", swampName)
	}

	// Only "a" should remain with updated value
	if len(index) != 1 {
		t.Fatalf("expected 1 live entry, got %d", len(index))
	}
	if string(index["a"]) != "3" {
		t.Errorf("key 'a': expected %q, got %q", "3", string(index["a"]))
	}
}

func TestV3Reader_ReadSwampName_Fast(t *testing.T) {
	filePath := createV3TestFile(t, "fast/scan/name", []Entry{
		{Operation: OpInsert, Key: "k", Data: []byte("v")},
	})

	name, err := ReadSwampName(filePath)
	if err != nil {
		t.Fatalf("ReadSwampName failed: %v", err)
	}
	if name != "fast/scan/name" {
		t.Errorf("expected %q, got %q", "fast/scan/name", name)
	}
}

func TestV3Reader_EmptyNameFile(t *testing.T) {
	filePath := createV3TestFile(t, "", []Entry{
		{Operation: OpInsert, Key: "k", Data: []byte("v")},
	})

	reader, err := NewFileReader(filePath)
	if err != nil {
		t.Fatalf("failed to open: %v", err)
	}
	defer reader.Close()

	if reader.GetSwampName() != "" {
		t.Errorf("expected empty name, got %q", reader.GetSwampName())
	}

	// Entries should still be readable
	index, _, err := reader.LoadIndex()
	if err != nil {
		t.Fatalf("LoadIndex failed: %v", err)
	}
	if len(index) != 1 {
		t.Errorf("expected 1 entry, got %d", len(index))
	}
}

func TestV3Reader_MultipleBlocks(t *testing.T) {
	// Use small block size to force multiple blocks
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "multiblock.hyd")

	writer, err := NewFileWriterWithName(filePath, 500, "multi/block/test")
	if err != nil {
		t.Fatalf("failed to create writer: %v", err)
	}

	for i := 0; i < 30; i++ {
		writer.WriteEntry(Entry{
			Operation: OpInsert,
			Key:       fmt.Sprintf("mb-key-%03d", i),
			Data:      []byte("some data content that takes up space in the block"),
		})
	}
	writer.Close()

	reader, err := NewFileReader(filePath)
	if err != nil {
		t.Fatalf("failed to open: %v", err)
	}
	defer reader.Close()

	if reader.GetSwampName() != "multi/block/test" {
		t.Errorf("expected %q, got %q", "multi/block/test", reader.GetSwampName())
	}

	blocks, err := reader.ReadAllBlocks()
	if err != nil {
		t.Fatalf("ReadAllBlocks failed: %v", err)
	}

	if len(blocks) < 2 {
		t.Errorf("expected multiple blocks, got %d", len(blocks))
	}

	totalEntries := 0
	for _, b := range blocks {
		totalEntries += len(b.Entries)
	}
	if totalEntries != 30 {
		t.Errorf("expected 30 total entries, got %d", totalEntries)
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
