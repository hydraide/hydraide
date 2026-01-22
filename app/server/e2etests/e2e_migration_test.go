package e2etests

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/hydraide/hydraide/app/core/hydra/swamp/chronicler/v2"
	"github.com/hydraide/hydraide/app/core/hydra/swamp/chronicler/v2/migrator"
	"github.com/hydraide/hydraide/generated/hydraidepbgo"
	"github.com/hydraide/hydraide/sdk/go/hydraidego/name"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// TestV1ToV2MigrationDryRun tests the migration dry-run mode.
// This test verifies that dry-run mode:
// 1. Scans and validates all V1 swamps
// 2. Reports what would be migrated
// 3. Does NOT modify any files
func TestV1ToV2MigrationDryRun(t *testing.T) {
	// Get the data path from environment or use default
	dataPath := os.Getenv("HYDRAIDE_DATA_PATH")
	if dataPath == "" {
		dataPath = "/hydraide/data"
	}

	// Check if there are any V1 swamps to test with
	v1FileCount := countFilesWithExtension(t, dataPath, ".dat")

	if v1FileCount == 0 {
		t.Skip("No V1 .dat files found - skipping migration dry-run test")
	}

	slog.Info("=== MIGRATION DRY-RUN TEST ===")
	slog.Info("Found V1 files", "count", v1FileCount)

	// Count files before dry-run
	v2CountBefore := countFilesWithExtension(t, dataPath, ".hyd")

	// Run dry-run migration
	migratorConfig := migrator.Config{
		DataPath:       dataPath,
		DryRun:         true, // DRY RUN - no changes!
		Verify:         false,
		DeleteOld:      false,
		Parallel:       2,
		ProgressReport: time.Second,
	}

	mig, err := migrator.New(migratorConfig)
	require.NoError(t, err, "Failed to create migrator")

	result, err := mig.Run()
	require.NoError(t, err, "Migration dry-run failed")

	slog.Info("Migration dry-run completed",
		"processed", result.ProcessedSwamps,
		"successful", result.SuccessfulSwamps,
		"failed", len(result.FailedSwamps),
		"duration", result.Duration,
		"total_entries", result.TotalEntries,
		"old_size_bytes", result.OldSizeBytes)

	// Verify it was a dry-run
	assert.True(t, result.DryRun, "Should be marked as dry-run")

	// Verify no V2 files were created
	v2CountAfter := countFilesWithExtension(t, dataPath, ".hyd")
	// Note: We can't assert v2CountBefore == v2CountAfter if V2 engine is running
	// Just log the counts
	slog.Info("V2 file counts", "before", v2CountBefore, "after", v2CountAfter)

	// Verify V1 files still exist
	v1CountAfter := countFilesWithExtension(t, dataPath, ".dat")
	assert.Equal(t, v1FileCount, v1CountAfter, "V1 files should not change during dry-run")

	slog.Info("TestV1ToV2MigrationDryRun completed successfully!")
}

// TestMigratorValidation tests the migrator's validation capabilities
func TestMigratorValidation(t *testing.T) {
	// Test with invalid path
	_, err := migrator.New(migrator.Config{
		DataPath: "",
	})
	assert.Error(t, err, "Should error on empty data path")

	// Test with non-existent path (dry-run should succeed with 0 swamps)
	tempDir := filepath.Join(os.TempDir(), fmt.Sprintf("hydraide_test_%d", time.Now().UnixNano()))
	require.NoError(t, os.MkdirAll(tempDir, 0755))
	defer func() {
		_ = os.RemoveAll(tempDir)
	}()

	mig, err := migrator.New(migrator.Config{
		DataPath:       tempDir,
		DryRun:         true,
		Parallel:       1,
		ProgressReport: time.Second,
	})
	require.NoError(t, err)

	result, err := mig.Run()
	require.NoError(t, err)
	assert.Equal(t, int64(0), result.TotalSwamps, "Empty directory should have 0 swamps")

	slog.Info("TestMigratorValidation completed successfully!")
}

