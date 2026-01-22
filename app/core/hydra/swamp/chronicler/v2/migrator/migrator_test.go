package migrator

import (
	"os"
	"path/filepath"
	"testing"

	v2 "github.com/hydraide/hydraide/app/core/hydra/swamp/chronicler/v2"
)

func TestByteReader(t *testing.T) {
	// Test data: two segments
	// Segment 1: length=5, data="hello"
	// Segment 2: length=5, data="world"
	data := []byte{
		0x05, 0x00, 0x00, 0x00, // length: 5 (LittleEndian)
		'h', 'e', 'l', 'l', 'o',
		0x05, 0x00, 0x00, 0x00, // length: 5 (LittleEndian)
		'w', 'o', 'r', 'l', 'd',
	}

	reader := NewByteReader(data)

	// Read first segment length
	len1, err := reader.ReadUint32()
	if err != nil {
		t.Fatalf("ReadUint32 failed: %v", err)
	}
	if len1 != 5 {
		t.Errorf("expected length 5, got %d", len1)
	}

	// Read first segment data
	seg1, err := reader.ReadBytes(int(len1))
	if err != nil {
		t.Fatalf("ReadBytes failed: %v", err)
	}
	if string(seg1) != "hello" {
		t.Errorf("expected 'hello', got '%s'", string(seg1))
	}

	// Read second segment length
	len2, err := reader.ReadUint32()
	if err != nil {
		t.Fatalf("ReadUint32 failed: %v", err)
	}
	if len2 != 5 {
		t.Errorf("expected length 5, got %d", len2)
	}

	// Read second segment data
	seg2, err := reader.ReadBytes(int(len2))
	if err != nil {
		t.Fatalf("ReadBytes failed: %v", err)
	}
	if string(seg2) != "world" {
		t.Errorf("expected 'world', got '%s'", string(seg2))
	}

	// Should have no remaining bytes
	if reader.Remaining() != 0 {
		t.Errorf("expected 0 remaining, got %d", reader.Remaining())
	}
}

func TestV1FileParser(t *testing.T) {
	// Create test data with two segments
	data := []byte{
		0x03, 0x00, 0x00, 0x00, // length: 3 (LittleEndian)
		'a', 'b', 'c',
		0x02, 0x00, 0x00, 0x00, // length: 2 (LittleEndian)
		'x', 'y',
	}

	parser := NewV1FileParser(data)
	segments, err := parser.ParseSegments()

	if err != nil {
		t.Fatalf("ParseSegments failed: %v", err)
	}

	if len(segments) != 2 {
		t.Fatalf("expected 2 segments, got %d", len(segments))
	}

	if string(segments[0]) != "abc" {
		t.Errorf("expected 'abc', got '%s'", string(segments[0]))
	}

	if string(segments[1]) != "xy" {
		t.Errorf("expected 'xy', got '%s'", string(segments[1]))
	}
}

func TestV1FileParser_Empty(t *testing.T) {
	parser := NewV1FileParser(nil)
	segments, err := parser.ParseSegments()

	if err != nil {
		t.Fatalf("ParseSegments failed: %v", err)
	}

	if len(segments) != 0 {
		t.Errorf("expected 0 segments, got %d", len(segments))
	}
}

func TestV1FileParser_ZeroLengthSegment(t *testing.T) {
	// Zero-length segments should be skipped
	data := []byte{
		0x00, 0x00, 0x00, 0x00, // length: 0 (LittleEndian) (skip)
		0x02, 0x00, 0x00, 0x00, // length: 2 (LittleEndian)
		'o', 'k',
	}

	parser := NewV1FileParser(data)
	segments, err := parser.ParseSegments()

	if err != nil {
		t.Fatalf("ParseSegments failed: %v", err)
	}

	if len(segments) != 1 {
		t.Fatalf("expected 1 segment, got %d", len(segments))
	}

	if string(segments[0]) != "ok" {
		t.Errorf("expected 'ok', got '%s'", string(segments[0]))
	}
}

