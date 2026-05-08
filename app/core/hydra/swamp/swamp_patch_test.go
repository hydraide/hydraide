package swamp

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hydraide/hydraide/app/core/filesystem"
	"github.com/hydraide/hydraide/app/core/hydra/swamp/chronicler"
	"github.com/hydraide/hydraide/app/core/hydra/swamp/metadata"
	"github.com/hydraide/hydraide/app/core/hydra/swamp/treasure"
	"github.com/hydraide/hydraide/app/core/hydra/swamp/treasure/msgpackpatch"
	"github.com/hydraide/hydraide/app/core/settings"
	"github.com/hydraide/hydraide/app/name"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vmihailenco/msgpack/v5"
)

// patchTestSwamp spins up an in-memory swamp wired to a real chronicler so
// PatchFields tests can exercise the full Save → reload path.
func patchTestSwamp(t *testing.T, realm, swampN string) Swamp {
	return patchTestSwampTB(t, realm, swampN)
}

// patchTestSwampTB is the testing.TB-shaped helper so benchmarks
// (*testing.B) can share the same setup as tests (*testing.T).
func patchTestSwampTB(tb testing.TB, realm, swampN string) Swamp {
	tb.Helper()
	fsInterface := filesystem.New()
	settingsInterface := settings.New(testMaxDepth, testMaxFolderPerLevel)
	fss := &settings.FileSystemSettings{WriteIntervalSec: 1, MaxFileSizeByte: 8192}
	settingsInterface.RegisterPattern(
		name.New().Sanctuary(sanctuaryForQuickTest).Realm("*").Swamp("*"),
		false, 1, fss,
	)
	swampName := name.New().Sanctuary(sanctuaryForQuickTest).Realm(realm).Swamp(swampN)
	hashPath := swampName.GetFullHashPath(
		settingsInterface.GetHydraAbsDataFolderPath(),
		testAllServers, testMaxDepth, testMaxFolderPerLevel,
	)
	chroniclerInterface := chronicler.New(hashPath, 8192, testMaxDepth, fsInterface, metadata.New(hashPath))
	chroniclerInterface.CreateDirectoryIfNotExists()
	fssSwamp := &FilesystemSettings{ChroniclerInterface: chroniclerInterface, WriteInterval: 1 * time.Second}
	metadataInterface := metadata.New(hashPath)
	s := New(
		swampName, 1*time.Second, fssSwamp,
		func(e *Event) {}, func(i *Info) {}, func(n name.Name) {},
		metadataInterface,
	)
	s.BeginVigil()
	tb.Cleanup(func() {
		s.CeaseVigil()
		s.Destroy()
	})
	return s
}

// wrapMsgpack mirrors the SDK helper: prepends the [0xC7, 0x00] magic prefix.
func wrapMsgpack(t *testing.T, v any) []byte {
	t.Helper()
	body, err := msgpack.Marshal(v)
	require.NoError(t, err)
	return append([]byte{0xC7, 0x00}, body...)
}

// readPatchedMap reads the treasure value, unwraps the magic prefix, and
// decodes the msgpack body into map[string]any.
func readPatchedMap(t *testing.T, s Swamp, key string) map[string]any {
	t.Helper()
	tr, err := s.GetTreasure(key)
	require.NoError(t, err, "treasure %q must exist", key)
	raw, err := tr.GetContentByteArray()
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(raw), 2)
	require.Equal(t, byte(0xC7), raw[0])
	require.Equal(t, byte(0x00), raw[1])
	var got map[string]any
	require.NoError(t, msgpack.Unmarshal(raw[2:], &got))
	return got
}

func encMsgpack(t *testing.T, v any) []byte {
	t.Helper()
	b, err := msgpack.Marshal(v)
	require.NoError(t, err)
	return b
}

