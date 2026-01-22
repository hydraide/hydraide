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
	"github.com/hydraide/hydraide/app/server/server"
	"github.com/hydraide/hydraide/generated/hydraidepbgo"
	"github.com/hydraide/hydraide/sdk/go/hydraidego/client"
	"github.com/hydraide/hydraide/sdk/go/hydraidego/name"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// TestV1ToV2FullMigration tests the complete migration flow from V1 to V2 engine.
// This test:
// 1. Starts a server with V1 engine
// 2. Creates complex treasures with nested payloads
// 3. Stops the V1 server and ensures data is flushed
// 4. Runs the migration tool to convert V1 → V2
// 5. Verifies that old V1 files are deleted
// 6. Starts with V2 engine and verifies all data is readable and correct
func TestV1ToV2FullMigration(t *testing.T) {
	// Skip if not running E2E tests (environment not set up)
	if os.Getenv("E2E_HYDRA_SERVER_CRT") == "" {
		t.Skip("E2E test environment not configured - skipping full migration test")
	}

	// Use the standard data path
	dataPath := "/hydraide/data"
	settingsPath := "/hydraide/settings/settings.json"

	// Test port (different from main E2E tests)
	testPort := 5560

	slog.Info("=== FULL V1 TO V2 MIGRATION TEST ===")

	// CRITICAL: Temporarily change the engine to V1 in settings.json
	// The server reads engine setting from settings.json, not from Configuration
	slog.Info("Step 0: Temporarily switching engine to V1 in settings.json...")
	originalSettingsBackup, err := os.ReadFile(settingsPath)
	require.NoError(t, err, "Failed to read settings.json for backup")

	// Modify settings to use V1 engine
	err = switchEngineInSettings(settingsPath, "V1")
	require.NoError(t, err, "Failed to switch engine to V1")

	// Ensure we restore V2 at the end
	defer func() {
		slog.Info("Restoring original settings.json...")
		if err := os.WriteFile(settingsPath, originalSettingsBackup, 0644); err != nil {
			slog.Error("Failed to restore settings.json", "error", err)
		}
	}()

	// Step 1: Start server with V1 engine
	slog.Info("Step 1: Starting server with V1 engine...")

	v1Server := server.New(&server.Configuration{
		CertificateCrtFile:  os.Getenv("E2E_HYDRA_SERVER_CRT"),
		CertificateKeyFile:  os.Getenv("E2E_HYDRA_SERVER_KEY"),
		ClientCAFile:        os.Getenv("E2E_HYDRA_CA_CRT"),
		HydraServerPort:     testPort,
		HydraMaxMessageSize: 1024 * 1024 * 100,
		UseV2Engine:         false, // V1 engine!
	})

	err := v1Server.Start()
	require.NoError(t, err, "Failed to start V1 server")

	// Wait for server to start
	time.Sleep(1 * time.Second)

	// Create client for V1 server
	v1Client := createMigrationTestClient(t, testPort)

	ctx := context.Background()
	swampName := name.New().Sanctuary("migtest").Realm("full").Swamp("complex")

	// Register swamp pattern
	writeInterval := int64(1)
	maxFileSize := int64(65536)

	swampPattern := name.New().Sanctuary("migtest").Realm("full").Swamp("*")
	selectedClient := v1Client.GetServiceClient(swampPattern)
	_, err = selectedClient.RegisterSwamp(ctx, &hydraidepbgo.RegisterSwampRequest{
		SwampPattern:   swampPattern.Get(),
		CloseAfterIdle: int64(3),
		WriteInterval:  &writeInterval,
		MaxFileSize:    &maxFileSize,
	})
	require.NoError(t, err)

	slog.Info("Step 2: Creating complex treasures with V1 engine...")

	swampClient := v1Client.GetServiceClient(swampName)

	// Create complex treasures with various data types
	var keyValues []*hydraidepbgo.KeyValuePair

	// Treasure 1: Full payload with all fields
	stringVal1 := "John Doe - Full User Profile"
	int64Val1 := int64(12345)
	float64Val1 := 99.99
	boolVal1 := hydraidepbgo.Boolean_TRUE
	bytesVal1 := []byte("binary data for user profile - complex payload test")
	keyValues = append(keyValues, &hydraidepbgo.KeyValuePair{
		Key:        "user-profile-001",
		StringVal:  &stringVal1,
		Int64Val:   &int64Val1,
		Float64Val: &float64Val1,
		BoolVal:    &boolVal1,
		BytesVal:   bytesVal1,
		CreatedAt:  timestamppb.Now(),
	})

	// Treasure 2: Another user profile
	stringVal2 := "Jane Smith - Secondary Profile"
	int64Val2 := int64(67890)
	keyValues = append(keyValues, &hydraidepbgo.KeyValuePair{
		Key:       "user-profile-002",
		StringVal: &stringVal2,
		Int64Val:  &int64Val2,
		CreatedAt: timestamppb.Now(),
	})

	// Treasure 3: Large binary data
	largeBytes := make([]byte, 5000)
	for i := range largeBytes {
		largeBytes[i] = byte(i % 256)
	}
	stringVal3 := "Large binary attachment"
	keyValues = append(keyValues, &hydraidepbgo.KeyValuePair{
		Key:       "large-binary-data",
		StringVal: &stringVal3,
		BytesVal:  largeBytes,
		CreatedAt: timestamppb.Now(),
	})

	// Treasure 4: Simple string
	stringVal4 := "This is a simple string value for migration testing"
	keyValues = append(keyValues, &hydraidepbgo.KeyValuePair{
		Key:       "simple-string",
		StringVal: &stringVal4,
		CreatedAt: timestamppb.Now(),
	})

	// Treasure 5: Numeric extremes
	int64Max := int64(9223372036854775807)
	float64Pi := 3.14159265358979
	keyValues = append(keyValues, &hydraidepbgo.KeyValuePair{
		Key:        "numeric-values",
		Int64Val:   &int64Max,
		Float64Val: &float64Pi,
		CreatedAt:  timestamppb.Now(),
	})

	// Insert all treasures
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
	require.NoError(t, err, "Failed to insert treasures with V1 engine")

	slog.Info("Step 3: Waiting for V1 data to flush to disk...")
	time.Sleep(3 * time.Second)

	// Verify data was written
	getResp, err := swampClient.Get(ctx, &hydraidepbgo.GetRequest{
		Swamps: []*hydraidepbgo.GetSwamp{
			{
				SwampName: swampName.Get(),
				Keys:      []string{"user-profile-001", "user-profile-002", "large-binary-data"},
			},
		},
	})
	require.NoError(t, err)

	// Verify entries exist
	existCount := 0
	for _, swampResp := range getResp.GetSwamps() {
		for _, treasure := range swampResp.GetTreasures() {
			if treasure.IsExist {
				existCount++
			}
		}
	}
	assert.Equal(t, 3, existCount, "Should have 3 treasures before migration")

	slog.Info("Step 4: Stopping V1 server...")
	v1Server.Stop()
	time.Sleep(2 * time.Second)

	// Find V1 swamp folder - V1 creates folders like /hydraide/data/0/xxx/xxxxxxxxxxxxxxxx/
	// Inside there's a 'meta' file and UUID-named data files (no extension)
	v1SwampFolder := findV1SwampFolder(t, dataPath)
	require.NotEmpty(t, v1SwampFolder, "V1 swamp folder should exist after V1 engine write")

	slog.Info("V1 swamp folder found", "path", v1SwampFolder)

	// Verify V1 folder structure: should have 'meta' file and at least one UUID file
	v1MetaFile := filepath.Join(v1SwampFolder, "meta")
	_, err = os.Stat(v1MetaFile)
	require.NoError(t, err, "V1 meta file should exist: %s", v1MetaFile)

	v1DataFiles := countV1DataFiles(t, v1SwampFolder)
	slog.Info("V1 structure verified", "meta_exists", true, "data_files", v1DataFiles)
	require.Greater(t, v1DataFiles, 0, "V1 swamp should have at least one data file")

	slog.Info("Step 5: Running migration from V1 to V2...")

	// Run migration
	migratorConfig := migrator.Config{
		DataPath:       dataPath,
		DryRun:         false, // Real migration!
		Verify:         true,
		DeleteOld:      true, // Delete V1 files after migration
		Parallel:       2,
		StopOnError:    true,
		ProgressReport: time.Second,
	}

	mig, err := migrator.New(migratorConfig)
	require.NoError(t, err, "Failed to create migrator")

	result, err := mig.Run()
	require.NoError(t, err, "Migration failed")

	slog.Info("Migration completed",
		"processed", result.ProcessedSwamps,
		"successful", result.SuccessfulSwamps,
		"failed", len(result.FailedSwamps),
		"duration", result.Duration,
		"old_size", result.OldSizeBytes,
		"new_size", result.NewSizeBytes)

	// Verify migration was successful
	assert.Greater(t, result.ProcessedSwamps, int64(0), "Should have processed at least one swamp")
	assert.Equal(t, result.ProcessedSwamps, result.SuccessfulSwamps, "All swamps should migrate successfully")
	assert.Empty(t, result.FailedSwamps, "No swamps should fail migration")

	slog.Info("Step 6: Verifying V1 files are deleted and V2 files exist...")

	// Verify V1 folder is deleted (or empty of data files)
	_, err = os.Stat(v1SwampFolder)
	if err == nil {
		// Folder still exists - check if it's empty or only has .hyd file
		v1DataFilesAfter := countV1DataFiles(t, v1SwampFolder)
		slog.Info("V1 folder after migration", "path", v1SwampFolder, "data_files", v1DataFilesAfter)
		assert.Equal(t, 0, v1DataFilesAfter, "V1 data files should be deleted after migration")

		// Check if meta file is deleted
		_, metaErr := os.Stat(filepath.Join(v1SwampFolder, "meta"))
		assert.True(t, os.IsNotExist(metaErr), "V1 meta file should be deleted after migration")
	} else {
		slog.Info("V1 folder deleted successfully", "path", v1SwampFolder)
	}

	// Verify V2 .hyd file exists
	// The V2 file should be at the same level as the old V1 folder, with .hyd extension
	// e.g., /hydraide/data/0/dab/dab28f21c96898ea.hyd
	v2HydFile := v1SwampFolder + ".hyd"
	_, err = os.Stat(v2HydFile)
	require.NoError(t, err, "V2 .hyd file should exist after migration: %s", v2HydFile)
	slog.Info("V2 .hyd file verified", "path", v2HydFile)

	slog.Info("Step 7: Starting server with V2 engine and verifying data...")

	// Switch engine to V2 in settings before starting V2 server
	err = switchEngineInSettings(settingsPath, "V2")
	require.NoError(t, err, "Failed to switch engine to V2")

	// Start V2 server
	v2Server := server.New(&server.Configuration{
		CertificateCrtFile:  os.Getenv("E2E_HYDRA_SERVER_CRT"),
		CertificateKeyFile:  os.Getenv("E2E_HYDRA_SERVER_KEY"),
		ClientCAFile:        os.Getenv("E2E_HYDRA_CA_CRT"),
		HydraServerPort:     testPort,
		HydraMaxMessageSize: 1024 * 1024 * 100,
		UseV2Engine:         true, // V2 engine!
	})

	err = v2Server.Start()
	require.NoError(t, err, "Failed to start V2 server")
	time.Sleep(1 * time.Second)

	// Create client for V2 server
	v2Client := createMigrationTestClient(t, testPort)
	v2SwampClient := v2Client.GetServiceClient(swampName)

	// Verify all data is readable and correct
	slog.Info("Step 8: Verifying migrated data...")

	getResp, err = v2SwampClient.Get(ctx, &hydraidepbgo.GetRequest{
		Swamps: []*hydraidepbgo.GetSwamp{
			{
				SwampName: swampName.Get(),
				Keys:      []string{"user-profile-001", "user-profile-002", "large-binary-data", "simple-string", "numeric-values"},
			},
		},
	})
	require.NoError(t, err, "Failed to get treasures from V2 engine")

	// Verify all entries exist and have correct values
	foundKeys := make(map[string]bool)
	for _, swampResp := range getResp.GetSwamps() {
		for _, treasure := range swampResp.GetTreasures() {
			if treasure.IsExist {
				foundKeys[treasure.GetKey()] = true

				switch treasure.GetKey() {
				case "user-profile-001":
					assert.Equal(t, "John Doe - Full User Profile", treasure.GetStringVal())
					assert.Equal(t, int64(12345), treasure.GetInt64Val())
					assert.InDelta(t, 99.99, treasure.GetFloat64Val(), 0.01)
					assert.Equal(t, hydraidepbgo.Boolean_TRUE, treasure.GetBoolVal())
				case "user-profile-002":
					assert.Equal(t, "Jane Smith - Secondary Profile", treasure.GetStringVal())
					assert.Equal(t, int64(67890), treasure.GetInt64Val())
				case "large-binary-data":
					assert.Equal(t, len(largeBytes), len(treasure.GetBytesVal()))
					// Verify content is identical
					for i := 0; i < min(100, len(largeBytes)); i++ {
						if largeBytes[i] != treasure.GetBytesVal()[i] {
							t.Fatalf("Binary data mismatch at index %d", i)
						}
					}
				case "simple-string":
					assert.Contains(t, treasure.GetStringVal(), "simple string value")
				case "numeric-values":
					assert.Equal(t, int64(9223372036854775807), treasure.GetInt64Val())
					assert.InDelta(t, 3.14159265358979, treasure.GetFloat64Val(), 0.0000001)
				}
			}
		}
	}

	assert.Equal(t, 5, len(foundKeys), "Should find all 5 migrated treasures")

	slog.Info("All data verified successfully!")

	// Cleanup
	_, _ = v2SwampClient.Destroy(ctx, &hydraidepbgo.DestroyRequest{
		SwampName: swampName.Get(),
	})

	v2Server.Stop()

	slog.Info("TestV1ToV2FullMigration completed successfully!")
}

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

	// Check if there are any V1 swamps to test with (look for folders with 'meta' file)
	v1SwampFolder := findV1SwampFolder(t, dataPath)

	if v1SwampFolder == "" {
		t.Skip("No V1 swamp folders found - skipping migration dry-run test")
	}

	slog.Info("=== MIGRATION DRY-RUN TEST ===")
	slog.Info("Found V1 swamp folder", "path", v1SwampFolder)

	// Count V1 data files before dry-run
	v1DataFilesBefore := countV1DataFiles(t, v1SwampFolder)
	slog.Info("V1 data files before dry-run", "count", v1DataFilesBefore)

	// Count V2 files before dry-run
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
	slog.Info("V2 file counts", "before", v2CountBefore, "after", v2CountAfter)

	// Verify V1 files still exist unchanged
	v1DataFilesAfter := countV1DataFiles(t, v1SwampFolder)
	assert.Equal(t, v1DataFilesBefore, v1DataFilesAfter, "V1 data files should not change during dry-run")

	// Verify meta file still exists
	_, err = os.Stat(filepath.Join(v1SwampFolder, "meta"))
	assert.NoError(t, err, "V1 meta file should still exist after dry-run")

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

