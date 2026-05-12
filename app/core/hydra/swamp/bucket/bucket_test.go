package bucket

import (
	"fmt"
	"sync"
	"testing"

	"github.com/hydraide/hydraide/app/core/hydra/swamp/bucket/valuecanon"
	"github.com/hydraide/hydraide/app/core/hydra/swamp/treasure/guard"
	"github.com/hydraide/hydraide/app/core/hydra/swamp/treasure"
	"github.com/vmihailenco/msgpack/v5"
)

// makeTreasure builds a Treasure whose body is the msgpack encoding of m.
// Tests pass `nil` to skip the body (used to exercise null-key paths).
func makeTreasure(t *testing.T, key string, body map[string]any) treasure.Treasure {
	t.Helper()
	tr := treasure.New(nil)
	g := tr.StartTreasureGuard(false, guard.BodyAuthID)
	defer tr.ReleaseTreasureGuard(g)
	tr.BodySetKey(g, key)
	if body == nil {
		tr.SetContentVoid(g)
		return tr
	}
	enc, err := msgpack.Marshal(body)
	if err != nil {
		t.Fatalf("msgpack marshal: %v", err)
	}
	tr.SetContentByteArray(g, enc)
	return tr
}

func snapshotFrom(treasures []treasure.Treasure) map[string]treasure.Treasure {
	m := map[string]treasure.Treasure{}
	for _, t := range treasures {
		m[t.GetKey()] = t
	}
	return m
}

// --- Build & lookup ---

func TestBucket_BuildEquality_Empty(t *testing.T) {
	b := New("asn")
	if err := b.BuildEquality(map[string]treasure.Treasure{}); err != nil {
		t.Fatalf("build: %v", err)
	}
	if !b.EqualityInitialized() {
		t.Fatalf("equality should be initialized")
	}
	if b.Count() != 0 {
		t.Fatalf("count: want 0, got %d", b.Count())
	}
	if got := b.LookupEqual(1); len(got) != 0 {
		t.Fatalf("lookup on empty bucket should be empty, got %d", len(got))
	}
}

func TestBucket_BuildEquality_SingleValue(t *testing.T) {
	b := New("asn")
	tr := makeTreasure(t, "k1", map[string]any{"asn": int64(10)})
	if err := b.BuildEquality(snapshotFrom([]treasure.Treasure{tr})); err != nil {
		t.Fatalf("build: %v", err)
	}
	got := b.LookupEqual(int64(10))
	if len(got) != 1 || got[0].GetKey() != "k1" {
		t.Fatalf("lookup: %+v", keys(got))
	}
}

func TestBucket_BuildEquality_MultipleValuesUnique(t *testing.T) {
	b := New("asn")
	trs := []treasure.Treasure{
		makeTreasure(t, "k1", map[string]any{"asn": int64(1)}),
		makeTreasure(t, "k2", map[string]any{"asn": int64(2)}),
		makeTreasure(t, "k3", map[string]any{"asn": int64(3)}),
	}
	_ = b.BuildEquality(snapshotFrom(trs))
	for _, v := range []int64{1, 2, 3} {
		if got := b.LookupEqual(v); len(got) != 1 {
			t.Errorf("asn=%d → %d hits, want 1", v, len(got))
		}
	}
	if b.Count() != 3 {
		t.Fatalf("count: want 3, got %d", b.Count())
	}
}

func TestBucket_BuildEquality_MultipleValuesDuplicate(t *testing.T) {
	b := New("asn")
	trs := []treasure.Treasure{
		makeTreasure(t, "a", map[string]any{"asn": int64(1)}),
		makeTreasure(t, "b", map[string]any{"asn": int64(1)}),
		makeTreasure(t, "c", map[string]any{"asn": int64(2)}),
		makeTreasure(t, "d", map[string]any{"asn": int64(2)}),
		makeTreasure(t, "e", map[string]any{"asn": int64(3)}),
	}
	_ = b.BuildEquality(snapshotFrom(trs))
	if got := b.LookupEqual(int64(1)); len(got) != 2 {
		t.Errorf("asn=1 → %d hits, want 2", len(got))
	}
	if got := b.LookupEqual(int64(2)); len(got) != 2 {
		t.Errorf("asn=2 → %d hits, want 2", len(got))
	}
	if got := b.LookupEqual(int64(3)); len(got) != 1 {
		t.Errorf("asn=3 → %d hits, want 1", len(got))
	}
}

