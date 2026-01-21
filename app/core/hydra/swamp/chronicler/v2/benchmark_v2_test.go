package v2

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
)

// BenchmarkV2_Insert100K benchmarks inserting 100,000 entries with V2 chronicler
func BenchmarkV2_Insert100K(b *testing.B) {
	for n := 0; n < b.N; n++ {
		b.StopTimer()
		tmpDir := b.TempDir()
		hydFile := filepath.Join(tmpDir, "test.hyd")

		writer, err := NewFileWriter(hydFile, DefaultMaxBlockSize)
		if err != nil {
			b.Fatal(err)
		}

		entries := make([]Entry, 100000)
		for i := 0; i < 100000; i++ {
			entries[i] = Entry{
				Operation: OpInsert,
				Key:       fmt.Sprintf("key-%d", i),
				Data:      []byte(fmt.Sprintf(`{"data":"test-data-%d","index":%d,"timestamp":%d}`, i, i, time.Now().Unix())),
			}
		}

		b.StartTimer()
		for _, entry := range entries {
			writer.WriteEntry(entry)
		}
		writer.Close()
		b.StopTimer()

		// Record metrics
		fileInfo, _ := os.Stat(hydFile)
		totalSize := fileInfo.Size()
		b.ReportMetric(float64(totalSize), "bytes")
		b.ReportMetric(float64(totalSize)/100000, "bytes/entry")
	}
}

// BenchmarkV2_Update10K benchmarks updating 10,000 entries from existing 100K
func BenchmarkV2_Update10K(b *testing.B) {
	// Setup: create 100K entries first
	tmpDir := b.TempDir()
	hydFile := filepath.Join(tmpDir, "test.hyd")

	writer, _ := NewFileWriter(hydFile, DefaultMaxBlockSize)
	for i := 0; i < 100000; i++ {
		writer.WriteEntry(Entry{
			Operation: OpInsert,
			Key:       fmt.Sprintf("key-%d", i),
			Data:      []byte(fmt.Sprintf(`{"data":"initial-%d","index":%d}`, i, i)),
		})
	}
	writer.Close()

	fileInfoBefore, _ := os.Stat(hydFile)
	sizeBefore := fileInfoBefore.Size()

	b.ResetTimer()

	for n := 0; n < b.N; n++ {
		b.StopTimer()
		writer, _ := NewFileWriter(hydFile, DefaultMaxBlockSize)

		updateEntries := make([]Entry, 10000)
		for i := 0; i < 10000; i++ {
			updateEntries[i] = Entry{
				Operation: OpUpdate,
				Key:       fmt.Sprintf("key-%d", i),
				Data:      []byte(fmt.Sprintf(`{"data":"updated-%d-%d","index":%d,"timestamp":%d}`, i, n, i, time.Now().Unix())),
			}
		}

		b.StartTimer()
		for _, entry := range updateEntries {
			writer.WriteEntry(entry)
		}
		writer.Close()
		b.StopTimer()
	}

	fileInfoAfter, _ := os.Stat(hydFile)
	sizeAfter := fileInfoAfter.Size()

	b.ReportMetric(float64(sizeBefore), "bytes_before")
	b.ReportMetric(float64(sizeAfter), "bytes_after")
	b.ReportMetric(float64(sizeAfter-sizeBefore), "bytes_growth")
}

// BenchmarkV2_Delete10K benchmarks deleting 10,000 entries
func BenchmarkV2_Delete10K(b *testing.B) {
	// Setup: create 100K entries first
	tmpDir := b.TempDir()
	hydFile := filepath.Join(tmpDir, "test.hyd")

	writer, _ := NewFileWriter(hydFile, DefaultMaxBlockSize)
	for i := 0; i < 100000; i++ {
		writer.WriteEntry(Entry{
			Operation: OpInsert,
			Key:       fmt.Sprintf("key-%d", i),
			Data:      []byte(fmt.Sprintf(`{"data":"test-%d"}`, i)),
		})
	}
	writer.Close()

	b.ResetTimer()

	for n := 0; n < b.N; n++ {
		b.StopTimer()
		writer, _ := NewFileWriter(hydFile, DefaultMaxBlockSize)

		deleteEntries := make([]Entry, 10000)
		for i := 0; i < 10000; i++ {
			deleteEntries[i] = Entry{
				Operation: OpDelete,
				Key:       fmt.Sprintf("key-%d", i),
				Data:      nil,
			}
		}

		b.StartTimer()
		for _, entry := range deleteEntries {
			writer.WriteEntry(entry)
		}
		writer.Close()
		b.StopTimer()
	}
}

// BenchmarkV2_Read100K benchmarks reading all 100K entries
func BenchmarkV2_Read100K(b *testing.B) {
	// Setup: create 100K entries first
	tmpDir := b.TempDir()
	hydFile := filepath.Join(tmpDir, "test.hyd")

	writer, _ := NewFileWriter(hydFile, DefaultMaxBlockSize)
	for i := 0; i < 100000; i++ {
		writer.WriteEntry(Entry{
			Operation: OpInsert,
			Key:       fmt.Sprintf("key-%d", i),
			Data:      []byte(fmt.Sprintf(`{"data":"test-%d"}`, i)),
		})
	}
	writer.Close()

	b.ResetTimer()

	for n := 0; n < b.N; n++ {
		reader, _ := NewFileReader(hydFile)
		reader.LoadIndex()
		reader.Close()
	}
}

