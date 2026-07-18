package swamp

import (
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hydraide/hydraide/app/core/hydra/swamp/chronicler"
	"github.com/hydraide/hydraide/app/core/hydra/swamp/metadata"
	"github.com/hydraide/hydraide/app/core/hydra/swamp/treasure"
	"github.com/hydraide/hydraide/app/name"
)

// TestLostWriteOnCloseListenerRace reproduces a SILENT LOST WRITE: a Save that
// reports success to its caller while the treasure never reaches disk.
//
// The protection that is supposed to prevent this: hydra.SummonSwamp calls
// swampObject.IsClosing() before handing the swamp to the caller. IsClosing()
// deliberately bumps lastInteractionTime so the idle evictor cannot close the
// swamp out from under a caller that is still on its way to BeginVigil().
//
// The defect: startCloseListener samples lastInteractionTime into a LOCAL
// variable at the top of the tick, then acquires closeWriteMutex, and only
// then decides — using that stale copy. A bump that lands in between is
// invisible. The decision therefore predates the hand-out while its execution
// postdates it: the caller legitimately observes closing==0, receives a
// healthy swamp, and the evictor closes it anyway a moment later.
//
// Because Close() has already flushed and stopped the writer goroutine, and
// because SaveFunction does not consult the closing flag, the subsequent write
// lands in an orphaned in-memory instance. It is reported as StatusNew — a
// successful save — and is then lost forever.
//
// The window is nanoseconds wide in production, so the test drives it through
// closeListenerTestHook, which fires exactly between the clock sample and the
// close decision. Inside the hook the test performs the summon-side bump. This
// is not an artificial scenario; it is the same interleaving, made reliable.
//
// Fixed behaviour: the evictor must observe the bump and leave the swamp open,
// so the write survives the round trip to disk.
func TestLostWriteOnCloseListenerRace(t *testing.T) {

	const (
		closeAfterIdle = 1 * time.Second
		writeInterval  = 100 * time.Millisecond
		// closeAfterIdle + closeGapDuration (1s, hardcoded in the listener),
		// plus margin, so the sampled clock is already close-eligible.
		idleWait = closeAfterIdle + 1*time.Second + 1500*time.Millisecond
		// The key written through the unprotected window. If the bug is live,
		// this is the key that vanishes.
		racedKey = "raced-write.hu"
	)

	tmpDir := t.TempDir()
	dataRoot := filepath.Join(tmpDir, "hydraide-data")
	if err := os.MkdirAll(dataRoot, 0o755); err != nil {
		t.Fatalf("mkdir data root: %v", err)
	}

	swampName := name.New().
		Sanctuary("lost-write-test").
		Realm("close-listener-race").
		Swamp("hu")

	hashPath := swampName.GetFullHashPath(dataRoot, dataLossIslandID, dataLossMaxDepth, dataLossMaxFolders)

	lwNoopEvent := func(*Event) {}
	lwNoopInfo := func(*Info) {}

	// === Phase 1: resident swamp with a seeded treasure ===

	chron := chronicler.NewV2WithName(hashPath, dataLossMaxDepth, swampName.Get())
	chron.CreateDirectoryIfNotExists()
	meta := metadata.NewNoop()
	meta.SetSwampName(swampName)

	var closed int32
	closeCb := func(name.Name) { atomic.StoreInt32(&closed, 1) }

	fss := &FilesystemSettings{ChroniclerInterface: chron, WriteInterval: writeInterval}
	sw := New(swampName, closeAfterIdle, fss, lwNoopEvent, lwNoopInfo, closeCb, meta)

	sw.BeginVigil()
	seedTr := sw.CreateTreasure("seed.hu")
	if seedTr == nil {
		sw.CeaseVigil()
		t.Fatal("CreateTreasure returned nil for the seed key")
	}
	seedGuard := seedTr.StartTreasureGuard(true)
	seedTr.SetContentString(seedGuard, "seed")
	seedTr.SetCreatedAt(seedGuard, time.Now())
	_ = seedTr.Save(seedGuard)
	seedTr.ReleaseTreasureGuard(seedGuard)
	sw.CeaseVigil()

	// === Phase 2: arm the hook and let the swamp go idle ===

	// The hook is armed straight away but stays inert until the tick whose own
	// sample says the swamp is close-eligible. That is the only tick that can
	// evict, so it is the only one worth racing.
	var (
		once         sync.Once
		bumpObserved = make(chan bool, 1)
	)

	// The listener's own threshold: closeAfterIdle + its hardcoded 1s gap.
	const evictionThreshold = closeAfterIdle + 1*time.Second

	hook := func(sampledIdle time.Duration) {
		if sampledIdle <= evictionThreshold {
			return
		}
		once.Do(func() {
			// This models the summon side: hydra.SummonSwamp calls IsClosing()
			// on the resident swamp, which bumps the interaction clock and
			// reports whether the swamp is already closing. The listener has
			// ALREADY sampled the clock at this point but has NOT yet decided.
			bumpObserved <- sw.IsClosing()
		})
	}
	closeListenerTestHook.Store(&hook)
	t.Cleanup(func() { closeListenerTestHook.Store(nil) })

	var isClosingAtSummon bool
	select {
	case isClosingAtSummon = <-bumpObserved:
	case <-time.After(idleWait + 10*time.Second):
		t.Fatal("close listener never reached the test hook; the tick loop is not running")
	}

	// If the swamp already reported itself as closing, the caller would have
	// taken hydra's WaitForGracefulClose + re-summon path and no write would
	// ever touch this instance. That is the correct, non-racy interleaving and
	// there is nothing to prove here.
	if isClosingAtSummon {
		t.Skip("swamp was already closing at summon time; the raced interleaving did not occur")
	}

	// === Phase 3: the write, through the unprotected summon -> BeginVigil gap ===

	// The caller was handed a healthy swamp (closing==0 above). It now does
	// what every call site does: BeginVigil, mutate, CeaseVigil.
	sw.BeginVigil()
	tr := sw.CreateTreasure(racedKey)
	if tr == nil {
		sw.CeaseVigil()
		t.Fatalf("CreateTreasure returned nil for %q", racedKey)
	}
	guardID := tr.StartTreasureGuard(true)
	tr.SetContentString(guardID, "must-survive")
	tr.SetCreatedAt(guardID, time.Now())
	saveStatus := tr.Save(guardID)
	tr.ReleaseTreasureGuard(guardID)
	sw.CeaseVigil()

	// The save must not silently claim success. Either it is rejected (so the
	// caller can retry), or it is honoured and survives to disk. Reporting
	// StatusNew for a write that never lands is the failure mode under test.
	saveReportedSuccess := saveStatus == treasure.StatusNew || saveStatus == treasure.StatusModified

	// Flush whatever state the swamp still holds. If the evictor already
	// closed it this is a no-op; the treasure is either on disk or gone.
	sw.Close()

	// Read the eviction flag only after Close() has fully settled: the closed
	// event is emitted at the very end of the teardown, so an earlier read
	// would race the listener's own in-flight close.
	evicted := atomic.LoadInt32(&closed) == 1

	// === Phase 4: re-summon — a fresh instance reading the same files ===

	chron2 := chronicler.NewV2WithName(hashPath, dataLossMaxDepth, swampName.Get())
	chron2.CreateDirectoryIfNotExists()
	meta2 := metadata.NewNoop()
	meta2.SetSwampName(swampName)

	fss2 := &FilesystemSettings{ChroniclerInterface: chron2, WriteInterval: writeInterval}
	sw2 := New(swampName, 1*time.Hour, fss2, lwNoopEvent, lwNoopInfo, func(name.Name) {}, meta2)
	sw2.BeginVigil()
	_, survived := sw2.GetTreasure(racedKey)
	postCount := sw2.CountTreasures()
	sw2.CeaseVigil()
	sw2.Close()

	t.Logf("evicted during the raced window: %v, save status: %v, treasures after re-summon: %d",
		evicted, saveStatus, postCount)

	if saveReportedSuccess && survived != nil {
		t.Fatalf("SILENT LOST WRITE: Save(%q) reported %v (success) but the treasure is absent "+
			"after re-summon (%v). The close listener evicted the swamp using a clock sample taken "+
			"before the summon-side IsClosing() bump, and SaveFunction wrote into the orphaned instance.",
			racedKey, saveStatus, survived)
	}

	if !saveReportedSuccess {
		t.Logf("Save was rejected with status %v — no silent loss, the caller can retry", saveStatus)
	}
}
