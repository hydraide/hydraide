package v2

import (
	"bytes"
	"testing"
)

func TestFileHeader_SerializeDeserialize(t *testing.T) {
	original := NewFileHeader()
	original.EntryCount = 12345
	original.BlockCount = 100

	// Serialize
	data := original.Serialize()

	if len(data) != FileHeaderSize {
		t.Errorf("expected header size %d, got %d", FileHeaderSize, len(data))
	}

	// Deserialize
	restored := &FileHeader{}
	err := restored.Deserialize(data)
	if err != nil {
		t.Fatalf("failed to deserialize: %v", err)
	}

	// Verify
	if string(restored.Magic[:]) != MagicBytes {
		t.Errorf("magic mismatch: expected %s, got %s", MagicBytes, string(restored.Magic[:]))
	}
	if restored.Version != CurrentVersion {
		t.Errorf("version mismatch: expected %d, got %d", CurrentVersion, restored.Version)
	}
	if restored.EntryCount != original.EntryCount {
		t.Errorf("entry count mismatch: expected %d, got %d", original.EntryCount, restored.EntryCount)
	}
	if restored.BlockCount != original.BlockCount {
		t.Errorf("block count mismatch: expected %d, got %d", original.BlockCount, restored.BlockCount)
	}
	if restored.BlockSize != DefaultMaxBlockSize {
		t.Errorf("block size mismatch: expected %d, got %d", DefaultMaxBlockSize, restored.BlockSize)
	}
}

func TestFileHeader_InvalidMagic(t *testing.T) {
	data := make([]byte, FileHeaderSize)
	copy(data[0:4], "XXXX") // Invalid magic

	header := &FileHeader{}
	err := header.Deserialize(data)

	if err != ErrInvalidMagic {
		t.Errorf("expected ErrInvalidMagic, got %v", err)
	}
}

func TestFileHeader_UnsupportedVersion(t *testing.T) {
	header := NewFileHeader()
	data := header.Serialize()

	// Modify version to unsupported value
	data[4] = 99
	data[5] = 0

	restored := &FileHeader{}
	err := restored.Deserialize(data)

	if err != ErrUnsupportedVer {
		t.Errorf("expected ErrUnsupportedVer, got %v", err)
	}
}

func TestBlockHeader_SerializeDeserialize(t *testing.T) {
	original := &BlockHeader{
		CompressedSize:   5000,
		UncompressedSize: 16000,
		EntryCount:       50,
		Checksum:         0x12345678,
		Flags:            0,
	}

	// Serialize
	data := original.Serialize()

	if len(data) != BlockHeaderSize {
		t.Errorf("expected block header size %d, got %d", BlockHeaderSize, len(data))
	}

	// Deserialize
	restored := &BlockHeader{}
	err := restored.Deserialize(data)
	if err != nil {
		t.Fatalf("failed to deserialize: %v", err)
	}

	// Verify
	if restored.CompressedSize != original.CompressedSize {
		t.Errorf("compressed size mismatch: expected %d, got %d", original.CompressedSize, restored.CompressedSize)
	}
	if restored.UncompressedSize != original.UncompressedSize {
		t.Errorf("uncompressed size mismatch: expected %d, got %d", original.UncompressedSize, restored.UncompressedSize)
	}
	if restored.EntryCount != original.EntryCount {
		t.Errorf("entry count mismatch: expected %d, got %d", original.EntryCount, restored.EntryCount)
	}
	if restored.Checksum != original.Checksum {
		t.Errorf("checksum mismatch: expected %d, got %d", original.Checksum, restored.Checksum)
	}
}

