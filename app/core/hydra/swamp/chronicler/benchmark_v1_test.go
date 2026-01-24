package chronicler

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/hydraide/hydraide/app/core/filesystem"
	"github.com/hydraide/hydraide/app/core/hydra/swamp/beacon"
	"github.com/hydraide/hydraide/app/core/hydra/swamp/metadata"
	"github.com/hydraide/hydraide/app/core/hydra/swamp/treasure"
	"github.com/hydraide/hydraide/app/core/hydra/swamp/treasure/guard"
)

// BenchmarkV1_Insert100K benchmarks inserting 100,000 treasures with V1 chronicler
func BenchmarkV1_Insert100K(b *testing.B) {
	for n := 0; n < b.N; n++ {
		b.StopTimer()
		tmpDir := b.TempDir()
		swampPath := filepath.Join(tmpDir, "test-swamp")

		fs := filesystem.New()
		meta := metadata.New(swampPath)
		chron := New(swampPath, 250*1024, 10, fs, meta)
		chron.CreateDirectoryIfNotExists()

		treasures := make([]treasure.Treasure, 100000)

		// Generate treasures
		for i := 0; i < 100000; i++ {
			t := treasure.New(nil)
			guardID := t.StartTreasureGuard(false, guard.BodyAuthID)
			t.BodySetKey(guardID, fmt.Sprintf("key-%d", i))
			t.SetContentString(guardID, fmt.Sprintf("test-data-%d-with-some-extra-content-to-simulate-real-data", i))
			t.ReleaseTreasureGuard(guardID)
			treasures[i] = t
		}

		b.StartTimer()
		chron.Write(treasures)
		b.StopTimer()

		// Record metrics
		totalSize := calculateDirSize(swampPath)
		fileCount := countFiles(swampPath)
		b.ReportMetric(float64(totalSize), "bytes")
		b.ReportMetric(float64(totalSize)/100000, "bytes/treasure")
		b.ReportMetric(float64(fileCount), "files")
	}
}

// BenchmarkV1_Update10K benchmarks updating 10,000 treasures from existing 100K
func BenchmarkV1_Update10K(b *testing.B) {
	// Setup: create 100K treasures first
	tmpDir := b.TempDir()
	swampPath := filepath.Join(tmpDir, "test-swamp")

	fs := filesystem.New()
	meta := metadata.New(swampPath)
	chron := New(swampPath, 250*1024, 10, fs, meta)
	chron.CreateDirectoryIfNotExists()

	// Initial insert
	initialTreasures := make([]treasure.Treasure, 100000)
	for i := 0; i < 100000; i++ {
		t := treasure.New(nil)
		guardID := t.StartTreasureGuard(false, guard.BodyAuthID)
		t.BodySetKey(guardID, fmt.Sprintf("key-%d", i))
		t.SetContentString(guardID, fmt.Sprintf("initial-data-%d", i))
		t.ReleaseTreasureGuard(guardID)
		initialTreasures[i] = t
	}
	chron.Write(initialTreasures)

	sizeBefore := calculateDirSize(swampPath)
	fileCountBefore := countFiles(swampPath)

	b.ResetTimer()

	for n := 0; n < b.N; n++ {
		b.StopTimer()
		// Update 10K treasures
		updateTreasures := make([]treasure.Treasure, 10000)
		for i := 0; i < 10000; i++ {
			t := treasure.New(nil)
			guardID := t.StartTreasureGuard(false, guard.BodyAuthID)
			t.BodySetKey(guardID, fmt.Sprintf("key-%d", i))
			t.SetContentString(guardID, fmt.Sprintf("updated-data-%d-iteration-%d", i, n))
			t.ReleaseTreasureGuard(guardID)
			updateTreasures[i] = t
		}

		b.StartTimer()
		chron.Write(updateTreasures)
		b.StopTimer()
	}

	sizeAfter := calculateDirSize(swampPath)
	fileCountAfter := countFiles(swampPath)

	b.ReportMetric(float64(sizeBefore), "bytes_before")
	b.ReportMetric(float64(sizeAfter), "bytes_after")
	b.ReportMetric(float64(sizeAfter-sizeBefore), "bytes_growth")
	b.ReportMetric(float64(fileCountBefore), "files_before")
	b.ReportMetric(float64(fileCountAfter), "files_after")
}

// BenchmarkV1_Delete10K benchmarks deleting 10,000 treasures
func BenchmarkV1_Delete10K(b *testing.B) {
	// Setup: create 100K treasures first
	tmpDir := b.TempDir()
	swampPath := filepath.Join(tmpDir, "test-swamp")

	fs := filesystem.New()
	meta := metadata.New(swampPath)
	chron := New(swampPath, 250*1024, 10, fs, meta)
	chron.CreateDirectoryIfNotExists()

	// Initial insert
	initialTreasures := make([]treasure.Treasure, 100000)
	for i := 0; i < 100000; i++ {
		t := treasure.New(nil)
		guardID := t.StartTreasureGuard(false, guard.BodyAuthID)
		t.BodySetKey(guardID, fmt.Sprintf("key-%d", i))
		t.SetContentString(guardID, fmt.Sprintf("test-data-%d", i))
		t.ReleaseTreasureGuard(guardID)
		initialTreasures[i] = t
	}
	chron.Write(initialTreasures)

	b.ResetTimer()

	for n := 0; n < b.N; n++ {
		b.StopTimer()
		// Delete 10K treasures
		deleteTreasures := make([]treasure.Treasure, 10000)
		for i := 0; i < 10000; i++ {
			t := treasure.New(nil)
			guardID := t.StartTreasureGuard(false, guard.BodyAuthID)
			t.BodySetKey(guardID, fmt.Sprintf("key-%d", i))
			t.BodySetForDeletion(guardID, "benchmark", true)
			t.ReleaseTreasureGuard(guardID)
			deleteTreasures[i] = t
		}

		b.StartTimer()
		chron.Write(deleteTreasures)
		b.StopTimer()
	}
}

