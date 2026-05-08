package swamp

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/hydraide/hydraide/app/core/hydra/swamp/treasure/msgpackpatch"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// seedExpiredMsgpack creates a treasure with a wrapped msgpack body and
// stamps a past ExpirationTime so it is immediately visible to
// PatchExpired's selection.
func seedExpiredMsgpack(t *testing.T, s Swamp, key string, v any, expiredAgo time.Duration) {
	t.Helper()
	seedMsgpack(t, s, key, v)
	tr, err := s.GetTreasure(key)
	require.NoError(t, err)
	gid := tr.StartTreasureGuard(true)
	tr.SetExpirationTime(gid, time.Now().UTC().Add(-expiredAgo))
	tr.Save(gid)
	tr.ReleaseTreasureGuard(gid)
}

// seedFutureExpiringMsgpack creates a treasure with a wrapped msgpack body
// and stamps a future ExpirationTime so PatchExpired's selection skips it.
func seedFutureExpiringMsgpack(t *testing.T, s Swamp, key string, v any, expiresIn time.Duration) {
	t.Helper()
	seedMsgpack(t, s, key, v)
	tr, err := s.GetTreasure(key)
	require.NoError(t, err)
	gid := tr.StartTreasureGuard(true)
	tr.SetExpirationTime(gid, time.Now().UTC().Add(expiresIn))
	tr.Save(gid)
	tr.ReleaseTreasureGuard(gid)
}

// ---------- E.1 — happy path: patch all expired, slide ExpiredAt ----------

func TestSwampPatchExpired_SlidesExpiredAtAndPatchesBody(t *testing.T) {
	s := patchTestSwamp(t, "patch", "expired-slide")
	for i := 0; i < 5; i++ {
		seedExpiredMsgpack(t, s, fmt.Sprintf("k-%d", i),
			map[string]any{"counter": int8(0), "claimedBy": ""},
			time.Hour)
	}

	newExp := time.Now().UTC().Add(24 * time.Hour)
	entries, err := s.PatchExpired(10,
		[]msgpackpatch.Op{
			{Kind: msgpackpatch.OpSet, Path: "claimedBy", Value: encMsgpack(t, "worker-A")},
			{Kind: msgpackpatch.OpInc, Path: "counter", Value: encMsgpack(t, int8(1))},
		},
		nil,
		&PatchFieldsMeta{SetExpiredAt: newExp, SetUpdatedAt: true},
	)
	require.NoError(t, err)
	assert.Equal(t, 5, len(entries))
	for _, e := range entries {
		assert.Equal(t, PatchStatusPatched, e.Status, "key %s", e.Key)
		assert.WithinDuration(t, newExp, e.ExpiredAt, time.Second)
	}

	// Verify on-disk side: each treasure now has the new ExpiredAt and the
	// patched body.
	for i := 0; i < 5; i++ {
		key := fmt.Sprintf("k-%d", i)
		body := readPatchedMap(t, s, key)
		assert.Equal(t, "worker-A", body["claimedBy"])
		assert.Equal(t, int8(1), body["counter"])

		tr, err := s.GetTreasure(key)
		require.NoError(t, err)
		assert.WithinDuration(t, newExp, time.Unix(0, tr.GetExpirationTime()).UTC(), time.Second)
	}

	// A second PatchExpired call must return nothing — every treasure
	// now has a future ExpiredAt.
	again, err := s.PatchExpired(10, []msgpackpatch.Op{
		{Kind: msgpackpatch.OpSet, Path: "marker", Value: encMsgpack(t, true)},
	}, nil, nil)
	require.NoError(t, err)
	assert.Empty(t, again, "no entries should be expired after the slide")
}

// ---------- E.2 — howMany caps the result count ----------

func TestSwampPatchExpired_HowManyCapsCount(t *testing.T) {
	s := patchTestSwamp(t, "patch", "expired-cap")
	for i := 0; i < 10; i++ {
		seedExpiredMsgpack(t, s, fmt.Sprintf("k-%d", i),
			map[string]any{"x": int8(0)}, time.Hour)
	}

	entries, err := s.PatchExpired(3,
		[]msgpackpatch.Op{{Kind: msgpackpatch.OpSet, Path: "x", Value: encMsgpack(t, int8(1))}},
		nil,
		&PatchFieldsMeta{SetExpiredAt: time.Now().UTC().Add(time.Hour)},
	)
	require.NoError(t, err)
	assert.Equal(t, 3, len(entries))

	// 7 treasures must remain expired (untouched).
	rest, err := s.PatchExpired(100,
		[]msgpackpatch.Op{{Kind: msgpackpatch.OpSet, Path: "x", Value: encMsgpack(t, int8(2))}},
		nil,
		&PatchFieldsMeta{SetExpiredAt: time.Now().UTC().Add(time.Hour)},
	)
	require.NoError(t, err)
	assert.Equal(t, 7, len(rest))
}

