package v2

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

// 8.5. Visszafele kompatibilitas tesztek — V3 kod SEMMIT NEM ront el V2 fajlokon.

// createV2File creates a V2 format file with OpMetadata for swamp name (legacy format).
func createV2File(t *testing.T, dir string, name string, swampName string, entries []Entry) string {
	t.Helper()
	filePath := filepath.Join(dir, name)

	// Create file manually with V2 header
	file, err := os.Create(filePath)
	if err != nil {
		t.Fatalf("failed to create file: %v", err)
	}

	header := NewFileHeader()
	header.Version = Version2
	header.NameLength = 0
	headerBytes := header.Serialize()
	// Ensure V2 header has version=2
	binary.LittleEndian.PutUint16(headerBytes[4:6], Version2)

	if _, err := file.Write(headerBytes); err != nil {
		file.Close()
		t.Fatalf("failed to write header: %v", err)
	}
	file.Close()

	// Now use the writer to add entries (it will open existing V2 file)
	writer, err := NewFileWriter(filePath, DefaultMaxBlockSize)
	if err != nil {
		t.Fatalf("failed to create writer: %v", err)
	}

	// Write metadata entry (V2 style - in block)
	if swampName != "" {
		metaEntry := Entry{
			Operation: OpMetadata,
			Key:       MetadataEntryKey,
			Data:      []byte(swampName),
		}
		if err := writer.WriteEntry(metaEntry); err != nil {
			writer.Close()
			t.Fatalf("failed to write metadata: %v", err)
		}
	}

	for _, e := range entries {
		if err := writer.WriteEntry(e); err != nil {
			writer.Close()
			t.Fatalf("failed to write entry: %v", err)
		}
	}

	if err := writer.Close(); err != nil {
		t.Fatalf("failed to close writer: %v", err)
	}

	return filePath
}

func TestCompat_V2FileReadByV3Reader(t *testing.T) {
	tmpDir := t.TempDir()

	entries := []Entry{
		{Operation: OpInsert, Key: "user-1", Data: []byte("alice")},
		{Operation: OpInsert, Key: "user-2", Data: []byte("bob")},
		{Operation: OpUpdate, Key: "user-1", Data: []byte("alice-updated")},
	}

	filePath := createV2File(t, tmpDir, "legacy.hyd", "users/profiles/test", entries)

	// V3 reader should handle V2 file
	reader, err := NewFileReader(filePath)
	if err != nil {
		t.Fatalf("failed to open V2 file: %v", err)
	}
	defer reader.Close()

	// Header should be V2
	if reader.GetHeader().IsV3() {
		t.Error("expected V2 header, got V3")
	}
	if reader.GetHeader().Version != Version2 {
		t.Errorf("expected version %d, got %d", Version2, reader.GetHeader().Version)
	}

	// GetSwampName returns empty for V2 (name is in blocks, not header)
	if reader.GetSwampName() != "" {
		t.Errorf("expected empty GetSwampName for V2, got %q", reader.GetSwampName())
	}

	// LoadIndex should get the name from OpMetadata in blocks
	index, swampName, err := reader.LoadIndex()
	if err != nil {
		t.Fatalf("failed to load index: %v", err)
	}

	if swampName != "users/profiles/test" {
		t.Errorf("expected swamp name %q, got %q", "users/profiles/test", swampName)
	}

	// Verify entries
	if len(index) != 2 {
		t.Fatalf("expected 2 live entries, got %d", len(index))
	}
	if string(index["user-1"]) != "alice-updated" {
		t.Errorf("user-1: expected %q, got %q", "alice-updated", string(index["user-1"]))
	}
	if string(index["user-2"]) != "bob" {
		t.Errorf("user-2: expected %q, got %q", "bob", string(index["user-2"]))
	}
}

func TestCompat_ReadSwampName_V2Fallback(t *testing.T) {
	tmpDir := t.TempDir()

	entries := []Entry{
		{Operation: OpInsert, Key: "k1", Data: []byte("v1")},
	}

	filePath := createV2File(t, tmpDir, "v2name.hyd", "sanctuary/realm/swamp", entries)

	name, err := ReadSwampName(filePath)
	if err != nil {
		t.Fatalf("ReadSwampName failed: %v", err)
	}

	if name != "sanctuary/realm/swamp" {
		t.Errorf("expected %q, got %q", "sanctuary/realm/swamp", name)
	}
}

