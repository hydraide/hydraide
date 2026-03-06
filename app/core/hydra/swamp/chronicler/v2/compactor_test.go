package v2

import (
	"fmt"
	"path/filepath"
	"testing"
)

func TestCompactor_ShouldCompact(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "test.hyd")

	writer, _ := NewFileWriter(filePath, DefaultMaxBlockSize)
	for i := 0; i < 5; i++ {
		writer.WriteEntry(Entry{Operation: OpInsert, Key: "key1", Data: []byte("version")})
	}
	writer.Close()

	compactor := NewCompactor(filePath, DefaultMaxBlockSize, 0.5)
	shouldCompact, fragmentation, _ := compactor.ShouldCompact()

	if fragmentation < 0.7 || fragmentation > 0.9 {
		t.Errorf("expected ~0.8 fragmentation, got %.2f", fragmentation)
	}
	if !shouldCompact {
		t.Error("should recommend compaction")
	}
}

func TestCompactor_Compact(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "test.hyd")

	writer, _ := NewFileWriter(filePath, DefaultMaxBlockSize)
	writer.WriteEntry(Entry{Operation: OpInsert, Key: "key1", Data: []byte("v1")})
	writer.WriteEntry(Entry{Operation: OpInsert, Key: "key2", Data: []byte("v1")})
	writer.WriteEntry(Entry{Operation: OpUpdate, Key: "key1", Data: []byte("v3")})
	writer.WriteEntry(Entry{Operation: OpDelete, Key: "key2", Data: nil})
	writer.Close()

	compactor := NewCompactor(filePath, DefaultMaxBlockSize, 0.3)
	result, err := compactor.Compact()
	if err != nil {
		t.Fatalf("Compact failed: %v", err)
	}
	if !result.Compacted {
		t.Error("expected compaction")
	}
	if result.LiveEntries != 1 {
		t.Errorf("expected 1 live entry, got %d", result.LiveEntries)
	}
}

// 8.7. Compactor V3 tesztek

func TestCompactor_V3toV3_NoDowngrade(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "v3compact.hyd")

	writer, err := NewFileWriterWithName(filePath, DefaultMaxBlockSize, "v3/compactor/test")
	if err != nil {
		t.Fatalf("failed to create writer: %v", err)
	}
	// Create fragmentation
	writer.WriteEntry(Entry{Operation: OpInsert, Key: "k1", Data: []byte("v1")})
	writer.WriteEntry(Entry{Operation: OpUpdate, Key: "k1", Data: []byte("v2")})
	writer.WriteEntry(Entry{Operation: OpUpdate, Key: "k1", Data: []byte("v3")})
	writer.WriteEntry(Entry{Operation: OpInsert, Key: "k2", Data: []byte("alive")})
	writer.Close()

	compactor := NewCompactor(filePath, DefaultMaxBlockSize, 0)
	result, err := compactor.ForceCompact()
	if err != nil {
		t.Fatalf("compaction failed: %v", err)
	}
	if !result.Compacted {
		t.Error("expected compaction to run")
	}

	// Verify still V3
	reader, err := NewFileReader(filePath)
	if err != nil {
		t.Fatalf("failed to open: %v", err)
	}
	defer reader.Close()

	if !reader.GetHeader().IsV3() {
		t.Errorf("expected V3 after compaction, got version %d", reader.GetHeader().Version)
	}
	if reader.GetSwampName() != "v3/compactor/test" {
		t.Errorf("name: expected %q, got %q", "v3/compactor/test", reader.GetSwampName())
	}

	index, _, err := reader.LoadIndex()
	if err != nil {
		t.Fatalf("LoadIndex failed: %v", err)
	}
	if len(index) != 2 {
		t.Fatalf("expected 2 live entries, got %d", len(index))
	}
	if string(index["k1"]) != "v3" {
		t.Errorf("k1: expected %q, got %q", "v3", string(index["k1"]))
	}
}

func TestCompactor_V3Compaction_PreservesAllLiveData(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "v3preserve.hyd")

	writer, err := NewFileWriterWithName(filePath, DefaultMaxBlockSize, "preserve/test")
	if err != nil {
		t.Fatalf("failed to create writer: %v", err)
	}

	// 500 inserts
	for i := 0; i < 500; i++ {
		writer.WriteEntry(Entry{
			Operation: OpInsert,
			Key:       fmt.Sprintf("ins-%03d", i),
			Data:      []byte(fmt.Sprintf("data-%03d", i)),
		})
	}
	// 200 deletes (first 200)
	for i := 0; i < 200; i++ {
		writer.WriteEntry(Entry{Operation: OpDelete, Key: fmt.Sprintf("ins-%03d", i)})
	}
	// 100 updates (keys 200-299)
	for i := 200; i < 300; i++ {
		writer.WriteEntry(Entry{
			Operation: OpUpdate,
			Key:       fmt.Sprintf("ins-%03d", i),
			Data:      []byte(fmt.Sprintf("updated-%03d", i)),
		})
	}
	writer.Close()

	compactor := NewCompactor(filePath, DefaultMaxBlockSize, 0)
	result, err := compactor.ForceCompact()
	if err != nil {
		t.Fatalf("compaction failed: %v", err)
	}
	if !result.Compacted {
		t.Error("expected compaction to run")
	}

	reader, err := NewFileReader(filePath)
	if err != nil {
		t.Fatalf("failed to open: %v", err)
	}
	defer reader.Close()

	index, name, err := reader.LoadIndex()
	if err != nil {
		t.Fatalf("LoadIndex failed: %v", err)
	}

	if name != "preserve/test" {
		t.Errorf("name: expected %q, got %q", "preserve/test", name)
	}

	// Expected: 300 live (500 - 200 deleted)
	if len(index) != 300 {
		t.Fatalf("expected 300 live entries, got %d", len(index))
	}

	// Verify deleted keys are gone
	for i := 0; i < 200; i++ {
		key := fmt.Sprintf("ins-%03d", i)
		if _, exists := index[key]; exists {
			t.Errorf("deleted key %q should not exist", key)
		}
	}

	// Verify updated keys have new data
	for i := 200; i < 300; i++ {
		key := fmt.Sprintf("ins-%03d", i)
		expected := fmt.Sprintf("updated-%03d", i)
		if string(index[key]) != expected {
			t.Errorf("%s: expected %q, got %q", key, expected, string(index[key]))
		}
	}

	// Verify untouched keys have original data
	for i := 300; i < 500; i++ {
		key := fmt.Sprintf("ins-%03d", i)
		expected := fmt.Sprintf("data-%03d", i)
		if string(index[key]) != expected {
			t.Errorf("%s: expected %q, got %q", key, expected, string(index[key]))
		}
	}
}

func TestCompactor_V3_NoOpMetadataAfterCompaction(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "nometa.hyd")

	writer, err := NewFileWriterWithName(filePath, DefaultMaxBlockSize, "meta/check")
	if err != nil {
		t.Fatalf("failed to create writer: %v", err)
	}
	writer.WriteEntry(Entry{Operation: OpInsert, Key: "k1", Data: []byte("v1")})
	writer.WriteEntry(Entry{Operation: OpUpdate, Key: "k1", Data: []byte("v2")})
	writer.Close()

	compactor := NewCompactor(filePath, DefaultMaxBlockSize, 0)
	compactor.ForceCompact()

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
		t.Error("V3 compacted file should not have OpMetadata entries in blocks")
	}
}

func TestCompactionResult_String(t *testing.T) {
	result := &CompactionResult{Compacted: true, LiveEntries: 10}
	if result.String() == "" {
		t.Error("expected non-empty string")
	}
}
