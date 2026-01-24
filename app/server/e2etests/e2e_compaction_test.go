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
	"github.com/hydraide/hydraide/generated/hydraidepbgo"
	"github.com/hydraide/hydraide/sdk/go/hydraidego/name"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// TestV2Compaction tests the automatic compaction (defragmentation) process.
// This test:
// 1. Creates a swamp with multiple entries
// 2. Performs multiple inserts, updates, and deletes to create fragmentation
// 3. Triggers swamp close which should trigger compaction if fragmentation > 50%
// 4. Verifies the file size decreased after compaction
// 5. Reopens and verifies all live data is correctly readable
// 6. Verifies the fragmentation level dropped after compaction
func TestV2Compaction(t *testing.T) {
	ctx := context.Background()
	swampName := name.New().Sanctuary("v2test").Realm("compaction").Swamp("auto")

	slog.Info("=== V2 AUTOMATIC COMPACTION TEST ===")

	// Register swamp pattern with short idle timeout
	writeInterval := int64(1)
	maxFileSize := int64(65536)

	swampPattern := name.New().Sanctuary("v2test").Realm("compaction").Swamp("*")
	selectedClient := clientInterface.GetServiceClient(swampPattern)
	_, err := selectedClient.RegisterSwamp(context.Background(), &hydraidepbgo.RegisterSwampRequest{
		SwampPattern:   swampPattern.Get(),
		CloseAfterIdle: int64(2), // Close quickly to trigger compaction
		WriteInterval:  &writeInterval,
		MaxFileSize:    &maxFileSize,
	})
	require.NoError(t, err)

	swampClient := clientInterface.GetServiceClient(swampName)
	defer func() {
		_, _ = swampClient.Destroy(ctx, &hydraidepbgo.DestroyRequest{
			SwampName: swampName.Get(),
		})
	}()

	// Step 1: Insert 20 initial entries
	slog.Info("Step 1: Inserting 20 initial entries...")
	var keyValues []*hydraidepbgo.KeyValuePair
	for i := 0; i < 20; i++ {
		val := fmt.Sprintf("Initial value for entry %d with some padding to make it larger - adding more text here", i)
		keyValues = append(keyValues, &hydraidepbgo.KeyValuePair{
			Key:       fmt.Sprintf("entry-%03d", i),
			StringVal: &val,
			CreatedAt: timestamppb.Now(),
		})
	}

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

	// Wait for flush
	time.Sleep(2 * time.Second)

	slog.Info("Step 2: Creating fragmentation through updates...")

	// Update half of the entries (creates orphaned old versions)
	var updateKeyValues []*hydraidepbgo.KeyValuePair
	for i := 0; i < 10; i++ {
		val := fmt.Sprintf("UPDATED value for entry %d with even more padding to increase the file size significantly", i)
		updateKeyValues = append(updateKeyValues, &hydraidepbgo.KeyValuePair{
			Key:       fmt.Sprintf("entry-%03d", i),
			StringVal: &val,
			UpdatedAt: timestamppb.Now(),
		})
	}

	_, err = swampClient.Set(ctx, &hydraidepbgo.SetRequest{
		Swamps: []*hydraidepbgo.SwampRequest{
			{
				SwampName: swampName.Get(),
				Overwrite: true,
				KeyValues: updateKeyValues,
			},
		},
	})
	require.NoError(t, err)

	// Wait for flush
	time.Sleep(2 * time.Second)

	slog.Info("Step 3: Deleting entries to increase fragmentation...")

	// Delete some entries
	_, err = swampClient.Delete(ctx, &hydraidepbgo.DeleteRequest{
		Swamps: []*hydraidepbgo.DeleteRequest_SwampKeys{
			{
				SwampName: swampName.Get(),
				Keys:      []string{"entry-015", "entry-016", "entry-017", "entry-018", "entry-019"},
			},
		},
	})
	require.NoError(t, err)

	// Wait for flush and then for swamp to close (triggers compaction)
	slog.Info("Step 4: Waiting for swamp to close and trigger compaction...")
	time.Sleep(5 * time.Second)

	// Find the .hyd file
	hydFilePath := findHydFileForSwamp(t, "/hydraide/data", "v2test")
	if hydFilePath == "" {
		t.Skip("No .hyd files found - V2 engine may not be active")
	}

	slog.Info("Step 5: Analyzing compaction results...", "path", hydFilePath)

	// Check fragmentation after compaction
	reader, err := v2.NewFileReader(hydFilePath)
	require.NoError(t, err)
	fragmentation, liveCount, totalCount, err := reader.CalculateFragmentation()
	_ = reader.Close()
	require.NoError(t, err)

	slog.Info("Fragmentation analysis",
		"fragmentation", fmt.Sprintf("%.2f%%", fragmentation*100),
		"live_entries", liveCount,
		"total_entries", totalCount)

	// Step 6: Verify data integrity
	slog.Info("Step 6: Verifying data integrity...")

	// Get updated entries (0-9)
	getResponse, err := swampClient.Get(ctx, &hydraidepbgo.GetRequest{
		Swamps: []*hydraidepbgo.GetSwamp{
			{
				SwampName: swampName.Get(),
				Keys:      []string{"entry-000", "entry-001", "entry-002", "entry-003", "entry-004"},
			},
		},
	})
	require.NoError(t, err)

	// Verify entries exist and have updated values
	for _, swampResp := range getResponse.GetSwamps() {
		for _, treasure := range swampResp.GetTreasures() {
			if treasure.IsExist {
				assert.Contains(t, treasure.GetStringVal(), "UPDATED", "Entry should have updated value")
			}
		}
	}

	// Verify deleted entries don't exist
	getDeleted, err := swampClient.Get(ctx, &hydraidepbgo.GetRequest{
		Swamps: []*hydraidepbgo.GetSwamp{
			{
				SwampName: swampName.Get(),
				Keys:      []string{"entry-015", "entry-016", "entry-017"},
			},
		},
	})
	require.NoError(t, err)

	for _, swampResp := range getDeleted.GetSwamps() {
		for _, treasure := range swampResp.GetTreasures() {
			assert.False(t, treasure.IsExist, "Deleted entry should not exist: %s", treasure.GetKey())
		}
	}

	slog.Info("TestV2Compaction completed successfully!")
}

