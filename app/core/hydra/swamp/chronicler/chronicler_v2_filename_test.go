package chronicler

import (
	"path/filepath"
	"testing"

	"github.com/hydraide/hydraide/app/core/hydra/swamp/beacon"
	"github.com/hydraide/hydraide/app/core/hydra/swamp/treasure"
	"github.com/hydraide/hydraide/app/core/hydra/swamp/treasure/guard"
)

// TestV2Write_SetsFileNameUnderTheWriteGuard: Write must mark a written
// treasure as on disk while it still holds the treasure's guard, not only in
// the file pointer callback that runs after the batch. A delete that landed
// between the two saw FileName == nil, wrote no delete entry, and the row came
// back on the next load. Here no callback is registered at all, so only the
// in-loop marking can set the file name.
func TestV2Write_SetsFileNameUnderTheWriteGuard(t *testing.T) {
	swampPath := filepath.Join(t.TempDir(), "filename-swamp")
	chron := NewV2WithName(swampPath, 10, "test/filename/swamp")
	chron.CreateDirectoryIfNotExists()
	b := beacon.New()
	chron.Load(b)
	chron.RegisterLiveCountFunction(b.Count)

	tr := treasure.New(func(treasure.Treasure, guard.ID) treasure.TreasureStatus { return treasure.StatusNew })
	gid := tr.StartTreasureGuard(true, guard.BodyAuthID)
	tr.BodySetKey(gid, "k")
	tr.SetContentString(gid, "v")
	tr.ReleaseTreasureGuard(gid)
	b.Add(tr)

	if tr.GetFileName() != nil {
		t.Fatal("precondition: a fresh treasure has no file name")
	}
	chron.Write([]treasure.Treasure{tr})
	if tr.GetFileName() == nil {
		t.Fatal("Write did not mark the written treasure as on disk")
	}
	if err := chron.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
}
