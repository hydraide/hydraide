package swamp

import (
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hydraide/hydraide/app/core/hydra/swamp/chronicler"
	v2 "github.com/hydraide/hydraide/app/core/hydra/swamp/chronicler/v2"
	"github.com/hydraide/hydraide/app/core/hydra/swamp/metadata"
	"github.com/hydraide/hydraide/app/name"
)

// These tests target the Trendizz "large swamp Count() loses ~99% after
// CloseAfterIdle eviction" bug report. They reproduce the full swamp
// lifecycle: New -> Save N -> idle eviction (Close fires from
// startCloseListener) -> re-summon (fresh swamp.New on the same path) ->
// Count.
//
// If this test passes for large N, the eviction-and-reload pipeline is
// proven symmetric with a process restart, and the reported asymmetry
// (eviction=12, restart=256K) cannot originate inside the engine.

const (
	dataLossTestSanctuary = "dataloss-test"
	dataLossMaxDepth      = 3
	dataLossMaxFolders    = 2000
	dataLossIslandID      = uint64(100)
)

// trendizzPayload returns a string of roughly the size of a Trendizz
// DomainStateCatalog record so per-block entry counts match production.
func trendizzPayload(i int) string {
	return fmt.Sprintf(
		"asn=12345 https=true rawq=false readyq=true crawled=%d indexed=%d "+
			"rejected=false banned=false proxy=false reason=0 "+
			"crc=4283919817 title=test-domain-%d ts=1715238912",
		i%2, i%3, i,
	)
}

