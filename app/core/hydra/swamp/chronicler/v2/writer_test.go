package v2

import (
	"fmt"
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

// 8.2. Writer V3 — Unit tesztek

func TestV3Writer_CreatesFileWithName(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := tmpDir + "/v3test.hyd"
	swampName := "users/profiles/peter"

	writer, err := NewFileWriterWithName(filePath, DefaultMaxBlockSize, swampName)
	if err != nil {
		t.Fatalf("failed to create writer: %v", err)
	}
	writer.Close()

	// Verify file exists
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		t.Fatal("file was not created")
	}

	// Read raw bytes to verify layout
	data, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}

	// First 64 bytes: valid V3 header
	header := &FileHeader{}
	if err := header.Deserialize(data[:FileHeaderSize]); err != nil {
		t.Fatalf("failed to parse header: %v", err)
	}
	if !header.IsV3() {
		t.Error("expected V3 header")
	}
	if header.NameLength != uint16(len(swampName)) {
		t.Errorf("NameLength: expected %d, got %d", len(swampName), header.NameLength)
	}

	// Next bytes: swamp name
	nameBytes := data[FileHeaderSize : FileHeaderSize+int(header.NameLength)]
	if string(nameBytes) != swampName {
		t.Errorf("name: expected %q, got %q", swampName, string(nameBytes))
	}
}

func TestV3Writer_WriteEntries_AfterName(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := tmpDir + "/v3entries.hyd"
	swampName := "test/realm/swamp"

	writer, err := NewFileWriterWithName(filePath, DefaultMaxBlockSize, swampName)
	if err != nil {
		t.Fatalf("failed to create writer: %v", err)
	}

	for i := 0; i < 10; i++ {
		entry := Entry{
			Operation: OpInsert,
			Key:       fmt.Sprintf("key-%d", i),
			Data:      []byte(fmt.Sprintf("data-%d", i)),
		}
		if err := writer.WriteEntry(entry); err != nil {
			t.Fatalf("failed to write entry: %v", err)
		}
	}
	writer.Close()

	// Read back
	reader, err := NewFileReader(filePath)
	if err != nil {
		t.Fatalf("failed to create reader: %v", err)
	}
	defer reader.Close()

	if reader.GetSwampName() != swampName {
		t.Errorf("swamp name: expected %q, got %q", swampName, reader.GetSwampName())
	}

	var count int
	reader.ReadAllEntries(func(entry Entry) bool {
		count++
		return true
	})
	if count != 10 {
		t.Errorf("expected 10 entries, got %d", count)
	}
}

func TestV3Writer_EmptyName(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := tmpDir + "/v3empty.hyd"

	writer, err := NewFileWriterWithName(filePath, DefaultMaxBlockSize, "")
	if err != nil {
		t.Fatalf("failed to create writer: %v", err)
	}
	writer.WriteEntry(Entry{Operation: OpInsert, Key: "test", Data: []byte("data")})
	writer.Close()

	reader, err := NewFileReader(filePath)
	if err != nil {
		t.Fatalf("failed to open: %v", err)
	}
	defer reader.Close()

	if reader.GetHeader().NameLength != 0 {
		t.Errorf("expected NameLength=0 for empty name, got %d", reader.GetHeader().NameLength)
	}
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

func TestV3Writer_LongName(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := tmpDir + "/v3long.hyd"

	longName := string(make([]byte, 1000))
	for i := range longName {
		longName = longName[:i] + string(rune('a'+i%26)) + longName[i+1:]
	}
	// Use bytes.Repeat approach for cleaner long name
	longNameBytes := make([]byte, 1000)
	for i := range longNameBytes {
		longNameBytes[i] = byte('a' + i%26)
	}
	longName = string(longNameBytes)

	writer, err := NewFileWriterWithName(filePath, DefaultMaxBlockSize, longName)
	if err != nil {
		t.Fatalf("failed to create writer: %v", err)
	}
	writer.WriteEntry(Entry{Operation: OpInsert, Key: "k", Data: []byte("v")})
	writer.Close()

	reader, err := NewFileReader(filePath)
	if err != nil {
		t.Fatalf("failed to open: %v", err)
	}
	defer reader.Close()

	if reader.GetHeader().NameLength != 1000 {
		t.Errorf("expected NameLength=1000, got %d", reader.GetHeader().NameLength)
	}
	if reader.GetSwampName() != longName {
		t.Errorf("long name mismatch (len expected %d, got %d)", len(longName), len(reader.GetSwampName()))
	}
}

func TestV3Writer_NoOpMetadataInBlocks(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := tmpDir + "/v3nometa.hyd"

	writer, err := NewFileWriterWithName(filePath, DefaultMaxBlockSize, "some/swamp/name")
	if err != nil {
		t.Fatalf("failed to create writer: %v", err)
	}
	for i := 0; i < 20; i++ {
		writer.WriteEntry(Entry{
			Operation: OpInsert,
			Key:       fmt.Sprintf("k%d", i),
			Data:      []byte(fmt.Sprintf("v%d", i)),
		})
	}
	writer.Close()

	reader, err := NewFileReader(filePath)
	if err != nil {
		t.Fatalf("failed to open: %v", err)
	}
	defer reader.Close()

	var hasOpMetadata bool
	reader.ReadAllEntries(func(entry Entry) bool {
		if entry.Operation == OpMetadata {
			hasOpMetadata = true
			return false
		}
		return true
	})

	if hasOpMetadata {
		t.Error("V3 writer should not write OpMetadata entries into blocks")
	}
}

func TestV3Writer_DataNotDoubleCompressed(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := tmpDir + "/v3nodc.hyd"

	// Write known raw data
	rawData := []byte("hello world this is a test of double compression detection")

	writer, err := NewFileWriterWithName(filePath, DefaultMaxBlockSize, "test")
	if err != nil {
		t.Fatalf("failed to create writer: %v", err)
	}
	writer.WriteEntry(Entry{Operation: OpInsert, Key: "test-key", Data: rawData})
	writer.Close()

	// Read back — if double-compressed, the data would be garbled
	reader, err := NewFileReader(filePath)
	if err != nil {
		t.Fatalf("failed to open: %v", err)
	}
	defer reader.Close()

	index, _, err := reader.LoadIndex()
	if err != nil {
		t.Fatalf("LoadIndex failed: %v", err)
	}

	if string(index["test-key"]) != string(rawData) {
		t.Errorf("data mismatch (possible double compression)\nexpected: %q\ngot:      %q",
			string(rawData), string(index["test-key"]))
	}
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
