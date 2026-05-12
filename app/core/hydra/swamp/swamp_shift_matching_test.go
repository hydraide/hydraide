package swamp

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hydraide/hydraide/app/core/hydra/swamp/treasure"
	"github.com/hydraide/hydraide/app/core/hydra/swamp/treasure/msgpackpatch"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vmihailenco/msgpack/v5"
)

// seedKeyedMsgpack creates N msgpack-bodied treasures with sequential
// keys and an optional CreatedAt offset (so creation-time ordering is
// deterministic). Used by the Shift / Cap test suite.
func seedKeyedMsgpack(t *testing.T, s Swamp, prefix string, n int, body map[string]any) {
	t.Helper()
	for i := 0; i < n; i++ {
		seedMsgpack(t, s, fmt.Sprintf("%s-%d", prefix, i), body)
	}
}

// ---------- Shift S.1 — basic ordering by key (ASC) ----------

func TestSwamp_ShiftMatching_KeyOrderAsc(t *testing.T) {
	s := patchTestSwamp(t, "shift", "fifo-key")
	for i := 0; i < 5; i++ {
		seedMsgpack(t, s, fmt.Sprintf("k-%d", i), map[string]any{"i": int8(i)})
	}

	predicate := func(_ treasure.Treasure) bool { return true }

	shifted, capReached, err := s.CloneAndDeleteMatchingTreasures(
		BeaconTypeKey, IndexOrderAsc, 3, predicate, nil, 0,
	)
	require.NoError(t, err)
	assert.False(t, capReached, "no cap → capReached must be false")
	require.Len(t, shifted, 3)
	// First three keys alphabetically: k-0, k-1, k-2.
	for i, tr := range shifted {
		assert.Equal(t, fmt.Sprintf("k-%d", i), tr.GetKey())
	}
}

// ---------- Shift S.2 — filter narrows selection ----------

func TestSwamp_ShiftMatching_FilterPredicate(t *testing.T) {
	s := patchTestSwamp(t, "shift", "filter-narrow")
	seedKeyedMsgpack(t, s, "x", 10, map[string]any{"flag": true})

	// Predicate that matches only keys with even suffix index.
	even := func(tr treasure.Treasure) bool {
		k := tr.GetKey()
		// last byte of key encodes digit '0'..'9'
		return (k[len(k)-1]-'0')%2 == 0
	}

	shifted, capReached, err := s.CloneAndDeleteMatchingTreasures(
		BeaconTypeKey, IndexOrderAsc, 100, even, nil, 0,
	)
	require.NoError(t, err)
	assert.False(t, capReached)
	assert.Len(t, shifted, 5, "5 even-suffix keys must be shifted")
	for _, tr := range shifted {
		k := tr.GetKey()
		assert.Equal(t, byte('0'), (k[len(k)-1]-'0')%2+'0')
	}
}

// ---------- Shift S.3 — Cap budget bounds result ----------

func TestSwamp_ShiftMatching_CapBoundsResult(t *testing.T) {
	s := patchTestSwamp(t, "shift", "cap-bounds")
	// 10 candidates, all match the selection predicate.
	seedKeyedMsgpack(t, s, "j", 10, map[string]any{"claimed": false})

	matchAll := func(_ treasure.Treasure) bool { return true }
	// Cap.Filter counts records flagged as claimed. Initially zero → budget=3.
	capPredicate := func(tr treasure.Treasure) bool {
		// In a real Cap flow, the SHIFTED records would later be patched
		// into "claimed" status; here we just measure the budget at the
		// time of the Shift call. With no claimed records pre-existing,
		// budget = MaxMatching - 0 = 3.
		return false
	}

	shifted, capReached, err := s.CloneAndDeleteMatchingTreasures(
		BeaconTypeKey, IndexOrderAsc, 100, matchAll, capPredicate, 3,
	)
	require.NoError(t, err)
	assert.True(t, capReached, "cap budget bounded the result → capReached=true")
	assert.Len(t, shifted, 3)
}

// ---------- Shift S.4 — Cap=nil is regression twin ----------