// runSwampLifecycleRoundTrip writes N treasures into a fresh swamp, waits
// for idle-eviction Close(), then opens a brand-new swamp on the same
// on-disk file and returns its Count(). Forensic stats from the .hyd file
// are returned alongside.
func runSwampLifecycleRoundTrip(t *testing.T, n int) (
	postEvictionCount int,
	headerEntryCount uint64,
	scanEntryCount uint64,
	scanBlockCount uint64,
	fileSize int64,
) {
	t.Helper()

	tmpDir := t.TempDir()
	dataRoot := filepath.Join(tmpDir, "hydraide-data")
	if err := os.MkdirAll(dataRoot, 0o755); err != nil {
		t.Fatalf("mkdir data root: %v", err)
	}

	swampName := name.New().
		Sanctuary(dataLossTestSanctuary).
		Realm("domain-state").
		Swamp(fmt.Sprintf("hu-%d", n))

	hashPath := swampName.GetFullHashPath(dataRoot, dataLossIslandID, dataLossMaxDepth, dataLossMaxFolders)
	hydPath := hashPath + ".hyd"

	// Aggressive idle/write timings so the eviction fires within the test.
	closeAfterIdle := 2 * time.Second
	writeInterval := 200 * time.Millisecond

	// === Phase 1: build the first swamp, save N treasures ===
	closed1 := int32(0)
	close1Cb := func(n name.Name) { atomic.StoreInt32(&closed1, 1) }
	noopEvent := func(*Event) {}
	noopInfo := func(*Info) {}

	chron1 := chronicler.NewV2WithName(hashPath, dataLossMaxDepth, swampName.Get())
	chron1.CreateDirectoryIfNotExists()
	meta1 := metadata.NewNoop()
	meta1.SetSwampName(swampName)

	fss1 := &FilesystemSettings{ChroniclerInterface: chron1, WriteInterval: writeInterval}
	sw1 := New(swampName, closeAfterIdle, fss1, noopEvent, noopInfo, close1Cb, meta1)

	// Hold the swamp open with a vigil while we write — otherwise a slow
	// SaveFunction-driven Save() might race the closeListener for very
	// small N.
	sw1.BeginVigil()

	for i := 0; i < n; i++ {
		key := fmt.Sprintf("test%07d.hu", i)
		tr := sw1.CreateTreasure(key)
		if tr == nil {
			sw1.CeaseVigil()
			t.Fatalf("CreateTreasure returned nil at i=%d", i)
		}
		guardID := tr.StartTreasureGuard(true)
		tr.SetContentString(guardID, trendizzPayload(i))
		_ = tr.Save(guardID)
		tr.ReleaseTreasureGuard(guardID)
	}

	// Sanity check: in-memory count matches what we just wrote.
	if got := sw1.CountTreasures(); got != n {
		sw1.CeaseVigil()
		t.Fatalf("in-memory CountTreasures right after Save loop: got %d, expected %d", got, n)
	}

	sw1.CeaseVigil()

	// === Phase 2: wait for idle-eviction Close() ===
	// closeListener checks every 1s with closeGapDuration=1s. With
	// closeAfterIdle=2s we expect close to fire by ~3-4s after the last
	// interaction. Give it a generous bound.
	// The evictor deliberately refuses to close while the file writer is
	// still flushing (isFilesystemWritingActive), so the wait has to scale
	// with N rather than being a flat budget. Measured under -race: a 100k
	// swamp closes well inside 15s, while 256k needs ~30s because the flush
	// of that backlog dominates. The 15s floor keeps small-N behaviour
	// exactly as before.
	evictionWait := 15*time.Second + time.Duration(n/5000)*time.Second
	deadline := time.Now().Add(evictionWait)
	for time.Now().Before(deadline) {
		if atomic.LoadInt32(&closed1) == 1 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if atomic.LoadInt32(&closed1) != 1 {
		t.Fatalf("swamp did not close within %s of idle (n=%d)", evictionWait, n)
	}

	// Give Close() a moment to finish the chronicler.Close() inside it.
	// (The close callback fires AFTER chronicler.Close in swamp.Close.)
	time.Sleep(100 * time.Millisecond)

	// === Phase 3: disk forensics on the .hyd file ===
	fi, err := os.Stat(hydPath)
	if err != nil {
		t.Fatalf("stat .hyd after eviction: %v", err)
	}
	fileSize = fi.Size()

	reader, err := v2.NewFileReader(hydPath)
	if err != nil {
		t.Fatalf("open reader after eviction: %v", err)
	}
	if hdr := reader.GetHeader(); hdr != nil {
		headerEntryCount = hdr.EntryCount
	}
	scan, err := reader.ScanBlockHeaders()
	if err != nil {
		_ = reader.Close()
		t.Fatalf("scan block headers: %v", err)
	}
	scanEntryCount = scan.TotalEntryCount
	scanBlockCount = scan.BlockCount
	_ = reader.Close()

	// === Phase 4: simulate re-summon — fresh swamp on the same path ===
	chron2 := chronicler.NewV2WithName(hashPath, dataLossMaxDepth, swampName.Get())
	chron2.CreateDirectoryIfNotExists()
	meta2 := metadata.NewNoop()
	meta2.SetSwampName(swampName)

	closed2 := int32(0)
	close2Cb := func(n name.Name) { atomic.StoreInt32(&closed2, 1) }

	fss2 := &FilesystemSettings{ChroniclerInterface: chron2, WriteInterval: writeInterval}
	// Long closeAfterIdle so we can safely Count without a race.
	sw2 := New(swampName, 1*time.Hour, fss2, noopEvent, noopInfo, close2Cb, meta2)
	sw2.BeginVigil()
	postEvictionCount = sw2.CountTreasures()
	sw2.CeaseVigil()
	sw2.Destroy()
	_ = closed2

	return
}

// TestDataLoss_MultiEvictionCycles writes a base set, evicts, re-summons,
// adds more records, evicts again, and so on. This stresses the inline-Load
// compaction path on a non-trivial file and detects state-accumulation bugs
// (e.g. EntryCount drift in the header, beacon double-add corrupting Count).
//
// Trendizz's own measurement shows a swamp that has been idle-evicted MANY
// times across 12h — if any single eviction step subtly drops a fraction
// of entries, this test catches it cumulatively.
func TestDataLoss_MultiEvictionCycles(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping multi-eviction stress in -short mode")
	}

	tmpDir := t.TempDir()
	dataRoot := filepath.Join(tmpDir, "hydraide-data")
	if err := os.MkdirAll(dataRoot, 0o755); err != nil {
		t.Fatalf("mkdir data root: %v", err)
	}

	swampName := name.New().
		Sanctuary(dataLossTestSanctuary).
		Realm("multi-eviction").
		Swamp("hu")
	hashPath := swampName.GetFullHashPath(dataRoot, dataLossIslandID, dataLossMaxDepth, dataLossMaxFolders)
	hydPath := hashPath + ".hyd"

	closeAfterIdle := 2 * time.Second
	writeInterval := 200 * time.Millisecond

	noopEvent := func(*Event) {}
	noopInfo := func(*Info) {}

	const baseN = 50_000
	const cycles = 5
	const perCycle = 1_000

	// === Initial bootstrap with baseN entries ===
	addAndEvict := func(label string, addFrom int, addCount int) (postCount int, hdrCount uint64, scanCount uint64) {
		t.Helper()

		closed := int32(0)
		closeCb := func(n name.Name) { atomic.StoreInt32(&closed, 1) }

		chron := chronicler.NewV2WithName(hashPath, dataLossMaxDepth, swampName.Get())
		chron.CreateDirectoryIfNotExists()
		meta := metadata.NewNoop()
		meta.SetSwampName(swampName)
		fss := &FilesystemSettings{ChroniclerInterface: chron, WriteInterval: writeInterval}
		sw := New(swampName, closeAfterIdle, fss, noopEvent, noopInfo, closeCb, meta)
		sw.BeginVigil()

		// Sanity: in-memory count after Load equals the previously persisted count.
		// (For the first bootstrap call this is 0.)
		preCount := sw.CountTreasures()

		for i := addFrom; i < addFrom+addCount; i++ {
			key := fmt.Sprintf("test%07d.hu", i)
			tr := sw.CreateTreasure(key)
			guardID := tr.StartTreasureGuard(true)
			tr.SetContentString(guardID, trendizzPayload(i))
			_ = tr.Save(guardID)
			tr.ReleaseTreasureGuard(guardID)
		}

		expected := preCount + addCount
		if got := sw.CountTreasures(); got != expected {
			sw.CeaseVigil()
			t.Fatalf("[%s] in-memory count after add: got %d expected %d (preCount=%d addCount=%d)",
				label, got, expected, preCount, addCount)
		}
		sw.CeaseVigil()

		// Wait for eviction Close
		deadline := time.Now().Add(15 * time.Second)
		for time.Now().Before(deadline) {
			if atomic.LoadInt32(&closed) == 1 {
				break
			}
			time.Sleep(50 * time.Millisecond)
		}
		if atomic.LoadInt32(&closed) != 1 {
			t.Fatalf("[%s] swamp did not close within 15s", label)
		}
		time.Sleep(100 * time.Millisecond)

		// Disk forensics
		reader, err := v2.NewFileReader(hydPath)
		if err != nil {
			t.Fatalf("[%s] open reader: %v", label, err)
		}
		if hdr := reader.GetHeader(); hdr != nil {
			hdrCount = hdr.EntryCount
		}
		scan, err := reader.ScanBlockHeaders()
		if err != nil {
			_ = reader.Close()
			t.Fatalf("[%s] scan: %v", label, err)
		}
		scanCount = scan.TotalEntryCount
		_ = reader.Close()

		// Re-summon — fresh swamp, identical path
		chron2 := chronicler.NewV2WithName(hashPath, dataLossMaxDepth, swampName.Get())
		chron2.CreateDirectoryIfNotExists()
		meta2 := metadata.NewNoop()
		meta2.SetSwampName(swampName)
		fss2 := &FilesystemSettings{ChroniclerInterface: chron2, WriteInterval: writeInterval}
		sw2 := New(swampName, 1*time.Hour, fss2, noopEvent, noopInfo, func(name.Name) {}, meta2)
		sw2.BeginVigil()
		postCount = sw2.CountTreasures()
		sw2.CeaseVigil()
		// IMPORTANT: Close (not Destroy) — Destroy would delete the .hyd file
		// and break the next cycle. Close just flushes and releases the writer.
		sw2.Close()
		// Wait briefly for Close goroutine teardown.
		time.Sleep(100 * time.Millisecond)
		return
	}

	expectedTotal := 0
	post, hdr, scan := addAndEvict("bootstrap", 0, baseN)
	expectedTotal = baseN
	t.Logf("bootstrap: post=%d hdr=%d scan=%d (expected %d)", post, hdr, scan, expectedTotal)
	if post != expectedTotal {
		t.Fatalf("BOOTSTRAP DATA LOSS: post=%d expected=%d (delta %d, hdr=%d scan=%d)",
			post, expectedTotal, expectedTotal-post, hdr, scan)
	}

	for cycle := 1; cycle <= cycles; cycle++ {
		addFrom := baseN + (cycle-1)*perCycle
		post, hdr, scan = addAndEvict(fmt.Sprintf("cycle-%d", cycle), addFrom, perCycle)
		expectedTotal += perCycle
		t.Logf("cycle %d: added [%d..%d) post=%d hdr=%d scan=%d (expected %d)",
			cycle, addFrom, addFrom+perCycle, post, hdr, scan, expectedTotal)
		if post != expectedTotal {
			t.Fatalf("CYCLE %d DATA LOSS: post=%d expected=%d (delta %d, hdr=%d scan=%d)",
				cycle, post, expectedTotal, expectedTotal-post, hdr, scan)
		}
		// In each cycle the header.EntryCount should match the cumulative
		// total (since all writes are inserts, not updates/deletes).
		if int(hdr) != expectedTotal {
			t.Errorf("CYCLE %d header drift: header=%d expected=%d", cycle, hdr, expectedTotal)
		}
	}
}