func TestBucket_LookupEqual_NotFound(t *testing.T) {
	b := New("asn")
	tr := makeTreasure(t, "k", map[string]any{"asn": int64(1)})
	_ = b.BuildEquality(snapshotFrom([]treasure.Treasure{tr}))
	if got := b.LookupEqual(int64(999)); len(got) != 0 {
		t.Fatalf("not-found lookup: got %d hits", len(got))
	}
}

func TestBucket_LookupIn_Empty(t *testing.T) {
	b := New("asn")
	tr := makeTreasure(t, "k", map[string]any{"asn": int64(1)})
	_ = b.BuildEquality(snapshotFrom([]treasure.Treasure{tr}))
	if got := b.LookupIn(nil); len(got) != 0 {
		t.Fatalf("nil values: got %d", len(got))
	}
	if got := b.LookupIn([]any{}); len(got) != 0 {
		t.Fatalf("empty values: got %d", len(got))
	}
}

func TestBucket_LookupIn_AllFound(t *testing.T) {
	b := New("asn")
	trs := []treasure.Treasure{
		makeTreasure(t, "k1", map[string]any{"asn": int64(1)}),
		makeTreasure(t, "k2", map[string]any{"asn": int64(2)}),
		makeTreasure(t, "k3", map[string]any{"asn": int64(3)}),
	}
	_ = b.BuildEquality(snapshotFrom(trs))
	got := b.LookupIn([]any{int64(1), int64(3)})
	if len(got) != 2 {
		t.Fatalf("IN [1,3]: got %d, want 2", len(got))
	}
}

func TestBucket_LookupIn_PartiallyFound(t *testing.T) {
	b := New("asn")
	trs := []treasure.Treasure{
		makeTreasure(t, "k1", map[string]any{"asn": int64(1)}),
		makeTreasure(t, "k2", map[string]any{"asn": int64(2)}),
	}
	_ = b.BuildEquality(snapshotFrom(trs))
	got := b.LookupIn([]any{int64(1), int64(99), int64(2)})
	if len(got) != 2 {
		t.Fatalf("IN partial: got %d, want 2", len(got))
	}
}

func TestBucket_LookupIn_DeduplicatesByKey(t *testing.T) {
	// Two values that canonicalize to the same Key set should not return
	// the same treasure twice. Use cross-kind equality: int64(5) and
	// float64(5.0) collapse together in valuecanon.Equal.
	b := New("asn")
	tr := makeTreasure(t, "k", map[string]any{"asn": int64(5)})
	_ = b.BuildEquality(snapshotFrom([]treasure.Treasure{tr}))
	got := b.LookupIn([]any{int64(5), float64(5.0)})
	if len(got) != 1 {
		t.Fatalf("IN dedup: got %d, want 1", len(got))
	}
}

func TestBucket_CountForValue(t *testing.T) {
	b := New("asn")
	trs := []treasure.Treasure{
		makeTreasure(t, "k1", map[string]any{"asn": int64(1)}),
		makeTreasure(t, "k2", map[string]any{"asn": int64(1)}),
		makeTreasure(t, "k3", map[string]any{"asn": int64(2)}),
	}
	_ = b.BuildEquality(snapshotFrom(trs))
	if got := b.CountForValue(int64(1)); got != 2 {
		t.Errorf("count(1): %d, want 2", got)
	}
	if got := b.CountForValue(int64(2)); got != 1 {
		t.Errorf("count(2): %d, want 1", got)
	}
	if got := b.CountForValue(int64(99)); got != 0 {
		t.Errorf("count(missing): %d, want 0", got)
	}
}

// --- Field path ---

func TestBucket_FieldPath_TopLevel(t *testing.T) {
	b := New("asn")
	tr := makeTreasure(t, "k", map[string]any{"asn": int64(7), "other": "x"})
	_ = b.BuildEquality(snapshotFrom([]treasure.Treasure{tr}))
	if len(b.LookupEqual(int64(7))) != 1 {
		t.Fatalf("top-level miss")
	}
}

func TestBucket_FieldPath_Nested(t *testing.T) {
	b := New("metadata.asn")
	tr := makeTreasure(t, "k", map[string]any{
		"metadata": map[string]any{"asn": int64(7)},
	})
	_ = b.BuildEquality(snapshotFrom([]treasure.Treasure{tr}))
	if len(b.LookupEqual(int64(7))) != 1 {
		t.Fatalf("nested miss")
	}
}