// ---------- E.3 — non-expired treasures are skipped ----------

func TestSwampPatchExpired_SkipsNonExpired(t *testing.T) {
	s := patchTestSwamp(t, "patch", "expired-skip-fresh")
	for i := 0; i < 3; i++ {
		seedExpiredMsgpack(t, s, fmt.Sprintf("expired-%d", i),
			map[string]any{"x": int8(0)}, time.Hour)
	}
	for i := 0; i < 3; i++ {
		seedFutureExpiringMsgpack(t, s, fmt.Sprintf("fresh-%d", i),
			map[string]any{"x": int8(0)}, time.Hour)
	}

	entries, err := s.PatchExpired(100,
		[]msgpackpatch.Op{{Kind: msgpackpatch.OpSet, Path: "x", Value: encMsgpack(t, int8(9))}},
		nil,
		&PatchFieldsMeta{SetExpiredAt: time.Now().UTC().Add(time.Hour)},
	)
	require.NoError(t, err)
	assert.Equal(t, 3, len(entries))
	for _, e := range entries {
		assert.Contains(t, e.Key, "expired-", "fresh entries must not be patched")
	}

	// Fresh entries must keep x=0.
	for i := 0; i < 3; i++ {
		body := readPatchedMap(t, s, fmt.Sprintf("fresh-%d", i))
		assert.Equal(t, int8(0), body["x"])
	}
}

// ---------- E.4 — meta-only patch (empty Ops, slide ExpiredAt) ----------

func TestSwampPatchExpired_MetaOnlySlide(t *testing.T) {
	s := patchTestSwamp(t, "patch", "expired-meta-only")
	for i := 0; i < 4; i++ {
		seedExpiredMsgpack(t, s, fmt.Sprintf("k-%d", i),
			map[string]any{"x": int8(7)}, time.Hour)
	}

	newExp := time.Now().UTC().Add(2 * time.Hour)
	entries, err := s.PatchExpired(10,
		nil, // no ops
		nil,
		&PatchFieldsMeta{SetExpiredAt: newExp},
	)
	require.NoError(t, err)
	assert.Equal(t, 4, len(entries))

	// Body untouched, ExpiredAt slid.
	for i := 0; i < 4; i++ {
		key := fmt.Sprintf("k-%d", i)
		body := readPatchedMap(t, s, key)
		assert.Equal(t, int8(7), body["x"], "body must not change on meta-only patch")
		tr, err := s.GetTreasure(key)
		require.NoError(t, err)
		assert.WithinDuration(t, newExp, time.Unix(0, tr.GetExpirationTime()).UTC(), time.Second)
	}
}

// ---------- E.5 — Condition gates which treasures get patched ----------

func TestSwampPatchExpired_ConditionGate(t *testing.T) {
	s := patchTestSwamp(t, "patch", "expired-cond")
	// 5 entries: 3 unclaimed (claimedBy=""), 2 already claimed.
	for i := 0; i < 3; i++ {
		seedExpiredMsgpack(t, s, fmt.Sprintf("free-%d", i),
			map[string]any{"claimedBy": ""}, time.Hour)
	}
	for i := 0; i < 2; i++ {
		seedExpiredMsgpack(t, s, fmt.Sprintf("taken-%d", i),
			map[string]any{"claimedBy": "someone-else"}, time.Hour)
	}

	cond := &msgpackpatch.Condition{
		Path:      "claimedBy",
		Op:        msgpackpatch.CondEqual,
		Threshold: encMsgpack(t, ""),
	}
	entries, err := s.PatchExpired(10,
		[]msgpackpatch.Op{{Kind: msgpackpatch.OpSet, Path: "claimedBy", Value: encMsgpack(t, "worker-A")}},
		cond,
		&PatchFieldsMeta{SetExpiredAt: time.Now().UTC().Add(time.Hour)},
	)
	require.NoError(t, err)

	patched := 0
	condFailed := 0
	for _, e := range entries {
		switch e.Status {
		case PatchStatusPatched:
			patched++
		case PatchStatusConditionNotMet:
			condFailed++
		}
	}
	assert.Equal(t, 3, patched, "free-* should patch")
	assert.Equal(t, 2, condFailed, "taken-* should report CONDITION_NOT_MET")

	// CONDITION_NOT_MET treasures retain their original body and original
	// ExpireAt — the next PatchExpired call must see them again.
	again, err := s.PatchExpired(10,
		[]msgpackpatch.Op{{Kind: msgpackpatch.OpSet, Path: "claimedBy", Value: encMsgpack(t, "worker-B")}},
		nil, // no condition this time
		&PatchFieldsMeta{SetExpiredAt: time.Now().UTC().Add(time.Hour)},
	)
	require.NoError(t, err)
	assert.Equal(t, 2, len(again), "the 2 condition-failed entries must still be expired-visible")
	for _, e := range again {
		assert.Contains(t, e.Key, "taken-")
	}
}