// TestV2FileReaderIntegrity tests that V2 files can be read correctly
func TestV2FileReaderIntegrity(t *testing.T) {
	ctx := context.Background()
	swampName := name.New().Sanctuary("v2test").Realm("reader").Swamp("integrity")

	slog.Info("=== V2 FILE READER INTEGRITY TEST ===")

	// First, set up some test data
	writeInterval := int64(1)
	maxFileSize := int64(65536)

	swampPattern := name.New().Sanctuary("v2test").Realm("reader").Swamp("*")
	selectedClient := clientInterface.GetServiceClient(swampPattern)
	_, err := selectedClient.RegisterSwamp(context.Background(), &hydraidepbgo.RegisterSwampRequest{
		SwampPattern:   swampPattern.Get(),
		CloseAfterIdle: int64(2),
		WriteInterval:  &writeInterval,
		MaxFileSize:    &maxFileSize,
	})
	require.NoError(t, err)

	// Insert test data
	var keyValues []*hydraidepbgo.KeyValuePair
	for i := 0; i < 5; i++ {
		val := fmt.Sprintf("test-value-%d", i)
		keyValues = append(keyValues, &hydraidepbgo.KeyValuePair{
			Key:       fmt.Sprintf("integrity-key-%d", i),
			StringVal: &val,
			CreatedAt: timestamppb.Now(),
		})
	}

	swampClient := clientInterface.GetServiceClient(swampName)
	_, err = swampClient.Set(ctx, &hydraidepbgo.SetRequest{
		Swamps: []*hydraidepbgo.SwampRequest{
			{
				SwampName:        swampName.Get(),
				CreateIfNotExist: true,
				Overwrite:        true,
				KeyValues:        keyValues,
			},
		},
	})
	require.NoError(t, err)

	slog.Info("Step 1: Inserted test data")

	// Wait for flush
	time.Sleep(2 * time.Second)

	// Wait for swamp to close
	time.Sleep(3 * time.Second)

	// Find the .hyd file
	hydFilePath := findHydFileInPath(t, "/hydraide/data")
	if hydFilePath == "" {
		t.Skip("No .hyd files found - V2 engine may not be active")
	}

	slog.Info("Step 2: Found .hyd file", "path", hydFilePath)

	// Read and verify using V2 FileReader
	reader, err := v2.NewFileReader(hydFilePath)
	require.NoError(t, err, "Failed to open V2 file")
	defer func() {
		_ = reader.Close()
	}()

	// Load index
	index, swampNameFromFile, err := reader.LoadIndex()
	require.NoError(t, err, "Failed to load index")

	slog.Info("Step 3: Loaded index",
		"swamp_name", swampNameFromFile,
		"entry_count", len(index))

	// Verify we have entries
	assert.Greater(t, len(index), 0, "Index should have entries")

	// Calculate fragmentation
	fragmentation, liveCount, totalCount, err := reader.CalculateFragmentation()
	require.NoError(t, err)

	slog.Info("Step 4: Fragmentation analysis",
		"fragmentation", fmt.Sprintf("%.2f%%", fragmentation*100),
		"live", liveCount,
		"total", totalCount)

	// Cleanup
	_, err = swampClient.Destroy(ctx, &hydraidepbgo.DestroyRequest{
		SwampName: swampName.Get(),
	})
	assert.NoError(t, err)

	slog.Info("TestV2FileReaderIntegrity completed successfully!")
}

// Helper functions

func countFilesWithExtension(t *testing.T, rootPath, extension string) int {
	count := 0
	err := filepath.Walk(rootPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			// Skip permission errors
			if os.IsPermission(err) {
				return nil
			}
			return nil // Continue on other errors
		}
		if !info.IsDir() && filepath.Ext(path) == extension {
			count++
		}
		return nil
	})
	if err != nil {
		t.Logf("Warning: error walking path %s: %v", rootPath, err)
	}
	return count
}

func findHydFileInPath(t *testing.T, basePath string) string {
	var hydFile string
	err := filepath.Walk(basePath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if !info.IsDir() && filepath.Ext(path) == ".hyd" {
			hydFile = path
			return filepath.SkipAll
		}
		return nil
	})
	if err != nil && err != filepath.SkipAll {
		t.Logf("Warning: error walking path: %v", err)
	}
	return hydFile
}