func TestBucket_FieldPath_DeeplyNested(t *testing.T) {
	b := New("a.b.c.d")
	tr := makeTreasure(t, "k", map[string]any{
		"a": map[string]any{"b": map[string]any{"c": map[string]any{"d": int64(42)}}},
	})
	_ = b.BuildEquality(snapshotFrom([]treasure.Treasure{tr}))
	if len(b.LookupEqual(int64(42))) != 1 {
		t.Fatalf("deeply nested miss")
	}
}

func TestBucket_FieldPath_Missing(t *testing.T) {
	b := New("asn")
	tr := makeTreasure(t, "k", map[string]any{"other": "x"})
	_ = b.BuildEquality(snapshotFrom([]treasure.Treasure{tr}))
	// Missing field → KindNull bucket.
	if got := b.LookupEqual(nil); len(got) != 1 {
		t.Fatalf("missing-field lookup: got %d, want 1", len(got))
	}
}

func TestBucket_FieldPath_ExplicitNull(t *testing.T) {
	b := New("asn")
	tr := makeTreasure(t, "k", map[string]any{"asn": nil})
	_ = b.BuildEquality(snapshotFrom([]treasure.Treasure{tr}))
	if got := b.LookupEqual(nil); len(got) != 1 {
		t.Fatalf("explicit-nil lookup: got %d, want 1", len(got))
	}
}

func TestBucket_FieldPath_CaseSensitive(t *testing.T) {
	b := New("asn")
	tr := makeTreasure(t, "k", map[string]any{"ASN": int64(7)})
	_ = b.BuildEquality(snapshotFrom([]treasure.Treasure{tr}))
	if len(b.LookupEqual(int64(7))) != 0 {
		t.Fatalf("case-sensitivity violated")
	}
}

// --- Cross-kind equality through the bucket ---

func TestBucket_CrossKindEquality(t *testing.T) {
	b := New("n")
	// Body has uint64; lookup uses int64; valuecanon.Equal collapses them.
	tr := makeTreasure(t, "k", map[string]any{"n": uint64(5)})
	_ = b.BuildEquality(snapshotFrom([]treasure.Treasure{tr}))
	if got := b.LookupEqual(int64(5)); len(got) != 1 {
		t.Errorf("uint64(5) body, int64(5) lookup: got %d", len(got))
	}
	if got := b.LookupEqual(float64(5.0)); len(got) != 1 {
		t.Errorf("uint64(5) body, float64(5) lookup: got %d", len(got))
	}
}

// --- OnInsert ---

func TestBucket_OnInsert_NewKey_NewValue(t *testing.T) {
	b := New("asn")
	_ = b.BuildEquality(map[string]treasure.Treasure{})
	tr := makeTreasure(t, "k", map[string]any{"asn": int64(1)})
	_ = b.OnInsert(tr)
	if got := b.LookupEqual(int64(1)); len(got) != 1 {
		t.Fatalf("insert: got %d", len(got))
	}
	if b.Count() != 1 {
		t.Fatalf("count: %d", b.Count())
	}
}

func TestBucket_OnInsert_NewKey_ExistingValue(t *testing.T) {
	b := New("asn")
	_ = b.BuildEquality(snapshotFrom([]treasure.Treasure{
		makeTreasure(t, "a", map[string]any{"asn": int64(1)}),
	}))
	_ = b.OnInsert(makeTreasure(t, "b", map[string]any{"asn": int64(1)}))
	if got := b.LookupEqual(int64(1)); len(got) != 2 {
		t.Fatalf("insert into existing value: got %d, want 2", len(got))
	}
}

func TestBucket_OnInsert_NullValue(t *testing.T) {
	b := New("asn")
	_ = b.BuildEquality(map[string]treasure.Treasure{})
	_ = b.OnInsert(makeTreasure(t, "k", map[string]any{"other": "x"}))
	if got := b.LookupEqual(nil); len(got) != 1 {
		t.Fatalf("null-bucket insert: got %d", len(got))
	}
}

func TestBucket_OnInsert_NotInitialized(t *testing.T) {
	b := New("asn")
	// No BuildEquality.
	_ = b.OnInsert(makeTreasure(t, "k", map[string]any{"asn": int64(1)}))
	if b.Count() != 0 {
		t.Fatalf("uninitialized insert should be no-op, count=%d", b.Count())
	}
}

func TestBucket_OnInsert_BuildInFlight(t *testing.T) {
	b := New("asn")
	b.SetBuildInFlight(true)
	_ = b.OnInsert(makeTreasure(t, "k", map[string]any{"asn": int64(1)}))
	// Pending buffer holds it; equality maps untouched.
	if b.Count() != 0 {
		t.Fatalf("during build, insert should be pending, count=%d", b.Count())
	}
}

