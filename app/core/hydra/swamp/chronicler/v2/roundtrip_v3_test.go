package v2

import (
	"bytes"
	"crypto/rand"
	"fmt"
	"testing"
)

// 8.4. Round-trip tesztek — amit beirunk, PONTOSAN ugyanugy kapjuk vissza.

func TestV3RoundTrip_SingleEntry(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := tmpDir + "/roundtrip.hyd"

	original := Entry{
		Operation: OpInsert,
		Key:       "test-key",
		Data:      []byte("hello world test data"),
	}

	writer, err := NewFileWriterWithName(filePath, DefaultMaxBlockSize, "test/realm/swamp")
	if err != nil {
		t.Fatalf("failed to create writer: %v", err)
	}
	if err := writer.WriteEntry(original); err != nil {
		t.Fatalf("failed to write entry: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("failed to close writer: %v", err)
	}

	reader, err := NewFileReader(filePath)
	if err != nil {
		t.Fatalf("failed to create reader: %v", err)
	}
	defer reader.Close()

	var readEntries []Entry
	_, err = reader.ReadAllEntries(func(entry Entry) bool {
		readEntries = append(readEntries, entry)
		return true
	})
	if err != nil {
		t.Fatalf("failed to read entries: %v", err)
	}

	if len(readEntries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(readEntries))
	}

	got := readEntries[0]
	if got.Operation != original.Operation {
		t.Errorf("operation mismatch: expected %d, got %d", original.Operation, got.Operation)
	}
	if got.Key != original.Key {
		t.Errorf("key mismatch: expected %q, got %q", original.Key, got.Key)
	}
	if !bytes.Equal(got.Data, original.Data) {
		t.Errorf("data mismatch: expected %q, got %q", original.Data, got.Data)
	}
}

func TestV3RoundTrip_ManyEntries(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := tmpDir + "/roundtrip_many.hyd"

	// Create entries with varying data sizes
	dataSizes := []int{0, 1, 100, 10000, 64000}
	var originals []Entry
	for i, size := range dataSizes {
		for j := 0; j < 200; j++ {
			idx := i*200 + j
			data := bytes.Repeat([]byte{byte(idx % 256)}, size)
			originals = append(originals, Entry{
				Operation: OpInsert,
				Key:       fmt.Sprintf("key-%05d", idx),
				Data:      data,
			})
		}
	}

	writer, err := NewFileWriterWithName(filePath, DefaultMaxBlockSize, "test/roundtrip")
	if err != nil {
		t.Fatalf("failed to create writer: %v", err)
	}
	for _, e := range originals {
		if err := writer.WriteEntry(e); err != nil {
			t.Fatalf("failed to write entry %s: %v", e.Key, err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("failed to close writer: %v", err)
	}

	reader, err := NewFileReader(filePath)
	if err != nil {
		t.Fatalf("failed to create reader: %v", err)
	}
	defer reader.Close()

	var readEntries []Entry
	_, err = reader.ReadAllEntries(func(entry Entry) bool {
		readEntries = append(readEntries, entry)
		return true
	})
	if err != nil {
		t.Fatalf("failed to read entries: %v", err)
	}

	if len(readEntries) != len(originals) {
		t.Fatalf("expected %d entries, got %d", len(originals), len(readEntries))
	}

	for i, orig := range originals {
		got := readEntries[i]
		if got.Key != orig.Key {
			t.Errorf("entry %d: key mismatch: expected %q, got %q", i, orig.Key, got.Key)
		}
		if !bytes.Equal(got.Data, orig.Data) {
			t.Errorf("entry %d: data mismatch (len expected %d, got %d)", i, len(orig.Data), len(got.Data))
		}
	}
}

func TestV3RoundTrip_MixedOperations(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := tmpDir + "/roundtrip_mixed.hyd"

	entries := []Entry{
		{Operation: OpInsert, Key: "key-a", Data: []byte("version1")},
		{Operation: OpInsert, Key: "key-b", Data: []byte("b-data")},
		{Operation: OpInsert, Key: "key-c", Data: []byte("c-data")},
		{Operation: OpUpdate, Key: "key-a", Data: []byte("version2")},
		{Operation: OpDelete, Key: "key-b"},
		{Operation: OpUpdate, Key: "key-a", Data: []byte("version3")},
		{Operation: OpInsert, Key: "key-d", Data: []byte("d-data")},
		{Operation: OpDelete, Key: "key-d"},
	}

	writer, err := NewFileWriterWithName(filePath, DefaultMaxBlockSize, "test/mixed")
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

	reader, err := NewFileReader(filePath)
	if err != nil {
		t.Fatalf("failed to create reader: %v", err)
	}
	defer reader.Close()

	index, swampName, err := reader.LoadIndex()
	if err != nil {
		t.Fatalf("failed to load index: %v", err)
	}

	if swampName != "test/mixed" {
		t.Errorf("expected swamp name %q, got %q", "test/mixed", swampName)
	}

	// Expected live: key-a (version3), key-c (c-data)
	// Deleted: key-b, key-d
	if len(index) != 2 {
		t.Fatalf("expected 2 live entries, got %d", len(index))
	}

	if string(index["key-a"]) != "version3" {
		t.Errorf("key-a: expected %q, got %q", "version3", string(index["key-a"]))
	}
	if string(index["key-c"]) != "c-data" {
		t.Errorf("key-c: expected %q, got %q", "c-data", string(index["key-c"]))
	}
	if _, exists := index["key-b"]; exists {
		t.Error("key-b should not exist (deleted)")
	}
	if _, exists := index["key-d"]; exists {
		t.Error("key-d should not exist (deleted)")
	}
}

func TestV3RoundTrip_BinaryData(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := tmpDir + "/roundtrip_binary.hyd"

	// Generate random binary data
	randomData := make([]byte, 4096)
	if _, err := rand.Read(randomData); err != nil {
		t.Fatalf("failed to generate random data: %v", err)
	}

	// Include all possible byte values
	allBytes := make([]byte, 256)
	for i := range allBytes {
		allBytes[i] = byte(i)
	}

	entries := []Entry{
		{Operation: OpInsert, Key: "random-data", Data: randomData},
		{Operation: OpInsert, Key: "all-bytes", Data: allBytes},
		{Operation: OpInsert, Key: "null-bytes", Data: []byte{0, 0, 0, 0, 0}},
	}

	writer, err := NewFileWriterWithName(filePath, DefaultMaxBlockSize, "binary/test")
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

	reader, err := NewFileReader(filePath)
	if err != nil {
		t.Fatalf("failed to create reader: %v", err)
	}
	defer reader.Close()

	index, _, err := reader.LoadIndex()
	if err != nil {
		t.Fatalf("failed to load index: %v", err)
	}

	if !bytes.Equal(index["random-data"], randomData) {
		t.Error("random-data: binary data mismatch")
	}
	if !bytes.Equal(index["all-bytes"], allBytes) {
		t.Error("all-bytes: binary data mismatch")
	}
	if !bytes.Equal(index["null-bytes"], []byte{0, 0, 0, 0, 0}) {
		t.Error("null-bytes: binary data mismatch")
	}
}