func TestEntry_SerializeDeserialize(t *testing.T) {
	testCases := []struct {
		name  string
		entry Entry
	}{
		{
			name: "insert with data",
			entry: Entry{
				Operation: OpInsert,
				Key:       "test-key-123",
				Data:      []byte("this is some test data content"),
			},
		},
		{
			name: "update with data",
			entry: Entry{
				Operation: OpUpdate,
				Key:       "another-key",
				Data:      []byte{0x01, 0x02, 0x03, 0x04, 0x05},
			},
		},
		{
			name: "delete without data",
			entry: Entry{
				Operation: OpDelete,
				Key:       "key-to-delete",
				Data:      nil,
			},
		},
		{
			name: "entry with long key",
			entry: Entry{
				Operation: OpInsert,
				Key:       "this-is-a-very-long-key-that-might-be-used-for-domains-or-urls-like-example.com/path/to/resource",
				Data:      []byte("data"),
			},
		},
		{
			name: "entry with large data",
			entry: Entry{
				Operation: OpInsert,
				Key:       "large-data-key",
				Data:      bytes.Repeat([]byte("x"), 10000),
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Serialize
			data := tc.entry.Serialize()

			expectedSize := tc.entry.Size()
			if len(data) != expectedSize {
				t.Errorf("expected size %d, got %d", expectedSize, len(data))
			}

			// Deserialize
			restored := &Entry{}
			consumed, err := restored.Deserialize(data)
			if err != nil {
				t.Fatalf("failed to deserialize: %v", err)
			}

			if consumed != len(data) {
				t.Errorf("expected to consume %d bytes, consumed %d", len(data), consumed)
			}

			// Verify
			if restored.Operation != tc.entry.Operation {
				t.Errorf("operation mismatch: expected %d, got %d", tc.entry.Operation, restored.Operation)
			}
			if restored.Key != tc.entry.Key {
				t.Errorf("key mismatch: expected %s, got %s", tc.entry.Key, restored.Key)
			}
			if !bytes.Equal(restored.Data, tc.entry.Data) {
				t.Errorf("data mismatch")
			}
		})
	}
}

func TestEntry_DeserializeMultiple(t *testing.T) {
	// Create multiple entries and serialize them together
	entries := []Entry{
		{Operation: OpInsert, Key: "key1", Data: []byte("data1")},
		{Operation: OpUpdate, Key: "key2", Data: []byte("data2-longer")},
		{Operation: OpDelete, Key: "key3", Data: nil},
	}

	// Serialize all entries into one buffer
	var buf bytes.Buffer
	for _, e := range entries {
		buf.Write(e.Serialize())
	}
	data := buf.Bytes()

	// Deserialize all entries
	offset := 0
	for i, expected := range entries {
		restored := &Entry{}
		consumed, err := restored.Deserialize(data[offset:])
		if err != nil {
			t.Fatalf("failed to deserialize entry %d: %v", i, err)
		}

		if restored.Key != expected.Key {
			t.Errorf("entry %d: key mismatch: expected %s, got %s", i, expected.Key, restored.Key)
		}
		if restored.Operation != expected.Operation {
			t.Errorf("entry %d: operation mismatch", i)
		}
		if !bytes.Equal(restored.Data, expected.Data) {
			t.Errorf("entry %d: data mismatch", i)
		}

		offset += consumed
	}

	if offset != len(data) {
		t.Errorf("did not consume all bytes: consumed %d, total %d", offset, len(data))
	}
}

func TestEntry_EmptyKey(t *testing.T) {
	entry := Entry{
		Operation: OpInsert,
		Key:       "",
		Data:      []byte("data"),
	}

	data := entry.Serialize()

	restored := &Entry{}
	_, err := restored.Deserialize(data)

	if err != ErrEmptyKey {
		t.Errorf("expected ErrEmptyKey, got %v", err)
	}
}

func TestEntry_CorruptedData(t *testing.T) {
	// Too short buffer
	data := []byte{0x01, 0x00} // Only 2 bytes

	entry := &Entry{}
	_, err := entry.Deserialize(data)

	if err != ErrCorruptedEntry {
		t.Errorf("expected ErrCorruptedEntry, got %v", err)
	}
}

func TestCalculateChecksum(t *testing.T) {
	data := []byte("test data for checksum")
	checksum := CalculateChecksum(data)

	// Same data should produce same checksum
	if !ValidateChecksum(data, checksum) {
		t.Error("checksum validation failed for same data")
	}

	// Different data should produce different checksum
	differentData := []byte("different data")
	if ValidateChecksum(differentData, checksum) {
		t.Error("checksum validation should fail for different data")
	}
}

// 8.1. FileHeader V3 — Unit tesztek