// --- OnUpdate ---

func TestBucket_OnUpdate_SameValue_NoOp(t *testing.T) {
	b := New("asn")
	tr := makeTreasure(t, "k", map[string]any{"asn": int64(1)})
	_ = b.BuildEquality(snapshotFrom([]treasure.Treasure{tr}))
	_ = b.OnUpdate(makeTreasure(t, "k", map[string]any{"asn": int64(1)}))
	if got := b.LookupEqual(int64(1)); len(got) != 1 {
		t.Fatalf("same-value update: got %d", len(got))
	}
}

func TestBucket_OnUpdate_DifferentValue_Move(t *testing.T) {
	b := New("asn")
	tr := makeTreasure(t, "k", map[string]any{"asn": int64(1)})
	_ = b.BuildEquality(snapshotFrom([]treasure.Treasure{tr}))
	_ = b.OnUpdate(makeTreasure(t, "k", map[string]any{"asn": int64(2)}))
	if got := b.LookupEqual(int64(1)); len(got) != 0 {
		t.Errorf("old value still present: %d", len(got))
	}
	if got := b.LookupEqual(int64(2)); len(got) != 1 {
		t.Errorf("new value missing: %d", len(got))
	}
	if b.Count() != 1 {
		t.Errorf("count drift: %d", b.Count())
	}
}

func TestBucket_OnUpdate_ValueToNull(t *testing.T) {
	b := New("asn")
	tr := makeTreasure(t, "k", map[string]any{"asn": int64(1)})
	_ = b.BuildEquality(snapshotFrom([]treasure.Treasure{tr}))
	_ = b.OnUpdate(makeTreasure(t, "k", map[string]any{"other": "x"}))
	if got := b.LookupEqual(int64(1)); len(got) != 0 {
		t.Errorf("old value still present after null move")
	}
	if got := b.LookupEqual(nil); len(got) != 1 {
		t.Errorf("null bucket missing entry")
	}
}

func TestBucket_OnUpdate_NullToValue(t *testing.T) {
	b := New("asn")
	tr := makeTreasure(t, "k", map[string]any{"other": "x"})
	_ = b.BuildEquality(snapshotFrom([]treasure.Treasure{tr}))
	_ = b.OnUpdate(makeTreasure(t, "k", map[string]any{"asn": int64(7)}))
	if got := b.LookupEqual(nil); len(got) != 0 {
		t.Errorf("null bucket still has entry")
	}
	if got := b.LookupEqual(int64(7)); len(got) != 1 {
		t.Errorf("new value missing")
	}
}

func TestBucket_OnUpdate_TypeChange_StringToInt(t *testing.T) {
	b := New("v")
	tr := makeTreasure(t, "k", map[string]any{"v": "5"})
	_ = b.BuildEquality(snapshotFrom([]treasure.Treasure{tr}))
	_ = b.OnUpdate(makeTreasure(t, "k", map[string]any{"v": int64(5)}))
	if got := b.LookupEqual("5"); len(got) != 0 {
		t.Errorf("string bucket still has entry")
	}
	if got := b.LookupEqual(int64(5)); len(got) != 1 {
		t.Errorf("int bucket missing")
	}
}

func TestBucket_OnUpdate_TypeChange_IntToFloat(t *testing.T) {
	b := New("v")
	tr := makeTreasure(t, "k", map[string]any{"v": int64(5)})
	_ = b.BuildEquality(snapshotFrom([]treasure.Treasure{tr}))
	_ = b.OnUpdate(makeTreasure(t, "k", map[string]any{"v": float64(5.5)}))
	if got := b.LookupEqual(int64(5)); len(got) != 0 {
		t.Errorf("int bucket still has entry")
	}
	if got := b.LookupEqual(float64(5.5)); len(got) != 1 {
		t.Errorf("float bucket missing")
	}
}

func TestBucket_OnUpdate_NotInByKey(t *testing.T) {
	b := New("asn")
	_ = b.BuildEquality(map[string]treasure.Treasure{})
	_ = b.OnUpdate(makeTreasure(t, "k", map[string]any{"asn": int64(9)}))
	if got := b.LookupEqual(int64(9)); len(got) != 1 {
		t.Fatalf("update-as-insert: got %d", len(got))
	}
}