// TestDataLoss_TightLoopEvictionStress runs many short save+evict+re-summon
// cycles back-to-back. The point is to exercise the eviction Close() and the
// re-summon Load() under realistic timing pressure: each cycle the writer is
// freshly opened on an existing file and asked to append a small batch.
//
// If there is any subtle bug where a fraction of written entries is silently
// dropped per eviction (race between fileWriterHandler ticker and
// closeListener firing), the cumulative drift over 100 cycles would surface.
func TestDataLoss_TightLoopEvictionStress(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping tight-loop eviction stress in -short mode")
	}

	tmpDir := t.TempDir()
	dataRoot := filepath.Join(tmpDir, "hydraide-data")
	if err := os.MkdirAll(dataRoot, 0o755); err != nil {
		t.Fatalf("mkdir data root: %v", err)
	}

	swampName := name.New().
		Sanctuary(dataLossTestSanctuary).
		Realm("tight-loop").
		Swamp("hu")
	hashPath := swampName.GetFullHashPath(dataRoot, dataLossIslandID, dataLossMaxDepth, dataLossMaxFolders)
	hydPath := hashPath + ".hyd"

	closeAfterIdle := 1 * time.Second
	writeInterval := 50 * time.Millisecond

	noopEvent := func(*Event) {}
	noopInfo := func(*Info) {}

	const cycles = 100
	const perCycle = 50 // small batches — exercise short-lived swamp

	totalExpected := 0
	for cycle := 0; cycle < cycles; cycle++ {
		closed := int32(0)
		closeCb := func(n name.Name) { atomic.StoreInt32(&closed, 1) }

		chron := chronicler.NewV2WithName(hashPath, dataLossMaxDepth, swampName.Get())
		chron.CreateDirectoryIfNotExists()
		meta := metadata.NewNoop()
		meta.SetSwampName(swampName)
		fss := &FilesystemSettings{ChroniclerInterface: chron, WriteInterval: writeInterval}
		sw := New(swampName, closeAfterIdle, fss, noopEvent, noopInfo, closeCb, meta)
		sw.BeginVigil()

		preCount := sw.CountTreasures()
		if preCount != totalExpected {
			sw.CeaseVigil()
			t.Fatalf("cycle %d: post-Load count drift: got=%d expected=%d (file might be corrupted)",
				cycle, preCount, totalExpected)
		}

		for i := 0; i < perCycle; i++ {
			key := fmt.Sprintf("test%07d.hu", totalExpected+i)
			tr := sw.CreateTreasure(key)
			guardID := tr.StartTreasureGuard(true)
			tr.SetContentString(guardID, trendizzPayload(totalExpected+i))
			_ = tr.Save(guardID)
			tr.ReleaseTreasureGuard(guardID)
		}
		totalExpected += perCycle

		if got := sw.CountTreasures(); got != totalExpected {
			sw.CeaseVigil()
			t.Fatalf("cycle %d: post-save in-memory count: got=%d expected=%d",
				cycle, got, totalExpected)
		}
		sw.CeaseVigil()

		// Wait for idle eviction
		deadline := time.Now().Add(10 * time.Second)
		for time.Now().Before(deadline) {
			if atomic.LoadInt32(&closed) == 1 {
				break
			}
			time.Sleep(20 * time.Millisecond)
		}
		if atomic.LoadInt32(&closed) != 1 {
			t.Fatalf("cycle %d: swamp did not close within 10s", cycle)
		}
		// Brief gap so any post-close goroutine teardown completes.
		time.Sleep(50 * time.Millisecond)
	}

	// Final integrity check — fresh chronicler, scan + load, compare to total
	reader, err := v2.NewFileReader(hydPath)
	if err != nil {
		t.Fatalf("final reader open: %v", err)
	}
	hdr := reader.GetHeader()
	scan, err := reader.ScanBlockHeaders()
	if err != nil {
		_ = reader.Close()
		t.Fatalf("final scan: %v", err)
	}
	_ = reader.Close()

	chron2 := chronicler.NewV2WithName(hashPath, dataLossMaxDepth, swampName.Get())
	chron2.CreateDirectoryIfNotExists()
	meta2 := metadata.NewNoop()
	meta2.SetSwampName(swampName)
	fss2 := &FilesystemSettings{ChroniclerInterface: chron2, WriteInterval: writeInterval}
	sw2 := New(swampName, 1*time.Hour, fss2, noopEvent, noopInfo, func(name.Name) {}, meta2)
	sw2.BeginVigil()
	finalCount := sw2.CountTreasures()
	sw2.CeaseVigil()
	sw2.Close()
	time.Sleep(100 * time.Millisecond)

	t.Logf("after %d cycles of perCycle=%d: header.EntryCount=%d "+
		"scan.EntryCount=%d (cumulative live; compaction may have run) "+
		"final-summon-Count=%d expected=%d",
		cycles, perCycle, hdr.EntryCount, scan.TotalEntryCount, finalCount, totalExpected)

	if finalCount != totalExpected {
		t.Fatalf("FINAL DATA LOSS: got=%d expected=%d (delta=%d)",
			finalCount, totalExpected, totalExpected-finalCount)
	}
}