func TestCompat_MixedV2V3Directory(t *testing.T) {
	tmpDir := t.TempDir()

	// Create V2 file
	v2Entries := []Entry{{Operation: OpInsert, Key: "k1", Data: []byte("v1")}}
	v2Path := createV2File(t, tmpDir, "old.hyd", "v2/swamp/name", v2Entries)

	// Create V3 file
	v3Path := filepath.Join(tmpDir, "new.hyd")
	writer, err := NewFileWriterWithName(v3Path, DefaultMaxBlockSize, "v3/swamp/name")
	if err != nil {
		t.Fatalf("failed to create V3 writer: %v", err)
	}
	writer.WriteEntry(Entry{Operation: OpInsert, Key: "k2", Data: []byte("v2")})
	writer.Close()

	// ReadSwampName should work on both
	v2Name, err := ReadSwampName(v2Path)
	if err != nil {
		t.Fatalf("ReadSwampName V2 failed: %v", err)
	}
	if v2Name != "v2/swamp/name" {
		t.Errorf("V2: expected %q, got %q", "v2/swamp/name", v2Name)
	}

	v3Name, err := ReadSwampName(v3Path)
	if err != nil {
		t.Fatalf("ReadSwampName V3 failed: %v", err)
	}
	if v3Name != "v3/swamp/name" {
		t.Errorf("V3: expected %q, got %q", "v3/swamp/name", v3Name)
	}
}

func TestCompat_V2FileAppendPreservesFormat(t *testing.T) {
	tmpDir := t.TempDir()

	entries := []Entry{
		{Operation: OpInsert, Key: "existing", Data: []byte("data")},
	}
	filePath := createV2File(t, tmpDir, "append.hyd", "test/swamp", entries)

	// Append using regular writer (should preserve V2 format for existing files)
	writer, err := NewFileWriter(filePath, DefaultMaxBlockSize)
	if err != nil {
		t.Fatalf("failed to open for append: %v", err)
	}
	writer.WriteEntry(Entry{Operation: OpInsert, Key: "new-key", Data: []byte("new-data")})
	writer.Close()

	// File should still be V2
	reader, err := NewFileReader(filePath)
	if err != nil {
		t.Fatalf("failed to open file: %v", err)
	}
	defer reader.Close()

	if reader.GetHeader().Version != Version2 {
		t.Errorf("expected V2 format preserved, got version %d", reader.GetHeader().Version)
	}

	// Both entries should be present
	index, swampName, err := reader.LoadIndex()
	if err != nil {
		t.Fatalf("failed to load index: %v", err)
	}

	if swampName != "test/swamp" {
		t.Errorf("expected swamp name %q, got %q", "test/swamp", swampName)
	}
	if len(index) != 2 {
		t.Errorf("expected 2 entries, got %d", len(index))
	}
	if string(index["existing"]) != "data" {
		t.Errorf("existing entry: expected %q, got %q", "data", string(index["existing"]))
	}
	if string(index["new-key"]) != "new-data" {
		t.Errorf("new entry: expected %q, got %q", "new-data", string(index["new-key"]))
	}
}

func TestCompat_V2CompactionUpgradesToV3(t *testing.T) {
	tmpDir := t.TempDir()

	entries := []Entry{
		{Operation: OpInsert, Key: "key1", Data: []byte("v1")},
		{Operation: OpUpdate, Key: "key1", Data: []byte("v2")},
		{Operation: OpUpdate, Key: "key1", Data: []byte("v3")},
		{Operation: OpInsert, Key: "key2", Data: []byte("alive")},
	}
	filePath := createV2File(t, tmpDir, "compact.hyd", "upgrade/test", entries)

	// Verify it's V2
	reader, err := NewFileReader(filePath)
	if err != nil {
		t.Fatalf("failed to open: %v", err)
	}
	if reader.GetHeader().Version != Version2 {
		t.Errorf("expected V2 before compaction, got %d", reader.GetHeader().Version)
	}
	reader.Close()

	// Compact — should upgrade to V3
	compactor := NewCompactor(filePath, DefaultMaxBlockSize, 0)
	result, err := compactor.ForceCompact()
	if err != nil {
		t.Fatalf("compaction failed: %v", err)
	}
	if !result.Compacted {
		t.Error("expected compaction to run")
	}

	// Verify V3 format after compaction
	reader2, err := NewFileReader(filePath)
	if err != nil {
		t.Fatalf("failed to open after compaction: %v", err)
	}
	defer reader2.Close()

	if !reader2.GetHeader().IsV3() {
		t.Errorf("expected V3 after compaction, got version %d", reader2.GetHeader().Version)
	}

	// Name should be in header
	if reader2.GetSwampName() != "upgrade/test" {
		t.Errorf("expected swamp name %q in header, got %q", "upgrade/test", reader2.GetSwampName())
	}

	// Verify data integrity
	index, name, err := reader2.LoadIndex()
	if err != nil {
		t.Fatalf("failed to load index: %v", err)
	}
	if name != "upgrade/test" {
		t.Errorf("LoadIndex name: expected %q, got %q", "upgrade/test", name)
	}
	if len(index) != 2 {
		t.Fatalf("expected 2 live entries, got %d", len(index))
	}
	if string(index["key1"]) != "v3" {
		t.Errorf("key1: expected %q, got %q", "v3", string(index["key1"]))
	}
	if string(index["key2"]) != "alive" {
		t.Errorf("key2: expected %q, got %q", "alive", string(index["key2"]))
	}

	// Verify no OpMetadata in blocks
	var hasOpMetadata bool
	reader2b, _ := NewFileReader(filePath)
	reader2b.ReadAllEntries(func(entry Entry) bool {
		if entry.Operation == OpMetadata {
			hasOpMetadata = true
			return false
		}
		return true
	})
	reader2b.Close()

	if hasOpMetadata {
		t.Error("V3 file should not have OpMetadata entries in blocks")
	}
}

