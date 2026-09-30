package swamp

// Regression tests for the ShiftExpired / ShiftMatching vs. concurrent Save
// lock-order inversion documented in
// docs/bugs/2026-09-30-catalog-shift-expired-hang.md.
//
// Lock order on the two paths:
//
//	Shift  : expirationTimeBeaconASC.mu (beacon.ShiftExpired / ShiftMatching)
//	         -> per-treasure guard (StartTreasureGuard, for every row it scans)
//	Save   : per-treasure guard (gateway Set / test seed)
//	         -> expirationTimeBeaconASC.mu (SaveFunction's modified branch:
//	            deleteTreasureIfBeaconInitialized / addToExpirationTimeBeacon)
//
// A Save that overwrites an existing key with a (re)set ExpiredAt therefore
// deadlocks against a concurrent shift that scans the same row, and so does a
// DeleteTreasure (deleteHandler: guard -> every beacon mu). All tests
// below FAIL on the unfixed engine (they detect the hang with a watchdog and
// dump the goroutines instead of hanging the test binary).

import (
	"fmt"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hydraide/hydraide/app/core/hydra/swamp/treasure"
	"github.com/stretchr/testify/require"
)

// deadlockTestSwamp builds a swamp like patchTestSwamp, but only tears it down
// when the test did not detect a deadlock: Destroy() on a deadlocked swamp
// would itself block forever and hang the whole test binary.
func deadlockTestSwamp(t *testing.T, realm, swampN string, deadlocked *atomic.Bool) Swamp {
	t.Helper()
	var s Swamp
	// patchTestSwampTB registers its own Cleanup (CeaseVigil + Destroy); run it
	// on a throwaway TB wrapper so we control whether teardown happens.
	sub := &cleanupCapture{TB: t}
	s = patchTestSwampTB(sub, realm, swampN)
	t.Cleanup(func() {
		if deadlocked.Load() {
			t.Log("skipping swamp teardown: swamp is deadlocked, Destroy() would block forever")
			return
		}
		for i := len(sub.fns) - 1; i >= 0; i-- {
			sub.fns[i]()
		}
	})
	return s
}

type cleanupCapture struct {
	testing.TB
	fns []func()
}

func (c *cleanupCapture) Cleanup(f func()) { c.fns = append(c.fns, f) }

// allStacks returns the stack traces of every goroutine in the process.
func allStacks() string {
	buf := make([]byte, 1<<20)
	for {
		n := runtime.Stack(buf, true)
		if n < len(buf) {
			return string(buf[:n])
		}
		buf = make([]byte, 2*len(buf))
	}
}

// waitForGoroutineBlockedIn polls the goroutine dump until one goroutine's stack
// contains every given frame substring, or the timeout elapses.
func waitForGoroutineBlockedIn(timeout time.Duration, frames ...string) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		for _, g := range strings.Split(allStacks(), "\n\n") {
			ok := true
			for _, f := range frames {
				if !strings.Contains(g, f) {
					ok = false
					break
				}
			}
			if ok {
				return true
			}
		}
		time.Sleep(5 * time.Millisecond)
	}
	return false
}

// relevantStacks keeps only the goroutines that sit in the swamp / beacon /
// guard code, so a failure message stays readable.
func relevantStacks() string {
	var out []string
	for _, g := range strings.Split(allStacks(), "\n\n") {
		if strings.Contains(g, "beacon.(*beacon)") || strings.Contains(g, "guard.(*guard)") {
			out = append(out, g)
		}
	}
	return strings.Join(out, "\n\n")
}

