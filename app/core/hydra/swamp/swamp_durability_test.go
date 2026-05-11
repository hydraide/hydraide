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

// TestDurability_SmallPayloadFlushesOnTick is the regression test for the
// 2026-05-11 silent-data-loss incident on Trendizz t-outbound-live.
//
// The reported failure: a Catalog with two ~100-byte records was Save()'d,
// reads round-tripped for ~34 minutes, then an ungraceful power loss happened.
// On reboot the .hyd file was header-only (116 bytes) — neither the periodic
// WriteInterval ticks nor the implicit "swamp dirty" state had actually
// flushed the writer's in-memory block buffer to disk, because the dirty
// footprint was well below the V2 16 KiB block threshold.
//
// This test simulates the crash by:
//  1. opening a swamp with a tight WriteInterval (250 ms)
//  2. saving two small treasures
//  3. waiting for two WriteInterval ticks
//  4. NOT calling Close on the swamp (this is what SIGKILL / power loss
//     equivalently does — the writer's in-RAM block buffer is abandoned)
//  5. opening a fresh FileReader on the same .hyd path and confirming both
//     entries were already on disk before the simulated crash.
//
// Pre-fix this test fails: ReadAllEntries returns 0 because flushLocked is
// never reached for sub-16-KiB dirty payloads. Post-fix the periodic Sync()
// in fileWriterHandler forces the block out and fsyncs it.
func TestDurability_SmallPayloadFlushesOnTick(t *testing.T) {
	tmpDir := t.TempDir()
	dataRoot := filepath.Join(tmpDir, "hydraide-data")
	if err := os.MkdirAll(dataRoot, 0o755); err != nil {
		t.Fatalf("mkdir data root: %v", err)
	}

	swampName := name.New().
		Sanctuary("durability-test").
		Realm("small-catalog").
		Swamp("reitterrehab-hu")

	hashPath := swampName.GetFullHashPath(dataRoot, dataLossIslandID, dataLossMaxDepth, dataLossMaxFolders)
	hydPath := hashPath + ".hyd"

	writeInterval := 250 * time.Millisecond
	// Long idle so closeListener does not race the test — we want the flush
	// to be observable solely from the periodic write tick.
	closeAfterIdle := 1 * time.Hour

	noopEvent := func(*Event) {}
	noopInfo := func(*Info) {}
	noopClose := func(name.Name) {}

	chron := chronicler.NewV2WithName(hashPath, dataLossMaxDepth, swampName.Get())
	chron.CreateDirectoryIfNotExists()
	meta := metadata.NewNoop()
	meta.SetSwampName(swampName)

	fss := &FilesystemSettings{ChroniclerInterface: chron, WriteInterval: writeInterval}
	sw := New(swampName, closeAfterIdle, fss, noopEvent, noopInfo, noopClose, meta)
	sw.BeginVigil()

	const recordCount = 2
	for i := 0; i < recordCount; i++ {
		key := fmt.Sprintf("assistant-%d@reitterrehab.hu", i)
		tr := sw.CreateTreasure(key)
		if tr == nil {
			sw.CeaseVigil()
			t.Fatalf("CreateTreasure returned nil at i=%d", i)
		}
		guardID := tr.StartTreasureGuard(true)
		// ~100 bytes payload — same order of magnitude as a real assistant
		// record. Critically: well under 16 KiB even for the whole catalog.
		tr.SetContentString(guardID, fmt.Sprintf(
			"id=%d email=assistant-%d@reitterrehab.hu signature=plain phone=+36209167984",
			i, i,
		))
		_ = tr.Save(guardID)
		tr.ReleaseTreasureGuard(guardID)
	}
	sw.CeaseVigil()

	// Wait long enough that startWriteListener has fired at least twice. The
	// first tick after a Save is the one we are testing.
	time.Sleep(writeInterval * 4)

	// SIMULATED CRASH: do NOT call sw.Close(), sw.Destroy(), or anything that
	// would push the buffer through Close()'s flushLocked path. The bytes on
	// disk at this moment are exactly what would survive a power loss.
	//
	// We do not even attempt to cancel the swamp's internal goroutines; the
	// process-exit equivalent is "the file as it stands right now".

	fi, err := os.Stat(hydPath)
	if err != nil {
		t.Fatalf("stat .hyd: %v", err)
	}
	t.Logf("post-tick file size: %d bytes (header-only would be ~116)", fi.Size())

	reader, err := v2.NewFileReader(hydPath)
	if err != nil {
		t.Fatalf("open reader: %v", err)
	}
	defer reader.Close()

	gotEntries := 0
	if _, err := reader.ReadAllEntries(func(entry v2.Entry) bool {
		gotEntries++
		return true
	}); err != nil {
		t.Fatalf("ReadAllEntries: %v", err)
	}

	if gotEntries != recordCount {
		t.Fatalf("DURABILITY REGRESSION: expected %d entries on disk after "+
			"%d WriteInterval ticks of writes, got %d. file_size=%d. This is "+
			"the t-outbound-live 2026-05-11 silent data-loss class: periodic "+
			"flush did not reach the .hyd file because the dirty payload "+
			"never crossed the V2 block threshold.",
			recordCount, 4, gotEntries, fi.Size())
	}
}

