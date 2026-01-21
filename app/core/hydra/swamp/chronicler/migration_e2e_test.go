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
func TestV1ToV2Migration_SmallSwamp(t *testing.T) {
tmpDir := t.TempDir()
swampPath := filepath.Join(tmpDir, "test-swamp")
fs := filesystem.New()
meta := metadata.New(swampPath)
v1Chron := New(swampPath, 250*1024, 10, fs, meta)
v1Chron.CreateDirectoryIfNotExists()
originalTreasures := make(map[string]string)
v1Treasures := make([]treasure.Treasure, 10)
for i := 0; i < 10; i++ {
tr := treasure.New(nil)
guardID := tr.StartTreasureGuard(false, guard.BodyAuthID)
key := fmt.Sprintf("key-%d", i)
content := fmt.Sprintf("content-%d", i)
tr.BodySetKey(guardID, key)
tr.SetContentString(guardID, content)
tr.ReleaseTreasureGuard(guardID)
v1Treasures[i] = tr
originalTreasures[key] = content
}
v1Chron.Write(v1Treasures)
v1Beacon := beacon.New()
v1Chron.Load(v1Beacon)
if v1Beacon.Count() != 10 {
t.Fatalf("V1: expected 10, got %d", v1Beacon.Count())
}
v2SwampPath := filepath.Join(tmpDir, "test-swamp-v2")
v2Chron := NewV2(v2SwampPath, 10)
v2Chron.CreateDirectoryIfNotExists()
v2Treasures := make([]treasure.Treasure, 0, 10)
for _, tr := range v1Beacon.GetAll() {
v2Treasures = append(v2Treasures, tr)
}
v2Chron.Write(v2Treasures)
v2HydFile := v2SwampPath + ".hyd"
if _, err := os.Stat(v2HydFile); os.IsNotExist(err) {
t.Fatal("V2 .hyd file should exist")
}
v2Beacon := beacon.New()
v2Chron.Load(v2Beacon)
if v2Beacon.Count() != 10 {
t.Fatalf("V2: expected 10, got %d", v2Beacon.Count())
}
for key, expectedContent := range originalTreasures {
tr := v2Beacon.Get(key)
if tr == nil {
t.Errorf("V2 missing: %s", key)
continue
}
guardID := tr.StartTreasureGuard(true, guard.BodyAuthID)
content := tr.CloneContent(guardID)
tr.ReleaseTreasureGuard(guardID)
if content.String == nil || *content.String != expectedContent {
t.Errorf("V2 mismatch %s", key)
}
}
}
func TestV1ToV2Migration_Persistence(t *testing.T) {
tmpDir := t.TempDir()
v2SwampPath := filepath.Join(tmpDir, "persistent-swamp")
v2Chron1 := NewV2(v2SwampPath, 10)
v2Chron1.CreateDirectoryIfNotExists()
treasures := make([]treasure.Treasure, 50)
for i := 0; i < 50; i++ {
tr := treasure.New(nil)
guardID := tr.StartTreasureGuard(false, guard.BodyAuthID)
tr.BodySetKey(guardID, fmt.Sprintf("key-%d", i))
tr.SetContentString(guardID, fmt.Sprintf("content-%d", i))
tr.ReleaseTreasureGuard(guardID)
treasures[i] = tr
}
v2Chron1.Write(treasures)
v2Chron2 := NewV2(v2SwampPath, 10)
v2Beacon := beacon.New()
v2Chron2.Load(v2Beacon)
if v2Beacon.Count() != 50 {
t.Fatalf("After restart: expected 50, got %d", v2Beacon.Count())
}
}
func TestV1ToV2Migration_EmptySwamp(t *testing.T) {
tmpDir := t.TempDir()
v2SwampPath := filepath.Join(tmpDir, "empty-swamp")
v2Chron := NewV2(v2SwampPath, 10)
v2Chron.CreateDirectoryIfNotExists()
v2Beacon := beacon.New()
v2Chron.Load(v2Beacon)
if v2Beacon.Count() != 0 {
t.Errorf("Empty: expected 0, got %d", v2Beacon.Count())
}
}