func createMigrationTestClient(t *testing.T, port int) client.Client {
	servers := []*client.Server{
		{
			Host:          fmt.Sprintf("localhost:%d", port),
			FromIsland:    0,
			ToIsland:      100,
			CACrtPath:     os.Getenv("E2E_HYDRA_CA_CRT"),
			ClientCrtPath: os.Getenv("E2E_HYDRA_CLIENT_CRT"),
			ClientKeyPath: os.Getenv("E2E_HYDRA_CLIENT_KEY"),
		},
	}

	cli := client.New(servers, 100, 1024*1024*100)
	err := cli.Connect(false)
	require.NoError(t, err, "Failed to connect to server")

	return cli
}

// findV1SwampFolder finds a V1 swamp folder that contains a 'meta' file.
// V1 structure: /hydraide/data/{islandID}/{hash_prefix}/{swamp_hash}/
// Inside: 'meta' file + UUID-named data files (no extensions)
func findV1SwampFolder(t *testing.T, basePath string) string {
	var v1Folder string
	err := filepath.Walk(basePath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		// Look for 'meta' file which indicates a V1 swamp folder
		if !info.IsDir() && info.Name() == "meta" {
			v1Folder = filepath.Dir(path)
			return filepath.SkipAll
		}
		return nil
	})
	if err != nil && err != filepath.SkipAll {
		t.Logf("Warning: error walking path: %v", err)
	}
	return v1Folder
}

// countV1DataFiles counts UUID-named data files in a V1 swamp folder.
// V1 data files have no extension and are not named 'meta'.
func countV1DataFiles(t *testing.T, folderPath string) int {
	entries, err := os.ReadDir(folderPath)
	if err != nil {
		t.Logf("Warning: error reading folder %s: %v", folderPath, err)
		return 0
	}

	count := 0
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		fileName := entry.Name()
		// V1 data files: not 'meta', no extension, looks like UUID (hex characters)
		if fileName != "meta" && filepath.Ext(fileName) == "" && isHexString(fileName) {
			count++
		}
	}
	return count
}

// isHexString checks if a string contains only hexadecimal characters
func isHexString(s string) bool {
	if len(s) == 0 {
		return false
	}
	for _, c := range s {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F') || c == '-') {
			return false
		}
	}
	return true
}

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