func TestBucket_OnUpdate_BuildInFlight(t *testing.T) {
	b := New("asn")
	b.SetBuildInFlight(true)
	_ = b.OnUpdate(makeTreasure(t, "k", map[string]any{"asn": int64(1)}))
	if b.Count() != 0 {
		t.Fatalf("during build, update should be pending")
	}
}

// --- OnDelete ---

func TestBucket_OnDelete_Existing(t *testing.T) {
	b := New("asn")
	tr := makeTreasure(t, "k", map[string]any{"asn": int64(1)})
	_ = b.BuildEquality(snapshotFrom([]treasure.Treasure{tr}))
	b.OnDelete("k")
	if got := b.LookupEqual(int64(1)); len(got) != 0 {
		t.Fatalf("delete failed")
	}
	if b.Count() != 0 {
		t.Fatalf("count: %d", b.Count())
	}
}

func TestBucket_OnDelete_NonExisting(t *testing.T) {
	b := New("asn")
	_ = b.BuildEquality(map[string]treasure.Treasure{})
	b.OnDelete("missing") // must not panic
}

func TestBucket_OnDelete_LastInValueBucket(t *testing.T) {
	b := New("asn")
	tr := makeTreasure(t, "k", map[string]any{"asn": int64(1)})
	_ = b.BuildEquality(snapshotFrom([]treasure.Treasure{tr}))
	b.OnDelete("k")
	// Re-add the same value; if the value-slot was left around as an
	// empty map, this should still work. If it was deleted, ditto.
	_ = b.OnInsert(makeTreasure(t, "k2", map[string]any{"asn": int64(1)}))
	if got := b.LookupEqual(int64(1)); len(got) != 1 {
		t.Fatalf("re-insert after last-delete: %d", len(got))
	}
}

func TestBucket_OnDelete_BuildInFlight(t *testing.T) {
	b := New("asn")
	tr := makeTreasure(t, "k", map[string]any{"asn": int64(1)})
	_ = b.BuildEquality(snapshotFrom([]treasure.Treasure{tr}))
	b.SetBuildInFlight(true)
	b.OnDelete("k")
	if b.Count() != 1 {
		t.Fatalf("during build, delete should be pending, count=%d", b.Count())
	}
}

// --- Pending drain ---

func TestBucket_PendingDrain_AllInsert(t *testing.T) {
	b := New("asn")
	_ = b.BuildEquality(map[string]treasure.Treasure{})
	b.SetBuildInFlight(true)
	for i := 0; i < 100; i++ {
		_ = b.OnInsert(makeTreasure(t, fmt.Sprintf("k%d", i), map[string]any{"asn": int64(i % 10)}))
	}
	if b.Count() != 0 {
		t.Fatalf("count during build should stay 0")
	}
	if err := b.DrainPending(); err != nil {
		t.Fatalf("drain: %v", err)
	}
	b.SetBuildInFlight(false)
	if b.Count() != 100 {
		t.Fatalf("post-drain count: %d", b.Count())
	}
	if b.CountForValue(int64(0)) != 10 {
		t.Fatalf("post-drain per-value count: %d", b.CountForValue(int64(0)))
	}
}

func TestBucket_PendingDrain_InsertThenUpdate(t *testing.T) {
	b := New("asn")
	_ = b.BuildEquality(map[string]treasure.Treasure{})
	b.SetBuildInFlight(true)
	_ = b.OnInsert(makeTreasure(t, "k", map[string]any{"asn": int64(1)}))
	_ = b.OnUpdate(makeTreasure(t, "k", map[string]any{"asn": int64(2)}))
	_ = b.DrainPending()
	b.SetBuildInFlight(false)
	if len(b.LookupEqual(int64(1))) != 0 || len(b.LookupEqual(int64(2))) != 1 {
		t.Fatalf("FIFO drain wrong: %d/%d", len(b.LookupEqual(int64(1))), len(b.LookupEqual(int64(2))))
	}
}

func TestBucket_PendingDrain_InsertThenDelete(t *testing.T) {
	b := New("asn")
	_ = b.BuildEquality(map[string]treasure.Treasure{})
	b.SetBuildInFlight(true)
	_ = b.OnInsert(makeTreasure(t, "k", map[string]any{"asn": int64(1)}))
	b.OnDelete("k")
	_ = b.DrainPending()
	b.SetBuildInFlight(false)
	if b.Count() != 0 {
		t.Fatalf("insert+delete drain count: %d", b.Count())
	}
}

