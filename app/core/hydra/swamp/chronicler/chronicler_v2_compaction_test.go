package chronicler

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/hydraide/hydraide/app/core/hydra/swamp/beacon"
	"github.com/hydraide/hydraide/app/core/hydra/swamp/treasure"
	"github.com/hydraide/hydraide/app/core/hydra/swamp/treasure/guard"
)

// liveTracker simulates what the swamp's beacon provides to the chronicler:
// the current count of unique live keys. Tests update it as they Write entries.
type liveTracker struct {
	mu   sync.Mutex
	keys map[string]struct{}
}

func newLiveTracker() *liveTracker {
	return &liveTracker{keys: make(map[string]struct{})}
}

func (l *liveTracker) insert(key string) {
	l.mu.Lock()
	l.keys[key] = struct{}{}
	l.mu.Unlock()
}

func (l *liveTracker) delete(key string) {
	l.mu.Lock()
	delete(l.keys, key)
	l.mu.Unlock()
}

func (l *liveTracker) count() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.keys)
}

// makeTreasure creates a Treasure with a given key + content for tests.
func makeTreasure(key, content string) treasure.Treasure {
	tr := treasure.New(nil)
	gid := tr.StartTreasureGuard(false, guard.BodyAuthID)
	tr.BodySetKey(gid, key)
	tr.SetContentString(gid, content)
	tr.ReleaseTreasureGuard(gid)
	return tr
}

// fileSize returns the size of a file or 0 if it cannot be stat-ed.
func fileSize(t *testing.T, path string) int64 {
	t.Helper()
	fi, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return fi.Size()
}

// writeOverwriteCycles repeatedly overwrites the same set of keys, producing
// a heavily fragmented .hyd file (each cycle adds len(keys) entries).
func writeOverwriteCycles(t *testing.T, chron Chronicler, tracker *liveTracker, keys []string, cycles int) {
	t.Helper()
	for c := 0; c < cycles; c++ {
		batch := make([]treasure.Treasure, 0, len(keys))
		for _, k := range keys {
			tracker.insert(k)
			batch = append(batch, makeTreasure(k, fmt.Sprintf("v-%d", c)))
		}
		chron.Write(batch)
	}
}

// buildFragmentedFile creates a .hyd file at swampPath with the given fragmentation
// shape using the real chronicler API (so the encoded entries are valid GOB and
// can be decoded by Load). The chronicler's inline + close compaction is disabled
// here so the on-disk file actually contains the dead entries we want to test
// recovery against. Returns the path to the .hyd file.
func buildFragmentedFile(t *testing.T, swampPath string, keys []string, cycles int) string {
	t.Helper()
	chron := NewV2(swampPath, 10)
	chron.CreateDirectoryIfNotExists()

	cv2 := chron.(*chroniclerV2)
	cv2.compactionOnSave = false // build the file as-is, no auto-compaction

	tracker := newLiveTracker()
	for c := 0; c < cycles; c++ {
		batch := make([]treasure.Treasure, 0, len(keys))
		for _, k := range keys {
			tracker.insert(k)
			batch = append(batch, makeTreasure(k, fmt.Sprintf("v-%d", c)))
		}
		chron.Write(batch)
	}
	if err := chron.Close(); err != nil {
		t.Fatalf("Close (build): %v", err)
	}
	return swampPath + ".hyd"
}

// ---------------------------------------------------------------------------
// 1. Counter accuracy + inline trigger
// ---------------------------------------------------------------------------

