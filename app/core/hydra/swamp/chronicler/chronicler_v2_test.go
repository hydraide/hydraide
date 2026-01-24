package chronicler

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/hydraide/hydraide/app/core/hydra/swamp/beacon"
	v2 "github.com/hydraide/hydraide/app/core/hydra/swamp/chronicler/v2"
	"github.com/hydraide/hydraide/app/core/hydra/swamp/treasure"
	"github.com/hydraide/hydraide/app/core/hydra/swamp/treasure/guard"
)

func TestChroniclerV2_CreateAndDestroy(t *testing.T) {
	tmpDir := t.TempDir()
	swampPath := filepath.Join(tmpDir, "test-swamp")

	chron := NewV2(swampPath, 10)

	// Create directory
	chron.CreateDirectoryIfNotExists()

	if !chron.IsFilesystemInitiated() {
		t.Error("expected filesystem to be initiated")
	}

	// Check parent directory exists
	parentDir := filepath.Dir(swampPath + ".hyd")
	if _, err := os.Stat(parentDir); os.IsNotExist(err) {
		t.Error("expected parent directory to exist")
	}

	// Destroy
	chron.Destroy()

	// .hyd file should not exist (was never created since no data written)
	hydPath := swampPath + ".hyd"
	if _, err := os.Stat(hydPath); !os.IsNotExist(err) {
		t.Error("expected .hyd file to not exist after destroy")
	}
}

func TestChroniclerV2_WriteAndLoad(t *testing.T) {
	tmpDir := t.TempDir()
	swampPath := filepath.Join(tmpDir, "test-swamp")

	chron := NewV2(swampPath, 10)
	chron.CreateDirectoryIfNotExists()

	// Create test treasures
	treasures := make([]treasure.Treasure, 100)
	for i := 0; i < 100; i++ {
		tr := treasure.New(nil)
		guardID := tr.StartTreasureGuard(false, guard.BodyAuthID)
		tr.BodySetKey(guardID, fmt.Sprintf("key-%d", i))
		tr.SetContentString(guardID, fmt.Sprintf("content-%d", i))
		tr.ReleaseTreasureGuard(guardID)
		treasures[i] = tr
	}

	// Write treasures
	chron.Write(treasures)

	// Close to flush buffer (required for persistent writer)
	if err := chron.Close(); err != nil {
		t.Fatalf("failed to close chronicler: %v", err)
	}

	// Verify .hyd file exists
	hydPath := swampPath + ".hyd"
	if _, err := os.Stat(hydPath); os.IsNotExist(err) {
		t.Fatal("expected .hyd file to exist after write")
	}

	// Load treasures into a new beacon
	beac := beacon.New()
	chron.Load(beac)

	// Verify all treasures loaded
	if beac.Count() != 100 {
		t.Errorf("expected 100 treasures, got %d", beac.Count())
	}

	// Verify specific keys
	for i := 0; i < 100; i++ {
		key := fmt.Sprintf("key-%d", i)
		if !beac.IsExists(key) {
			t.Errorf("missing key: %s", key)
		}
	}
}

func TestChroniclerV2_Update(t *testing.T) {
	tmpDir := t.TempDir()
	swampPath := filepath.Join(tmpDir, "test-swamp")

	chron := NewV2(swampPath, 10)
	chron.CreateDirectoryIfNotExists()

	// Initial write
	tr1 := treasure.New(nil)
	guardID := tr1.StartTreasureGuard(false, guard.BodyAuthID)
	tr1.BodySetKey(guardID, "test-key")
	tr1.SetContentString(guardID, "original-content")
	tr1.ReleaseTreasureGuard(guardID)

	chron.Write([]treasure.Treasure{tr1})

	// Update
	tr2 := treasure.New(nil)
	guardID = tr2.StartTreasureGuard(false, guard.BodyAuthID)
	tr2.BodySetKey(guardID, "test-key")
	tr2.SetContentString(guardID, "updated-content")
	tr2.ReleaseTreasureGuard(guardID)

	chron.Write([]treasure.Treasure{tr2})

	// Close to flush buffer
	if err := chron.Close(); err != nil {
		t.Fatalf("failed to close chronicler: %v", err)
	}

	// Load and verify
	beac := beacon.New()
	chron.Load(beac)

	if beac.Count() != 1 {
		t.Errorf("expected 1 treasure, got %d", beac.Count())
	}

	// Get the treasure and check content
	loadedTr := beac.Get("test-key")
	if loadedTr == nil {
		t.Fatal("expected to find test-key")
	}

	guardID = loadedTr.StartTreasureGuard(true, guard.BodyAuthID)
	content := loadedTr.CloneContent(guardID)
	loadedTr.ReleaseTreasureGuard(guardID)

	if content.String == nil || *content.String != "updated-content" {
		t.Errorf("expected updated-content, got %v", content)
	}
}

