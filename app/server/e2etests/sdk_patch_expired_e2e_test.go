//go:build e2e

package e2etests

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/hydraide/hydraide/sdk/go/hydraidego/v3"
	"github.com/hydraide/hydraide/sdk/go/hydraidego/v3/name"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// E2E round-trip tests for CatalogPatchExpired and per-request Cond on
// CatalogPatchFieldsMany. Run with:
//
//   go test -tags e2e -run TestPatchExpiredE2E ./app/server/e2etests/...

// QueueClaimCatalog mirrors the Trendizz crawler-queue redesign model.
// Use msgpack-tagged fields so the body the engine writes is the same
// shape we'd see in production.
type QueueClaimCatalog struct {
	Domain    string    `hydraide:"key"      msgpack:"-"`
	ExpireAt  time.Time `hydraide:"expireAt" msgpack:"-"`
	ClaimedBy string    `hydraide:"-"        msgpack:"ClaimedBy"`
	Counter   int32     `hydraide:"-"        msgpack:"Counter"`
}

func patchExpiredE2ESwamp(t *testing.T, suffix string) name.Name {
	t.Helper()
	swamp := name.New().
		Sanctuary("tests").
		Realm("patch-expired").
		Swamp(fmt.Sprintf("%s-%d", suffix, time.Now().UnixNano()))

	errs := hydraidegoIface().RegisterSwamp(context.Background(), &hydraidego.RegisterSwampRequest{
		SwampPattern: swamp,
		FilesystemSettings: &hydraidego.SwampFilesystemSettings{
			WriteInterval:  time.Second,
			MaxFileSize:    8192,
			EncodingFormat: hydraidego.EncodingMsgPack,
		},
	})
	require.Empty(t, errs, "RegisterSwamp must succeed")

	t.Cleanup(func() {
		_ = hydraidegoIface().Destroy(context.Background(), swamp)
	})
	return swamp
}

// seedExpiredEntry creates a treasure with the given fields and stamps
// a past ExpiredAt via PatchBuilder.WithExpiredAt — the same path
// Trendizz uses to put items into the expired queue.
func seedExpiredEntry(t *testing.T, ctx context.Context, swamp name.Name, key string, claimedBy string, counter int32) {
	t.Helper()
	st, err := hydraidegoIface().
		CatalogPatch(ctx, swamp, key).
		Set("ClaimedBy", claimedBy).
		Set("Counter", counter).
		WithExpiredAt(time.Now().UTC().Add(-time.Hour)).
		Exec()
	require.NoError(t, err)
	require.Contains(t, []hydraidego.PatchStatus{hydraidego.PatchStatusCreated, hydraidego.PatchStatusPatched}, st)
}

// ---------- F.1 — concurrent queue claim across the gRPC layer ----------

func TestPatchExpiredE2E_ConcurrentQueueClaim(t *testing.T) {
	const total = 100
	const workers = 5
	const batch = 20

	swamp := patchExpiredE2ESwamp(t, "concurrent-claim")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	for i := 0; i < total; i++ {
		seedExpiredEntry(t, ctx, swamp, fmt.Sprintf("d%05d.hu", i), "", 0)
	}

	type claimedKey struct {
		key      string
		workerID int
	}
	resCh := make(chan []claimedKey, workers)

	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			workerName := fmt.Sprintf("worker-%d", workerID)
			builder := hydraidego.NewPatchExpiredOps().
				Set("ClaimedBy", workerName).
				WithExpiredAt(time.Now().UTC().Add(24 * time.Hour))

			var claimed []claimedKey
			err := hydraidegoIface().CatalogPatchExpired(ctx, swamp, batch,
				QueueClaimCatalog{},
				func(model any, st hydraidego.PatchStatus) error {
					m := model.(*QueueClaimCatalog)
					assert.Equal(t, hydraidego.PatchStatusPatched, st)
					assert.Equal(t, workerName, m.ClaimedBy)
					claimed = append(claimed, claimedKey{key: m.Domain, workerID: workerID})
					return nil
				}, builder)
			require.NoError(t, err)
			resCh <- claimed
		}(w)
	}
	wg.Wait()
	close(resCh)

	seen := make(map[string]int)
	totalClaimed := 0
	for r := range resCh {
		totalClaimed += len(r)
		for _, ck := range r {
			seen[ck.key]++
		}
	}
	assert.Equal(t, workers*batch, totalClaimed)
	for k, n := range seen {
		assert.Equal(t, 1, n, "key %s claimed %d times — must be exactly 1", k, n)
	}
}

// ---------- F.2 — recovery flow: re-claim after lease elapses ----------