func TestMigratorConfig(t *testing.T) {
	// Test with empty data path
	_, err := New(Config{DataPath: ""})
	if err == nil {
		t.Error("expected error for empty data path")
	}

	// Test with valid config
	tmpDir := t.TempDir()
	m, err := New(Config{DataPath: tmpDir})
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	if m == nil {
		t.Fatal("migrator is nil")
	}
}

func TestMigrator_NoSwamps(t *testing.T) {
	tmpDir := t.TempDir()

	m, err := New(Config{
		DataPath: tmpDir,
		DryRun:   true,
	})
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	result, err := m.Run()
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	if result.TotalSwamps != 0 {
		t.Errorf("expected 0 swamps, got %d", result.TotalSwamps)
	}
}

func TestMigrator_WriteAndVerify(t *testing.T) {
	tmpDir := t.TempDir()
	hydFile := filepath.Join(tmpDir, "test.hyd")

	// Create migrator to test writeV2File
	m, _ := New(Config{DataPath: tmpDir})

	entries := []v2.Entry{
		{Operation: v2.OpInsert, Key: "key1", Data: []byte("data1")},
		{Operation: v2.OpInsert, Key: "key2", Data: []byte("data2")},
	}

	// Write V2 file with swamp name
	err := m.writeV2File(hydFile, entries, "test/swamp/name")
	if err != nil {
		t.Fatalf("writeV2File failed: %v", err)
	}

	// Verify the file exists
	if _, err := os.Stat(hydFile); os.IsNotExist(err) {
		t.Error("expected file to exist after write")
	}

	// Verify the file
	err = m.verifyMigration(hydFile, entries)
	if err != nil {
		t.Fatalf("verifyMigration failed: %v", err)
	}

	// Verify file exists
	if _, err := os.Stat(hydFile); os.IsNotExist(err) {
		t.Error("expected file to exist")
	}
}

func TestResult_Summary(t *testing.T) {
	result := &Result{
		TotalSwamps:      100,
		SuccessfulSwamps: 98,
		FailedSwamps:     []FailedSwamp{{Path: "/test", Error: "test error", Phase: "load"}},
		TotalEntries:     5000,
		OldSizeBytes:     1024 * 1024 * 100, // 100MB
		NewSizeBytes:     1024 * 1024 * 60,  // 60MB
		DryRun:           false,
	}

	summary := result.Summary()

	if summary == "" {
		t.Error("summary should not be empty")
	}

	// Should contain key information
	if len(summary) < 100 {
		t.Error("summary seems too short")
	}
}

func TestResult_SummaryDryRun(t *testing.T) {
	result := &Result{
		TotalSwamps:      50,
		SuccessfulSwamps: 50,
		TotalEntries:     1000,
		DryRun:           true,
	}

	summary := result.Summary()

	if summary == "" {
		t.Error("summary should not be empty")
	}
}

func TestResult_ToJSON(t *testing.T) {
	result := &Result{
		TotalSwamps:      10,
		SuccessfulSwamps: 10,
	}

	jsonData, err := result.ToJSON()
	if err != nil {
		t.Fatalf("ToJSON failed: %v", err)
	}

	if len(jsonData) == 0 {
		t.Error("JSON output is empty")
	}
}

func TestFormatBytes(t *testing.T) {
	testCases := []struct {
		bytes    int64
		expected string
	}{
		{500, "500 B"},
		{1024, "1.0 KB"},
		{1536, "1.5 KB"},
		{1048576, "1.0 MB"},
		{1073741824, "1.0 GB"},
	}

	for _, tc := range testCases {
		result := formatBytes(tc.bytes)
		if result != tc.expected {
			t.Errorf("formatBytes(%d) = %s, expected %s", tc.bytes, result, tc.expected)
		}
	}
}
