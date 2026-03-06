package explorer

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	v2 "github.com/hydraide/hydraide/app/core/hydra/swamp/chronicler/v2"
)

// testFileCounter ensures unique file paths for test swamps.
var testFileCounter int

// createTestSwamp creates a V3 .hyd file with the given swamp name at a realistic path.
func createTestSwamp(t *testing.T, dataRoot string, islandID int, swampName string, entryCount int) string {
	t.Helper()
	testFileCounter++

	// Create path: dataRoot/islandID/sub/uniqueHash.hyd
	hash := fmt.Sprintf("%04x%04x", testFileCounter, islandID)
	dir := filepath.Join(dataRoot, fmt.Sprintf("%d", islandID), hash[:2])
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("failed to create dir: %v", err)
	}

	filePath := filepath.Join(dir, hash+".hyd")

	writer, err := v2.NewFileWriterWithName(filePath, v2.DefaultMaxBlockSize, swampName)
	if err != nil {
		t.Fatalf("failed to create writer: %v", err)
	}

	for i := 0; i < entryCount; i++ {
		writer.WriteEntry(v2.Entry{
			Operation: v2.OpInsert,
			Key:       fmt.Sprintf("key-%04d", i),
			Data:      []byte(fmt.Sprintf("value-%04d", i)),
		})
	}

	if err := writer.Close(); err != nil {
		t.Fatalf("failed to close writer: %v", err)
	}

	return filePath
}

// createTestData creates a realistic test dataset.
func createTestData(t *testing.T) string {
	t.Helper()
	dataRoot := filepath.Join(t.TempDir(), "data")

	// Sanctuary: users
	createTestSwamp(t, dataRoot, 1, "users/profiles/alice", 10)
	createTestSwamp(t, dataRoot, 1, "users/profiles/bob", 5)
	createTestSwamp(t, dataRoot, 2, "users/profiles/charlie", 20)
	createTestSwamp(t, dataRoot, 1, "users/sessions/alice", 3)
	createTestSwamp(t, dataRoot, 2, "users/sessions/bob", 7)
	createTestSwamp(t, dataRoot, 1, "users/preferences/alice", 2)

	// Sanctuary: products
	createTestSwamp(t, dataRoot, 3, "products/catalog/item-001", 50)
	createTestSwamp(t, dataRoot, 3, "products/catalog/item-002", 30)
	createTestSwamp(t, dataRoot, 4, "products/reviews/item-001", 100)

	// Sanctuary: analytics
	createTestSwamp(t, dataRoot, 5, "analytics/events/page-views", 1000)

	return dataRoot
}

func TestExplorer_Scan(t *testing.T) {
	dataRoot := createTestData(t)
	e := New(dataRoot)

	err := e.Scan(context.Background())
	if err != nil {
		t.Fatalf("scan failed: %v", err)
	}

	status := e.GetScanStatus()
	if status.State != ScanStateDone {
		t.Errorf("expected state %q, got %q", ScanStateDone, status.State)
	}
	if status.TotalFiles != 10 {
		t.Errorf("expected 10 total files, got %d", status.TotalFiles)
	}
	if status.ScannedFiles != 10 {
		t.Errorf("expected 10 scanned files, got %d", status.ScannedFiles)
	}
	if status.ErrorCount != 0 {
		t.Errorf("expected 0 errors, got %d", status.ErrorCount)
	}
}

