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

// runShiftVsUpsertNoLostWrite checks that a shift never loses a concurrent
// upsert. Each writer owns its keys and remembers the last payload it saved
// per key; the shifter records every (key, payload) it shifted. After the run
// the last saved payload of every key must be either the current value in the
// swamp or one the shifter returned. Before the shift kept the guard from
// selection to delete, a Save could land in between and be deleted without
// being returned.
func runShiftVsUpsertNoLostWrite(t *testing.T, realm string, shift func(s Swamp) []treasure.Treasure) {
	var deadlocked atomic.Bool
	s := deadlockTestSwamp(t, realm, "queue", &deadlocked)

	tr := s.CreateTreasure("anchor")
	gid := tr.StartTreasureGuard(true)
	tr.SetContentString(gid, "v")
	tr.SetExpirationTime(gid, time.Now().UTC().Add(24*time.Hour))
	tr.Save(gid)
	tr.ReleaseTreasureGuard(gid)

	const (
		writers       = 4
		keysPerWriter = 8
		runFor        = 2 * time.Second
	)
	var (
		stop     atomic.Bool
		progress atomic.Int64
		wg       sync.WaitGroup
		shiftMu  sync.Mutex
		shifted  = map[string]map[string]bool{}
	)
	lastSaved := make([]map[string]string, writers)

	wg.Add(1)
	go func() {
		defer wg.Done()
		for !stop.Load() {
			for _, st := range shift(s) {
				v, err := st.GetContentString()
				if err != nil {
					continue
				}
				shiftMu.Lock()
				if shifted[st.GetKey()] == nil {
					shifted[st.GetKey()] = map[string]bool{}
				}
				shifted[st.GetKey()][v] = true
				shiftMu.Unlock()
			}
			progress.Add(1)
		}
	}()
	for w := 0; w < writers; w++ {
		lastSaved[w] = map[string]string{}
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i := 0; !stop.Load(); i++ {
				key := fmt.Sprintf("w%d-k%d", w, i%keysPerWriter)
				payload := fmt.Sprintf("w%d-i%d", w, i)
				tr := s.CreateTreasure(key)
				gid := tr.StartTreasureGuard(true)
				tr.SetContentString(gid, payload)
				// alternate due and not-due rows so both the shift and the
				// "moved into the future" re-index path are exercised
				due := time.Now().UTC().Add(-time.Minute)
				if i%2 == 1 {
					due = time.Now().UTC().Add(time.Hour)
				}
				tr.SetExpirationTime(gid, due)
				tr.Save(gid)
				tr.ReleaseTreasureGuard(gid)
				lastSaved[w][key] = payload
				progress.Add(1)
			}
		}(w)
	}

	allDone := make(chan struct{})
	go func() { wg.Wait(); close(allDone) }()
	deadline := time.Now().Add(runFor)
	last := progress.Load()
	for time.Now().Before(deadline) {
		time.Sleep(500 * time.Millisecond)
		cur := progress.Load()
		if cur == last {
			deadlocked.Store(true)
			stop.Store(true)
			t.Fatalf("DEADLOCK: no progress after %d operations.\nRelevant goroutines:\n%s", cur, relevantStacks())
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

	lost := 0
	for w := 0; w < writers; w++ {
		for key, payload := range lastSaved[w] {
			if cur, err := s.GetTreasure(key); err == nil {
				if v, _ := cur.GetContentString(); v == payload {
					continue
				}
			}
			if shifted[key][payload] {
				continue
			}
			lost++
			t.Errorf("lost write: key %s, last saved payload %s is neither in the swamp nor shifted", key, payload)
		}
	}
	require.Zero(t, lost)
}

func TestSwamp_ShiftExpired_ConcurrentUpsert_NoLostWrite(t *testing.T) {
	runShiftVsUpsertNoLostWrite(t, "shift-lost-write-expired", func(s Swamp) []treasure.Treasure {
		shifted, _ := s.CloneAndDeleteExpiredTreasures(10)
		return shifted
	})
}

func TestSwamp_ShiftMatching_ConcurrentUpsert_NoLostWrite(t *testing.T) {
	runShiftVsUpsertNoLostWrite(t, "shift-lost-write-matching", func(s Swamp) []treasure.Treasure {
		now := time.Now().UTC().UnixNano()
		expired := func(tr treasure.Treasure) bool {
			exp := tr.GetExpirationTime()
			return exp != 0 && exp < now
		}
		shifted, _, _ := s.CloneAndDeleteMatchingTreasures(BeaconTypeExpirationTime, IndexOrderAsc, 10, expired, nil, 0)
		return shifted
	})
}

func TestSwamp_CloneAndDeleteByKeys_ConcurrentUpsert_NoLostWrite(t *testing.T) {
	runShiftVsUpsertNoLostWrite(t, "shift-lost-write-bykeys", func(s Swamp) []treasure.Treasure {
		keys := make([]string, 0, 32)
		for w := 0; w < 4; w++ {
			for k := 0; k < 8; k++ {
				keys = append(keys, fmt.Sprintf("w%d-k%d", w, k))
			}
		}
		shifted, _ := s.CloneAndDeleteTreasuresByKeys(keys)
		return shifted
	})
}

// countGoroutinesBlockedIn returns how many goroutines have every given frame
// substring in their stack.
func countGoroutinesBlockedIn(frames ...string) int {
	n := 0
	for _, g := range strings.Split(allStacks(), "\n\n") {
		ok := true
		for _, f := range frames {
			if !strings.Contains(g, f) {
				ok = false
				break
			}
		}
		if ok {
			n++
		}
	}
	return n
}

func waitForGoroutineCount(t *testing.T, want int, frames ...string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if countGoroutinesBlockedIn(frames...) >= want {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("expected %d goroutines blocked in %v.\nRelevant goroutines:\n%s", want, frames, relevantStacks())
}

// TestSwamp_DeleteTreasure_StaleWaiterKeepsRecreatedKey: two DeleteTreasure
// calls for the same key queue on its guard. The first deletes the key; the
// key is then re-created with a new treasure object before the second one
// gets the guard. The second delete must not remove the re-created key: it
// resolved the old object before waiting and that object is no longer the
// one the key maps to.
func TestSwamp_DeleteTreasure_StaleWaiterKeepsRecreatedKey(t *testing.T) {
	var deadlocked atomic.Bool
	s := deadlockTestSwamp(t, "stale-delete-waiter", "queue", &deadlocked)

	for _, k := range []string{"anchor", "k"} {
		tr := s.CreateTreasure(k)
		gid := tr.StartTreasureGuard(true)
		tr.SetContentString(gid, "old")
		require.Equal(t, treasure.StatusNew, tr.Save(gid))
		tr.ReleaseTreasureGuard(gid)
	}

	old := s.CreateTreasure("k")
	holder := old.StartTreasureGuard(true)

	deleteFrames := []string{"(*swamp).deleteHandler", "guard.(*guard).StartTreasureGuard"}
	d1 := make(chan struct{})
	go func() { defer close(d1); _ = s.DeleteTreasure("k", false) }()
	waitForGoroutineCount(t, 1, deleteFrames...)

	// queue the re-creator behind the first delete and ahead of the second
	recreated := make(chan struct{})
	go func() {
		defer close(recreated)
		gid := old.StartTreasureGuard(true)
		defer old.ReleaseTreasureGuard(gid)
		nt := s.CreateTreasure("k")
		ngid := nt.StartTreasureGuard(true)
		nt.SetContentString(ngid, "new")
		nt.Save(ngid)
		nt.ReleaseTreasureGuard(ngid)
	}()
	waitForGoroutineCount(t, 1, "StaleWaiterKeepsRecreatedKey.func", "guard.(*guard).StartTreasureGuard")

	d2 := make(chan struct{})
	go func() { defer close(d2); _ = s.DeleteTreasure("k", false) }()
	waitForGoroutineCount(t, 2, deleteFrames...)

	old.ReleaseTreasureGuard(holder)
	for _, c := range []chan struct{}{d1, recreated, d2} {
		select {
		case <-c:
		case <-time.After(5 * time.Second):
			deadlocked.Store(true)
			t.Fatalf("DEADLOCK.\nRelevant goroutines:\n%s", relevantStacks())
		}
	}

	cur, err := s.GetTreasure("k")
	require.NoError(t, err, "the re-created key was deleted by a stale delete")
	v, _ := cur.GetContentString()
	require.Equal(t, "new", v)
}

// TestSwamp_CloneTreasures_ConcurrentOverwriteSave_NoDeadlock: a full clone
// (CloneTreasures, also used by the field-bucket build) must not wait for a
// treasure guard while holding beaconKey's lock, because an overwrite-Save
// holds the guard and then reads beaconKey.
func TestSwamp_CloneTreasures_ConcurrentOverwriteSave_NoDeadlock(t *testing.T) {
	var deadlocked atomic.Bool
	s := deadlockTestSwamp(t, "clone-vs-overwrite", "queue", &deadlocked)

	for i := 0; i < 32; i++ {
		tr := s.CreateTreasure(fmt.Sprintf("k%d", i))
		gid := tr.StartTreasureGuard(true)
		tr.SetContentString(gid, "v")
		tr.Save(gid)
		tr.ReleaseTreasureGuard(gid)
	}

	var (
		stop     atomic.Bool
		progress atomic.Int64
		wg       sync.WaitGroup
	)
	wg.Add(1)
	go func() {
		defer wg.Done()
		for !stop.Load() {
			_ = s.CloneTreasures()
			progress.Add(1)
		}
	}()
	// new keys take beaconKey's write lock, which is what turns a reader
	// waiting behind the clone into a deadlock
	for w := 0; w < 4; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i := 0; !stop.Load(); i++ {
				key := fmt.Sprintf("k%d", i%32)
				if i%4 == 0 {
					key = fmt.Sprintf("new-w%d-%d", w, i)
				}
				tr := s.CreateTreasure(key)
				gid := tr.StartTreasureGuard(true)
				tr.SetContentString(gid, fmt.Sprintf("v%d", i))
				tr.Save(gid)
				tr.ReleaseTreasureGuard(gid)
				progress.Add(1)
			}
		}(w)
	}

	allDone := make(chan struct{})
	go func() { wg.Wait(); close(allDone) }()
	deadline := time.Now().Add(2 * time.Second)
	last := progress.Load()
	for time.Now().Before(deadline) {
		time.Sleep(500 * time.Millisecond)
		cur := progress.Load()
		if cur == last {
			deadlocked.Store(true)
			stop.Store(true)
			t.Fatalf("DEADLOCK: no progress after %d operations.\nRelevant goroutines:\n%s", cur, relevantStacks())
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
