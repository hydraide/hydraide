package swamp

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/hydraide/hydraide/app/core/filesystem"
	"github.com/hydraide/hydraide/app/core/hydra/swamp/chronicler"
	"github.com/hydraide/hydraide/app/core/hydra/swamp/metadata"
	"github.com/hydraide/hydraide/app/core/hydra/swamp/treasure/guard"
	"github.com/hydraide/hydraide/app/core/settings"
	"github.com/hydraide/hydraide/app/name"
	"github.com/vmihailenco/msgpack/v5"
)

const sanctuaryBucket = "qb"

// newBucketSwamp builds an isolated swamp for the bucket integration
// tests. Mirrors newSwampForTest but registers under a dedicated realm
// so parallel test runs don't collide.
func newBucketSwamp(t *testing.T, realm, swamp string) Swamp {
	t.Helper()

	fsInterface := filesystem.New()
	settingsInterface := settings.New(testMaxDepth, testMaxFolderPerLevel)
	fss := &settings.FileSystemSettings{
		WriteIntervalSec: 1,
		MaxFileSizeByte:  8192,
	}
	settingsInterface.RegisterPattern(
		name.New().Sanctuary(sanctuaryBucket).Realm(realm).Swamp("*"),
		false, 1, fss,
	)
	closeAfterIdle := 5 * time.Second
	writeInterval := 1 * time.Second
	maxFileSize := int64(8192)

	swampNameObj := name.New().Sanctuary(sanctuaryBucket).Realm(realm).Swamp(swamp)
	hashPath := swampNameObj.GetFullHashPath(
		settingsInterface.GetHydraAbsDataFolderPath(),
		testAllServers, testMaxDepth, testMaxFolderPerLevel,
	)

	cleanup := chronicler.New(hashPath, maxFileSize, testMaxDepth, fsInterface, metadata.New(hashPath))
	cleanup.Destroy()

	chr := chronicler.New(hashPath, maxFileSize, testMaxDepth, fsInterface, metadata.New(hashPath))
	chr.CreateDirectoryIfNotExists()

	fssSwamp := &FilesystemSettings{ChroniclerInterface: chr, WriteInterval: writeInterval}
	sw := New(swampNameObj, closeAfterIdle, fssSwamp,
		func(e *Event) {}, func(i *Info) {}, func(n name.Name) {},
		metadata.New(hashPath))

	t.Cleanup(func() {
		sw.CeaseVigil()
		sw.Close()
		chr.Destroy()
	})
	return sw
}

// saveBody saves a treasure with the given key and msgpack-encoded body.
func saveBody(t *testing.T, sw Swamp, key string, body map[string]any) {
	t.Helper()
	enc, err := msgpack.Marshal(body)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	tr := sw.CreateTreasure(key)
	g := tr.StartTreasureGuard(true, guard.BodyAuthID)
	tr.SetContentByteArray(g, enc)
	tr.Save(g)
	tr.ReleaseTreasureGuard(g)
}

func deleteTreasure(t *testing.T, sw Swamp, key string) {
	t.Helper()
	sw.DeleteTreasure(key, false)
}

// --- Build & lookup integration ---

func TestSwamp_AutoBucket_FirstFilterTriggersBuild(t *testing.T) {
	sw := newBucketSwamp(t, "first-filter", "s")
	sw.BeginVigil()
	defer sw.CeaseVigil()

	for i := 0; i < 100; i++ {
		saveBody(t, sw, fmt.Sprintf("k%d", i), map[string]any{
			"asn": int64(i % 5),
		})
	}
	if sw.BucketCount() != 0 {
		t.Fatalf("BucketCount before first lookup: %d, want 0", sw.BucketCount())
	}
	got := sw.LookupByBucketEqual("asn", int64(2))
	if len(got) != 20 {
		t.Fatalf("LookupByBucketEqual(asn=2): %d hits, want 20", len(got))
	}
	if sw.BucketCount() != 1 {
		t.Fatalf("BucketCount after first lookup: %d, want 1", sw.BucketCount())
	}
}

func TestSwamp_AutoBucket_DifferentFieldTriggersSecondBuild(t *testing.T) {
	sw := newBucketSwamp(t, "two-fields", "s")
	sw.BeginVigil()
	defer sw.CeaseVigil()

	for i := 0; i < 50; i++ {
		saveBody(t, sw, fmt.Sprintf("k%d", i), map[string]any{
			"asn":    int64(i % 5),
			"status": fmt.Sprintf("st%d", i%3),
		})
	}
	_ = sw.LookupByBucketEqual("asn", int64(1))
	_ = sw.LookupByBucketEqual("status", "st1")
	if sw.BucketCount() != 2 {
		t.Fatalf("BucketCount: %d, want 2", sw.BucketCount())
	}
}

