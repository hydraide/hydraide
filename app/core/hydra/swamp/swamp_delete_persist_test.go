package swamp

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/hydraide/hydraide/app/core/hydra/swamp/chronicler"
	v2 "github.com/hydraide/hydraide/app/core/hydra/swamp/chronicler/v2"
	"github.com/hydraide/hydraide/app/core/hydra/swamp/metadata"
	"github.com/hydraide/hydraide/app/core/hydra/swamp/treasure"
	"github.com/hydraide/hydraide/app/core/hydra/swamp/treasure/guard"
	"github.com/hydraide/hydraide/app/name"
	"github.com/stretchr/testify/require"
)

// newV2PersistSwamp opens a V2 swamp whose .hyd file the test can replay.
func newV2PersistSwamp(t *testing.T, swampN string) (*swamp, string) {
	t.Helper()
	dataRoot := filepath.Join(t.TempDir(), "hydraide-data")
	require.NoError(t, os.MkdirAll(dataRoot, 0o755))
	swampName := name.New().Sanctuary("delete-persist").Realm("queue").Swamp(swampN)
	hashPath := swampName.GetFullHashPath(dataRoot, dataLossIslandID, dataLossMaxDepth, dataLossMaxFolders)
	chron := chronicler.NewV2WithName(hashPath, dataLossMaxDepth, swampName.Get())
	chron.CreateDirectoryIfNotExists()
	meta := metadata.NewNoop()
	meta.SetSwampName(swampName)
	fss := &FilesystemSettings{ChroniclerInterface: chron, WriteInterval: time.Hour}
	sw := New(swampName, time.Hour, fss, func(*Event) {}, func(*Info) {}, func(name.Name) {}, meta)
	sw.BeginVigil()
	t.Cleanup(func() {
		sw.CeaseVigil()
		sw.Destroy()
	})
	return sw.(*swamp), hashPath + ".hyd"
}

// liveKeysOnDisk replays the .hyd file the way a load does and returns the
// keys whose last entry is an insert or update.
func liveKeysOnDisk(t *testing.T, hydPath string) map[string]bool {
	t.Helper()
	reader, err := v2.NewFileReader(hydPath)
	require.NoError(t, err)
	defer reader.Close()
	live := map[string]bool{}
	_, err = reader.ReadAllEntries(func(e v2.Entry) bool {
		switch e.Operation {
		case v2.OpDelete:
			delete(live, e.Key)
		default:
			live[e.Key] = true
		}
		return true
	})
	require.NoError(t, err)
	return live
}

func putString(t *testing.T, s *swamp, key, value string) {
	t.Helper()
	tr := s.CreateTreasure(key)
	gid := tr.StartTreasureGuard(true)
	tr.SetContentString(gid, value)
	tr.Save(gid)
	tr.ReleaseTreasureGuard(gid)
}

func flushV2(s *swamp) {
	s.fileWriterHandler(false)
}

// TestDelete_RecreatedKeyDeletedBeforeFlush_StaysDeletedOnDisk: the key is on
// disk (A), gets deleted (pending delete in the write buffer), is re-created
// as a new object (B) and B is deleted again before the next flush. The
// re-create used to drop A's pending delete from the write buffer, and the
// delete of the never-written B wrote nothing, so A came back after a restart.
func TestDelete_RecreatedKeyDeletedBeforeFlush_StaysDeletedOnDisk(t *testing.T) {
	s, hyd := newV2PersistSwamp(t, "recreate-then-delete")
	putString(t, s, "anchor", "v")
	putString(t, s, "k", "A")
	flushV2(s)
	require.NotNil(t, s.CreateTreasure("k").GetFileName(), "precondition: A is on disk")

	require.NoError(t, s.DeleteTreasure("k", false))
	putString(t, s, "k", "B")
	require.NoError(t, s.DeleteTreasure("k", false))
	flushV2(s)

	require.False(t, s.TreasureExists("k"))
	require.False(t, liveKeysOnDisk(t, hyd)["k"], "k was deleted but is still live on disk: it resurrects on the next load")
}

// TestDelete_RecreatedKeyFlushed_PersistsNewValue: the same sequence without
// the second delete must persist B.
func TestDelete_RecreatedKeyFlushed_PersistsNewValue(t *testing.T) {
	s, hyd := newV2PersistSwamp(t, "recreate-then-flush")
	putString(t, s, "anchor", "v")
	putString(t, s, "k", "A")
	flushV2(s)

	require.NoError(t, s.DeleteTreasure("k", false))
	putString(t, s, "k", "B")
	flushV2(s)

	cur, err := s.GetTreasure("k")
	require.NoError(t, err)
	v, _ := cur.GetContentString()
	require.Equal(t, "B", v)
	require.True(t, liveKeysOnDisk(t, hyd)["k"])
}

// TestShift_FlushedRowsStayDeletedOnDisk: rows shifted after they were
// written must be deleted on disk too.
func TestShift_FlushedRowsStayDeletedOnDisk(t *testing.T) {
	s, hyd := newV2PersistSwamp(t, "shift-flushed")
	putString(t, s, "anchor", "v")
	for _, k := range []string{"a", "b", "c"} {
		tr := s.CreateTreasure(k)
		gid := tr.StartTreasureGuard(true)
		tr.SetContentString(gid, k)
		tr.SetExpirationTime(gid, time.Now().Add(-time.Minute))
		require.Equal(t, treasure.StatusNew, tr.Save(gid))
		tr.ReleaseTreasureGuard(gid)
	}
	flushV2(s)
	shifted, err := s.CloneAndDeleteExpiredTreasures(10)
	require.NoError(t, err)
	require.Len(t, shifted, 3)
	flushV2(s)

	live := liveKeysOnDisk(t, hyd)
	for _, k := range []string{"a", "b", "c"} {
		require.Falsef(t, live[k], "shifted key %s is still live on disk", k)
	}
	require.True(t, live["anchor"])
}

// TestDelete_UnwrittenRowInWriterBatch_StaysDeletedOnDisk: the file writer
// takes the write buffer and then writes each treasure under its guard. A row
// that had never been written and was deleted while the writer waited for its
// guard used to be removed from the (already taken) buffer without a delete
// marker, so the writer persisted it as live and it came back on the next load.
func TestDelete_UnwrittenRowInWriterBatch_StaysDeletedOnDisk(t *testing.T) {
	s, hyd := newV2PersistSwamp(t, "unwritten-in-batch")
	putString(t, s, "anchor", "v")
	flushV2(s)
	putString(t, s, "x", "never-written")

	x := s.CreateTreasure("x")
	require.Nil(t, x.GetFileName(), "precondition: x is only in the write buffer")
	gid := x.StartTreasureGuard(true, guard.BodyAuthID)

	writerDone := make(chan struct{})
	go func() { defer close(writerDone); flushV2(s) }()
	if !waitForGoroutineBlockedIn(5*time.Second, "(*chroniclerV2).Write", "guard.(*guard).StartTreasureGuard") {
		x.ReleaseTreasureGuard(gid)
		t.Fatal("the writer never waited for x's guard")
	}

	// delete x while the writer holds it in its batch
	require.NotNil(t, s.deleteGuardedTreasure(x, gid, false))
	x.ReleaseTreasureGuard(gid)
	select {
	case <-writerDone:
	case <-time.After(5 * time.Second):
		t.Fatalf("writer did not finish.\n%s", relevantStacks())
	}

	require.False(t, s.TreasureExists("x"))
	require.False(t, liveKeysOnDisk(t, hyd)["x"], "x was deleted but the writer persisted it as live")
}