// runShiftVsOverwriteDeterministic forces the exact interleaving:
//  1. a writer holds the per-treasure guard of an indexed key (as gateway.Set
//     does between CreateTreasure/StartTreasureGuard and Save),
//  2. the shifter takes the expiration beacon mu and blocks on that guard,
//  3. the writer calls Save() with a new ExpiredAt, which needs the beacon mu.
func runShiftVsOverwriteDeterministic(t *testing.T, realm string, shift func(s Swamp), shiftFrame string) {
	var deadlocked atomic.Bool
	s := deadlockTestSwamp(t, realm, "shift-vs-overwrite", &deadlocked)

	// Seed a non-expired row plus an anchor row so the swamp never becomes
	// empty (empty => auto-Destroy, which is a different code path).
	for _, k := range []string{"anchor", "k"} {
		tr := s.CreateTreasure(k)
		gid := tr.StartTreasureGuard(true)
		tr.SetContentString(gid, "v")
		tr.SetExpirationTime(gid, time.Now().UTC().Add(time.Hour))
		require.Equal(t, treasure.StatusNew, tr.Save(gid))
		tr.ReleaseTreasureGuard(gid)
	}
	// First shift builds the expiration beacon (nothing is expired yet).
	shift(s)

	// (1) writer takes the guard of "k"
	tr := s.CreateTreasure("k")
	gid := tr.StartTreasureGuard(true)

	// (2) shifter: takes beacon mu, then blocks on k's guard
	shiftDone := make(chan struct{})
	go func() {
		defer close(shiftDone)
		shift(s)
	}()
	if !waitForGoroutineBlockedIn(5*time.Second, shiftFrame, "guard.(*guard).StartTreasureGuard") {
		// The shifter did not block on the guard: either it finished (fixed
		// engine that no longer takes guards under beacon mu) or it is not
		// scanning this row. Either way the inversion cannot happen.
		t.Log("shifter never blocked on a treasure guard while holding the beacon mu")
	}

	// (3) writer overwrites ExpiredAt and saves while still holding the guard
	saveDone := make(chan struct{})
	go func() {
		defer close(saveDone)
		tr.SetExpirationTime(gid, time.Now().UTC().Add(2*time.Hour))
		tr.Save(gid)
		tr.ReleaseTreasureGuard(gid)
	}()

	timeout := time.After(5 * time.Second)
	for shiftDone != nil || saveDone != nil {
		select {
		case <-shiftDone:
			shiftDone = nil
		case <-saveDone:
			saveDone = nil
		case <-timeout:
			deadlocked.Store(true)
			t.Fatalf("DEADLOCK: shift and overwrite-Save never finished (shiftDone=%v saveDone=%v).\n"+
				"Relevant goroutines:\n%s", shiftDone == nil, saveDone == nil, relevantStacks())
		}
	}
}

func TestSwamp_ShiftExpired_ConcurrentOverwriteSave_NoDeadlock(t *testing.T) {
	runShiftVsOverwriteDeterministic(t, "shift-deadlock-expired", func(s Swamp) {
		_, _ = s.CloneAndDeleteExpiredTreasures(10)
	}, "beacon.(*beacon).ShiftExpired")
}

func TestSwamp_ShiftMatching_ConcurrentOverwriteSave_NoDeadlock(t *testing.T) {
	runShiftVsOverwriteDeterministic(t, "shift-deadlock-matching", func(s Swamp) {
		now := time.Now().UTC().UnixNano()
		expired := func(tr treasure.Treasure) bool {
			exp := tr.GetExpirationTime()
			return exp != 0 && exp < now
		}
		_, _, _ = s.CloneAndDeleteMatchingTreasures(BeaconTypeExpirationTime, IndexOrderAsc, 10, expired, nil, 0)
	}, "beacon.(*beacon).ShiftMatching")
}