func TestV3Header_SerializeDeserialize(t *testing.T) {
	original := NewFileHeader()
	original.NameLength = 25
	original.EntryCount = 999
	original.BlockCount = 50

	data := original.Serialize()
	if len(data) != FileHeaderSize {
		t.Errorf("expected header size %d, got %d", FileHeaderSize, len(data))
	}

	restored := &FileHeader{}
	if err := restored.Deserialize(data); err != nil {
		t.Fatalf("failed to deserialize: %v", err)
	}

	if restored.Version != Version3 {
		t.Errorf("version: expected %d, got %d", Version3, restored.Version)
	}
	if restored.NameLength != 25 {
		t.Errorf("NameLength: expected 25, got %d", restored.NameLength)
	}
	if restored.EntryCount != 999 {
		t.Errorf("EntryCount: expected 999, got %d", restored.EntryCount)
	}
	if restored.BlockCount != 50 {
		t.Errorf("BlockCount: expected 50, got %d", restored.BlockCount)
	}
	if !restored.IsV3() {
		t.Error("IsV3() should return true")
	}
}

func TestV3Header_NameLengthPreserved(t *testing.T) {
	testCases := []uint16{0, 1, 100, 255, 65535}
	for _, nl := range testCases {
		header := NewFileHeader()
		header.NameLength = nl
		data := header.Serialize()

		restored := &FileHeader{}
		if err := restored.Deserialize(data); err != nil {
			t.Fatalf("NameLength=%d: deserialize failed: %v", nl, err)
		}
		if restored.NameLength != nl {
			t.Errorf("NameLength=%d: expected %d, got %d", nl, nl, restored.NameLength)
		}
	}
}

func TestV2Header_StillReadable(t *testing.T) {
	// Create a V2-style header
	header := NewFileHeader()
	data := header.Serialize()
	// Set version to 2
	data[4] = byte(Version2)
	data[5] = byte(Version2 >> 8)

	restored := &FileHeader{}
	if err := restored.Deserialize(data); err != nil {
		t.Fatalf("V2 header should be readable: %v", err)
	}
	if restored.Version != Version2 {
		t.Errorf("expected version %d, got %d", Version2, restored.Version)
	}
	if restored.NameLength != 0 {
		t.Errorf("V2 NameLength should be 0, got %d", restored.NameLength)
	}
	if restored.IsV3() {
		t.Error("IsV3() should return false for V2")
	}
}

func TestV3Header_BufferTooSmall(t *testing.T) {
	header := &FileHeader{}
	err := header.Deserialize(make([]byte, 63))
	if err == nil {
		t.Error("expected error for 63-byte buffer")
	}
}

func TestV3Header_Version2And3Accepted(t *testing.T) {
	tests := []struct {
		version  uint16
		wantErr  bool
		errValue error
	}{
		{Version2, false, nil},
		{Version3, false, nil},
		{1, true, ErrUnsupportedVer},
		{4, true, ErrUnsupportedVer},
		{99, true, ErrUnsupportedVer},
	}

	for _, tc := range tests {
		header := NewFileHeader()
		data := header.Serialize()
		data[4] = byte(tc.version)
		data[5] = byte(tc.version >> 8)

		restored := &FileHeader{}
		err := restored.Deserialize(data)
		if tc.wantErr {
			if err != tc.errValue {
				t.Errorf("version=%d: expected %v, got %v", tc.version, tc.errValue, err)
			}
		} else {
			if err != nil {
				t.Errorf("version=%d: unexpected error: %v", tc.version, err)
			}
		}
	}
}

func TestV3Header_DataStartOffset(t *testing.T) {
	v3 := &FileHeader{Version: Version3, NameLength: 20}
	if v3.DataStartOffset() != int64(FileHeaderSize)+20 {
		t.Errorf("V3 offset: expected %d, got %d", int64(FileHeaderSize)+20, v3.DataStartOffset())
	}

	v2 := &FileHeader{Version: Version2}
	if v2.DataStartOffset() != int64(FileHeaderSize) {
		t.Errorf("V2 offset: expected %d, got %d", int64(FileHeaderSize), v2.DataStartOffset())
	}
}

func TestEntry_Size(t *testing.T) {
	entry := Entry{
		Operation: OpInsert,
		Key:       "test",      // 4 bytes
		Data:      []byte("x"), // 1 byte
	}

	// Size = op(1) + keyLen(2) + key(4) + dataLen(4) + data(1) = 12
	expectedSize := 1 + 2 + 4 + 4 + 1
	if entry.Size() != expectedSize {
		t.Errorf("expected size %d, got %d", expectedSize, entry.Size())
	}
}