func TestInlineCompaction_TriggersAtThreshold(t *testing.T) {
	tmpDir := t.TempDir()
	chron := NewV2(filepath.Join(tmpDir, "swamp"), 10)
	chron.CreateDirectoryIfNotExists()

	tracker := newLiveTracker()
	chron.RegisterLiveCountFunction(tracker.count)

	keys := make([]string, 10)
	for i := range keys {
		keys[i] = fmt.Sprintf("k-%d", i)
	}

	// 30 cycles × 10 keys = 300 entries, only 10 live → ~96% fragmentation.
	// minEntriesForCompact default is 100, so the first cycles won't trigger.
	writeOverwriteCycles(t, chron, tracker, keys, 30)

	// Closing flushes any pending writer state and runs the safety-net check.
	if err := chron.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// After compaction, the file must contain only the 10 live entries.
	beac := beacon.New()
	chron2 := NewV2(filepath.Join(tmpDir, "swamp"), 10)
	chron2.RegisterLiveCountFunction(tracker.count)
	chron2.Load(beac)

	if beac.Count() != 10 {
		t.Errorf("expected 10 live entries after compaction, got %d", beac.Count())
	}

	// File should be far smaller than the worst case (300 entries' worth).
	hydPath := filepath.Join(tmpDir, "swamp.hyd")
	if sz := fileSize(t, hydPath); sz > 8*1024 {
		t.Errorf("expected compacted file < 8KB, got %d bytes", sz)
	}
}