// --- Mutation propagation ---

func TestSwamp_AutoBucket_InsertReflectedInBucket(t *testing.T) {
	sw := newBucketSwamp(t, "insert-propagation", "s")
	sw.BeginVigil()
	defer sw.CeaseVigil()

	saveBody(t, sw, "k1", map[string]any{"asn": int64(1)})
	// First lookup builds the bucket.
	if got := sw.LookupByBucketEqual("asn", int64(1)); len(got) != 1 {
		t.Fatalf("pre-insert: got %d", len(got))
	}
	// Now insert a second matching treasure — the bucket should pick it up.
	saveBody(t, sw, "k2", map[string]any{"asn": int64(1)})
	if got := sw.LookupByBucketEqual("asn", int64(1)); len(got) != 2 {
		t.Fatalf("post-insert: got %d, want 2", len(got))
	}
}

func TestSwamp_AutoBucket_UpdateMovesBetweenBuckets(t *testing.T) {
	sw := newBucketSwamp(t, "update-move", "s")
	sw.BeginVigil()
	defer sw.CeaseVigil()

	saveBody(t, sw, "k", map[string]any{"asn": int64(1)})
	_ = sw.LookupByBucketEqual("asn", int64(1)) // build
	saveBody(t, sw, "k", map[string]any{"asn": int64(2)})
	if got := sw.LookupByBucketEqual("asn", int64(1)); len(got) != 0 {
		t.Errorf("old value still present: %d", len(got))
	}
	if got := sw.LookupByBucketEqual("asn", int64(2)); len(got) != 1 {
		t.Errorf("new value missing: %d", len(got))
	}
}

func TestSwamp_AutoBucket_DeleteRemovesFromBucket(t *testing.T) {
	sw := newBucketSwamp(t, "delete-removes", "s")
	sw.BeginVigil()
	defer sw.CeaseVigil()

	saveBody(t, sw, "k", map[string]any{"asn": int64(1)})
	_ = sw.LookupByBucketEqual("asn", int64(1)) // build
	deleteTreasure(t, sw, "k")
	if got := sw.LookupByBucketEqual("asn", int64(1)); len(got) != 0 {
		t.Fatalf("post-delete: got %d, want 0", len(got))
	}
}

func TestSwamp_AutoBucket_MultipleBucketsUpdatedOnSingleSave(t *testing.T) {
	sw := newBucketSwamp(t, "multi-bucket-update", "s")
	sw.BeginVigil()
	defer sw.CeaseVigil()

	saveBody(t, sw, "k", map[string]any{"asn": int64(1), "status": "old"})
	_ = sw.LookupByBucketEqual("asn", int64(1))
	_ = sw.LookupByBucketEqual("status", "old")

	saveBody(t, sw, "k", map[string]any{"asn": int64(2), "status": "new"})
	if got := sw.LookupByBucketEqual("asn", int64(2)); len(got) != 1 {
		t.Errorf("asn bucket: %d", len(got))
	}
	if got := sw.LookupByBucketEqual("status", "new"); len(got) != 1 {
		t.Errorf("status bucket: %d", len(got))
	}
	if got := sw.LookupByBucketEqual("asn", int64(1)); len(got) != 0 {
		t.Errorf("old asn still present")
	}
	if got := sw.LookupByBucketEqual("status", "old"); len(got) != 0 {
		t.Errorf("old status still present")
	}
}

// --- IN lookup ---

func TestSwamp_AutoBucket_LookupIn(t *testing.T) {
	sw := newBucketSwamp(t, "lookup-in", "s")
	sw.BeginVigil()
	defer sw.CeaseVigil()

	for i := 0; i < 30; i++ {
		saveBody(t, sw, fmt.Sprintf("k%d", i), map[string]any{"asn": int64(i % 6)})
	}
	got := sw.LookupByBucketIn("asn", []any{int64(0), int64(3)})
	if len(got) != 10 {
		t.Fatalf("IN [0,3]: got %d, want 10", len(got))
	}
}