// ---------- E.6 — concurrent disjoint subsets ----------

func TestSwampPatchExpired_ConcurrentDisjointSubsets(t *testing.T) {
	const total = 100
	s := patchTestSwamp(t, "patch", "expired-concurrent")
	for i := 0; i < total; i++ {
		seedExpiredMsgpack(t, s, fmt.Sprintf("k-%05d", i),
			map[string]any{"x": int8(0)}, time.Hour)
	}

	const goroutines = 5
	const batch = 20
	var (
		mu    sync.Mutex
		seen  = make(map[string]int)
		wg    sync.WaitGroup
		total2 int
	)
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			entries, err := s.PatchExpired(batch,
				[]msgpackpatch.Op{
					{Kind: msgpackpatch.OpSet, Path: "claimedBy",
						Value: encMsgpack(t, fmt.Sprintf("worker-%d", workerID))},
				},
				nil,
				&PatchFieldsMeta{SetExpiredAt: time.Now().UTC().Add(24 * time.Hour)},
			)
			require.NoError(t, err)
			mu.Lock()
			defer mu.Unlock()
			total2 += len(entries)
			for _, e := range entries {
				seen[e.Key]++
			}
		}(g)
	}
	wg.Wait()

	assert.Equal(t, goroutines*batch, total2, "total patched across all workers")
	for k, n := range seen {
		assert.Equal(t, 1, n, "key %s claimed %d times — must be exactly 1", k, n)
	}
}

// ---------- E.7 — recovery flow: re-claim after the lease expires ----------

func TestSwampPatchExpired_RecoveryReclaimsAfterLease(t *testing.T) {
	s := patchTestSwamp(t, "patch", "expired-recovery")
	for i := 0; i < 3; i++ {
		seedExpiredMsgpack(t, s, fmt.Sprintf("k-%d", i),
			map[string]any{"claimedBy": ""}, time.Hour)
	}

	// First claim — short lease (already-elapsed) so the treasures
	// remain "expired" right away.
	entries, err := s.PatchExpired(10,
		[]msgpackpatch.Op{{Kind: msgpackpatch.OpSet, Path: "claimedBy", Value: encMsgpack(t, "worker-A")}},
		nil,
		// new ExpiredAt is 50ms in the past → the recovery path picks them up.
		&PatchFieldsMeta{SetExpiredAt: time.Now().UTC().Add(-50 * time.Millisecond)},
	)
	require.NoError(t, err)
	require.Equal(t, 3, len(entries))

	// Second claim — same swamp, different worker should re-claim.
	entries2, err := s.PatchExpired(10,
		[]msgpackpatch.Op{{Kind: msgpackpatch.OpSet, Path: "claimedBy", Value: encMsgpack(t, "worker-B")}},
		nil,
		&PatchFieldsMeta{SetExpiredAt: time.Now().UTC().Add(time.Hour)},
	)
	require.NoError(t, err)
	require.Equal(t, 3, len(entries2), "all 3 leases elapsed → all 3 must be re-claimable")
	for _, e := range entries2 {
		assert.Equal(t, PatchStatusPatched, e.Status)
	}
	// Bodies must show worker-B (the latest writer wins).
	for i := 0; i < 3; i++ {
		body := readPatchedMap(t, s, fmt.Sprintf("k-%d", i))
		assert.Equal(t, "worker-B", body["claimedBy"])
	}
}

// ---------- E.8 — howMany == 0 is no-op ----------