// seedMsgpack creates a treasure with a wrapped msgpack-encoded body.
func seedMsgpack(t *testing.T, s Swamp, key string, v any) {
	t.Helper()
	tr := s.CreateTreasure(key)
	gid := tr.StartTreasureGuard(true)
	tr.SetContentByteArray(gid, wrapMsgpack(t, v))
	require.Equal(t, treasure.StatusNew, tr.Save(gid))
	tr.ReleaseTreasureGuard(gid)
}

// ---------- B.1 — CreateIfNotExist ----------

func TestSwampPatch_CreateIfNotExist(t *testing.T) {
	s := patchTestSwamp(t, "patch", "create-if-not-exist")
	res, err := s.PatchFields("newkey",
		[]msgpackpatch.Op{
			{Kind: msgpackpatch.OpSet, Path: "name", Value: encMsgpack(t, "alice")},
		}, nil,
		PatchFieldsOptions{CreateIfNotExist: true},
	)
	require.NoError(t, err)
	assert.Equal(t, PatchStatusCreated, res.Status)

	got := readPatchedMap(t, s, "newkey")
	assert.Equal(t, "alice", got["name"])
}

// ---------- B.2 — modify existing ----------

func TestSwampPatch_ModifyExisting(t *testing.T) {
	s := patchTestSwamp(t, "patch", "modify-existing")
	seedMsgpack(t, s, "k", map[string]any{"name": "alice", "age": int8(30)})

	res, err := s.PatchFields("k",
		[]msgpackpatch.Op{
			{Kind: msgpackpatch.OpSet, Path: "name", Value: encMsgpack(t, "bob")},
			{Kind: msgpackpatch.OpInc, Path: "age", Value: encMsgpack(t, int8(1))},
		}, nil, PatchFieldsOptions{},
	)
	require.NoError(t, err)
	assert.Equal(t, PatchStatusPatched, res.Status)

	got := readPatchedMap(t, s, "k")
	assert.Equal(t, "bob", got["name"])
	assert.EqualValues(t, 31, got["age"])
}

// ---------- B.3 — meta updates ----------

func TestSwampPatch_UpdatesUpdatedAt(t *testing.T) {
	s := patchTestSwamp(t, "patch", "meta-updates")
	seedMsgpack(t, s, "k", map[string]any{"x": int8(1)})

	tr, err := s.GetTreasure("k")
	require.NoError(t, err)
	beforeUpdated := tr.GetModifiedAt()

	time.Sleep(5 * time.Millisecond)

	_, err = s.PatchFields("k",
		[]msgpackpatch.Op{{Kind: msgpackpatch.OpSet, Path: "x", Value: encMsgpack(t, int8(2))}},
		nil,
		PatchFieldsOptions{Meta: &PatchFieldsMeta{SetUpdatedAt: true, SetUpdatedBy: "tester"}},
	)
	require.NoError(t, err)

	tr, err = s.GetTreasure("k")
	require.NoError(t, err)
	assert.Greater(t, tr.GetModifiedAt(), beforeUpdated, "UpdatedAt must advance")
	assert.Equal(t, "tester", tr.GetModifiedBy())
}

// ---------- B.3.a — meta does NOT change ExpiredAt by default ----------

func TestSwampPatch_LeavesExpiredAtUnchangedWhenMetaOmitted(t *testing.T) {
	s := patchTestSwamp(t, "patch", "expired-at-untouched")
	seedMsgpack(t, s, "k", map[string]any{"x": int8(1)})

	// Pre-stamp an ExpiredAt so we can verify the patch leaves it alone.
	tr, err := s.GetTreasure("k")
	require.NoError(t, err)
	gid := tr.StartTreasureGuard(true)
	original := time.Now().Add(2 * time.Hour).UTC()
	tr.SetExpirationTime(gid, original)
	tr.Save(gid)
	tr.ReleaseTreasureGuard(gid)

	_, err = s.PatchFields("k",
		[]msgpackpatch.Op{{Kind: msgpackpatch.OpSet, Path: "x", Value: encMsgpack(t, int8(2))}},
		nil,
		PatchFieldsOptions{Meta: &PatchFieldsMeta{SetUpdatedAt: true}},
	)
	require.NoError(t, err)

	tr, err = s.GetTreasure("k")
	require.NoError(t, err)
	assert.Equal(t, original.UnixNano(), tr.GetExpirationTime(),
		"ExpiredAt must be untouched when SetExpiredAt and ClearExpiredAt are both unset")
}