func TestInlineCompaction_DoesNotTriggerBelowThreshold(t *testing.T) {
	tmpDir := t.TempDir()
	chron := NewV2(filepath.Join(tmpDir, "swamp"), 10)
	chron.CreateDirectoryIfNotExists()

	tracker := newLiveTracker()
	chron.RegisterLiveCountFunction(tracker.count)

	// 200 unique keys, no overwrites → 0% fragmentation.
	batch := make([]treasure.Treasure, 200)
	for i := 0; i < 200; i++ {
		key := fmt.Sprintf("k-%d", i)
		tracker.insert(key)
		batch[i] = makeTreasure(key, "v")
	}
	chron.Write(batch)

	// fragmentation must be 0; no compaction.
	cv2, ok := chron.(*chroniclerV2)
	if !ok {
		t.Fatal("type assertion failed")
	}
	if cv2.lastFragmentation != 0 {
		t.Errorf("expected 0 fragmentation, got %f", cv2.lastFragmentation)
	}

	if err := chron.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

func TestInlineCompaction_RespectsMinEntries(t *testing.T) {
	tmpDir := t.TempDir()
	chron := NewV2(filepath.Join(tmpDir, "swamp"), 10)
	chron.CreateDirectoryIfNotExists()

	cv2 := chron.(*chroniclerV2)
	cv2.minEntriesForCompact = 100 // explicit, matches default

	tracker := newLiveTracker()
	chron.RegisterLiveCountFunction(tracker.count)

	// 5 cycles × 1 key = 5 entries (4 dead, 1 live = 80% fragmentation).
	// Below minEntriesForCompact (100) → no compaction.
	writeOverwriteCycles(t, chron, tracker, []string{"only"}, 5)
	if err := chron.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	hydPath := filepath.Join(tmpDir, "swamp.hyd")
	sizeBeforeReload := fileSize(t, hydPath)

	// Reload: also should not compact (still below min).
	chron2 := NewV2(filepath.Join(tmpDir, "swamp"), 10)
	chron2.RegisterLiveCountFunction(tracker.count)
	chron2.Load(beacon.New())
	if err := chron2.Close(); err != nil {
		t.Fatalf("Close 2: %v", err)
	}

	if sz := fileSize(t, hydPath); sz != sizeBeforeReload {
		t.Errorf("file size changed unexpectedly: before=%d after=%d", sizeBeforeReload, sz)
	}
}

// ---------------------------------------------------------------------------
// 2. Counter accuracy across Load/Compact
// ---------------------------------------------------------------------------

func TestCounterAccuracy_AfterLoad(t *testing.T) {
	tmpDir := t.TempDir()
	chron := NewV2(filepath.Join(tmpDir, "swamp"), 10)
	chron.CreateDirectoryIfNotExists()

	tracker := newLiveTracker()
	chron.RegisterLiveCountFunction(tracker.count)

	// 10 unique inserts, no compaction trigger.
	batch := make([]treasure.Treasure, 10)
	for i := 0; i < 10; i++ {
		key := fmt.Sprintf("k-%d", i)
		tracker.insert(key)
		batch[i] = makeTreasure(key, "v")
	}
	chron.Write(batch)
	if err := chron.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// Reload and verify totalEntriesInFile matches header.
	chron2 := NewV2(filepath.Join(tmpDir, "swamp"), 10)
	chron2.RegisterLiveCountFunction(tracker.count)
	chron2.Load(beacon.New())

	cv2 := chron2.(*chroniclerV2)
	cv2.mu.RLock()
	got := cv2.totalEntriesInFile
	cv2.mu.RUnlock()
	if got != 10 {
		t.Errorf("expected totalEntriesInFile=10 after load, got %d", got)
	}
}

func TestCounterAccuracy_AfterCompaction(t *testing.T) {
	tmpDir := t.TempDir()
	swampPath := filepath.Join(tmpDir, "swamp")

	// Pre-build a heavily fragmented file (no auto-compact during build).
	keys := make([]string, 5)
	for i := range keys {
		keys[i] = fmt.Sprintf("k-%d", i)
	}
	buildFragmentedFile(t, swampPath, keys, 30) // 150 entries total, 5 live

	// Now open with a real chronicler — Load self-heal will compact.
	chron := NewV2(swampPath, 10)
	tracker := newLiveTracker()
	for _, k := range keys {
		tracker.insert(k)
	}
	chron.RegisterLiveCountFunction(tracker.count)
	chron.Load(beacon.New())

	cv2 := chron.(*chroniclerV2)
	cv2.mu.RLock()
	got := cv2.totalEntriesInFile
	frag := cv2.lastFragmentation
	cv2.mu.RUnlock()
	if got != int64(tracker.count()) {
		t.Errorf("after compaction expected totalEntriesInFile=%d, got %d", tracker.count(), got)
	}
	if frag != 0 {
		t.Errorf("after compaction expected fragmentation=0, got %f", frag)
	}
	if err := chron.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

// ---------------------------------------------------------------------------
// 3. Close() bug fix — compaction runs even with no writes in this session
// ---------------------------------------------------------------------------

func TestCloseTriggersCompaction_NoWritesInSession(t *testing.T) {
	tmpDir := t.TempDir()
	swampPath := filepath.Join(tmpDir, "swamp")

	keys := make([]string, 10)
	for i := range keys {
		keys[i] = fmt.Sprintf("k-%d", i)
	}
	hydPath := buildFragmentedFile(t, swampPath, keys, 20) // 200 entries, 10 live
	sizeBefore := fileSize(t, hydPath)

	// Open via chronicler — Load self-heal compacts the file. Then Close()
	// must succeed cleanly even though no Write() was made in this session.
	chron := NewV2(swampPath, 10)
	tracker := newLiveTracker()
	for _, k := range keys {
		tracker.insert(k)
	}
	chron.RegisterLiveCountFunction(tracker.count)
	chron.Load(beacon.New())

	if sz := fileSize(t, hydPath); sz >= sizeBefore {
		t.Errorf("expected Load self-heal to shrink file: before=%d after=%d", sizeBefore, sz)
	}

	if err := chron.Close(); err != nil {
		t.Fatalf("Close after no writes: %v", err)
	}
}

// ---------------------------------------------------------------------------
// 4. Self-heal in Load()
// ---------------------------------------------------------------------------

func TestLoadCompactsFragmentedFile(t *testing.T) {
	tmpDir := t.TempDir()
	swampPath := filepath.Join(tmpDir, "swamp")

	keys := make([]string, 10)
	for i := range keys {
		keys[i] = fmt.Sprintf("k-%d", i)
	}
	hydPath := buildFragmentedFile(t, swampPath, keys, 50) // 500 entries, 10 live
	sizeBefore := fileSize(t, hydPath)

	chron := NewV2(swampPath, 10)
	beac := beacon.New()
	chron.Load(beac)

	if beac.Count() != 10 {
		t.Errorf("expected 10 live entries after load self-heal, got %d", beac.Count())
	}
	sizeAfter := fileSize(t, hydPath)
	if sizeAfter >= sizeBefore/2 {
		t.Errorf("expected self-heal to shrink file by >50%%: before=%d after=%d", sizeBefore, sizeAfter)
	}
}

func TestLoadDoesNotCompactSmallFile(t *testing.T) {
	tmpDir := t.TempDir()
	swampPath := filepath.Join(tmpDir, "swamp")

	// 5 entries, 1 key — 80% fragmentation but well below minEntriesForCompact (100).
	hydPath := buildFragmentedFile(t, swampPath, []string{"k"}, 5)
	sizeBefore := fileSize(t, hydPath)

	chron := NewV2(swampPath, 10)
	chron.Load(beacon.New())

	if sz := fileSize(t, hydPath); sz != sizeBefore {
		t.Errorf("small file should not be compacted on load: before=%d after=%d", sizeBefore, sz)
	}
}

func TestLoadDoesNotCompactHugeFile(t *testing.T) {
	tmpDir := t.TempDir()
	swampPath := filepath.Join(tmpDir, "swamp")

	keys := make([]string, 10)
	for i := range keys {
		keys[i] = fmt.Sprintf("k-%d", i)
	}
	hydPath := buildFragmentedFile(t, swampPath, keys, 30) // 300 entries
	sizeBefore := fileSize(t, hydPath)

	chron := NewV2(swampPath, 10)
	cv2 := chron.(*chroniclerV2)
	cv2.maxFileSizeForLoadCompact = 1 // anything > 1 byte → skip self-heal

	chron.Load(beacon.New())

	if sz := fileSize(t, hydPath); sz != sizeBefore {
		t.Errorf("oversized file should be skipped on load self-heal: before=%d after=%d", sizeBefore, sz)
	}
}

// ---------------------------------------------------------------------------
// 5. Compaction failure leaves original intact
// ---------------------------------------------------------------------------

func TestCompactionFailure_OldFileIntact(t *testing.T) {
	tmpDir := t.TempDir()
	swampPath := filepath.Join(tmpDir, "swamp")

	keys := make([]string, 5)
	for i := range keys {
		keys[i] = fmt.Sprintf("k-%d", i)
	}
	hydPath := buildFragmentedFile(t, swampPath, keys, 30) // 150 entries, 5 live
	sizeBefore := fileSize(t, hydPath)

	// Force the temp path to be unusable: create a non-empty directory there.
	// CleanupCompactionTemp uses os.Remove which fails on non-empty dirs, and
	// NewFileWriterWithName.os.Create also fails on a directory path.
	tempPath := hydPath + ".compact"
	if err := os.MkdirAll(tempPath, 0o755); err != nil {
		t.Fatalf("mkdir temp: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tempPath, "blocker"), []byte("x"), 0o644); err != nil {
		t.Fatalf("blocker file: %v", err)
	}
	defer os.RemoveAll(tempPath)

	chron := NewV2(swampPath, 10)
	chron.Load(beacon.New())

	sizeAfter := fileSize(t, hydPath)
	if sizeAfter != sizeBefore {
		t.Errorf("original file changed despite compaction failure: before=%d after=%d", sizeBefore, sizeAfter)
	}

	// Cleanup the blocker so we can read the original file with a fresh chronicler.
	os.RemoveAll(tempPath)

	beac := beacon.New()
	chron2 := NewV2(swampPath, 10)
	chron2.Load(beac)
	if beac.Count() != 5 {
		t.Errorf("expected 5 live entries after recovery, got %d", beac.Count())
	}
}

// ---------------------------------------------------------------------------
// 6. Crash recovery — leftover .compact temp must be removed on Load
// ---------------------------------------------------------------------------

func TestCrashRecovery_LeftoverCompactFile(t *testing.T) {
	tmpDir := t.TempDir()
	swampPath := filepath.Join(tmpDir, "swamp")

	keys := make([]string, 5)
	for i := range keys {
		keys[i] = fmt.Sprintf("k-%d", i)
	}
	hydPath := buildFragmentedFile(t, swampPath, keys, 1) // 5 entries — small file, no auto-compact

	// Simulate leftover .compact temp from a previous crash.
	tempPath := hydPath + ".compact"
	if err := os.WriteFile(tempPath, []byte("garbage-from-crash"), 0o644); err != nil {
		t.Fatalf("write temp: %v", err)
	}

	chron := NewV2(swampPath, 10)
	beac := beacon.New()
	chron.Load(beac)

	if _, err := os.Stat(tempPath); !os.IsNotExist(err) {
		t.Error("leftover .compact temp was not cleaned up by Load")
	}
	if beac.Count() != 5 {
		t.Errorf("expected 5 live entries, got %d", beac.Count())
	}
}

// ---------------------------------------------------------------------------
// 7. Concurrent writes — no data loss under inline compaction
// ---------------------------------------------------------------------------

func TestNoDataLoss_ConcurrentWrites(t *testing.T) {
	tmpDir := t.TempDir()
	chron := NewV2(filepath.Join(tmpDir, "swamp"), 10)
	chron.CreateDirectoryIfNotExists()

	tracker := newLiveTracker()
	chron.RegisterLiveCountFunction(tracker.count)

	const goroutines = 8
	const cyclesPer = 30
	const keysPerGoroutine = 5

	var wg sync.WaitGroup
	var writeOps int64

	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func(gid int) {
			defer wg.Done()
			for c := 0; c < cyclesPer; c++ {
				batch := make([]treasure.Treasure, keysPerGoroutine)
				for k := 0; k < keysPerGoroutine; k++ {
					key := fmt.Sprintf("g%d-k%d", gid, k)
					tracker.insert(key)
					batch[k] = makeTreasure(key, fmt.Sprintf("g%d-c%d", gid, c))
				}
				chron.Write(batch)
				atomic.AddInt64(&writeOps, int64(keysPerGoroutine))
			}
		}(g)
	}
	wg.Wait()

	if err := chron.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	expectedLive := goroutines * keysPerGoroutine
	if tracker.count() != expectedLive {
		t.Errorf("tracker has %d live keys, expected %d", tracker.count(), expectedLive)
	}

	// Reload from disk: every key must still be there.
	chron2 := NewV2(filepath.Join(tmpDir, "swamp"), 10)
	chron2.RegisterLiveCountFunction(tracker.count)
	beac := beacon.New()
	chron2.Load(beac)
	if beac.Count() != expectedLive {
		t.Errorf("disk has %d live keys, expected %d (data loss!)", beac.Count(), expectedLive)
	}
}

// ---------------------------------------------------------------------------
// 8. The crawler workload — auto-compacts without explicit Close
// ---------------------------------------------------------------------------

func TestCrawlerWorkload_AutoCompacts(t *testing.T) {
	tmpDir := t.TempDir()
	chron := NewV2(filepath.Join(tmpDir, "swamp"), 10)
	chron.CreateDirectoryIfNotExists()

	tracker := newLiveTracker()
	chron.RegisterLiveCountFunction(tracker.count)

	// 125 unique keys, each rewritten 160 times (matches the user's real swamp).
	keys := make([]string, 125)
	for i := range keys {
		keys[i] = fmt.Sprintf("domain-%d", i)
	}

	for cycle := 0; cycle < 160; cycle++ {
		batch := make([]treasure.Treasure, 0, len(keys))
		for _, k := range keys {
			tracker.insert(k)
			batch = append(batch, makeTreasure(k, fmt.Sprintf("payload-cycle-%d", cycle)))
		}
		chron.Write(batch)
	}

	// We have NOT called Close yet — but inline trigger should already have
	// compacted the file at least once. Check the on-disk size right now.
	hydPath := filepath.Join(tmpDir, "swamp.hyd")
	sizeWhileOpen := fileSize(t, hydPath)
	cv2 := chron.(*chroniclerV2)
	cv2.mu.RLock()
	totalAfterWrites := cv2.totalEntriesInFile
	cv2.mu.RUnlock()

	if totalAfterWrites > 8000 {
		t.Errorf("expected inline compaction to keep totalEntriesInFile bounded, got %d", totalAfterWrites)
	}

	if err := chron.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	finalSize := fileSize(t, hydPath)
	if finalSize > sizeWhileOpen {
		// Close runs final compaction; should never grow the file.
		t.Errorf("Close grew the file unexpectedly: open=%d closed=%d", sizeWhileOpen, finalSize)
	}

	// Reload and verify all 125 keys are intact.
	chron2 := NewV2(filepath.Join(tmpDir, "swamp"), 10)
	chron2.RegisterLiveCountFunction(tracker.count)
	beac := beacon.New()
	chron2.Load(beac)
	if beac.Count() != 125 {
		t.Errorf("expected 125 live keys, got %d (data loss!)", beac.Count())
	}
}