func TestSwampPatchExpired_HowManyZeroIsNoOp(t *testing.T) {
	s := patchTestSwamp(t, "patch", "expired-zero")
	seedExpiredMsgpack(t, s, "k", map[string]any{"x": int8(0)}, time.Hour)

	entries, err := s.PatchExpired(0,
		[]msgpackpatch.Op{{Kind: msgpackpatch.OpSet, Path: "x", Value: encMsgpack(t, int8(9))}},
		nil,
		&PatchFieldsMeta{SetExpiredAt: time.Now().UTC().Add(time.Hour)},
	)
	require.NoError(t, err)
	assert.Empty(t, entries)

	body := readPatchedMap(t, s, "k")
	assert.Equal(t, int8(0), body["x"], "howMany=0 must not touch any body")
}

// ---------- E.9 — empty ops + nil meta is no-op ----------

func TestSwampPatchExpired_EmptyOpsAndNilMetaIsNoOp(t *testing.T) {
	s := patchTestSwamp(t, "patch", "expired-empty")
	seedExpiredMsgpack(t, s, "k", map[string]any{"x": int8(0)}, time.Hour)

	entries, err := s.PatchExpired(10, nil, nil, nil)
	require.NoError(t, err)
	assert.Empty(t, entries)
}

// ---------- E.10 — empty swamp ----------

func TestSwampPatchExpired_EmptySwamp(t *testing.T) {
	s := patchTestSwamp(t, "patch", "expired-empty-swamp")
	entries, err := s.PatchExpired(10,
		[]msgpackpatch.Op{{Kind: msgpackpatch.OpSet, Path: "x", Value: encMsgpack(t, int8(1))}},
		nil,
		&PatchFieldsMeta{SetExpiredAt: time.Now().UTC().Add(time.Hour)},
	)
	require.NoError(t, err)
	assert.Empty(t, entries)
}

// ---------- E.11 — non-msgpack treasure surfaces TYPE_MISMATCH ----------

func TestSwampPatchExpired_NonMsgpackBodySurfacesEncoding(t *testing.T) {
	s := patchTestSwamp(t, "patch", "expired-nonmsgpack")

	// Seed a string-valued treasure (not msgpack) and expire it.
	tr := s.CreateTreasure("k")
	gid := tr.StartTreasureGuard(true)
	tr.SetContentString(gid, "plain-string")
	tr.SetExpirationTime(gid, time.Now().UTC().Add(-time.Hour))
	tr.Save(gid)
	tr.ReleaseTreasureGuard(gid)

	entries, err := s.PatchExpired(10,
		[]msgpackpatch.Op{{Kind: msgpackpatch.OpSet, Path: "x", Value: encMsgpack(t, int8(1))}},
		nil,
		&PatchFieldsMeta{SetExpiredAt: time.Now().UTC().Add(time.Hour)},
	)
	require.NoError(t, err)
	require.Equal(t, 1, len(entries))
	// String treasures hit the "not a ByteArray" branch first.
	assert.Equal(t, PatchStatusTypeMismatch, entries[0].Status)
}

// ---------- E.12 — clear ExpiredAt removes from expired index ----------

func TestSwampPatchExpired_ClearExpiredAtRemovesFromExpiredIndex(t *testing.T) {
	s := patchTestSwamp(t, "patch", "expired-clear")
	for i := 0; i < 3; i++ {
		seedExpiredMsgpack(t, s, fmt.Sprintf("k-%d", i),
			map[string]any{"x": int8(0)}, time.Hour)
	}

	entries, err := s.PatchExpired(10,
		[]msgpackpatch.Op{{Kind: msgpackpatch.OpSet, Path: "x", Value: encMsgpack(t, int8(1))}},
		nil,
		&PatchFieldsMeta{ClearExpiredAt: true},
	)
	require.NoError(t, err)
	assert.Equal(t, 3, len(entries))
	for _, e := range entries {
		assert.Equal(t, PatchStatusPatched, e.Status)
		assert.True(t, e.ExpiredAt.IsZero(), "ExpiredAt must be reported as zero after clear")
	}

	// Subsequent call must find none — they no longer have ExpirationTime.
	again, err := s.PatchExpired(10,
		[]msgpackpatch.Op{{Kind: msgpackpatch.OpSet, Path: "x", Value: encMsgpack(t, int8(9))}},
		nil,
		&PatchFieldsMeta{SetExpiredAt: time.Now().UTC().Add(time.Hour)},
	)
	require.NoError(t, err)
	assert.Empty(t, again, "treasures with ExpiredAt cleared must not be re-claimable")
}