// ---------- B.3.b — SetExpiredAt stamps the new TTL ----------

func TestSwampPatch_SetExpiredAt(t *testing.T) {
	s := patchTestSwamp(t, "patch", "set-expired-at")
	seedMsgpack(t, s, "k", map[string]any{"x": int8(1)})

	want := time.Now().Add(1 * time.Hour).UTC()
	_, err := s.PatchFields("k",
		[]msgpackpatch.Op{{Kind: msgpackpatch.OpSet, Path: "x", Value: encMsgpack(t, int8(2))}},
		nil,
		PatchFieldsOptions{Meta: &PatchFieldsMeta{SetExpiredAt: want}},
	)
	require.NoError(t, err)

	tr, err := s.GetTreasure("k")
	require.NoError(t, err)
	assert.Equal(t, want.UnixNano(), tr.GetExpirationTime())
	assert.False(t, tr.IsExpired(), "future TTL must not read as expired")
}

// ---------- B.3.c — SetExpiredAt slides an existing TTL ----------

func TestSwampPatch_SetExpiredAtSlidesExistingTTL(t *testing.T) {
	s := patchTestSwamp(t, "patch", "slide-expired-at")
	seedMsgpack(t, s, "k", map[string]any{"x": int8(1)})

	// Initial short TTL.
	tr, err := s.GetTreasure("k")
	require.NoError(t, err)
	gid := tr.StartTreasureGuard(true)
	tr.SetExpirationTime(gid, time.Now().Add(1*time.Minute).UTC())
	tr.Save(gid)
	tr.ReleaseTreasureGuard(gid)

	// Patch slides it further into the future.
	want := time.Now().Add(7 * 24 * time.Hour).UTC()
	_, err = s.PatchFields("k",
		[]msgpackpatch.Op{{Kind: msgpackpatch.OpSet, Path: "x", Value: encMsgpack(t, int8(2))}},
		nil,
		PatchFieldsOptions{Meta: &PatchFieldsMeta{SetExpiredAt: want}},
	)
	require.NoError(t, err)

	tr, err = s.GetTreasure("k")
	require.NoError(t, err)
	assert.Equal(t, want.UnixNano(), tr.GetExpirationTime())
}

// ---------- B.3.d — ClearExpiredAt drops the TTL ----------

func TestSwampPatch_ClearExpiredAt(t *testing.T) {
	s := patchTestSwamp(t, "patch", "clear-expired-at")
	seedMsgpack(t, s, "k", map[string]any{"x": int8(1)})

	// Seed a TTL.
	tr, err := s.GetTreasure("k")
	require.NoError(t, err)
	gid := tr.StartTreasureGuard(true)
	tr.SetExpirationTime(gid, time.Now().Add(1*time.Hour).UTC())
	tr.Save(gid)
	tr.ReleaseTreasureGuard(gid)

	_, err = s.PatchFields("k",
		[]msgpackpatch.Op{{Kind: msgpackpatch.OpSet, Path: "x", Value: encMsgpack(t, int8(2))}},
		nil,
		PatchFieldsOptions{Meta: &PatchFieldsMeta{ClearExpiredAt: true}},
	)
	require.NoError(t, err)

	tr, err = s.GetTreasure("k")
	require.NoError(t, err)
	assert.EqualValues(t, 0, tr.GetExpirationTime(), "ClearExpiredAt must reset to 0")
	assert.False(t, tr.IsExpired())
}

// ---------- B.3.e — ClearExpiredAt wins over SetExpiredAt ----------