// BenchmarkV2_MixedWorkload benchmarks a realistic mixed workload
func BenchmarkV2_MixedWorkload(b *testing.B) {
	tmpDir := b.TempDir()
	hydFile := filepath.Join(tmpDir, "test.hyd")

	// Initial 100K
	writer, _ := NewFileWriter(hydFile, DefaultMaxBlockSize)
	for i := 0; i < 100000; i++ {
		writer.WriteEntry(Entry{
			Operation: OpInsert,
			Key:       fmt.Sprintf("key-%d", i),
			Data:      []byte(fmt.Sprintf(`{"data":"test-%d"}`, i)),
		})
	}
	writer.Close()

	fileInfoBefore, _ := os.Stat(hydFile)
	sizeBefore := fileInfoBefore.Size()

	b.ResetTimer()

	for n := 0; n < b.N; n++ {
		b.StopTimer()
		writer, _ := NewFileWriter(hydFile, DefaultMaxBlockSize)

		mixedEntries := make([]Entry, 10000)

		// 50% updates, 30% inserts, 20% deletes
		for i := 0; i < 10000; i++ {
			if i < 5000 {
				// Update existing
				mixedEntries[i] = Entry{
					Operation: OpUpdate,
					Key:       fmt.Sprintf("key-%d", i),
					Data:      []byte(fmt.Sprintf(`{"data":"updated-%d"}`, n)),
				}
			} else if i < 8000 {
				// Insert new
				mixedEntries[i] = Entry{
					Operation: OpInsert,
					Key:       fmt.Sprintf("key-new-%s", uuid.New().String()),
					Data:      []byte(`{"data":"new"}`),
				}
			} else {
				// Delete
				mixedEntries[i] = Entry{
					Operation: OpDelete,
					Key:       fmt.Sprintf("key-%d", 50000+i),
					Data:      nil,
				}
			}
		}

		b.StartTimer()
		for _, entry := range mixedEntries {
			writer.WriteEntry(entry)
		}
		writer.Close()
		b.StopTimer()
	}

	fileInfoAfter, _ := os.Stat(hydFile)
	sizeAfter := fileInfoAfter.Size()

	b.ReportMetric(float64(sizeBefore), "bytes_before")
	b.ReportMetric(float64(sizeAfter), "bytes_after")
	b.ReportMetric(1.0, "files") // Always 1 file in V2
}

// BenchmarkV2_CompactionNeeded benchmarks compaction on fragmented data
func BenchmarkV2_CompactionNeeded(b *testing.B) {
	tmpDir := b.TempDir()
	hydFile := filepath.Join(tmpDir, "test.hyd")

	// Create heavily fragmented data: 100K entries, then update all 10 times
	writer, _ := NewFileWriter(hydFile, DefaultMaxBlockSize)
	for i := 0; i < 100000; i++ {
		writer.WriteEntry(Entry{
			Operation: OpInsert,
			Key:       fmt.Sprintf("key-%d", i),
			Data:      []byte(fmt.Sprintf(`{"data":"v0-%d"}`, i)),
		})
	}

	// Create fragmentation by updating each key 10 times
	for v := 1; v <= 10; v++ {
		for i := 0; i < 100000; i++ {
			writer.WriteEntry(Entry{
				Operation: OpUpdate,
				Key:       fmt.Sprintf("key-%d", i),
				Data:      []byte(fmt.Sprintf(`{"data":"v%d-%d"}`, v, i)),
			})
		}
	}
	writer.Close()


	// Calculate fragmentation
	reader, _ := NewFileReader(hydFile)
	fragmentation, liveCount, totalCount, _ := reader.CalculateFragmentation()
	reader.Close()

	b.ResetTimer()

	for n := 0; n < b.N; n++ {
		compactor := NewCompactor(hydFile, DefaultMaxBlockSize, 0.5)
		result, _ := compactor.Compact()

		b.ReportMetric(fragmentation*100, "fragmentation_%")
		b.ReportMetric(float64(liveCount), "live_entries")
		b.ReportMetric(float64(totalCount), "total_entries")
		b.ReportMetric(float64(result.OldFileSize), "size_before_compact")
		b.ReportMetric(float64(result.NewFileSize), "size_after_compact")
		b.ReportMetric(float64(result.OldFileSize-result.NewFileSize), "bytes_saved")
	}
}

// BenchmarkV2_BlockSizes tests different block sizes
func BenchmarkV2_BlockSizes(b *testing.B) {
	blockSizes := []int{8 * 1024, 16 * 1024, 32 * 1024, 64 * 1024, 128 * 1024}

	for _, blockSize := range blockSizes {
		b.Run(fmt.Sprintf("BlockSize_%dKB", blockSize/1024), func(b *testing.B) {
			for n := 0; n < b.N; n++ {
				b.StopTimer()
				tmpDir := b.TempDir()
				hydFile := filepath.Join(tmpDir, "test.hyd")

				writer, _ := NewFileWriter(hydFile, blockSize)

				b.StartTimer()
				for i := 0; i < 10000; i++ {
					writer.WriteEntry(Entry{
						Operation: OpInsert,
						Key:       fmt.Sprintf("key-%d", i),
						Data:      []byte(fmt.Sprintf(`{"data":"test-%d"}`, i)),
					})
				}
				writer.Close()
				b.StopTimer()

				fileInfo, _ := os.Stat(hydFile)
				b.ReportMetric(float64(fileInfo.Size()), "bytes")
			}
		})
	}
}
