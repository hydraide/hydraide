package hydra

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hydraide/hydraide/app/core/filesystem"
	"github.com/hydraide/hydraide/app/core/hydra/lock"
	"github.com/hydraide/hydraide/app/core/safeops"
	"github.com/hydraide/hydraide/app/core/settings"
	"github.com/hydraide/hydraide/app/name"
)

// TestInMemRapidSummonDestroyRace reproduces, at the engine level, the
// "the swamp can not be closed in 30 seconds, so we need to drop it" symptom
// observed against the Trendizz crawler-unit instance on swamp
// llm/logs/calls, even on server/v3.19.2 which already contains the
// gateway CeaseVigil-on-panic fix.
//
// Trendizz-side shape being modelled (inference-service log_service
// TestConcurrentToggle + TearDownTest):
//   - one in-memory swamp (IsInMemorySwamp: true)
//   - the SAME swamp name reused across the whole test
//   - many goroutines doing SummonSwamp -> CreateTreasure+Save concurrently
//   - other goroutines doing SummonSwamp -> Destroy() on the same name
//     (this is what ClearCallLogs / TearDownTest do)
//   - each SummonSwamp uses a short (5s) context, like the SDK's
//     hydraidehelper.CreateHydraContext()
//
// What we measure:
//   - how many SummonSwamp calls return an error,
//   - the slowest single SummonSwamp (a value anywhere near 30s means the
//     caller hit the WaitForGracefulClose(30s) deadline = the field bug),
//   - whether the whole workload finishes within a sane wall-clock budget.
//
// This is a DIAGNOSTIC reproducer, not a CI gate. If it hangs, run with
// `go test -run TestInMemRapidSummonDestroyRace -timeout 90s` and read the
// goroutine dump: the parked stack tells us exactly where the swamp
// lifecycle deadlocks (Destroy at s.mu.Lock, WaitForActiveVigilsClosed,
// or SummonSwamp at WaitForGracefulClose).
func TestInMemRapidSummonDestroyRace(t *testing.T) {

	elysiumInterface := safeops.New()
	lockerInterface := lock.New()
	fsInterface := filesystem.New()
	settingsInterface := settings.New(testMaxDepth, testMaxFolderPerLevel)

	fss := &settings.FileSystemSettings{
		WriteIntervalSec: 1,
		MaxFileSizeByte:  8192,
	}

	// inMemorySwamp = true  → mirrors the Trendizz RegisterPattern
	// (IsInMemorySwamp: true). closeAfterIdleSec large so the ONLY close
	// trigger is the explicit Destroy(), exactly like the Trendizz flow
	// (CloseAfterIdle: 30 * time.Minute there).
	settingsInterface.RegisterPattern(
		name.New().Sanctuary(sanctuaryForQuickTest).Realm("*").Swamp("*"),
		true, // in-memory
		3600, // closeAfterIdleSec — effectively "never" for this test
		fss,
	)

	hydraInterface := New(settingsInterface, elysiumInterface, lockerInterface, fsInterface)

	swampName := name.New().
		Sanctuary(sanctuaryForQuickTest).
		Realm("inmem-rapid").
		Swamp("calls")

	const (
		writers          = 8
		destroyers       = 3
		iterationsPerGor = 25
		summonTimeout    = 5 * time.Second // == hydraidehelper.CreateHydraContext
		wallClockBudget  = 60 * time.Second
	)

	var (
		summonErrs   int64
		summonOK     int64
		destroyCount int64
		maxSummonNs  int64 // slowest single SummonSwamp, nanoseconds
	)

	recordSummonDur := func(d time.Duration) {
		ns := d.Nanoseconds()
		for {
			cur := atomic.LoadInt64(&maxSummonNs)
			if ns <= cur || atomic.CompareAndSwapInt64(&maxSummonNs, cur, ns) {
				return
			}
		}
	}

	var wg sync.WaitGroup

	// Writers: Summon -> BeginVigil -> CreateTreasure+Save -> CeaseVigil.
	for w := 0; w < writers; w++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for i := 0; i < iterationsPerGor; i++ {
				ctx, cancel := context.WithTimeout(context.Background(), summonTimeout)
				start := time.Now()
				sw, err := hydraInterface.SummonSwamp(ctx, 10, swampName)
				recordSummonDur(time.Since(start))
				cancel()
				if err != nil || sw == nil {
					atomic.AddInt64(&summonErrs, 1)
					continue
				}
				atomic.AddInt64(&summonOK, 1)

				sw.BeginVigil()
				tr := sw.CreateTreasure(fmt.Sprintf("w%d-i%d", id, i))
				gid := tr.StartTreasureGuard(true)
				tr.SetContentString(gid, "x")
				tr.Save(gid)
				tr.ReleaseTreasureGuard(gid)
				sw.CeaseVigil()
			}
		}(w)
	}

	// Destroyers: Summon -> Destroy(), same name. Mirrors ClearCallLogs /
	// TearDownTest hammering Destroy while writers keep re-summoning.
	for d := 0; d < destroyers; d++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < iterationsPerGor; i++ {
				ctx, cancel := context.WithTimeout(context.Background(), summonTimeout)
				start := time.Now()
				sw, err := hydraInterface.SummonSwamp(ctx, 10, swampName)
				recordSummonDur(time.Since(start))
				cancel()
				if err != nil || sw == nil {
					atomic.AddInt64(&summonErrs, 1)
					continue
				}
				sw.Destroy()
				atomic.AddInt64(&destroyCount, 1)
				time.Sleep(2 * time.Millisecond)
			}
		}()
	}

	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()

	select {
	case <-done:
		// finished within budget
	case <-time.After(wallClockBudget):
		t.Fatalf("workload did NOT finish within %s — likely a swamp-lifecycle "+
			"deadlock. summonOK=%d summonErrs=%d destroys=%d maxSummon=%s. "+
			"Re-run with -timeout to capture the goroutine dump.",
			wallClockBudget,
			atomic.LoadInt64(&summonOK),
			atomic.LoadInt64(&summonErrs),
			atomic.LoadInt64(&destroyCount),
			time.Duration(atomic.LoadInt64(&maxSummonNs)))
	}

	maxSummon := time.Duration(atomic.LoadInt64(&maxSummonNs))
	t.Logf("summonOK=%d summonErrs=%d destroys=%d maxSummon=%s",
		atomic.LoadInt64(&summonOK),
		atomic.LoadInt64(&summonErrs),
		atomic.LoadInt64(&destroyCount),
		maxSummon)

	// A single SummonSwamp anywhere near the hard-coded WaitForGracefulClose
	// timeout (30s) is the field bug reproduced in isolation.
	if maxSummon >= 20*time.Second {
		t.Fatalf("a SummonSwamp took %s — reproduces the WaitForGracefulClose(30s) "+
			"stall on a rapidly Save/Destroy/re-Summoned in-memory swamp", maxSummon)
	}

	// Clean up best-effort.
	ctx, cancel := context.WithTimeout(context.Background(), summonTimeout)
	defer cancel()
	if sw, err := hydraInterface.SummonSwamp(ctx, 10, swampName); err == nil && sw != nil {
		sw.Destroy()
	}
}