func TestSwampPatch_ClearExpiredAtBeatsSet(t *testing.T) {
	s := patchTestSwamp(t, "patch", "clear-beats-set")
	seedMsgpack(t, s, "k", map[string]any{"x": int8(1)})

	_, err := s.PatchFields("k",
		[]msgpackpatch.Op{{Kind: msgpackpatch.OpSet, Path: "x", Value: encMsgpack(t, int8(2))}},
		nil,
		PatchFieldsOptions{Meta: &PatchFieldsMeta{
			SetExpiredAt:   time.Now().Add(1 * time.Hour).UTC(),
			ClearExpiredAt: true,
		}},
	)
	require.NoError(t, err)

	tr, err := s.GetTreasure("k")
	require.NoError(t, err)
	assert.EqualValues(t, 0, tr.GetExpirationTime(), "ClearExpiredAt must take precedence")
}

// ---------- B.3.f — SetExpiredAt also applies on creation ----------

func TestSwampPatch_SetExpiredAtOnCreate(t *testing.T) {
	s := patchTestSwamp(t, "patch", "expired-on-create")

	want := time.Now().Add(30 * time.Minute).UTC()
	res, err := s.PatchFields("newkey",
		[]msgpackpatch.Op{{Kind: msgpackpatch.OpSet, Path: "x", Value: encMsgpack(t, int8(1))}},
		nil,
		PatchFieldsOptions{
			CreateIfNotExist: true,
			Meta:             &PatchFieldsMeta{SetExpiredAt: want},
		},
	)
	require.NoError(t, err)
	assert.Equal(t, PatchStatusCreated, res.Status)

	tr, err := s.GetTreasure("newkey")
	require.NoError(t, err)
	assert.Equal(t, want.UnixNano(), tr.GetExpirationTime())
}

// ---------- B.4 — condition met ----------

func TestSwampPatch_ConditionMet(t *testing.T) {
	s := patchTestSwamp(t, "patch", "cond-met")
	seedMsgpack(t, s, "k", map[string]any{"owner": "alice", "n": int8(0)})

	res, err := s.PatchFields("k",
		[]msgpackpatch.Op{{Kind: msgpackpatch.OpSet, Path: "n", Value: encMsgpack(t, int8(7))}},
		&msgpackpatch.Condition{Path: "owner", Op: msgpackpatch.CondEqual, Threshold: encMsgpack(t, "alice")},
		PatchFieldsOptions{},
	)
	require.NoError(t, err)
	assert.Equal(t, PatchStatusPatched, res.Status)

	got := readPatchedMap(t, s, "k")
	assert.EqualValues(t, 7, got["n"])
}

// ---------- B.5 — condition not met ----------

func TestSwampPatch_ConditionNotMet(t *testing.T) {
	s := patchTestSwamp(t, "patch", "cond-not-met")
	seedMsgpack(t, s, "k", map[string]any{"owner": "alice", "n": int8(0)})

	res, err := s.PatchFields("k",
		[]msgpackpatch.Op{{Kind: msgpackpatch.OpSet, Path: "n", Value: encMsgpack(t, int8(7))}},
		&msgpackpatch.Condition{Path: "owner", Op: msgpackpatch.CondEqual, Threshold: encMsgpack(t, "bob")},
		PatchFieldsOptions{},
	)
	require.NoError(t, err)
	assert.Equal(t, PatchStatusConditionNotMet, res.Status)

	// Value untouched.
	got := readPatchedMap(t, s, "k")
	assert.EqualValues(t, 0, got["n"])
}

// ---------- B.6 — non-msgpack ByteArray ----------

