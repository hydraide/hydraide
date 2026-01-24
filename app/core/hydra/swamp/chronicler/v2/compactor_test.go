package v2

import (
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

func TestCompactionResult_String(t *testing.T) {
	result := &CompactionResult{Compacted: true, LiveEntries: 10}
	if result.String() == "" {
		t.Error("expected non-empty string")
	}
}