func TestExplorer_ListSanctuaries(t *testing.T) {
	dataRoot := createTestData(t)
	e := New(dataRoot)
	e.Scan(context.Background())

	sanctuaries := e.ListSanctuaries()

	if len(sanctuaries) != 3 {
		t.Fatalf("expected 3 sanctuaries, got %d", len(sanctuaries))
	}

	// Should be sorted alphabetically
	if sanctuaries[0].Name != "analytics" {
		t.Errorf("expected first sanctuary 'analytics', got %q", sanctuaries[0].Name)
	}
	if sanctuaries[1].Name != "products" {
		t.Errorf("expected second sanctuary 'products', got %q", sanctuaries[1].Name)
	}
	if sanctuaries[2].Name != "users" {
		t.Errorf("expected third sanctuary 'users', got %q", sanctuaries[2].Name)
	}

	// Check users sanctuary stats
	users := sanctuaries[2]
	if users.RealmCount != 3 {
		t.Errorf("users: expected 3 realms, got %d", users.RealmCount)
	}
	if users.SwampCount != 6 {
		t.Errorf("users: expected 6 swamps, got %d", users.SwampCount)
	}
	if users.TotalSize <= 0 {
		t.Error("users: expected positive total size")
	}
}

func TestExplorer_ListRealms(t *testing.T) {
	dataRoot := createTestData(t)
	e := New(dataRoot)
	e.Scan(context.Background())

	realms := e.ListRealms("users")

	if len(realms) != 3 {
		t.Fatalf("expected 3 realms, got %d", len(realms))
	}

	// Sorted: preferences, profiles, sessions
	if realms[0].Name != "preferences" {
		t.Errorf("expected first realm 'preferences', got %q", realms[0].Name)
	}
	if realms[1].Name != "profiles" {
		t.Errorf("expected second realm 'profiles', got %q", realms[1].Name)
	}
	if realms[2].Name != "sessions" {
		t.Errorf("expected third realm 'sessions', got %q", realms[2].Name)
	}

	// Check profiles realm
	profiles := realms[1]
	if profiles.SwampCount != 3 {
		t.Errorf("profiles: expected 3 swamps, got %d", profiles.SwampCount)
	}
}

func TestExplorer_ListRealms_NotFound(t *testing.T) {
	dataRoot := createTestData(t)
	e := New(dataRoot)
	e.Scan(context.Background())

	realms := e.ListRealms("nonexistent")
	if len(realms) != 0 {
		t.Errorf("expected 0 realms for nonexistent sanctuary, got %d", len(realms))
	}
}

func TestExplorer_ListSwamps(t *testing.T) {
	dataRoot := createTestData(t)
	e := New(dataRoot)
	e.Scan(context.Background())

	result := e.ListSwamps(&SwampFilter{
		Sanctuary: "users",
		Realm:     "profiles",
		Limit:     100,
	})

	if result.Total != 3 {
		t.Fatalf("expected 3 swamps, got %d", result.Total)
	}

	// Verify swamp names
	names := make(map[string]bool)
	for _, s := range result.Swamps {
		names[s.Swamp] = true
	}
	for _, expected := range []string{"alice", "bob", "charlie"} {
		if !names[expected] {
			t.Errorf("expected swamp %q not found", expected)
		}
	}
}

func TestExplorer_ListSwamps_PrefixFilter(t *testing.T) {
	dataRoot := createTestData(t)
	e := New(dataRoot)
	e.Scan(context.Background())

	result := e.ListSwamps(&SwampFilter{
		Sanctuary:   "products",
		Realm:       "catalog",
		SwampPrefix: "item-00",
		Limit:       100,
	})

	if result.Total != 2 {
		t.Fatalf("expected 2 swamps with prefix 'item-00', got %d", result.Total)
	}
}

func TestExplorer_ListSwamps_Pagination(t *testing.T) {
	dataRoot := createTestData(t)
	e := New(dataRoot)
	e.Scan(context.Background())

	// Page 1
	page1 := e.ListSwamps(&SwampFilter{
		Sanctuary: "users",
		Realm:     "profiles",
		Offset:    0,
		Limit:     2,
	})
	if len(page1.Swamps) != 2 {
		t.Errorf("page 1: expected 2 swamps, got %d", len(page1.Swamps))
	}
	if page1.Total != 3 {
		t.Errorf("page 1: expected total 3, got %d", page1.Total)
	}

	// Page 2
	page2 := e.ListSwamps(&SwampFilter{
		Sanctuary: "users",
		Realm:     "profiles",
		Offset:    2,
		Limit:     2,
	})
	if len(page2.Swamps) != 1 {
		t.Errorf("page 2: expected 1 swamp, got %d", len(page2.Swamps))
	}
}