func TestPatchExpiredE2E_RecoveryReclaimsAfterLease(t *testing.T) {
	swamp := patchExpiredE2ESwamp(t, "recovery")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	for i := 0; i < 5; i++ {
		seedExpiredEntry(t, ctx, swamp, fmt.Sprintf("d%d.hu", i), "", 0)
	}

	// First claim with a lease that's already in the past, so recovery
	// can pick the entries up immediately.
	firstBuilder := hydraidego.NewPatchExpiredOps().
		Set("ClaimedBy", "worker-A").
		WithExpiredAt(time.Now().UTC().Add(-50 * time.Millisecond))
	firstClaims := 0
	err := hydraidegoIface().CatalogPatchExpired(ctx, swamp, 100,
		QueueClaimCatalog{},
		func(model any, st hydraidego.PatchStatus) error {
			firstClaims++
			return nil
		}, firstBuilder)
	require.NoError(t, err)
	require.Equal(t, 5, firstClaims)

	// Worker-B reclaims because the worker-A lease is already in the past.
	secondBuilder := hydraidego.NewPatchExpiredOps().
		Set("ClaimedBy", "worker-B").
		WithExpiredAt(time.Now().UTC().Add(time.Hour))
	secondClaims := 0
	err = hydraidegoIface().CatalogPatchExpired(ctx, swamp, 100,
		QueueClaimCatalog{},
		func(model any, st hydraidego.PatchStatus) error {
			m := model.(*QueueClaimCatalog)
			assert.Equal(t, "worker-B", m.ClaimedBy, "key=%s", m.Domain)
			secondClaims++
			return nil
		}, secondBuilder)
	require.NoError(t, err)
	assert.Equal(t, 5, secondClaims, "all 5 stuck-claimed entries must be re-claimable")
}

// ---------- F.3 — per-request Cond in CatalogPatchFieldsMany ----------

func TestPatchExpiredE2E_PatchManyWithCondition(t *testing.T) {
	swamp := patchExpiredE2ESwamp(t, "patch-many-cond")
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// Seed 10 entries, half with Counter=0, half with Counter=1.
	for i := 0; i < 5; i++ {
		_, err := hydraidegoIface().CatalogPatchField(ctx, swamp, fmt.Sprintf("k%d", i), "Counter", int32(0))
		require.NoError(t, err)
	}
	for i := 5; i < 10; i++ {
		_, err := hydraidegoIface().CatalogPatchField(ctx, swamp, fmt.Sprintf("k%d", i), "Counter", int32(1))
		require.NoError(t, err)
	}

	// Patch all 10 with IfFieldLessThan("Counter", 1) — only the
	// Counter==0 entries should be incremented.
	requests := make([]*hydraidego.PatchManyRequest, 0, 10)
	for i := 0; i < 10; i++ {
		requests = append(requests, &hydraidego.PatchManyRequest{
			Builder: hydraidego.NewPatchBuilder(fmt.Sprintf("k%d", i)).
				Set("Counter", int32(99)).
				IfFieldLessThan("Counter", int32(1)),
		})
	}

	type kv struct {
		key string
		st  hydraidego.PatchStatus
	}
	var results []kv
	err := hydraidegoIface().CatalogPatchFieldsMany(ctx, swamp, requests,
		func(key string, st hydraidego.PatchStatus, errMsg string) error {
			results = append(results, kv{key, st})
			return nil
		})
	require.NoError(t, err)
	require.Equal(t, 10, len(results))

	patched := 0
	condFailed := 0
	for _, r := range results {
		switch r.st {
		case hydraidego.PatchStatusPatched:
			patched++
		case hydraidego.PatchStatusConditionNotMet:
			condFailed++
		}
	}
	assert.Equal(t, 5, patched, "5 entries with Counter=0 should be patched")
	assert.Equal(t, 5, condFailed, "5 entries with Counter=1 should report CONDITION_NOT_MET")
}

// ---------- F.4 — meta-only patch (empty ops, slide ExpireAt) ----------

func TestPatchExpiredE2E_MetaOnlySlide(t *testing.T) {
	swamp := patchExpiredE2ESwamp(t, "meta-only")
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	for i := 0; i < 4; i++ {
		seedExpiredEntry(t, ctx, swamp, fmt.Sprintf("d%d.hu", i), "init", int32(7))
	}

	newExp := time.Now().UTC().Add(2 * time.Hour)
	builder := hydraidego.NewPatchExpiredOps().
		WithExpiredAt(newExp).
		WithUpdatedAt()

	patched := 0
	err := hydraidegoIface().CatalogPatchExpired(ctx, swamp, 100,
		QueueClaimCatalog{},
		func(model any, st hydraidego.PatchStatus) error {
			m := model.(*QueueClaimCatalog)
			assert.Equal(t, hydraidego.PatchStatusPatched, st)
			// Body untouched — Counter and ClaimedBy preserved.
			assert.Equal(t, "init", m.ClaimedBy)
			assert.Equal(t, int32(7), m.Counter)
			assert.WithinDuration(t, newExp, m.ExpireAt, time.Second)
			patched++
			return nil
		}, builder)
	require.NoError(t, err)
	assert.Equal(t, 4, patched)

	// A second call must return nothing — every entry has a future ExpireAt.
	again := 0
	err = hydraidegoIface().CatalogPatchExpired(ctx, swamp, 100,
		QueueClaimCatalog{},
		func(model any, st hydraidego.PatchStatus) error {
			again++
			return nil
		}, hydraidego.NewPatchExpiredOps().Set("Counter", int32(99)))
	require.NoError(t, err)
	assert.Equal(t, 0, again)
}