// BenchmarkV1_Read100K benchmarks reading all 100K treasures
func BenchmarkV1_Read100K(b *testing.B) {
	// Setup: create 100K treasures first
	tmpDir := b.TempDir()
	swampPath := filepath.Join(tmpDir, "test-swamp")

	fs := filesystem.New()
	meta := metadata.New(swampPath)
	chron := New(swampPath, 250*1024, 10, fs, meta)
	chron.CreateDirectoryIfNotExists()

	// Initial insert
	initialTreasures := make([]treasure.Treasure, 100000)
	for i := 0; i < 100000; i++ {
		t := treasure.New(nil)
		guardID := t.StartTreasureGuard(false, guard.BodyAuthID)
		t.BodySetKey(guardID, fmt.Sprintf("key-%d", i))
		t.SetContentString(guardID, fmt.Sprintf("test-data-%d", i))
		t.ReleaseTreasureGuard(guardID)
		initialTreasures[i] = t
	}
	chron.Write(initialTreasures)

	fileCount := countFiles(swampPath)
	totalSize := calculateDirSize(swampPath)

	b.ResetTimer()

	for n := 0; n < b.N; n++ {
		newBeacon := beacon.New()
		chron.Load(newBeacon)
	}

	b.ReportMetric(float64(totalSize), "bytes")
	b.ReportMetric(float64(fileCount), "files")
}

// BenchmarkV1_MixedWorkload benchmarks a realistic mixed workload
func BenchmarkV1_MixedWorkload(b *testing.B) {
	tmpDir := b.TempDir()
	swampPath := filepath.Join(tmpDir, "test-swamp")

	fs := filesystem.New()
	meta := metadata.New(swampPath)
	chron := New(swampPath, 250*1024, 10, fs, meta)
	chron.CreateDirectoryIfNotExists()

	// Initial 100K
	initialTreasures := make([]treasure.Treasure, 100000)
	for i := 0; i < 100000; i++ {
		t := treasure.New(nil)
		guardID := t.StartTreasureGuard(false, guard.BodyAuthID)
		t.BodySetKey(guardID, fmt.Sprintf("key-%d", i))
		t.SetContentString(guardID, fmt.Sprintf("test-data-%d", i))
		t.ReleaseTreasureGuard(guardID)
		initialTreasures[i] = t
	}
	chron.Write(initialTreasures)

	sizeBefore := calculateDirSize(swampPath)
	fileCountBefore := countFiles(swampPath)

	b.ResetTimer()

	for n := 0; n < b.N; n++ {
		b.StopTimer()
		mixedTreasures := make([]treasure.Treasure, 10000)

		// 50% updates, 30% inserts, 20% deletes
		for i := 0; i < 10000; i++ {
			t := treasure.New(nil)
			guardID := t.StartTreasureGuard(false, guard.BodyAuthID)

			if i < 5000 {
				// Update existing
				t.BodySetKey(guardID, fmt.Sprintf("key-%d", i))
				t.SetContentString(guardID, fmt.Sprintf("updated-data-%d", n))
			} else if i < 8000 {
				// Insert new
				t.BodySetKey(guardID, fmt.Sprintf("key-new-%d-%d", n, i))
				t.SetContentString(guardID, "new-data")
			} else {
				// Delete
				t.BodySetKey(guardID, fmt.Sprintf("key-%d", 50000+i))
				t.BodySetForDeletion(guardID, "benchmark", true)
			}

			t.ReleaseTreasureGuard(guardID)
			mixedTreasures[i] = t
		}

		b.StartTimer()
		chron.Write(mixedTreasures)
		b.StopTimer()
	}

	sizeAfter := calculateDirSize(swampPath)
	fileCountAfter := countFiles(swampPath)

	b.ReportMetric(float64(sizeBefore), "bytes_before")
	b.ReportMetric(float64(sizeAfter), "bytes_after")
	b.ReportMetric(float64(fileCountBefore), "files_before")
	b.ReportMetric(float64(fileCountAfter), "files_after")
}

// Helper: calculate directory size recursively
func calculateDirSize(path string) int64 {
	var size int64
	filepath.Walk(path, func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			size += info.Size()
		}
		return nil
	})
	return size
}

// Helper: count files recursively
func countFiles(path string) int {
	count := 0
	filepath.Walk(path, func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			count++
		}
		return nil
	})
	return count
}