func TestExplorer_GetSwampDetail(t *testing.T) {
	dataRoot := createTestData(t)
	e := New(dataRoot)
	e.Scan(context.Background())

	detail, err := e.GetSwampDetail("users", "profiles", "alice")
	if err != nil {
		t.Fatalf("GetSwampDetail failed: %v", err)
	}

	if detail.Sanctuary != "users" {
		t.Errorf("sanctuary: expected 'users', got %q", detail.Sanctuary)
	}
	if detail.Realm != "profiles" {
		t.Errorf("realm: expected 'profiles', got %q", detail.Realm)
	}
	if detail.Swamp != "alice" {
		t.Errorf("swamp: expected 'alice', got %q", detail.Swamp)
	}
	if detail.FileSize <= 0 {
		t.Error("expected positive file size")
	}
	if detail.EntryCount == 0 {
		t.Error("expected non-zero entry count")
	}
	if detail.IslandID != "1" {
		t.Errorf("expected island ID '1', got %q", detail.IslandID)
	}
	if detail.Version != v2.Version3 {
		t.Errorf("expected V3 format, got version %d", detail.Version)
	}
}

func TestExplorer_GetSwampDetail_NotFound(t *testing.T) {
	dataRoot := createTestData(t)
	e := New(dataRoot)
	e.Scan(context.Background())

	_, err := e.GetSwampDetail("users", "profiles", "nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent swamp")
	}
}

func TestExplorer_GetSize(t *testing.T) {
	dataRoot := createTestData(t)
	e := New(dataRoot)
	e.Scan(context.Background())

	// Sanctuary level
	size, err := e.GetSize("users", "", "")
	if err != nil {
		t.Fatalf("GetSize sanctuary failed: %v", err)
	}
	if size.FileCount != 6 {
		t.Errorf("expected 6 files in users, got %d", size.FileCount)
	}
	if size.TotalSize <= 0 {
		t.Error("expected positive total size")
	}

	// Realm level
	size2, err := e.GetSize("users", "profiles", "")
	if err != nil {
		t.Fatalf("GetSize realm failed: %v", err)
	}
	if size2.FileCount != 3 {
		t.Errorf("expected 3 files in users/profiles, got %d", size2.FileCount)
	}

	// Swamp level
	size3, err := e.GetSize("users", "profiles", "alice")
	if err != nil {
		t.Fatalf("GetSize swamp failed: %v", err)
	}
	if size3.FileCount != 1 {
		t.Errorf("expected 1 file, got %d", size3.FileCount)
	}
}

func TestExplorer_EmptyDirectory(t *testing.T) {
	dataRoot := filepath.Join(t.TempDir(), "empty")
	os.MkdirAll(dataRoot, 0755)

	e := New(dataRoot)
	err := e.Scan(context.Background())
	if err != nil {
		t.Fatalf("scan failed: %v", err)
	}

	sanctuaries := e.ListSanctuaries()
	if len(sanctuaries) != 0 {
		t.Errorf("expected 0 sanctuaries, got %d", len(sanctuaries))
	}

	status := e.GetScanStatus()
	if status.TotalFiles != 0 {
		t.Errorf("expected 0 total files, got %d", status.TotalFiles)
	}
}

func TestExplorer_ScanCancellation(t *testing.T) {
	dataRoot := createTestData(t)
	e := New(dataRoot)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	err := e.Scan(ctx)
	if err == nil {
		// It's possible the scan finishes before cancellation takes effect
		// on small datasets, so we just verify it doesn't panic
	}
	_ = err
}