// TestSwamp_ShiftExpired_ProductionPattern_NoDeadlock mirrors the production
// traffic that hung the Trendizz scheduler queue: one shifter loop plus four
// writers upserting a small key set with fresh ExpiredAt values (some already
// due). No forced interleaving; on the unfixed engine it hangs within well
// under a second on a normal machine.
func TestSwamp_ShiftExpired_ProductionPattern_NoDeadlock(t *testing.T) {
	var deadlocked atomic.Bool
	s := deadlockTestSwamp(t, "shift-deadlock-stress", "queue", &deadlocked)

	// anchor keeps the swamp non-empty so the auto-destroy path stays out
	tr := s.CreateTreasure("anchor")
	gid := tr.StartTreasureGuard(true)
	tr.SetContentString(gid, "v")
	tr.SetExpirationTime(gid, time.Now().UTC().Add(24*time.Hour))
	tr.Save(gid)
	tr.ReleaseTreasureGuard(gid)

	const (
		writers  = 4
		keySpace = 32
		runFor   = 3 * time.Second
	)
	var (
		stop     atomic.Bool
		progress atomic.Int64
		wg       sync.WaitGroup
	)

	wg.Add(1)
	go func() {
		defer wg.Done()
		for !stop.Load() {
			_, _ = s.CloneAndDeleteExpiredTreasures(10)
			progress.Add(1)
		}
	}()
	for w := 0; w < writers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i := 0; !stop.Load(); i++ {
				key := fmt.Sprintf("meeting-%d:%d", (i*7+w)%keySpace, w%3)
				// mimic gateway.Set: CreateTreasure -> guard -> set fields -> Save
				tr := s.CreateTreasure(key)
				gid := tr.StartTreasureGuard(true)
				tr.SetContentString(gid, "payload")
				due := time.Now().UTC().Add(time.Duration(i%3-1) * time.Minute) // past, now, future
				tr.SetExpirationTime(gid, due)
				tr.Save(gid)
				tr.ReleaseTreasureGuard(gid)
				progress.Add(1)
			}
		}(w)
	}

	allDone := make(chan struct{})
	go func() { wg.Wait(); close(allDone) }()

	deadline := time.Now().Add(runFor)
	last := progress.Load()
	for time.Now().Before(deadline) {
		time.Sleep(time.Second)
		cur := progress.Load()
		if cur == last {
			deadlocked.Store(true)
			stop.Store(true)
			t.Fatalf("DEADLOCK: no progress for 1s after %d operations.\nRelevant goroutines:\n%s",
				cur, relevantStacks())
		}
		last = cur
	}
	stop.Store(true)
	select {
	case <-allDone:
	case <-time.After(5 * time.Second):
		deadlocked.Store(true)
		t.Fatalf("DEADLOCK: workers did not stop.\nRelevant goroutines:\n%s", relevantStacks())
	}
}

// TestSwamp_ShiftExpired_ConcurrentDelete_NoDeadlock covers the delete side of
// the same inversion: deleteHandler takes the per-treasure guard first and then
// the expiration beacon mu (deleteTreasureFromBeacons), while the shifter holds
// that mu and waits for the guard. Writers here only insert fresh keys (the
// new-key Save path is not itself part of the cycle) and delete them again.
func TestSwamp_ShiftExpired_ConcurrentDelete_NoDeadlock(t *testing.T) {
	var deadlocked atomic.Bool
	s := deadlockTestSwamp(t, "shift-deadlock-delete", "queue", &deadlocked)

	tr := s.CreateTreasure("anchor")
	gid := tr.StartTreasureGuard(true)
	tr.SetContentString(gid, "v")
	tr.SetExpirationTime(gid, time.Now().UTC().Add(24*time.Hour))
	tr.Save(gid)
	tr.ReleaseTreasureGuard(gid)

	var (
		stop     atomic.Bool
		progress atomic.Int64
		wg       sync.WaitGroup
	)
	wg.Add(1)
	go func() {
		defer wg.Done()
		for !stop.Load() {
			_, _ = s.CloneAndDeleteExpiredTreasures(10)
			progress.Add(1)
		}
	}()
	for w := 0; w < 4; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i := 0; !stop.Load(); i++ {
				key := fmt.Sprintf("w%d-%d", w, i)
				tr := s.CreateTreasure(key)
				gid := tr.StartTreasureGuard(true)
				tr.SetContentString(gid, "payload")
				tr.SetExpirationTime(gid, time.Now().UTC().Add(time.Hour))
				tr.Save(gid)
				tr.ReleaseTreasureGuard(gid)
				_ = s.DeleteTreasure(key, false)
				progress.Add(1)
			}
		}(w)
	}
	allDone := make(chan struct{})
	go func() { wg.Wait(); close(allDone) }()

	deadline := time.Now().Add(3 * time.Second)
	last := progress.Load()
	for time.Now().Before(deadline) {
		time.Sleep(time.Second)
		cur := progress.Load()
		if cur == last {
			deadlocked.Store(true)
			stop.Store(true)
			t.Fatalf("DEADLOCK: no progress for 1s after %d operations.\nRelevant goroutines:\n%s",
				cur, relevantStacks())
		}
		last = cur
	}
	stop.Store(true)
	select {
	case <-allDone:
	case <-time.After(5 * time.Second):
		deadlocked.Store(true)
		t.Fatalf("DEADLOCK: workers did not stop.\nRelevant goroutines:\n%s", relevantStacks())
	}
}
