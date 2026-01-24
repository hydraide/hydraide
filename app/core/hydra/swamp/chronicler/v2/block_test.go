package v2

import (
	"bytes"
	"testing"
)

func TestWriteBuffer_Add(t *testing.T) {
	buffer := NewWriteBuffer(1000) // 1KB max

	entry1 := Entry{Operation: OpInsert, Key: "key1", Data: []byte("data1")}
	entry2 := Entry{Operation: OpInsert, Key: "key2", Data: []byte("data2")}

	// Add first entry - should not trigger flush
	shouldFlush := buffer.Add(entry1)
	if shouldFlush {
		t.Error("should not trigger flush after first small entry")
	}

	if buffer.Count() != 1 {
		t.Errorf("expected count 1, got %d", buffer.Count())
	}

	// Add second entry
	shouldFlush = buffer.Add(entry2)
	if shouldFlush {
		t.Error("should not trigger flush after two small entries")
	}

	if buffer.Count() != 2 {
		t.Errorf("expected count 2, got %d", buffer.Count())
	}
}

func TestWriteBuffer_FlushOnMaxSize(t *testing.T) {
	buffer := NewWriteBuffer(50) // Very small buffer

	// Add entries until buffer should flush
	entry := Entry{Operation: OpInsert, Key: "key", Data: bytes.Repeat([]byte("x"), 30)}
	// Entry size: 1 + 2 + 3 + 4 + 30 = 40 bytes

	// First entry should not trigger flush (40 < 50)
	shouldFlush := buffer.Add(entry)
	if shouldFlush {
		t.Error("first entry should not trigger flush")
	}

	// Second entry should trigger flush (80 > 50)
	shouldFlush = buffer.Add(entry)
	if !shouldFlush {
		t.Error("should trigger flush when buffer exceeds max size")
	}
}

func TestWriteBuffer_Flush(t *testing.T) {
	buffer := NewWriteBuffer(DefaultMaxBlockSize)

	entries := []Entry{
		{Operation: OpInsert, Key: "key1", Data: []byte("data1")},
		{Operation: OpUpdate, Key: "key2", Data: []byte("data2")},
		{Operation: OpDelete, Key: "key3", Data: nil},
	}

	for _, e := range entries {
		buffer.Add(e)
	}

	// Flush
	header, compressed, err := buffer.Flush()
	if err != nil {
		t.Fatalf("flush failed: %v", err)
	}

	if header == nil {
		t.Fatal("header should not be nil")
	}

	if header.EntryCount != 3 {
		t.Errorf("expected 3 entries, got %d", header.EntryCount)
	}

	if len(compressed) == 0 {
		t.Error("compressed data should not be empty")
	}

	// Buffer should be empty after flush
	if !buffer.IsEmpty() {
		t.Error("buffer should be empty after flush")
	}
}

func TestWriteBuffer_FlushEmpty(t *testing.T) {
	buffer := NewWriteBuffer(DefaultMaxBlockSize)

	header, compressed, err := buffer.Flush()

	if err != nil {
		t.Fatalf("flush empty buffer should not error: %v", err)
	}

	if header != nil {
		t.Error("header should be nil for empty buffer")
	}

	if compressed != nil {
		t.Error("compressed should be nil for empty buffer")
	}
}

func TestParseBlock(t *testing.T) {
	// Create entries
	entries := []Entry{
		{Operation: OpInsert, Key: "key1", Data: []byte("data1")},
		{Operation: OpUpdate, Key: "key2", Data: []byte("data2-with-more-content")},
		{Operation: OpDelete, Key: "key3", Data: nil},
	}

	// Compress entries
	header, compressed, err := CompressEntries(entries)
	if err != nil {
		t.Fatalf("compress failed: %v", err)
	}

	// Parse block
	block, err := ParseBlock(header, compressed)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	// Verify
	if len(block.Entries) != len(entries) {
		t.Errorf("expected %d entries, got %d", len(entries), len(block.Entries))
	}

	for i, expected := range entries {
		got := block.Entries[i]
		if got.Key != expected.Key {
			t.Errorf("entry %d: key mismatch: expected %s, got %s", i, expected.Key, got.Key)
		}
		if got.Operation != expected.Operation {
			t.Errorf("entry %d: operation mismatch", i)
		}
		if !bytes.Equal(got.Data, expected.Data) {
			t.Errorf("entry %d: data mismatch", i)
		}
	}
}