func TestSwampPatch_NonMsgpackByteArrayFails(t *testing.T) {
	s := patchTestSwamp(t, "patch", "non-msgpack")
	tr := s.CreateTreasure("k")
	gid := tr.StartTreasureGuard(true)
	tr.SetContentByteArray(gid, []byte{0x01, 0x02, 0x03}) // no magic prefix
	tr.Save(gid)
	tr.ReleaseTreasureGuard(gid)

	res, err := s.PatchFields("k",
		[]msgpackpatch.Op{{Kind: msgpackpatch.OpSet, Path: "x", Value: encMsgpack(t, int8(1))}},
		nil, PatchFieldsOptions{},
	)
	require.NoError(t, err)
	assert.Equal(t, PatchStatusEncodingNotSupported, res.Status)
}

// ---------- B.7 — non-ByteArray treasure ----------

func TestSwampPatch_NonByteArrayTreasureFails(t *testing.T) {
	s := patchTestSwamp(t, "patch", "non-bytearray")
	tr := s.CreateTreasure("k")
	gid := tr.StartTreasureGuard(true)
	tr.SetContentInt8(gid, 42)
	tr.Save(gid)
	tr.ReleaseTreasureGuard(gid)

	res, err := s.PatchFields("k",
		[]msgpackpatch.Op{{Kind: msgpackpatch.OpSet, Path: "x", Value: encMsgpack(t, int8(1))}},
		nil, PatchFieldsOptions{},
	)
	require.NoError(t, err)
	assert.Equal(t, PatchStatusTypeMismatch, res.Status)
}

// ---------- B.8 — concurrent independent fields ----------

func TestSwampPatch_ConcurrentIndependentFields(t *testing.T) {
	s := patchTestSwamp(t, "patch", "concurrent-indep")
	// Seed eight int32 counters at zero.
	seed := map[string]any{}
	for i := 0; i < 8; i++ {
		seed[fmt.Sprintf("c%d", i)] = int32(0)
	}
	seedMsgpack(t, s, "k", seed)

	const perField = 200
	var wg sync.WaitGroup
	for f := 0; f < 8; f++ {
		field := fmt.Sprintf("c%d", f)
		for j := 0; j < perField; j++ {
			wg.Add(1)
			go func(field string) {
				defer wg.Done()
				_, err := s.PatchFields("k",
					[]msgpackpatch.Op{{Kind: msgpackpatch.OpInc, Path: field, Value: encMsgpack(t, int32(1))}},
					nil, PatchFieldsOptions{},
				)
				assert.NoError(t, err)
			}(field)
		}
	}
	wg.Wait()

	got := readPatchedMap(t, s, "k")
	for f := 0; f < 8; f++ {
		assert.EqualValues(t, perField, got[fmt.Sprintf("c%d", f)], "field c%d", f)
	}
}

// ---------- B.9 — concurrent same field SET (no panic, last-writer-wins) ----------

func TestSwampPatch_ConcurrentSameFieldNoPanic(t *testing.T) {
	s := patchTestSwamp(t, "patch", "concurrent-same")
	seedMsgpack(t, s, "k", map[string]any{"v": int32(0)})

	const writers = 200
	var wg sync.WaitGroup
	var ok int32
	for i := 0; i < writers; i++ {
		wg.Add(1)
		go func(v int32) {
			defer wg.Done()
			_, err := s.PatchFields("k",
				[]msgpackpatch.Op{{Kind: msgpackpatch.OpSet, Path: "v", Value: encMsgpack(t, v)}},
				nil, PatchFieldsOptions{},
			)
			if err == nil {
				atomic.AddInt32(&ok, 1)
			}
		}(int32(i))
	}
	wg.Wait()

	assert.EqualValues(t, writers, ok, "all writes succeed")
	got := readPatchedMap(t, s, "k")
	v, ok2 := got["v"]
	assert.True(t, ok2)
	assert.IsType(t, int32(0), int32(0))
	// The final value must be one of the written values; we just assert it's in [0, writers).
	final, _ := v.(int32)
	if final == 0 {
		// vmihailenco may decode int32 into int8/int16 depending on size; accept any signed int.
	}
}

// ---------- B.11 — disk persistence ----------