func TestSwamp_ShiftMatching_NoCap_RegressionTwin(t *testing.T) {
	s := patchTestSwamp(t, "shift", "nocap-twin")
	seedKeyedMsgpack(t, s, "p", 5, map[string]any{})
	predicate := func(_ treasure.Treasure) bool { return true }

	shifted, capReached, err := s.CloneAndDeleteMatchingTreasures(
		BeaconTypeKey, IndexOrderAsc, 100, predicate, nil, 0,
	)
	require.NoError(t, err)
	assert.False(t, capReached, "nil Cap → CapReached must be false")
	assert.Len(t, shifted, 5)
}

// ---------- Shift S.5 — Concurrency: 16 goroutines, no duplicates ----------

func TestSwamp_ShiftMatching_NoDuplicatesUnderConcurrency(t *testing.T) {
	s := patchTestSwamp(t, "shift", "concurrent-fifo")
	const total = 100
	for i := 0; i < total; i++ {
		seedMsgpack(t, s, fmt.Sprintf("k-%03d", i), map[string]any{})
	}

	matchAll := func(_ treasure.Treasure) bool { return true }

	const goroutines = 16
	var (
		mu         sync.Mutex
		allShifted = make(map[string]struct{}, total)
		wg         sync.WaitGroup
	)
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				shifted, _, err := s.CloneAndDeleteMatchingTreasures(
					BeaconTypeKey, IndexOrderAsc, 5, matchAll, nil, 0,
				)
				if err != nil || len(shifted) == 0 {
					return
				}
				mu.Lock()
				for _, tr := range shifted {
					key := tr.GetKey()
					if _, dup := allShifted[key]; dup {
						mu.Unlock()
						t.Errorf("duplicate shift of key %q", key)
						return
					}
					allShifted[key] = struct{}{}
				}
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	assert.Equal(t, total, len(allShifted), "every treasure must be shifted exactly once")
}

// ---------- PatchExpired Cap C.1 — Cap exact budget ----------

func TestSwamp_PatchExpired_CapExactBudget(t *testing.T) {
	s := patchTestSwamp(t, "patch", "cap-exact")
	// Seed 10 expired records, all currently "unclaimed" (claimedBy == "").
	for i := 0; i < 10; i++ {
		seedExpiredMsgpack(t, s, fmt.Sprintf("k-%d", i),
			map[string]any{"claimedBy": ""}, time.Hour)
	}

	// 3 are pre-claimed (matching Cap.Filter). MaxMatching=5 ⇒ budget=2.
	for i := 0; i < 3; i++ {
		// Manually flag as claimed by directly patching one record:
		// simulate the in-flight state Cap.Filter measures.
		key := fmt.Sprintf("k-%d", i)
		entries, _, err := s.PatchExpired(1,
			[]msgpackpatch.Op{{Kind: msgpackpatch.OpSet, Path: "claimedBy", Value: encMsgpack(t, fmt.Sprintf("w-%d", i))}},
			nil, &PatchFieldsMeta{SetExpiredAt: time.Now().UTC().Add(time.Hour)},
			nil, nil, 0,
		)
		require.NoError(t, err)
		require.Len(t, entries, 1, "preclaim step %d", i)
		// Now this record is no longer expired (slid forward); it remains claimed.
		_ = key
	}

	// Run Cap-bearing PatchExpired: cap counts "claimedBy != \"\"" records.
	capPredicate := func(tr treasure.Treasure) bool {
		raw, err := tr.GetContentByteArray()
		if err != nil || len(raw) < 2 {
			return false
		}
		body := readMsgpackMapInternal(raw[2:])
		v, ok := body["claimedBy"].(string)
		return ok && v != ""
	}

	entries, capReached, err := s.PatchExpired(100,
		[]msgpackpatch.Op{{Kind: msgpackpatch.OpSet, Path: "claimedBy", Value: encMsgpack(t, "later")}},
		nil, &PatchFieldsMeta{SetExpiredAt: time.Now().UTC().Add(time.Hour)},
		nil, capPredicate, 5,
	)
	require.NoError(t, err)
	assert.True(t, capReached, "budget 2 < remaining 7 expired matches → capReached=true")
	assert.Len(t, entries, 2, "budget = 5 - 3 already-claimed = 2")
}

// ---------- PatchExpired Cap C.2 — Cap exhausted, no claim ----------