func TestDataLoss_SwampLifecycle_SizeSweep(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping large-N swamp lifecycle test in -short mode")
	}

	cases := []struct {
		name string
		n    int
	}{
		{"N=300", 300},
		{"N=1000", 1000},
		{"N=10000", 10000},
		{"N=100000", 100_000},
		// 256K too is tractable but slow (~2-3 min for the Save loop).
		// Run it but allow the timeout to absorb it.
		{"N=256164", 256_164},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			loaded, hdrCount, scanCount, blocks, fileSize := runSwampLifecycleRoundTrip(t, c.n)
			t.Logf("N=%d -> file_size=%d blocks=%d header.EntryCount=%d "+
				"scan.EntryCount=%d post-eviction-Count=%d",
				c.n, fileSize, blocks, hdrCount, scanCount, loaded)

			if loaded != c.n {
				t.Fatalf("DATA LOSS after eviction: wrote %d, post-summon Count=%d "+
					"(delta %d, file_size=%d, header=%d, scan=%d)",
					c.n, loaded, c.n-loaded, fileSize, hdrCount, scanCount)
			}
			if int(hdrCount) != c.n {
				t.Errorf("header.EntryCount mismatch: header=%d expected=%d "+
					"(if header < N, the writer.Close header update is broken)",
					hdrCount, c.n)
			}
			if int(scanCount) != c.n {
				t.Errorf("scan.EntryCount mismatch: scan=%d expected=%d "+
					"(if scan < N, the buffer tail was dropped on Close)",
					scanCount, c.n)
			}
		})
	}
}