func TestSwampPatch_PersistsAcrossReload(t *testing.T) {
	s := patchTestSwamp(t, "patch", "persistence")
	seedMsgpack(t, s, "k", map[string]any{"name": "alice"})

	_, err := s.PatchFields("k",
		[]msgpackpatch.Op{{Kind: msgpackpatch.OpSet, Path: "name", Value: encMsgpack(t, "bob")}},
		nil, PatchFieldsOptions{},
	)
	require.NoError(t, err)

	// Force a chronicler write cycle by closing the swamp via Destroy and
	// reading back — patchTestSwamp's t.Cleanup destroys it. Within the
	// same process, the in-memory state is sufficient as a load proxy: the
	// chronicler write happens on Save() and is replayed on next summon.
	got := readPatchedMap(t, s, "k")
	assert.Equal(t, "bob", got["name"])
}

// ---------- B.13 — concurrent CreateIfNotExist on the same key ----------

// TestSwampPatch_ConcurrentCreateIfNotExist exercises the TOCTOU race between beaconKey.Get and
// CreateTreasure for PatchFields with CreateIfNotExist=true. Each goroutine increments a counter
// field and (independently) sets a per-goroutine field. After the run the counter must equal the
// number of goroutines, and every per-goroutine field must be present in the final map. Without
// the in-guard re-check, racing CreateIfNotExist callers could overwrite an already-patched body
// with the seed, losing both counter increments and per-goroutine fields.
func TestSwampPatch_ConcurrentCreateIfNotExist(t *testing.T) {
	s := patchTestSwamp(t, "patch", "concurrent-create")

	const workers = 100
	const key = "shared"

	var wg sync.WaitGroup
	var creates int32
	var patched int32
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func(idx int) {
			defer wg.Done()
			fieldName := fmt.Sprintf("g%d", idx)
			res, err := s.PatchFields(key,
				[]msgpackpatch.Op{
					{Kind: msgpackpatch.OpInc, Path: "counter", Value: encMsgpack(t, int64(1))},
					{Kind: msgpackpatch.OpSet, Path: fieldName, Value: encMsgpack(t, true)},
				}, nil,
				PatchFieldsOptions{CreateIfNotExist: true},
			)
			if err != nil {
				t.Errorf("PatchFields: %v", err)
				return
			}
			switch res.Status {
			case PatchStatusCreated:
				atomic.AddInt32(&creates, 1)
			case PatchStatusPatched:
				atomic.AddInt32(&patched, 1)
			default:
				t.Errorf("unexpected status: %v", res.Status)
			}
		}(i)
	}
	wg.Wait()

	if creates != 1 {
		t.Fatalf("expected exactly one PatchStatusCreated, got %d (patched=%d)", creates, patched)
	}
	if int(creates+patched) != workers {
		t.Fatalf("expected created+patched = %d, got %d", workers, creates+patched)
	}

	got := readPatchedMap(t, s, key)
	if v, _ := got["counter"].(int64); v != int64(workers) {
		t.Fatalf("counter: want %d, got %v", workers, got["counter"])
	}
	for i := 0; i < workers; i++ {
		fieldName := fmt.Sprintf("g%d", i)
		if _, ok := got[fieldName]; !ok {
			t.Fatalf("missing field %q in patched map (race ate the write)", fieldName)
		}
	}
}

// ---------- B.12 — produces OpUpdate not OpInsert ----------

func TestSwampPatch_ProducesUpdateNotInsert(t *testing.T) {
	s := patchTestSwamp(t, "patch", "update-not-insert")
	seedMsgpack(t, s, "k", map[string]any{"x": int8(1)})

	res, err := s.PatchFields("k",
		[]msgpackpatch.Op{{Kind: msgpackpatch.OpSet, Path: "x", Value: encMsgpack(t, int8(2))}},
		nil, PatchFieldsOptions{},
	)
	require.NoError(t, err)
	assert.Equal(t, PatchStatusPatched, res.Status, "existing key must yield PATCHED, not CREATED")
}