func TestSwamp_PatchExpired_CapExhausted(t *testing.T) {
	s := patchTestSwamp(t, "patch", "cap-exhausted")
	// 5 expired records all already match Cap.Filter (claimed sentinel).
	for i := 0; i < 5; i++ {
		seedExpiredMsgpack(t, s, fmt.Sprintf("k-%d", i),
			map[string]any{"claimedBy": "preset"}, time.Hour)
	}

	capPredicate := func(tr treasure.Treasure) bool {
		raw, _ := tr.GetContentByteArray()
		if len(raw) < 2 {
			return false
		}
		body := readMsgpackMapInternal(raw[2:])
		v, _ := body["claimedBy"].(string)
		return v != ""
	}

	entries, capReached, err := s.PatchExpired(100,
		[]msgpackpatch.Op{{Kind: msgpackpatch.OpSet, Path: "claimedBy", Value: encMsgpack(t, "new")}},
		nil, &PatchFieldsMeta{SetExpiredAt: time.Now().UTC().Add(time.Hour)},
		nil, capPredicate, 3, // already over cap (5 > 3) ⇒ budget ≤ 0
	)
	require.NoError(t, err)
	assert.True(t, capReached)
	assert.Empty(t, entries)
}

// ---------- PatchExpired Cap C.3 — nil Cap is regression twin ----------

func TestSwamp_PatchExpired_NoCap_RegressionTwin(t *testing.T) {
	s := patchTestSwamp(t, "patch", "nocap-twin")
	for i := 0; i < 5; i++ {
		seedExpiredMsgpack(t, s, fmt.Sprintf("k-%d", i),
			map[string]any{"x": int8(0)}, time.Hour)
	}
	entries, capReached, err := s.PatchExpired(100,
		[]msgpackpatch.Op{{Kind: msgpackpatch.OpSet, Path: "x", Value: encMsgpack(t, int8(1))}},
		nil, &PatchFieldsMeta{SetExpiredAt: time.Now().UTC().Add(time.Hour)},
		nil, nil, 0,
	)
	require.NoError(t, err)
	assert.False(t, capReached, "nil Cap → capReached must be false")
	assert.Len(t, entries, 5)
}

// ---------- PatchExpired Cap C.4 — concurrency: never over cap ----------

func TestSwamp_PatchExpired_CapNeverExceededUnderConcurrency(t *testing.T) {
	s := patchTestSwamp(t, "patch", "cap-concurrent")
	const total = 100
	for i := 0; i < total; i++ {
		seedExpiredMsgpack(t, s, fmt.Sprintf("k-%03d", i),
			map[string]any{"claimedBy": ""}, time.Hour)
	}

	const maxMatching = int32(10)
	capPredicate := func(tr treasure.Treasure) bool {
		raw, _ := tr.GetContentByteArray()
		if len(raw) < 2 {
			return false
		}
		body := readMsgpackMapInternal(raw[2:])
		v, _ := body["claimedBy"].(string)
		return v != ""
	}

	const goroutines = 16
	var totalClaimed int64
	var wg sync.WaitGroup
	for g := 0; g < goroutines; g++ {
		gID := g
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				entries, _, err := s.PatchExpired(100,
					[]msgpackpatch.Op{{Kind: msgpackpatch.OpSet, Path: "claimedBy", Value: encMsgpack(t, fmt.Sprintf("w-%d", gID))}},
					nil, &PatchFieldsMeta{SetExpiredAt: time.Now().UTC().Add(time.Hour)},
					nil, capPredicate, maxMatching,
				)
				if err != nil {
					t.Errorf("PatchExpired error: %v", err)
					return
				}
				atomic.AddInt64(&totalClaimed, int64(len(entries)))
				if len(entries) == 0 {
					return
				}
			}
		}()
	}
	wg.Wait()

	// Invariant: total claimed must never exceed maxMatching, because
	// claimed records have a future ExpiredAt and no longer match the
	// expired selection. The Cap pre-check counts current claimed under
	// the same Lock as selection, so 16 concurrent goroutines cannot
	// breach the cap.
	assert.LessOrEqual(t, totalClaimed, int64(maxMatching), "Σ claimed must respect Cap.MaxMatching")
}

// readMsgpackMapInternal decodes a msgpack body (sans magic prefix) into
// a map. Test-only helper.
func readMsgpackMapInternal(body []byte) map[string]any {
	var got map[string]any
	_ = msgpack.Unmarshal(body, &got)
	return got
}