func TestChroniclerV2_Delete(t *testing.T) {
	tmpDir := t.TempDir()
	swampPath := filepath.Join(tmpDir, "test-swamp")

	chron := NewV2(swampPath, 10)
	chron.CreateDirectoryIfNotExists()

	// Initial write
	tr1 := treasure.New(nil)
	guardID := tr1.StartTreasureGuard(false, guard.BodyAuthID)
	tr1.BodySetKey(guardID, "test-key")
	tr1.SetContentString(guardID, "content")
	tr1.ReleaseTreasureGuard(guardID)

	chron.Write([]treasure.Treasure{tr1})

	// Delete
	tr2 := treasure.New(nil)
	guardID = tr2.StartTreasureGuard(false, guard.BodyAuthID)
	tr2.BodySetKey(guardID, "test-key")
	tr2.BodySetForDeletion(guardID, "test", true)
	tr2.ReleaseTreasureGuard(guardID)

	chron.Write([]treasure.Treasure{tr2})

	// Close to flush buffer
	if err := chron.Close(); err != nil {
		t.Fatalf("failed to close chronicler: %v", err)
	}

	// Load and verify deletion
	beac := beacon.New()
	chron.Load(beac)

	if beac.Count() != 0 {
		t.Errorf("expected 0 treasures after delete, got %d", beac.Count())
	}

	if beac.IsExists("test-key") {
		t.Error("expected test-key to be deleted")
	}
}

func TestChroniclerV2_LargeDataset(t *testing.T) {
	tmpDir := t.TempDir()
	swampPath := filepath.Join(tmpDir, "test-swamp")

	chron := NewV2(swampPath, 10)
	chron.CreateDirectoryIfNotExists()

	// Write 10000 treasures
	treasures := make([]treasure.Treasure, 10000)
	for i := 0; i < 10000; i++ {
		tr := treasure.New(nil)
		guardID := tr.StartTreasureGuard(false, guard.BodyAuthID)
		tr.BodySetKey(guardID, fmt.Sprintf("key-%d", i))
		tr.SetContentString(guardID, fmt.Sprintf("content-%d-with-extra-data-to-make-it-bigger", i))
		tr.ReleaseTreasureGuard(guardID)
		treasures[i] = tr
	}

	chron.Write(treasures)

	// Close to flush buffer
	if err := chron.Close(); err != nil {
		t.Fatalf("failed to close chronicler: %v", err)
	}

	// Load and verify
	beac := beacon.New()
	chron.Load(beac)

	if beac.Count() != 10000 {
		t.Errorf("expected 10000 treasures, got %d", beac.Count())
	}

	// Verify file exists and is reasonable size
	hydPath := swampPath + ".hyd"
	info, err := os.Stat(hydPath)
	if err != nil {
		t.Fatalf("cannot stat .hyd file: %v", err)
	}

	// Should be compressed, so less than raw data
	t.Logf("10K treasures file size: %d bytes (%.2f KB)", info.Size(), float64(info.Size())/1024)
}

func TestChroniclerV2_Persistence(t *testing.T) {
	tmpDir := t.TempDir()
	swampPath := filepath.Join(tmpDir, "test-swamp")

	// First chronicler - write data
	{
		chron := NewV2(swampPath, 10)
		chron.CreateDirectoryIfNotExists()

		tr := treasure.New(nil)
		guardID := tr.StartTreasureGuard(false, guard.BodyAuthID)
		tr.BodySetKey(guardID, "persistent-key")
		tr.SetContentString(guardID, "persistent-content")
		tr.ReleaseTreasureGuard(guardID)

		chron.Write([]treasure.Treasure{tr})

		// Close to flush buffer (simulating graceful shutdown)
		if err := chron.Close(); err != nil {
			t.Fatalf("failed to close chronicler: %v", err)
		}
	}

	// Second chronicler - read data (simulating restart)
	{
		chron := NewV2(swampPath, 10)
		beac := beacon.New()
		chron.Load(beac)

		if beac.Count() != 1 {
			t.Errorf("expected 1 treasure after reload, got %d", beac.Count())
		}

		if !beac.IsExists("persistent-key") {
			t.Error("expected persistent-key to exist after reload")
		}
	}
}