// TestDurability_FileWriterCloseFsyncs is a narrow unit-level guard. Before
// the fix, FileWriter.Close called file.Close() without file.Sync(), so even
// the idle-eviction graceful-shutdown path left freshly written bytes in the
// OS page cache. We cannot reliably observe page-cache state from userspace,
// so instead we verify that Close advances the on-disk size past the header
// when a non-trivial entry has been written — i.e. flushLocked actually ran.
//
// This is mostly a regression marker against accidental reverts of the
// fsync + flush ordering inside FileWriter.Close.
func TestDurability_FileWriterCloseFlushes(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "small.hyd")

	fw, err := v2.NewFileWriterWithName(filePath, v2.DefaultMaxBlockSize, "test/swamp/small")
	if err != nil {
		t.Fatalf("NewFileWriterWithName: %v", err)
	}

	// Single tiny entry — well below 16 KiB block threshold, so Close is the
	// only thing that can put it on disk.
	if err := fw.WriteEntry(v2.Entry{
		Operation: v2.OpInsert,
		Key:       "k1",
		Data:      []byte("v1"),
	}); err != nil {
		t.Fatalf("WriteEntry: %v", err)
	}

	if err := fw.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	fi, err := os.Stat(filePath)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}

	// Header (64) + name ("test/swamp/small" = 16) + serialized block header
	// + compressed entry. Anything > header+name proves a block was flushed.
	const headerPlusName = v2.FileHeaderSize + 16
	if fi.Size() <= int64(headerPlusName) {
		t.Fatalf("FileWriter.Close did not flush block to disk: size=%d "+
			"expected > %d (header+name only). The single entry is sitting "+
			"in RAM and would be lost on crash.", fi.Size(), headerPlusName)
	}

	// Reader can see it.
	reader, err := v2.NewFileReader(filePath)
	if err != nil {
		t.Fatalf("NewFileReader: %v", err)
	}
	defer reader.Close()

	entries := 0
	if _, err := reader.ReadAllEntries(func(entry v2.Entry) bool {
		entries++
		return true
	}); err != nil {
		t.Fatalf("ReadAllEntries: %v", err)
	}
	if entries != 1 {
		t.Fatalf("expected 1 entry on disk after Close, got %d", entries)
	}
}

// TestDurability_TickFlushesBeforeCloseEvenWhenIdle confirms the periodic
// path is the load-bearing one, not Close. The swamp is held open with a
// vigil so closeListener cannot fire; the only path to disk is the
// WriteInterval tick + Sync.
func TestDurability_TickFlushesBeforeCloseEvenWhenIdle(t *testing.T) {
	tmpDir := t.TempDir()
	dataRoot := filepath.Join(tmpDir, "hydraide-data")
	if err := os.MkdirAll(dataRoot, 0o755); err != nil {
		t.Fatalf("mkdir data root: %v", err)
	}

	swampName := name.New().
		Sanctuary("durability-test").
		Realm("tick-only").
		Swamp("hu")
	hashPath := swampName.GetFullHashPath(dataRoot, dataLossIslandID, dataLossMaxDepth, dataLossMaxFolders)
	hydPath := hashPath + ".hyd"

	writeInterval := 200 * time.Millisecond
	closeAfterIdle := 1 * time.Hour

	noopEvent := func(*Event) {}
	noopInfo := func(*Info) {}

	closed := int32(0)
	closeCb := func(name.Name) { atomic.StoreInt32(&closed, 1) }

	chron := chronicler.NewV2WithName(hashPath, dataLossMaxDepth, swampName.Get())
	chron.CreateDirectoryIfNotExists()
	meta := metadata.NewNoop()
	meta.SetSwampName(swampName)

	fss := &FilesystemSettings{ChroniclerInterface: chron, WriteInterval: writeInterval}
	sw := New(swampName, closeAfterIdle, fss, noopEvent, noopInfo, closeCb, meta)

	// Hold the swamp open across the whole test so the close-on-idle path
	// cannot rescue us.
	sw.BeginVigil()
	defer sw.CeaseVigil()

	tr := sw.CreateTreasure("only-key")
	guardID := tr.StartTreasureGuard(true)
	tr.SetContentString(guardID, "tiny")
	_ = tr.Save(guardID)
	tr.ReleaseTreasureGuard(guardID)

	// Two full ticks should be enough; give a generous margin.
	time.Sleep(writeInterval * 5)

	if atomic.LoadInt32(&closed) == 1 {
		t.Fatalf("swamp closed while vigil was active — test setup invalid")
	}

	reader, err := v2.NewFileReader(hydPath)
	if err != nil {
		t.Fatalf("open reader: %v", err)
	}
	defer reader.Close()

	got := 0
	if _, err := reader.ReadAllEntries(func(entry v2.Entry) bool {
		got++
		return true
	}); err != nil {
		t.Fatalf("ReadAllEntries: %v", err)
	}
	if got != 1 {
		t.Fatalf("DURABILITY REGRESSION (tick path): expected 1 entry on "+
			"disk via tick-only flush, got %d. Close path was prevented by "+
			"the vigil, so the periodic Sync is the only thing that could "+
			"have put bytes on disk.", got)
	}
}