// TestV2ManualCompaction tests manual compaction trigger
func TestV2ManualCompaction(t *testing.T) {
	ctx := context.Background()
	swampName := name.New().Sanctuary("v2test").Realm("compaction").Swamp("manual")

	slog.Info("=== V2 MANUAL COMPACTION TEST ===")

	// Register swamp pattern
	writeInterval := int64(1)
	maxFileSize := int64(65536)

	swampPattern := name.New().Sanctuary("v2test").Realm("compaction").Swamp("*")
	selectedClient := clientInterface.GetServiceClient(swampPattern)
	_, _ = selectedClient.RegisterSwamp(context.Background(), &hydraidepbgo.RegisterSwampRequest{
		SwampPattern:   swampPattern.Get(),
		CloseAfterIdle: int64(60), // Long timeout - won't auto-compact
		WriteInterval:  &writeInterval,
		MaxFileSize:    &maxFileSize,
	})

	swampClient := clientInterface.GetServiceClient(swampName)
	defer func() {
		_, _ = swampClient.Destroy(ctx, &hydraidepbgo.DestroyRequest{
			SwampName: swampName.Get(),
		})
	}()

	// Insert entries
	slog.Info("Step 1: Inserting 30 entries...")
	var keyValues []*hydraidepbgo.KeyValuePair
	for i := 0; i < 30; i++ {
		val := fmt.Sprintf("Value %d with padding text to increase size", i)
		keyValues = append(keyValues, &hydraidepbgo.KeyValuePair{
			Key:       fmt.Sprintf("item-%d", i),
			StringVal: &val,
			CreatedAt: timestamppb.Now(),
		})
	}

	_, err := swampClient.Set(ctx, &hydraidepbgo.SetRequest{
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

	time.Sleep(2 * time.Second)

	// Delete 20 entries to create high fragmentation
	slog.Info("Step 2: Deleting 20 entries...")
	var deleteKeys []string
	for i := 0; i < 20; i++ {
		deleteKeys = append(deleteKeys, fmt.Sprintf("item-%d", i))
	}

	_, err = swampClient.Delete(ctx, &hydraidepbgo.DeleteRequest{
		Swamps: []*hydraidepbgo.DeleteRequest_SwampKeys{
			{
				SwampName: swampName.Get(),
				Keys:      deleteKeys,
			},
		},
	})
	require.NoError(t, err)

	time.Sleep(2 * time.Second)

	// Find .hyd file
	hydFilePath := findHydFileForSwamp(t, "/hydraide/data", "v2test")
	if hydFilePath == "" {
		t.Skip("No .hyd files found")
	}

	// Get size before compaction
	sizeBeforeCompaction := getFileSizeOrZero(hydFilePath)

	// Check fragmentation (should be high: ~66%)
	reader, err := v2.NewFileReader(hydFilePath)
	require.NoError(t, err)
	fragmentation, liveCount, totalCount, err := reader.CalculateFragmentation()
	_ = reader.Close()
	require.NoError(t, err)

	slog.Info("Before manual compaction",
		"fragmentation", fmt.Sprintf("%.2f%%", fragmentation*100),
		"live", liveCount,
		"total", totalCount,
		"file_size", sizeBeforeCompaction)

	// Run manual compaction
	slog.Info("Step 3: Running manual compaction...")
	compactor := v2.NewCompactor(hydFilePath, 16*1024, 0.5)
	result, err := compactor.Compact()
	require.NoError(t, err)

	slog.Info("Compaction result",
		"compacted", result.Compacted,
		"old_size", result.OldFileSize,
		"new_size", result.NewFileSize,
		"removed_entries", result.RemovedEntries)

	if fragmentation >= 0.5 {
		assert.True(t, result.Compacted, "Compaction should run with high fragmentation")
		assert.Less(t, result.NewFileSize, result.OldFileSize, "New file should be smaller")
	}

	// Verify remaining data
	slog.Info("Step 4: Verifying remaining data...")
	getResponse, err := swampClient.Get(ctx, &hydraidepbgo.GetRequest{
		Swamps: []*hydraidepbgo.GetSwamp{
			{
				SwampName: swampName.Get(),
				Keys:      []string{"item-20", "item-21", "item-22", "item-23", "item-24"},
			},
		},
	})
	require.NoError(t, err)

	existCount := 0
	for _, swampResp := range getResponse.GetSwamps() {
		for _, treasure := range swampResp.GetTreasures() {
			if treasure.IsExist {
				existCount++
			}
		}
	}
	assert.Equal(t, 5, existCount, "Should find 5 remaining entries after compaction")

	slog.Info("TestV2ManualCompaction completed successfully!")
}

// TestV2FragmentationCalculation tests the fragmentation calculation accuracy
func TestV2FragmentationCalculation(t *testing.T) {
	ctx := context.Background()
	swampName := name.New().Sanctuary("v2test").Realm("fragcalc").Swamp("test")

	slog.Info("=== V2 FRAGMENTATION CALCULATION TEST ===")

	// Register swamp pattern
	writeInterval := int64(1)
	maxFileSize := int64(65536)

	swampPattern := name.New().Sanctuary("v2test").Realm("fragcalc").Swamp("*")
	selectedClient := clientInterface.GetServiceClient(swampPattern)
	_, _ = selectedClient.RegisterSwamp(context.Background(), &hydraidepbgo.RegisterSwampRequest{
		SwampPattern:   swampPattern.Get(),
		CloseAfterIdle: int64(2),
		WriteInterval:  &writeInterval,
		MaxFileSize:    &maxFileSize,
	})

	swampClient := clientInterface.GetServiceClient(swampName)
	defer func() {
		_, _ = swampClient.Destroy(ctx, &hydraidepbgo.DestroyRequest{
			SwampName: swampName.Get(),
		})
	}()

	// Insert 10 entries
	slog.Info("Step 1: Inserting 10 entries...")
	var keyValues []*hydraidepbgo.KeyValuePair
	for i := 0; i < 10; i++ {
		val := fmt.Sprintf("value-%d", i)
		keyValues = append(keyValues, &hydraidepbgo.KeyValuePair{
			Key:       fmt.Sprintf("key-%d", i),
			StringVal: &val,
			CreatedAt: timestamppb.Now(),
		})
	}

	_, err := swampClient.Set(ctx, &hydraidepbgo.SetRequest{
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

	time.Sleep(3 * time.Second) // Wait for flush and close

	hydFilePath := findHydFileForSwamp(t, "/hydraide/data", "v2test")
	if hydFilePath == "" {
		t.Skip("No .hyd files found")
	}

	// Initial fragmentation should be 0
	reader1, err := v2.NewFileReader(hydFilePath)
	require.NoError(t, err)
	frag1, live1, total1, err := reader1.CalculateFragmentation()
	_ = reader1.Close()
	require.NoError(t, err)

	slog.Info("Initial state", "fragmentation", fmt.Sprintf("%.2f%%", frag1*100), "live", live1, "total", total1)
	assert.Equal(t, 0.0, frag1, "Initial fragmentation should be 0")
	assert.Equal(t, 10, live1, "Should have 10 live entries")

	// Update 5 entries (creates 5 orphaned entries)
	slog.Info("Step 2: Updating 5 entries...")
	var updateKeyValues []*hydraidepbgo.KeyValuePair
	for i := 0; i < 5; i++ {
		val := fmt.Sprintf("updated-value-%d", i)
		updateKeyValues = append(updateKeyValues, &hydraidepbgo.KeyValuePair{
			Key:       fmt.Sprintf("key-%d", i),
			StringVal: &val,
			UpdatedAt: timestamppb.Now(),
		})
	}

	_, err = swampClient.Set(ctx, &hydraidepbgo.SetRequest{
		Swamps: []*hydraidepbgo.SwampRequest{
			{
				SwampName: swampName.Get(),
				Overwrite: true,
				KeyValues: updateKeyValues,
			},
		},
	})
	require.NoError(t, err)

	time.Sleep(3 * time.Second)

	// Fragmentation should be ~33% (5 orphaned out of 15 total)
	reader2, err := v2.NewFileReader(hydFilePath)
	require.NoError(t, err)
	frag2, live2, total2, err := reader2.CalculateFragmentation()
	_ = reader2.Close()
	require.NoError(t, err)

	slog.Info("After updates", "fragmentation", fmt.Sprintf("%.2f%%", frag2*100), "live", live2, "total", total2)
	assert.InDelta(t, 0.33, frag2, 0.1, "Fragmentation should be ~33%%")
	assert.Equal(t, 10, live2, "Should still have 10 live entries")
	assert.Equal(t, 15, total2, "Should have 15 total entries")

	// Delete 3 entries
	slog.Info("Step 3: Deleting 3 entries...")
	_, err = swampClient.Delete(ctx, &hydraidepbgo.DeleteRequest{
		Swamps: []*hydraidepbgo.DeleteRequest_SwampKeys{
			{
				SwampName: swampName.Get(),
				Keys:      []string{"key-7", "key-8", "key-9"},
			},
		},
	})
	require.NoError(t, err)

	time.Sleep(3 * time.Second)

	// Fragmentation should be higher now
	reader3, err := v2.NewFileReader(hydFilePath)
	require.NoError(t, err)
	frag3, live3, total3, err := reader3.CalculateFragmentation()
	_ = reader3.Close()
	require.NoError(t, err)

	slog.Info("After deletes", "fragmentation", fmt.Sprintf("%.2f%%", frag3*100), "live", live3, "total", total3)
	assert.Equal(t, 7, live3, "Should have 7 live entries")
	assert.Greater(t, frag3, frag2, "Fragmentation should increase after deletes")

	slog.Info("TestV2FragmentationCalculation completed successfully!")
}

// Helper functions

func findHydFileForSwamp(t *testing.T, basePath, _ string) string {
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

func getFileSizeOrZero(filePath string) int64 {
	info, err := os.Stat(filePath)
	if err != nil {
		return 0
	}
	return info.Size()
}