func TestParseBlock_CorruptedChecksum(t *testing.T) {
	entries := []Entry{
		{Operation: OpInsert, Key: "key1", Data: []byte("data1")},
	}

	header, compressed, err := CompressEntries(entries)
	if err != nil {
		t.Fatalf("compress failed: %v", err)
	}

	// Corrupt checksum
	header.Checksum = 0x12345678

	_, err = ParseBlock(header, compressed)
	if err != ErrCorruptedBlock {
		t.Errorf("expected ErrCorruptedBlock, got %v", err)
	}
}

func TestParseBlock_CorruptedData(t *testing.T) {
	entries := []Entry{
		{Operation: OpInsert, Key: "key1", Data: []byte("data1")},
	}

	header, compressed, err := CompressEntries(entries)
	if err != nil {
		t.Fatalf("compress failed: %v", err)
	}

	// Corrupt the uncompressed size in header to force size mismatch
	header.UncompressedSize = header.UncompressedSize + 100

	_, err = ParseBlock(header, compressed)
	// Should fail on uncompressed size mismatch
	if err != ErrCorruptedBlock {
		t.Errorf("expected ErrCorruptedBlock for size mismatch, got %v", err)
	}
}

func TestCompressEntries_Empty(t *testing.T) {
	header, compressed, err := CompressEntries(nil)

	if err != nil {
		t.Fatalf("should not error: %v", err)
	}

	if header != nil {
		t.Error("header should be nil for empty entries")
	}

	if compressed != nil {
		t.Error("compressed should be nil for empty entries")
	}
}

func TestWriteBuffer_GetEntriesAndClear(t *testing.T) {
	buffer := NewWriteBuffer(DefaultMaxBlockSize)

	entries := []Entry{
		{Operation: OpInsert, Key: "key1", Data: []byte("data1")},
		{Operation: OpInsert, Key: "key2", Data: []byte("data2")},
	}

	for _, e := range entries {
		buffer.Add(e)
	}

	retrieved := buffer.GetEntriesAndClear()

	if len(retrieved) != 2 {
		t.Errorf("expected 2 entries, got %d", len(retrieved))
	}

	if !buffer.IsEmpty() {
		t.Error("buffer should be empty after GetEntriesAndClear")
	}

	// Verify entries are independent copies
	if retrieved[0].Key != "key1" {
		t.Error("retrieved entry key mismatch")
	}
}

func TestCompressDecompress(t *testing.T) {
	original := []byte("This is some test data that should be compressed and decompressed correctly.")

	compressed := CompressBlock(original)

	if len(compressed) >= len(original) {
		// For very small data, compression might not reduce size
		// This is OK, just a note
		t.Logf("note: compressed size (%d) >= original size (%d)", len(compressed), len(original))
	}

	decompressed, err := DecompressBlock(compressed)
	if err != nil {
		t.Fatalf("decompress failed: %v", err)
	}

	if !bytes.Equal(original, decompressed) {
		t.Error("decompressed data does not match original")
	}
}

func TestWriteBuffer_Concurrent(t *testing.T) {
	buffer := NewWriteBuffer(100000) // Large buffer to avoid flush during test

	// Concurrent adds
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func(id int) {
			for j := 0; j < 100; j++ {
				entry := Entry{
					Operation: OpInsert,
					Key:       string(rune('a'+id)) + string(rune('0'+j)),
					Data:      []byte("data"),
				}
				buffer.Add(entry)
			}
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	if buffer.Count() != 1000 {
		t.Errorf("expected 1000 entries, got %d", buffer.Count())
	}
}

func BenchmarkCompressEntries(b *testing.B) {
	// Create typical entries
	entries := make([]Entry, 50)
	for i := 0; i < 50; i++ {
		entries[i] = Entry{
			Operation: OpInsert,
			Key:       "test-key-" + string(rune('a'+i)),
			Data:      bytes.Repeat([]byte("x"), 200), // ~200 bytes each
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, _ = CompressEntries(entries)
	}
}

func BenchmarkParseBlock(b *testing.B) {
	// Create and compress entries
	entries := make([]Entry, 50)
	for i := 0; i < 50; i++ {
		entries[i] = Entry{
			Operation: OpInsert,
			Key:       "test-key-" + string(rune('a'+i)),
			Data:      bytes.Repeat([]byte("x"), 200),
		}
	}
	header, compressed, _ := CompressEntries(entries)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = ParseBlock(header, compressed)
	}
}