func TestChroniclerV2_EmptyLoad(t *testing.T) {
	tmpDir := t.TempDir()
	swampPath := filepath.Join(tmpDir, "test-swamp")

	chron := NewV2(swampPath, 10)

	// Load from non-existent file should not error
	beac := beacon.New()
	chron.Load(beac)

	if beac.Count() != 0 {
		t.Errorf("expected 0 treasures from empty swamp, got %d", beac.Count())
	}
}

func TestChroniclerV2_GetSwampAbsPath(t *testing.T) {
	tmpDir := t.TempDir()
	swampPath := filepath.Join(tmpDir, "test-swamp")

	chron := NewV2(swampPath, 10)

	if chron.GetSwampAbsPath() != swampPath {
		t.Errorf("expected %s, got %s", swampPath, chron.GetSwampAbsPath())
	}
}

func TestChroniclerV2_InterfaceCompatibility(t *testing.T) {
	tmpDir := t.TempDir()
	swampPath := filepath.Join(tmpDir, "test-swamp")

	// Verify that chroniclerV2 implements Chronicler interface
	var _ Chronicler = NewV2(swampPath, 10)
}

func TestChroniclerV2_MetadataRepair(t *testing.T) {
	tmpDir := t.TempDir()
	swampPath := filepath.Join(tmpDir, "001", "test-swamp")
	hydFilePath := swampPath + ".hyd"

	// Step 1: Create a file WITHOUT metadata using plain NewV2 (simulates old files)
	chron1 := NewV2(swampPath, 10)
	chron1.CreateDirectoryIfNotExists()

	// Write a treasure (this creates the .hyd file but without swamp name metadata)
	tr := treasure.New(nil)
	guardID := tr.StartTreasureGuard(false, guard.BodyAuthID)
	tr.BodySetKey(guardID, "test-key")
	tr.SetContentString(guardID, "test-content")
	tr.ReleaseTreasureGuard(guardID)
	chron1.Write([]treasure.Treasure{tr})
	if err := chron1.Close(); err != nil {
		t.Fatalf("failed to close first chronicler: %v", err)
	}

	// Verify file exists but has no metadata
	reader1, err := v2.NewFileReader(hydFilePath)
	if err != nil {
		t.Fatalf("failed to open .hyd file: %v", err)
	}
	_, swampName1, err := reader1.LoadIndex()
	reader1.Close()
	if err != nil {
		t.Fatalf("failed to load index: %v", err)
	}
	if swampName1 != "" {
		t.Errorf("expected no swamp name in initial file, got: %s", swampName1)
	}

	// Step 2: Create a new chronicler WITH swamp name and write to existing file
	expectedSwampName := "test/realm/swampname"
	chron2 := NewV2WithName(swampPath, 10, expectedSwampName)

	// Write another treasure - this should trigger metadata repair
	tr2 := treasure.New(nil)
	guardID2 := tr2.StartTreasureGuard(false, guard.BodyAuthID)
	tr2.BodySetKey(guardID2, "test-key-2")
	tr2.SetContentString(guardID2, "test-content-2")
	tr2.ReleaseTreasureGuard(guardID2)
	chron2.Write([]treasure.Treasure{tr2})
	if err := chron2.Close(); err != nil {
		t.Fatalf("failed to close second chronicler: %v", err)
	}

	// Step 3: Verify the file now has metadata
	reader2, err := v2.NewFileReader(hydFilePath)
	if err != nil {
		t.Fatalf("failed to open .hyd file after repair: %v", err)
	}
	index, swampName2, err := reader2.LoadIndex()
	reader2.Close()
	if err != nil {
		t.Fatalf("failed to load index after repair: %v", err)
	}

	if swampName2 != expectedSwampName {
		t.Errorf("expected swamp name %q after repair, got: %q", expectedSwampName, swampName2)
	}

	// Also verify both treasures are still present
	if len(index) != 2 {
		t.Errorf("expected 2 entries in index, got: %d", len(index))
	}
}
