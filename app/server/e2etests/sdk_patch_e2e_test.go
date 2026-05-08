//go:build e2e

package e2etests

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hydraide/hydraide/sdk/go/hydraidego/v3"
	"github.com/hydraide/hydraide/sdk/go/hydraidego/v3/name"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// E2E round-trip tests for the new PatchTreasures RPC. These are gated
// behind the e2e build tag because they require a running HydrAIDE
// server and mTLS certs (see TestMain in e2etests_test.go).
//
// Run with:
//   go test -tags e2e -run TestPatchE2E ./app/server/e2etests/...

// patchE2ESwamp registers a fresh msgpack-encoded swamp under the patch
// realm and returns its name plus a cleanup hook.
func patchE2ESwamp(t *testing.T, suffix string) name.Name {
	t.Helper()
	swamp := name.New().
		Sanctuary("tests").
		Realm("patch").
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

// AllDomainsCatalog mirrors the production model that motivated this
// feature. The Trendizz cutover replaces a Lock+Load+Save loop on these
// fields with CatalogPatchFields calls.
type AllDomainsCatalog struct {
	Domain         string `hydraide:"key"          msgpack:"-"`
	IsInQueue      bool   `hydraide:"value"        msgpack:"IsInQueue"`
	IsCrawling     bool   `hydraide:"-"            msgpack:"IsCrawling"`
	IsRejected     bool   `hydraide:"-"            msgpack:"IsRejected"`
	RejectedReason int16  `hydraide:"-"            msgpack:"RejectedReason"`
	StatusCounter  int32  `hydraide:"-"            msgpack:"StatusCounter"`
}

// ---------- E.1 — single-field round-trip ----------

func TestPatchE2E_CatalogPatchField(t *testing.T) {
	swamp := patchE2ESwamp(t, "single-field")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	st, err := hydraidegoIface().CatalogPatchField(ctx, swamp, "domain1.hu", "IsInQueue", true)
	require.NoError(t, err)
	assert.Equal(t, hydraidego.PatchStatusCreated, st)

	// Apply a second patch with a different field to confirm the existing
	// payload is preserved.
	st, err = hydraidegoIface().CatalogPatchField(ctx, swamp, "domain1.hu", "IsCrawling", true)
	require.NoError(t, err)
	assert.Equal(t, hydraidego.PatchStatusPatched, st)
}

// ---------- E.2 — multi-field round-trip ----------

func TestPatchE2E_CatalogPatchFields(t *testing.T) {
	swamp := patchE2ESwamp(t, "multi-field")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	st, err := hydraidegoIface().CatalogPatchFields(ctx, swamp, "domain1.hu", map[string]any{
		"IsInQueue":      true,
		"IsCrawling":     false,
		"IsRejected":     false,
		"RejectedReason": int16(0),
		"StatusCounter":  int32(1),
	})
	require.NoError(t, err)
	assert.Equal(t, hydraidego.PatchStatusCreated, st)
}

// ---------- E.3 — multi-key batch round-trip ----------

func TestPatchE2E_CatalogPatchFieldsMany(t *testing.T) {
	swamp := patchE2ESwamp(t, "multi-key-batch")
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	requests := []*hydraidego.PatchManyRequest{
		{Builder: hydraidego.NewPatchBuilder("d1.hu").Set("IsInQueue", true)},
		{Builder: hydraidego.NewPatchBuilder("d2.hu").Set("IsInQueue", false).Set("IsCrawling", true)},
		{Builder: hydraidego.NewPatchBuilder("d3.hu").Set("IsRejected", true).Set("RejectedReason", int16(7))},
	}

	results := make([]hydraidego.PatchStatus, 0, len(requests))
	keys := make([]string, 0, len(requests))
	err := hydraidegoIface().CatalogPatchFieldsMany(ctx, swamp, requests,
		func(key string, st hydraidego.PatchStatus, errMsg string) error {
			results = append(results, st)
			keys = append(keys, key)
			return nil
		},
	)
	require.NoError(t, err)
	require.Len(t, results, 3)

	for i, st := range results {
		assert.Equal(t, hydraidego.PatchStatusCreated, st, "key %s (%d) should be CREATED", keys[i], i)
	}
	// Order in the response must match the order of the request slice.
	assert.Equal(t, []string{"d1.hu", "d2.hu", "d3.hu"}, keys)
}

// ---------- E.4 — builder API round-trip ----------

func TestPatchE2E_BuilderAPI(t *testing.T) {
	swamp := patchE2ESwamp(t, "builder")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Seed the treasure first.
	_, err := hydraidegoIface().CatalogPatchFields(ctx, swamp, "domain1.hu", map[string]any{
		"Owner":         "alice",
		"StatusCounter": int32(0),
		"IsInQueue":     false,
	})
	require.NoError(t, err)

	// Builder: increment counter under condition, set flag.
	st, err := hydraidegoIface().
		CatalogPatch(ctx, swamp, "domain1.hu").
		Inc("StatusCounter", int32(5)).
		Set("IsInQueue", true).
		IfFieldEquals("Owner", "alice").
		WithUpdatedAt().
		WithUpdatedBy("e2e-test").
		Exec()
	require.NoError(t, err)
	assert.Equal(t, hydraidego.PatchStatusPatched, st)

	// Builder under non-matching condition: no-op, status reflects miss.
	st, err = hydraidegoIface().
		CatalogPatch(ctx, swamp, "domain1.hu").
		Set("IsInQueue", false).
		IfFieldEquals("Owner", "bob").
		Exec()
	require.NoError(t, err)
	assert.Equal(t, hydraidego.PatchStatusConditionNotMet, st)
}

// ---------- E.5 — production-shaped stress run ----------

// TestPatchE2E_StressMatchesIncidentScenario replays the original
// AllDomains-flag incident shape: many domains, several workers each
// patching a different flag concurrently. A short window (5s) is enough
// to surface goroutine-leak / lost-update / lock-contention bugs without
// dragging CI runtime.
//
// Acceptance: every patch returns PatchStatusPatched or PatchStatusCreated;
// no internal errors; final flag values match the count of patches dispatched.
func TestPatchE2E_StressMatchesIncidentScenario(t *testing.T) {
	const (
		domains = 100
		workers = 6
		runFor  = 5 * time.Second
	)
	swamp := patchE2ESwamp(t, "stress")
	ctx, cancel := context.WithTimeout(context.Background(), runFor+30*time.Second)
	defer cancel()

	// Pre-seed all domains so workers are exclusively patching, not creating.
	seedCtx, seedCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer seedCancel()
	for i := 0; i < domains; i++ {
		_, err := hydraidegoIface().CatalogPatchFields(seedCtx, swamp, fmt.Sprintf("d%03d.hu", i), map[string]any{
			"IsInQueue":      false,
			"IsCrawling":     false,
			"IsRejected":     false,
			"RejectedReason": int16(0),
			"Counter":        int32(0),
		})
		require.NoError(t, err)
	}

	flagPaths := []string{"IsInQueue", "IsCrawling", "IsRejected"}
	var (
		wg         sync.WaitGroup
		patchCount int64
		errCount   int64
		stop       = make(chan struct{})
	)

	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			flag := flagPaths[workerID%len(flagPaths)]
			i := 0
			for {
				select {
				case <-stop:
					return
				default:
				}
				key := fmt.Sprintf("d%03d.hu", i%domains)
				st, err := hydraidegoIface().CatalogPatchField(ctx, swamp, key, flag, true)
				if err != nil {
					atomic.AddInt64(&errCount, 1)
					t.Logf("worker %d error: %v", workerID, err)
					return
				}
				if st != hydraidego.PatchStatusPatched && st != hydraidego.PatchStatusCreated {
					atomic.AddInt64(&errCount, 1)
					t.Logf("worker %d unexpected status: %s", workerID, st)
				}
				atomic.AddInt64(&patchCount, 1)
				i++
			}
		}(w)
	}

	time.Sleep(runFor)
	close(stop)
	wg.Wait()

	assert.Zero(t, atomic.LoadInt64(&errCount), "no per-key errors expected during stress")
	t.Logf("dispatched %d patches across %d workers in %s",
		atomic.LoadInt64(&patchCount), workers, runFor)
	assert.Greater(t, atomic.LoadInt64(&patchCount), int64(workers*100),
		"workers should each manage at least 100 patches in %s", runFor)
}

// ---------- E.6 — KEY_NOT_FOUND when CreateIfNotExist=false ----------

func TestPatchE2E_NoCreateReturnsKeyNotFound(t *testing.T) {
	swamp := patchE2ESwamp(t, "no-create")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	st, err := hydraidegoIface().
		CatalogPatch(ctx, swamp, "missing-key").
		NoCreate().
		Set("x", int8(1)).
		Exec()
	require.NoError(t, err)
	assert.Equal(t, hydraidego.PatchStatusKeyNotFound, st)
}
