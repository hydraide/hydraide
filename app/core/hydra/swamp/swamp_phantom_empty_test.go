package swamp

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/hydraide/hydraide/app/core/hydra/swamp/chronicler"
	"github.com/hydraide/hydraide/app/core/hydra/swamp/metadata"
	"github.com/hydraide/hydraide/app/name"
)

// TestPhantomEmptyBeaconRace reproduces the Trendizz "phantom empty" bug:
// a beacon read (CatalogReadManyStream / GetTreasuresByBeacon) intermittently
// returns FEWER rows than the swamp actually holds — often zero — when a
// concurrent writer is mutating the same swamp, even though CountTreasures()
// (the main store) still reports the full N.
//
// Root cause under test: buildBeacon (swamp.go) publishes the read beacon as
// initialized (SetInitialized(true)) BEFORE it populates it (PushManyFromMap).
// A second reader that observes initialized==1 skips the build and reads a
// still-empty ordered slice -> 0 rows. The window is widened by a concurrent
// writer, because treasuresForBeacon() -> beaconKey.GetAll() blocks on the
// same beaconKey lock the writer's Add() holds.
//
// The production read path uses IndexCreationTime, so this test drives
// BeaconTypeCreationTime — the exact server-side path.
//
// Design: many fresh swamps (each re-arms the lazy first-build), and for each
// swamp we fire K readers + W writers simultaneously via a start barrier. A
// long closeAfterIdle keeps the idle-eviction listener out of the picture so
// this test isolates the buildBeacon race (the unload race has its own test).
func TestPhantomEmptyBeaconRace(t *testing.T) {

	const (
		iterations   = 300
		nInitial     = 200 // treasures written before the concurrent phase
		concReaders  = 8
		concWriters  = 2
		writeInterval = 200 * time.Millisecond
		// Long enough that startCloseListener never evicts during the test.
		closeAfterIdle = 60 * time.Second
	)

	tmpDir := t.TempDir()
	dataRoot := filepath.Join(tmpDir, "hydraide-data")
	if err := os.MkdirAll(dataRoot, 0o755); err != nil {
		t.Fatalf("mkdir data root: %v", err)
	}

	type shortRead struct {
		iter   int
		reader int
		got    int
		count  int
	}
	var (
		mu         sync.Mutex
		shortReads []shortRead
	)

	for iter := 0; iter < iterations; iter++ {

		swampName := name.New().
			Sanctuary("phantom-test").
			Realm("domain-state").
			Swamp(fmt.Sprintf("hu-%d", iter))

		hashPath := swampName.GetFullHashPath(dataRoot, dataLossIslandID, dataLossMaxDepth, dataLossMaxFolders)

		chron := chronicler.NewV2WithName(hashPath, dataLossMaxDepth, swampName.Get())
		chron.CreateDirectoryIfNotExists()
		meta := metadata.NewNoop()
		meta.SetSwampName(swampName)

		fss := &FilesystemSettings{ChroniclerInterface: chron, WriteInterval: writeInterval}
		noopEvent := func(*Event) {}
		noopInfo := func(*Info) {}
		noopClose := func(name.Name) {}

		sw := New(swampName, closeAfterIdle, fss, noopEvent, noopInfo, noopClose, meta)

		// Keep the swamp resident for the whole iteration.
		sw.BeginVigil()

		// Seed N treasures. These land in the main store (beaconKey); the
		// read beacons (creationTime) stay lazily uninitialized until the
		// first GetTreasuresByBeacon in the concurrent phase below.
		for i := 0; i < nInitial; i++ {
			key := fmt.Sprintf("seed%07d.hu", i)
			tr := sw.CreateTreasure(key)
			if tr == nil {
				sw.CeaseVigil()
				t.Fatalf("iter %d: CreateTreasure returned nil at i=%d", iter, i)
			}
			guardID := tr.StartTreasureGuard(true)
			tr.SetContentString(guardID, trendizzPayload(i))
			tr.SetCreatedAt(guardID, time.Now()) // production treasures carry createdAt; the CreationTime beacon filters on it
			_ = tr.Save(guardID)
			tr.ReleaseTreasureGuard(guardID)
		}

		if got := sw.CountTreasures(); got != nInitial {
			sw.CeaseVigil()
			t.Fatalf("iter %d: seed CountTreasures got %d want %d", iter, got, nInitial)
		}

		// Concurrent phase: K readers + W writers released at the same instant.
		var wg sync.WaitGroup
		start := make(chan struct{})

		// Writers: keep adding fresh treasures, holding the beaconKey lock in
		// Add() to widen the buildBeacon publish->populate window.
		for w := 0; w < concWriters; w++ {
			wg.Add(1)
			go func(w int) {
				defer wg.Done()
				<-start
				for j := 0; j < 50; j++ {
					key := fmt.Sprintf("w%02d-%05d.hu", w, j)
					tr := sw.CreateTreasure(key)
					if tr == nil {
						continue
					}
					guardID := tr.StartTreasureGuard(true)
					tr.SetContentString(guardID, trendizzPayload(j))
					tr.SetCreatedAt(guardID, time.Now())
					_ = tr.Save(guardID)
					tr.ReleaseTreasureGuard(guardID)
				}
			}(w)
		}

		// Readers: each performs the production read (creationTime beacon, all
		// rows). A healthy read must see at least nInitial rows, because the
		// swamp started with N and writers only ADD. Anything < nInitial means
		// the reader observed a phantom-empty / partial beacon.
		for r := 0; r < concReaders; r++ {
			wg.Add(1)
			go func(r int) {
				defer wg.Done()
				<-start
				treasures, err := sw.GetTreasuresByBeacon(BeaconTypeCreationTime, IndexOrderDesc, 0, 0, nil, nil)
				if err != nil {
					return
				}
				if len(treasures) < nInitial {
					mu.Lock()
					shortReads = append(shortReads, shortRead{
						iter: iter, reader: r, got: len(treasures), count: sw.CountTreasures(),
					})
					mu.Unlock()
				}
			}(r)
		}

		close(start) // release everyone at once
		wg.Wait()

		sw.CeaseVigil()
	}

	if len(shortReads) > 0 {
		// Report a compact summary; the very existence of a short read proves
		// the phantom-empty race.
		zeroCount := 0
		for _, s := range shortReads {
			if s.got == 0 {
				zeroCount++
			}
		}
		sample := shortReads
		if len(sample) > 10 {
			sample = sample[:10]
		}
		t.Errorf("PHANTOM-EMPTY BEACON RACE reproduced: %d short reads over %d iterations (%d of them read ZERO rows while the swamp held %d).\nSample: %+v",
			len(shortReads), iterations, zeroCount, nInitial, sample)
	}
}