func TestSwamp_AutoBucket_LookupIn_Empty(t *testing.T) {
	sw := newBucketSwamp(t, "lookup-in-empty", "s")
	sw.BeginVigil()
	defer sw.CeaseVigil()
	saveBody(t, sw, "k", map[string]any{"asn": int64(1)})
	if got := sw.LookupByBucketIn("asn", nil); len(got) != 0 {
		t.Fatalf("nil values: got %d", len(got))
	}
}

// --- Lifecycle ---

func TestSwamp_AutoBucket_CloseDropsAllBuckets(t *testing.T) {
	sw := newBucketSwamp(t, "close-drops", "s")
	sw.BeginVigil()

	saveBody(t, sw, "k", map[string]any{"asn": int64(1)})
	_ = sw.LookupByBucketEqual("asn", int64(1))
	if sw.BucketCount() != 1 {
		t.Fatalf("pre-close: %d", sw.BucketCount())
	}

	sw.CeaseVigil()
	sw.Close()

	if sw.BucketCount() != 0 {
		t.Fatalf("post-close: %d, want 0", sw.BucketCount())
	}
}

// --- Cross-kind body equality through the full pipeline ---

func TestSwamp_AutoBucket_CrossKindLookup(t *testing.T) {
	sw := newBucketSwamp(t, "cross-kind", "s")
	sw.BeginVigil()
	defer sw.CeaseVigil()

	// Body uses int64 (msgpack round-trip preserves kind via uint64 or
	// int64 depending on sign). Lookup with float64 — valuecanon.Equal
	// collapses them when lossless.
	saveBody(t, sw, "k", map[string]any{"n": int64(7)})
	if got := sw.LookupByBucketEqual("n", float64(7.0)); len(got) != 1 {
		t.Fatalf("cross-kind int→float lookup: %d", len(got))
	}
}

// --- Concurrency: build-vs-mutation ---

func TestSwamp_AutoBucket_BuildVsConcurrentSave(t *testing.T) {
	sw := newBucketSwamp(t, "build-vs-save", "s")
	sw.BeginVigil()
	defer sw.CeaseVigil()

	for i := 0; i < 200; i++ {
		saveBody(t, sw, fmt.Sprintf("k%d", i), map[string]any{"asn": int64(i % 5)})
	}

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		// Build through the first lookup.
		_ = sw.LookupByBucketEqual("asn", int64(0))
	}()
	go func() {
		defer wg.Done()
		// Concurrent saves during the build window.
		for i := 200; i < 400; i++ {
			saveBody(t, sw, fmt.Sprintf("k%d", i), map[string]any{"asn": int64(i % 5)})
		}
	}()
	wg.Wait()

	// End state: every asn=0 treasure (including those inserted during
	// the build) must be in the bucket.
	got := sw.LookupByBucketEqual("asn", int64(0))
	// 400 keys, 80 have asn==0 (i % 5 == 0 for i in [0,400)).
	if len(got) != 80 {
		t.Fatalf("post-race count: %d, want 80", len(got))
	}
}

func TestSwamp_AutoBucket_TwoConcurrentBuildsSameField(t *testing.T) {
	sw := newBucketSwamp(t, "two-builds-same-field", "s")
	sw.BeginVigil()
	defer sw.CeaseVigil()

	for i := 0; i < 100; i++ {
		saveBody(t, sw, fmt.Sprintf("k%d", i), map[string]any{"asn": int64(i % 4)})
	}

	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got := sw.LookupByBucketEqual("asn", int64(0))
			if len(got) != 25 {
				t.Errorf("parallel build: got %d, want 25", len(got))
			}
		}()
	}
	wg.Wait()
	if sw.BucketCount() != 1 {
		t.Fatalf("BucketCount after parallel builds: %d, want 1", sw.BucketCount())
	}
}

func TestSwamp_AutoBucket_TwoConcurrentBuildsDifferentField(t *testing.T) {
	sw := newBucketSwamp(t, "two-builds-diff-field", "s")
	sw.BeginVigil()
	defer sw.CeaseVigil()

	for i := 0; i < 50; i++ {
		saveBody(t, sw, fmt.Sprintf("k%d", i), map[string]any{
			"asn":    int64(i % 5),
			"status": fmt.Sprintf("s%d", i%3),
		})
	}

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		_ = sw.LookupByBucketEqual("asn", int64(2))
	}()
	go func() {
		defer wg.Done()
		_ = sw.LookupByBucketEqual("status", "s1")
	}()
	wg.Wait()

	if sw.BucketCount() != 2 {
		t.Fatalf("BucketCount: %d, want 2", sw.BucketCount())
	}
}