func TestCompat_V3FileNotReadableAsV2Only(t *testing.T) {
	// This documents that V3 files use Version=3 in header.
	// A strict V2-only reader would reject it with ErrUnsupportedVer.
	// Our current reader supports both V2 and V3.
	header := NewFileHeader() // Creates V3 by default
	data := header.Serialize()

	// Simulate a strict V2-only check
	version := binary.LittleEndian.Uint16(data[4:6])
	if version != Version3 {
		t.Errorf("expected V3 version in new header, got %d", version)
	}

	// A V2-only reader would reject version=3
	// Our reader accepts it — verify
	restored := &FileHeader{}
	err := restored.Deserialize(data)
	if err != nil {
		t.Fatalf("current reader should accept V3: %v", err)
	}
	if restored.Version != Version3 {
		t.Errorf("expected version %d, got %d", Version3, restored.Version)
	}
}

func TestCompat_DataStartOffset(t *testing.T) {
	// V2: data starts right after 64-byte header
	v2Header := &FileHeader{Version: Version2, NameLength: 0}
	if v2Header.DataStartOffset() != FileHeaderSize {
		t.Errorf("V2 DataStartOffset: expected %d, got %d", FileHeaderSize, v2Header.DataStartOffset())
	}

	// V3 with name: data starts after header + name
	v3Header := &FileHeader{Version: Version3, NameLength: 25}
	expected := int64(FileHeaderSize) + 25
	if v3Header.DataStartOffset() != expected {
		t.Errorf("V3 DataStartOffset: expected %d, got %d", expected, v3Header.DataStartOffset())
	}

	// V3 without name: same as V2
	v3NoName := &FileHeader{Version: Version3, NameLength: 0}
	if v3NoName.DataStartOffset() != FileHeaderSize {
		t.Errorf("V3 no-name DataStartOffset: expected %d, got %d", FileHeaderSize, v3NoName.DataStartOffset())
	}
}

func TestCompat_V2FileNoOpMetadata_EmptyName(t *testing.T) {
	// V2 file without OpMetadata entry — name should be empty
	tmpDir := t.TempDir()

	entries := []Entry{
		{Operation: OpInsert, Key: "data-only", Data: []byte("no-meta")},
	}
	// Create V2 file without swamp name
	filePath := createV2File(t, tmpDir, "no-meta.hyd", "", entries)

	name, err := ReadSwampName(filePath)
	if err != nil {
		t.Fatalf("ReadSwampName failed: %v", err)
	}
	if name != "" {
		t.Errorf("expected empty name for V2 without metadata, got %q", name)
	}

	// LoadIndex should also return empty name
	reader, err := NewFileReader(filePath)
	if err != nil {
		t.Fatalf("failed to open: %v", err)
	}
	defer reader.Close()

	_, indexName, err := reader.LoadIndex()
	if err != nil {
		t.Fatalf("LoadIndex failed: %v", err)
	}
	if indexName != "" {
		t.Errorf("expected empty name from LoadIndex, got %q", indexName)
	}
}

func TestCompat_V2EntryDataIntegrity(t *testing.T) {
	// Verify that reading V2 entries produces bit-exact data
	tmpDir := t.TempDir()
	testData := []byte{0x00, 0xFF, 0x01, 0xFE, 0x80, 0x7F}

	entries := []Entry{
		{Operation: OpInsert, Key: "binary", Data: testData},
	}
	filePath := createV2File(t, tmpDir, "binary.hyd", "", entries)

	reader, err := NewFileReader(filePath)
	if err != nil {
		t.Fatalf("failed to open: %v", err)
	}
	defer reader.Close()

	index, _, err := reader.LoadIndex()
	if err != nil {
		t.Fatalf("LoadIndex failed: %v", err)
	}

	if !bytes.Equal(index["binary"], testData) {
		t.Errorf("data mismatch: expected %v, got %v", testData, index["binary"])
	}
}