func TestBucket_PendingDrain_UpdateNonExisting(t *testing.T) {
	b := New("asn")
	_ = b.BuildEquality(map[string]treasure.Treasure{})
	b.SetBuildInFlight(true)
	_ = b.OnUpdate(makeTreasure(t, "k", map[string]any{"asn": int64(7)}))
	_ = b.DrainPending()
	b.SetBuildInFlight(false)
	if len(b.LookupEqual(int64(7))) != 1 {
		t.Fatalf("update-as-insert during build failed")
	}
}

func TestBucket_PendingDrain_FIFOOrder(t *testing.T) {
	b := New("asn")
	_ = b.BuildEquality(map[string]treasure.Treasure{})
	b.SetBuildInFlight(true)
	_ = b.OnInsert(makeTreasure(t, "k", map[string]any{"asn": int64(1)}))
	_ = b.OnUpdate(makeTreasure(t, "k", map[string]any{"asn": int64(2)}))
	_ = b.OnUpdate(makeTreasure(t, "k", map[string]any{"asn": int64(3)}))
	_ = b.DrainPending()
	b.SetBuildInFlight(false)
	if len(b.LookupEqual(int64(3))) != 1 {
		t.Fatalf("FIFO end-state: not 3")
	}
}

// --- Reset / lifecycle ---

func TestBucket_Reset_AfterBuild(t *testing.T) {
	b := New("asn")
	_ = b.BuildEquality(snapshotFrom([]treasure.Treasure{
		makeTreasure(t, "k", map[string]any{"asn": int64(1)}),
	}))
	b.Reset()
	if b.EqualityInitialized() {
		t.Errorf("equality init should be cleared")
	}
	if b.Count() != 0 {
		t.Errorf("count should be 0 after reset")
	}
}

func TestBucket_Count(t *testing.T) {
	b := New("asn")
	trs := make([]treasure.Treasure, 0, 50)
	for i := 0; i < 50; i++ {
		trs = append(trs, makeTreasure(t, fmt.Sprintf("k%d", i), map[string]any{"asn": int64(i)}))
	}
	_ = b.BuildEquality(snapshotFrom(trs))
	if b.Count() != 50 {
		t.Fatalf("count: %d, want 50", b.Count())
	}
}

func TestBucket_RangeViewStub_v1(t *testing.T) {
	b := New("score")
	_ = b.BuildEquality(map[string]treasure.Treasure{})
	if b.RangeInitialized() {
		t.Errorf("range view should NOT be initialized in v1")
	}
	if got := b.LookupRange(RangeOpGT, 100); len(got) != 0 {
		t.Errorf("range stub should return empty slice")
	}
	if got := b.LookupBetween(10, 50, true, true); len(got) != 0 {
		t.Errorf("between stub should return empty slice")
	}
	if err := b.BuildRange(map[string]treasure.Treasure{}); err != nil {
		t.Errorf("build-range stub should be a no-op")
	}
}

// --- Concurrency smoke ---

func TestBucket_ConcurrentLookupAndMutation(t *testing.T) {
	b := New("asn")
	trs := make([]treasure.Treasure, 0, 100)
	for i := 0; i < 100; i++ {
		trs = append(trs, makeTreasure(t, fmt.Sprintf("k%d", i), map[string]any{"asn": int64(i)}))
	}
	_ = b.BuildEquality(snapshotFrom(trs))

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := 0; i < 500; i++ {
			_ = b.LookupEqual(int64(i % 100))
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 500; i++ {
			_ = b.OnUpdate(makeTreasure(t, fmt.Sprintf("k%d", i%100), map[string]any{"asn": int64((i + 1) % 100)}))
		}
	}()
	wg.Wait()
}

func TestBucket_BuildOnceRaceProtection(t *testing.T) {
	// Two concurrent BuildEquality calls should not double-populate.
	b := New("asn")
	trs := []treasure.Treasure{
		makeTreasure(t, "a", map[string]any{"asn": int64(1)}),
		makeTreasure(t, "b", map[string]any{"asn": int64(2)}),
	}
	snap := snapshotFrom(trs)
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = b.BuildEquality(snap)
		}()
	}
	wg.Wait()
	if b.Count() != 2 {
		t.Fatalf("buildOnce failed: count=%d", b.Count())
	}
}

// helpers

func keys(ts []treasure.Treasure) []string {
	out := make([]string, len(ts))
	for i, t := range ts {
		out[i] = t.GetKey()
	}
	return out
}

// guard against accidental KindNull collisions with valuecanon.NullKey in
// future refactors.
var _ = valuecanon.NullKey
