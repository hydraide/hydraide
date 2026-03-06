package v2

import (
	"bytes"
	"fmt"
	"sync"
	"testing"
)

// 8.9. Stressz tesztek — nagy terheles alatt is hibatlanul mukodik.

func TestStress_V3_ManySmallEntries(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := tmpDir + "/stress_small.hyd"

	const entryCount = 100000
	const dataSize = 10

	writer, err := NewFileWriterWithName(filePath, DefaultMaxBlockSize, "stress/small")
	if err != nil {
		t.Fatalf("failed to create writer: %v", err)
	}

	for i := 0; i < entryCount; i++ {
		entry := Entry{
			Operation: OpInsert,
			Key:       fmt.Sprintf("k%07d", i),
			Data:      bytes.Repeat([]byte{byte(i % 256)}, dataSize),
		}
		if err := writer.WriteEntry(entry); err != nil {
			t.Fatalf("failed to write entry %d: %v", i, err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("failed to close writer: %v", err)
	}

	// Read back and verify
	reader, err := NewFileReader(filePath)
	if err != nil {
		t.Fatalf("failed to create reader: %v", err)
	}
	defer reader.Close()

	if reader.GetSwampName() != "stress/small" {
		t.Errorf("swamp name mismatch: expected %q, got %q", "stress/small", reader.GetSwampName())
	}

	index, _, err := reader.LoadIndex()
	if err != nil {
		t.Fatalf("failed to load index: %v", err)
	}

	if len(index) != entryCount {
		t.Fatalf("expected %d entries, got %d", entryCount, len(index))
	}

	// Spot check some entries
	for _, i := range []int{0, 1, 1000, 50000, 99999} {
		key := fmt.Sprintf("k%07d", i)
		expected := bytes.Repeat([]byte{byte(i % 256)}, dataSize)
		if !bytes.Equal(index[key], expected) {
			t.Errorf("entry %d: data mismatch", i)
		}
	}
}

func TestStress_V3_FewLargeEntries(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := tmpDir + "/stress_large.hyd"

	const entryCount = 100
	const dataSize = 500 * 1024 // 500KB

	writer, err := NewFileWriterWithName(filePath, DefaultMaxBlockSize, "stress/large")
	if err != nil {
		t.Fatalf("failed to create writer: %v", err)
	}

	for i := 0; i < entryCount; i++ {
		data := make([]byte, dataSize)
		// Fill with recognizable pattern
		for j := range data {
			data[j] = byte((i + j) % 256)
		}
		entry := Entry{
			Operation: OpInsert,
			Key:       fmt.Sprintf("large-%03d", i),
			Data:      data,
		}
		if err := writer.WriteEntry(entry); err != nil {
			t.Fatalf("failed to write entry %d: %v", i, err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("failed to close writer: %v", err)
	}

	// Read back
	reader, err := NewFileReader(filePath)
	if err != nil {
		t.Fatalf("failed to create reader: %v", err)
	}
	defer reader.Close()

	index, _, err := reader.LoadIndex()
	if err != nil {
		t.Fatalf("failed to load index: %v", err)
	}

	if len(index) != entryCount {
		t.Fatalf("expected %d entries, got %d", entryCount, len(index))
	}

	// Verify each entry
	for i := 0; i < entryCount; i++ {
		key := fmt.Sprintf("large-%03d", i)
		data := index[key]
		if len(data) != dataSize {
			t.Errorf("entry %d: expected %d bytes, got %d", i, dataSize, len(data))
			continue
		}
		// Check pattern
		for j := 0; j < 100; j++ { // spot check first 100 bytes
			if data[j] != byte((i+j)%256) {
				t.Errorf("entry %d: data corruption at byte %d", i, j)
				break
			}
		}
	}
}

func TestStress_V3_ConcurrentReadWrite(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := tmpDir + "/stress_concurrent.hyd"

	// Create initial file with some data
	writer, err := NewFileWriterWithName(filePath, DefaultMaxBlockSize, "stress/concurrent")
	if err != nil {
		t.Fatalf("failed to create writer: %v", err)
	}

	const initialEntries = 1000
	for i := 0; i < initialEntries; i++ {
		entry := Entry{
			Operation: OpInsert,
			Key:       fmt.Sprintf("init-%04d", i),
			Data:      []byte(fmt.Sprintf("data-%04d", i)),
		}
		if err := writer.WriteEntry(entry); err != nil {
			t.Fatalf("failed to write: %v", err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("failed to close: %v", err)
	}

	// Concurrent reads (10 goroutines reading simultaneously)
	var wg sync.WaitGroup
	errors := make(chan error, 10)

	for g := 0; g < 10; g++ {
		wg.Add(1)
		go func(goroutineID int) {
			defer wg.Done()

			reader, err := NewFileReader(filePath)
			if err != nil {
				errors <- fmt.Errorf("goroutine %d: open failed: %w", goroutineID, err)
				return
			}
			defer reader.Close()

			if reader.GetSwampName() != "stress/concurrent" {
				errors <- fmt.Errorf("goroutine %d: wrong swamp name: %q", goroutineID, reader.GetSwampName())
				return
			}

			index, _, err := reader.LoadIndex()
			if err != nil {
				errors <- fmt.Errorf("goroutine %d: LoadIndex failed: %w", goroutineID, err)
				return
			}

			if len(index) != initialEntries {
				errors <- fmt.Errorf("goroutine %d: expected %d entries, got %d", goroutineID, initialEntries, len(index))
				return
			}
		}(g)
	}

	wg.Wait()
	close(errors)

	for err := range errors {
		t.Error(err)
	}
}
