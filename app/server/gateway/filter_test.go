package gateway

import (
	"math"
	"testing"
	"time"

	"github.com/hydraide/hydraide/app/core/hydra/swamp/treasure"
	"github.com/hydraide/hydraide/app/core/hydra/swamp/treasure/guard"
	hydrapb "github.com/hydraide/hydraide/generated/hydraidepbgo"
	"github.com/vmihailenco/msgpack/v5"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func timestamppbNew(t time.Time) *timestamppb.Timestamp {
	return timestamppb.New(t)
}

// --- Test helpers ---

func makeMsgpackBytesVal(t *testing.T, data map[string]interface{}) []byte {
	t.Helper()
	encoded, err := msgpack.Marshal(data)
	if err != nil {
		t.Fatalf("msgpack.Marshal failed: %v", err)
	}
	return append([]byte{msgpackMagic0, msgpackMagic1}, encoded...)
}

func noopSave(t treasure.Treasure, guardID guard.ID) treasure.TreasureStatus {
	return treasure.StatusVoid
}

func newTreasureWithInt32(val int32) treasure.Treasure {
	tr := treasure.New(noopSave)
	gid := tr.StartTreasureGuard(true)
	tr.SetContentInt32(gid, val)
	tr.ReleaseTreasureGuard(gid)
	return tr
}

func newTreasureWithInt64(val int64) treasure.Treasure {
	tr := treasure.New(noopSave)
	gid := tr.StartTreasureGuard(true)
	tr.SetContentInt64(gid, val)
	tr.ReleaseTreasureGuard(gid)
	return tr
}

func newTreasureWithString(val string) treasure.Treasure {
	tr := treasure.New(noopSave)
	gid := tr.StartTreasureGuard(true)
	tr.SetContentString(gid, val)
	tr.ReleaseTreasureGuard(gid)
	return tr
}

func newTreasureWithBool(val bool) treasure.Treasure {
	tr := treasure.New(noopSave)
	gid := tr.StartTreasureGuard(true)
	tr.SetContentBool(gid, val)
	tr.ReleaseTreasureGuard(gid)
	return tr
}

func newTreasureWithBytes(val []byte) treasure.Treasure {
	tr := treasure.New(noopSave)
	gid := tr.StartTreasureGuard(true)
	tr.SetContentByteArray(gid, val)
	tr.ReleaseTreasureGuard(gid)
	return tr
}

func newTreasureWithCreatedAt(ts time.Time) treasure.Treasure {
	tr := treasure.New(noopSave)
	gid := tr.StartTreasureGuard(true)
	tr.SetCreatedAt(gid, ts)
	tr.ReleaseTreasureGuard(gid)
	return tr
}

func newTreasureWithModifiedAt(ts time.Time) treasure.Treasure {
	tr := treasure.New(noopSave)
	gid := tr.StartTreasureGuard(true)
	tr.SetModifiedAt(gid, ts)
	tr.ReleaseTreasureGuard(gid)
	return tr
}

func newTreasureWithExpiration(ts time.Time) treasure.Treasure {
	tr := treasure.New(noopSave)
	gid := tr.StartTreasureGuard(true)
	tr.SetExpirationTime(gid, ts)
	tr.ReleaseTreasureGuard(gid)
	return tr
}

func newTreasureWithInt32AndBytes(intVal int32, bytesVal []byte) treasure.Treasure {
	tr := treasure.New(noopSave)
	gid := tr.StartTreasureGuard(true)
	tr.SetContentInt32(gid, intVal)
	tr.ReleaseTreasureGuard(gid)
	// Int32 and ByteArray are in the same Content struct, only one can be set.
	// For combined tests we need bytes — the int32 filter won't work on bytes treasures.
	// This helper is actually for testing filter groups where we need both.
	// Since a treasure can only hold one content type, we'll use bytes and test int32 via separate treasures.
	return tr
}

func newEmptyTreasure() treasure.Treasure {
	return treasure.New(noopSave)
}

func stringPtr(s string) *string { return &s }

// --- Timestamp Filter Tests ---

func TestNativeTimestampFilter_CreatedAt(t *testing.T) {
	now := time.Now()
	cutoff := now.Add(-24 * time.Hour)

	tr := newTreasureWithCreatedAt(now)

	filter := &hydrapb.TreasureFilter{
		Operator: hydrapb.Relational_GREATER_THAN,
		CompareValue: &hydrapb.TreasureFilter_CreatedAtVal{
			CreatedAtVal: timestamppbNew(cutoff),
		},
	}

	if !evaluateNativeSingleFilter(tr, filter) {
		t.Error("expected CreatedAt filter to match (now > 24h ago)")
	}

	filter.Operator = hydrapb.Relational_LESS_THAN
	if evaluateNativeSingleFilter(tr, filter) {
		t.Error("expected CreatedAt filter NOT to match (now < 24h ago)")
	}
}

func TestNativeTimestampFilter_UpdatedAt(t *testing.T) {
	ts := time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC)
	tr := newTreasureWithModifiedAt(ts)

	filter := &hydrapb.TreasureFilter{
		Operator: hydrapb.Relational_EQUAL,
		CompareValue: &hydrapb.TreasureFilter_UpdatedAtVal{
			UpdatedAtVal: timestamppbNew(ts),
		},
	}

	if !evaluateNativeSingleFilter(tr, filter) {
		t.Error("expected UpdatedAt filter to match (same timestamp)")
	}
}

func TestNativeTimestampFilter_ExpiredAt(t *testing.T) {
	past := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	now := time.Now()

	tr := newTreasureWithExpiration(past)

	filter := &hydrapb.TreasureFilter{
		Operator: hydrapb.Relational_LESS_THAN,
		CompareValue: &hydrapb.TreasureFilter_ExpiredAtVal{
			ExpiredAtVal: timestamppbNew(now),
		},
	}

	if !evaluateNativeSingleFilter(tr, filter) {
		t.Error("expected ExpiredAt filter to match (past < now)")
	}
}

func TestNativeTimestampFilter_IsEmpty(t *testing.T) {
	tr := newTreasureWithCreatedAt(time.Now())

	// ExpiredAt IS_EMPTY (no expiration set)
	filter := &hydrapb.TreasureFilter{
		Operator: hydrapb.Relational_IS_EMPTY,
		CompareValue: &hydrapb.TreasureFilter_ExpiredAtVal{
			ExpiredAtVal: timestamppbNew(time.Now()),
		},
	}

	if !evaluateNativeSingleFilter(tr, filter) {
		t.Error("expected IS_EMPTY to be true for unset ExpiredAt")
	}

	// CreatedAt IS_NOT_EMPTY
	filter2 := &hydrapb.TreasureFilter{
		Operator: hydrapb.Relational_IS_NOT_EMPTY,
		CompareValue: &hydrapb.TreasureFilter_CreatedAtVal{
			CreatedAtVal: timestamppbNew(time.Now()),
		},
	}

	if !evaluateNativeSingleFilter(tr, filter2) {
		t.Error("expected IS_NOT_EMPTY to be true for set CreatedAt")
	}
}

func TestNativeTimestampFilter_NilTreasureTimestamp(t *testing.T) {
	tr := newEmptyTreasure()

	filter := &hydrapb.TreasureFilter{
		Operator: hydrapb.Relational_GREATER_THAN,
		CompareValue: &hydrapb.TreasureFilter_CreatedAtVal{
			CreatedAtVal: timestamppbNew(time.Now()),
		},
	}

	if evaluateNativeSingleFilter(tr, filter) {
		t.Error("expected filter to NOT match when treasure CreatedAt is 0")
	}
}

// --- HAS_KEY / HAS_NOT_KEY Tests ---

func TestNativeHasKey_KeyExists(t *testing.T) {
	bytesVal := makeMsgpackBytesVal(t, map[string]interface{}{
		"UserData": map[string]interface{}{
			"email": "user@example.com",
			"name":  "John",
		},
	})
	tr := newTreasureWithBytes(bytesVal)

	fieldPath := "UserData"
	filter := &hydrapb.TreasureFilter{
		Operator:       hydrapb.Relational_HAS_KEY,
		CompareValue:   &hydrapb.TreasureFilter_StringVal{StringVal: "email"},
		BytesFieldPath: &fieldPath,
	}

	if !evaluateNativeSingleFilter(tr, filter) {
		t.Error("expected HAS_KEY to match for existing key 'email'")
	}
}

func TestNativeHasKey_KeyNotExists(t *testing.T) {
	bytesVal := makeMsgpackBytesVal(t, map[string]interface{}{
		"UserData": map[string]interface{}{
			"name": "John",
		},
	})
	tr := newTreasureWithBytes(bytesVal)

	fieldPath := "UserData"
	filter := &hydrapb.TreasureFilter{
		Operator:       hydrapb.Relational_HAS_KEY,
		CompareValue:   &hydrapb.TreasureFilter_StringVal{StringVal: "email"},
		BytesFieldPath: &fieldPath,
	}

	if evaluateNativeSingleFilter(tr, filter) {
		t.Error("expected HAS_KEY NOT to match for non-existing key 'email'")
	}
}

func TestNativeHasNotKey_KeyExists(t *testing.T) {
	bytesVal := makeMsgpackBytesVal(t, map[string]interface{}{
		"UserData": map[string]interface{}{
			"email": "user@example.com",
		},
	})
	tr := newTreasureWithBytes(bytesVal)

	fieldPath := "UserData"
	filter := &hydrapb.TreasureFilter{
		Operator:       hydrapb.Relational_HAS_NOT_KEY,
		CompareValue:   &hydrapb.TreasureFilter_StringVal{StringVal: "email"},
		BytesFieldPath: &fieldPath,
	}

	if evaluateNativeSingleFilter(tr, filter) {
		t.Error("expected HAS_NOT_KEY NOT to match for existing key 'email'")
	}
}

func TestNativeHasNotKey_KeyNotExists(t *testing.T) {
	bytesVal := makeMsgpackBytesVal(t, map[string]interface{}{
		"UserData": map[string]interface{}{
			"name": "John",
		},
	})
	tr := newTreasureWithBytes(bytesVal)

	fieldPath := "UserData"
	filter := &hydrapb.TreasureFilter{
		Operator:       hydrapb.Relational_HAS_NOT_KEY,
		CompareValue:   &hydrapb.TreasureFilter_StringVal{StringVal: "email"},
		BytesFieldPath: &fieldPath,
	}

	if !evaluateNativeSingleFilter(tr, filter) {
		t.Error("expected HAS_NOT_KEY to match for non-existing key 'email'")
	}
}

func TestNativeHasKey_NotAMap(t *testing.T) {
	bytesVal := makeMsgpackBytesVal(t, map[string]interface{}{
		"UserData": "not-a-map",
	})
	tr := newTreasureWithBytes(bytesVal)

	fieldPath := "UserData"
	filter := &hydrapb.TreasureFilter{
		Operator:       hydrapb.Relational_HAS_KEY,
		CompareValue:   &hydrapb.TreasureFilter_StringVal{StringVal: "email"},
		BytesFieldPath: &fieldPath,
	}

	if evaluateNativeSingleFilter(tr, filter) {
		t.Error("expected HAS_KEY NOT to match when field is not a map")
	}
}

func TestNativeHasKey_NilBytesVal(t *testing.T) {
	tr := newEmptyTreasure()

	fieldPath := "UserData"
	filter := &hydrapb.TreasureFilter{
		Operator:       hydrapb.Relational_HAS_KEY,
		CompareValue:   &hydrapb.TreasureFilter_StringVal{StringVal: "email"},
		BytesFieldPath: &fieldPath,
	}

	if evaluateNativeSingleFilter(tr, filter) {
		t.Error("expected HAS_KEY NOT to match when content is empty")
	}
}

func TestNativeHasNotKey_NilBytesVal(t *testing.T) {
	tr := newEmptyTreasure()

	fieldPath := "UserData"
	filter := &hydrapb.TreasureFilter{
		Operator:       hydrapb.Relational_HAS_NOT_KEY,
		CompareValue:   &hydrapb.TreasureFilter_StringVal{StringVal: "email"},
		BytesFieldPath: &fieldPath,
	}

	if evaluateNativeSingleFilter(tr, filter) {
		t.Error("expected HAS_NOT_KEY NOT to match when content is empty (no data to inspect)")
	}
}

// --- PhraseFilter Tests ---

func TestNativePhraseFilter_ConsecutiveWords(t *testing.T) {
	bytesVal := makeMsgpackBytesVal(t, map[string]interface{}{
		"WordIndex": map[string]interface{}{
			"altalanos":  []interface{}{int64(5), int64(20)},
			"szerzodesi": []interface{}{int64(6), int64(30)},
			"feltetelek": []interface{}{int64(7), int64(40)},
		},
	})
	tr := newTreasureWithBytes(bytesVal)

	pf := &hydrapb.PhraseFilter{
		BytesFieldPath: "WordIndex",
		Words:          []string{"altalanos", "szerzodesi", "feltetelek"},
		Negate:         false,
	}

	if !evaluateNativePhraseFilter(tr, pf) {
		t.Error("expected phrase filter to match consecutive positions 5,6,7")
	}
}

func TestNativePhraseFilter_NonConsecutiveWords(t *testing.T) {
	bytesVal := makeMsgpackBytesVal(t, map[string]interface{}{
		"WordIndex": map[string]interface{}{
			"altalanos":  []interface{}{int64(5), int64(20)},
			"szerzodesi": []interface{}{int64(8), int64(30)},
			"feltetelek": []interface{}{int64(12), int64(40)},
		},
	})
	tr := newTreasureWithBytes(bytesVal)

	pf := &hydrapb.PhraseFilter{
		BytesFieldPath: "WordIndex",
		Words:          []string{"altalanos", "szerzodesi", "feltetelek"},
		Negate:         false,
	}

	if evaluateNativePhraseFilter(tr, pf) {
		t.Error("expected phrase filter NOT to match non-consecutive positions")
	}
}

func TestNativePhraseFilter_Negated(t *testing.T) {
	bytesVal := makeMsgpackBytesVal(t, map[string]interface{}{
		"WordIndex": map[string]interface{}{
			"hello": []interface{}{int64(1)},
			"world": []interface{}{int64(2)},
		},
	})
	tr := newTreasureWithBytes(bytesVal)

	pf := &hydrapb.PhraseFilter{
		BytesFieldPath: "WordIndex",
		Words:          []string{"hello", "world"},
		Negate:         true,
	}

	if evaluateNativePhraseFilter(tr, pf) {
		t.Error("expected negated phrase filter NOT to match when phrase is found")
	}
}

func TestNativePhraseFilter_NegatedNoMatch(t *testing.T) {
	bytesVal := makeMsgpackBytesVal(t, map[string]interface{}{
		"WordIndex": map[string]interface{}{
			"hello": []interface{}{int64(1)},
			"world": []interface{}{int64(5)},
		},
	})
	tr := newTreasureWithBytes(bytesVal)

	pf := &hydrapb.PhraseFilter{
		BytesFieldPath: "WordIndex",
		Words:          []string{"hello", "world"},
		Negate:         true,
	}

	if !evaluateNativePhraseFilter(tr, pf) {
		t.Error("expected negated phrase filter to match when phrase is NOT found")
	}
}

func TestNativePhraseFilter_SingleWord(t *testing.T) {
	bytesVal := makeMsgpackBytesVal(t, map[string]interface{}{
		"WordIndex": map[string]interface{}{
			"hello": []interface{}{int64(1), int64(5), int64(10)},
		},
	})
	tr := newTreasureWithBytes(bytesVal)

	pf := &hydrapb.PhraseFilter{
		BytesFieldPath: "WordIndex",
		Words:          []string{"hello"},
		Negate:         false,
	}

	if !evaluateNativePhraseFilter(tr, pf) {
		t.Error("expected single-word phrase filter to match")
	}
}

func TestNativePhraseFilter_MissingWord(t *testing.T) {
	bytesVal := makeMsgpackBytesVal(t, map[string]interface{}{
		"WordIndex": map[string]interface{}{
			"hello": []interface{}{int64(1)},
		},
	})
	tr := newTreasureWithBytes(bytesVal)

	pf := &hydrapb.PhraseFilter{
		BytesFieldPath: "WordIndex",
		Words:          []string{"hello", "missing"},
		Negate:         false,
	}

	if evaluateNativePhraseFilter(tr, pf) {
		t.Error("expected phrase filter NOT to match when a word is missing from index")
	}
}

func TestNativePhraseFilter_MultipleOccurrences(t *testing.T) {
	bytesVal := makeMsgpackBytesVal(t, map[string]interface{}{
		"WordIndex": map[string]interface{}{
			"hello": []interface{}{int64(1), int64(3), int64(10)},
			"world": []interface{}{int64(4), int64(8), int64(15)},
		},
	})
	tr := newTreasureWithBytes(bytesVal)

	pf := &hydrapb.PhraseFilter{
		BytesFieldPath: "WordIndex",
		Words:          []string{"hello", "world"},
		Negate:         false,
	}

	if !evaluateNativePhraseFilter(tr, pf) {
		t.Error("expected phrase filter to find consecutive occurrence at positions 3,4")
	}
}

func TestNativePhraseFilter_EmptyPositions(t *testing.T) {
	bytesVal := makeMsgpackBytesVal(t, map[string]interface{}{
		"WordIndex": map[string]interface{}{
			"hello": []interface{}{},
			"world": []interface{}{int64(1)},
		},
	})
	tr := newTreasureWithBytes(bytesVal)

	pf := &hydrapb.PhraseFilter{
		BytesFieldPath: "WordIndex",
		Words:          []string{"hello", "world"},
		Negate:         false,
	}

	if evaluateNativePhraseFilter(tr, pf) {
		t.Error("expected phrase filter NOT to match when a word has empty position list")
	}
}

func TestNativePhraseFilter_EmptyWords(t *testing.T) {
	bytesVal := makeMsgpackBytesVal(t, map[string]interface{}{"WordIndex": map[string]interface{}{}})
	tr := newTreasureWithBytes(bytesVal)

	pf := &hydrapb.PhraseFilter{
		BytesFieldPath: "WordIndex",
		Words:          []string{},
		Negate:         false,
	}

	if !evaluateNativePhraseFilter(tr, pf) {
		t.Error("expected empty words phrase filter to pass (vacuously true)")
	}
}

func TestNativePhraseFilter_NilBytesVal(t *testing.T) {
	tr := newEmptyTreasure()

	pf := &hydrapb.PhraseFilter{
		BytesFieldPath: "WordIndex",
		Words:          []string{"hello"},
		Negate:         false,
	}

	if evaluateNativePhraseFilter(tr, pf) {
		t.Error("expected phrase filter NOT to match when content is empty")
	}
}

func TestNativePhraseFilter_NilBytesValNegated(t *testing.T) {
	tr := newEmptyTreasure()

	pf := &hydrapb.PhraseFilter{
		BytesFieldPath: "WordIndex",
		Words:          []string{"hello"},
		Negate:         true,
	}

	if !evaluateNativePhraseFilter(tr, pf) {
		t.Error("expected negated phrase filter to match when content is empty (phrase can't exist)")
	}
}

// --- FilterGroup integration tests ---

func TestNativePhraseFilter_InFilterGroup_AND(t *testing.T) {
	bytesVal := makeMsgpackBytesVal(t, map[string]interface{}{
		"WordIndex": map[string]interface{}{
			"hello": []interface{}{int64(1)},
			"world": []interface{}{int64(2)},
		},
		"Score": int64(42),
	})
	tr := newTreasureWithBytes(bytesVal)

	scorePath := "Score"
	group := &hydrapb.FilterGroup{
		Logic: hydrapb.FilterLogic_AND,
		Filters: []*hydrapb.TreasureFilter{
			{
				Operator:       hydrapb.Relational_EQUAL,
				CompareValue:   &hydrapb.TreasureFilter_Int64Val{Int64Val: 42},
				BytesFieldPath: &scorePath,
			},
		},
		PhraseFilters: []*hydrapb.PhraseFilter{
			{
				BytesFieldPath: "WordIndex",
				Words:          []string{"hello", "world"},
			},
		},
	}

	if !evaluateNativeFilterGroup(tr, group) {
		t.Error("expected AND group to match (Score==42 AND phrase found)")
	}

	// Change score filter to not match
	group.Filters[0].CompareValue = &hydrapb.TreasureFilter_Int64Val{Int64Val: 99}
	if evaluateNativeFilterGroup(tr, group) {
		t.Error("expected AND group NOT to match (Score!=42)")
	}
}

func TestNativePhraseFilter_InFilterGroup_OR(t *testing.T) {
	bytesVal := makeMsgpackBytesVal(t, map[string]interface{}{
		"WordIndex": map[string]interface{}{
			"hello": []interface{}{int64(1)},
			"world": []interface{}{int64(2)},
		},
		"Score": int64(99),
	})
	tr := newTreasureWithBytes(bytesVal)

	scorePath := "Score"
	group := &hydrapb.FilterGroup{
		Logic: hydrapb.FilterLogic_OR,
		Filters: []*hydrapb.TreasureFilter{
			{
				Operator:       hydrapb.Relational_EQUAL,
				CompareValue:   &hydrapb.TreasureFilter_Int64Val{Int64Val: 42},
				BytesFieldPath: &scorePath,
			},
		},
		PhraseFilters: []*hydrapb.PhraseFilter{
			{
				BytesFieldPath: "WordIndex",
				Words:          []string{"hello", "world"},
			},
		},
	}

	if !evaluateNativeFilterGroup(tr, group) {
		t.Error("expected OR group to match (Score!=42 but phrase found)")
	}
}

// --- Profile FilterGroup Tests ---

func TestNativeProfileFilterGroup_AND_AllMatch(t *testing.T) {
	treasures := map[string]treasure.Treasure{
		"Age":    newTreasureWithInt32(30),
		"Name":   newTreasureWithString("Alice"),
		"Active": newTreasureWithBool(true),
	}

	ageKey := "Age"
	nameKey := "Name"
	activeKey := "Active"

	group := &hydrapb.FilterGroup{
		Logic: hydrapb.FilterLogic_AND,
		Filters: []*hydrapb.TreasureFilter{
			{Operator: hydrapb.Relational_GREATER_THAN, CompareValue: &hydrapb.TreasureFilter_Int32Val{Int32Val: 18}, TreasureKey: &ageKey},
			{Operator: hydrapb.Relational_EQUAL, CompareValue: &hydrapb.TreasureFilter_StringVal{StringVal: "Alice"}, TreasureKey: &nameKey},
			{Operator: hydrapb.Relational_EQUAL, CompareValue: &hydrapb.TreasureFilter_BoolVal{BoolVal: hydrapb.Boolean_TRUE}, TreasureKey: &activeKey},
		},
	}

	if !evaluateNativeProfileFilterGroup(treasures, group) {
		t.Error("expected AND group to match when all filters pass")
	}
}

func TestNativeProfileFilterGroup_AND_OneFails(t *testing.T) {
	treasures := map[string]treasure.Treasure{
		"Age":  newTreasureWithInt32(15),
		"Name": newTreasureWithString("Bob"),
	}

	ageKey := "Age"
	nameKey := "Name"

	group := &hydrapb.FilterGroup{
		Logic: hydrapb.FilterLogic_AND,
		Filters: []*hydrapb.TreasureFilter{
			{Operator: hydrapb.Relational_GREATER_THAN, CompareValue: &hydrapb.TreasureFilter_Int32Val{Int32Val: 18}, TreasureKey: &ageKey},
			{Operator: hydrapb.Relational_EQUAL, CompareValue: &hydrapb.TreasureFilter_StringVal{StringVal: "Bob"}, TreasureKey: &nameKey},
		},
	}

	if evaluateNativeProfileFilterGroup(treasures, group) {
		t.Error("expected AND group to fail when Age < 18")
	}
}

func TestNativeProfileFilterGroup_OR_OneMatch(t *testing.T) {
	treasures := map[string]treasure.Treasure{
		"Status": newTreasureWithString("inactive"),
		"Age":    newTreasureWithInt32(25),
	}

	statusKey := "Status"
	ageKey := "Age"

	group := &hydrapb.FilterGroup{
		Logic: hydrapb.FilterLogic_OR,
		Filters: []*hydrapb.TreasureFilter{
			{Operator: hydrapb.Relational_EQUAL, CompareValue: &hydrapb.TreasureFilter_StringVal{StringVal: "active"}, TreasureKey: &statusKey},
			{Operator: hydrapb.Relational_GREATER_THAN, CompareValue: &hydrapb.TreasureFilter_Int32Val{Int32Val: 20}, TreasureKey: &ageKey},
		},
	}

	if !evaluateNativeProfileFilterGroup(treasures, group) {
		t.Error("expected OR group to match because Age > 20")
	}
}

func TestNativeProfileFilterGroup_OR_NoneMatch(t *testing.T) {
	treasures := map[string]treasure.Treasure{
		"Status": newTreasureWithString("inactive"),
		"Age":    newTreasureWithInt32(10),
	}

	statusKey := "Status"
	ageKey := "Age"

	group := &hydrapb.FilterGroup{
		Logic: hydrapb.FilterLogic_OR,
		Filters: []*hydrapb.TreasureFilter{
			{Operator: hydrapb.Relational_EQUAL, CompareValue: &hydrapb.TreasureFilter_StringVal{StringVal: "active"}, TreasureKey: &statusKey},
			{Operator: hydrapb.Relational_GREATER_THAN, CompareValue: &hydrapb.TreasureFilter_Int32Val{Int32Val: 20}, TreasureKey: &ageKey},
		},
	}

	if evaluateNativeProfileFilterGroup(treasures, group) {
		t.Error("expected OR group to fail when neither filter matches")
	}
}

func TestNativeProfileFilterGroup_MissingKey(t *testing.T) {
	treasures := map[string]treasure.Treasure{
		"Name": newTreasureWithString("Alice"),
	}

	ageKey := "Age"

	group := &hydrapb.FilterGroup{
		Logic: hydrapb.FilterLogic_AND,
		Filters: []*hydrapb.TreasureFilter{
			{Operator: hydrapb.Relational_GREATER_THAN, CompareValue: &hydrapb.TreasureFilter_Int32Val{Int32Val: 18}, TreasureKey: &ageKey},
		},
	}

	if evaluateNativeProfileFilterGroup(treasures, group) {
		t.Error("expected filter to fail when TreasureKey does not exist in map")
	}
}

func TestNativeProfileFilterGroup_MissingKey_IsEmpty(t *testing.T) {
	treasures := map[string]treasure.Treasure{
		"Name": newTreasureWithString("Alice"),
	}

	ageKey := "Age"

	group := &hydrapb.FilterGroup{
		Logic: hydrapb.FilterLogic_AND,
		Filters: []*hydrapb.TreasureFilter{
			{Operator: hydrapb.Relational_IS_EMPTY, CompareValue: &hydrapb.TreasureFilter_Int32Val{Int32Val: 0}, TreasureKey: &ageKey},
		},
	}

	if !evaluateNativeProfileFilterGroup(treasures, group) {
		t.Error("expected IS_EMPTY to return true when TreasureKey is missing")
	}
}

func TestNativeProfileFilterGroup_PhraseFilter(t *testing.T) {
	bytesVal := makeMsgpackBytesVal(t, map[string]interface{}{
		"WordIndex": map[string]interface{}{
			"hello": []interface{}{int64(1), int64(5)},
			"world": []interface{}{int64(2), int64(6)},
		},
	})

	treasures := map[string]treasure.Treasure{
		"Content": newTreasureWithBytes(bytesVal),
	}

	contentKey := "Content"

	group := &hydrapb.FilterGroup{
		Logic: hydrapb.FilterLogic_AND,
		PhraseFilters: []*hydrapb.PhraseFilter{
			{BytesFieldPath: "WordIndex", Words: []string{"hello", "world"}, TreasureKey: &contentKey},
		},
	}

	if !evaluateNativeProfileFilterGroup(treasures, group) {
		t.Error("expected phrase filter to match consecutive positions 1,2")
	}
}

func TestNativeProfileFilterGroup_SubGroups(t *testing.T) {
	treasures := map[string]treasure.Treasure{
		"Age":    newTreasureWithInt32(25),
		"Status": newTreasureWithString("pending"),
		"Role":   newTreasureWithString("admin"),
	}

	ageKey := "Age"
	statusKey := "Status"
	roleKey := "Role"

	// AND(Age > 18, OR(Status == "active", Role == "admin"))
	group := &hydrapb.FilterGroup{
		Logic: hydrapb.FilterLogic_AND,
		Filters: []*hydrapb.TreasureFilter{
			{Operator: hydrapb.Relational_GREATER_THAN, CompareValue: &hydrapb.TreasureFilter_Int32Val{Int32Val: 18}, TreasureKey: &ageKey},
		},
		SubGroups: []*hydrapb.FilterGroup{
			{
				Logic: hydrapb.FilterLogic_OR,
				Filters: []*hydrapb.TreasureFilter{
					{Operator: hydrapb.Relational_EQUAL, CompareValue: &hydrapb.TreasureFilter_StringVal{StringVal: "active"}, TreasureKey: &statusKey},
					{Operator: hydrapb.Relational_EQUAL, CompareValue: &hydrapb.TreasureFilter_StringVal{StringVal: "admin"}, TreasureKey: &roleKey},
				},
			},
		},
	}

	if !evaluateNativeProfileFilterGroup(treasures, group) {
		t.Error("expected nested AND(OR) to match: Age>18 AND (Status=active OR Role=admin)")
	}
}

func TestNativeProfileFilterGroup_EmptyGroup(t *testing.T) {
	treasures := map[string]treasure.Treasure{
		"Name": newTreasureWithString("Alice"),
	}

	group := &hydrapb.FilterGroup{}

	if !evaluateNativeProfileFilterGroup(treasures, group) {
		t.Error("expected empty filter group to pass all profiles")
	}
}

func TestNativeProfileFilterGroup_NilGroup(t *testing.T) {
	treasures := map[string]treasure.Treasure{
		"Name": newTreasureWithString("Alice"),
	}

	if !evaluateNativeProfileFilterGroup(treasures, nil) {
		t.Error("expected nil filter group to pass all profiles")
	}
}

func TestNativeProfileFilterGroup_NoTreasureKey(t *testing.T) {
	treasures := map[string]treasure.Treasure{
		"Age": newTreasureWithInt32(25),
	}

	group := &hydrapb.FilterGroup{
		Logic: hydrapb.FilterLogic_AND,
		Filters: []*hydrapb.TreasureFilter{
			{Operator: hydrapb.Relational_GREATER_THAN, CompareValue: &hydrapb.TreasureFilter_Int32Val{Int32Val: 18}},
		},
	}

	if evaluateNativeProfileFilterGroup(treasures, group) {
		t.Error("expected filter without TreasureKey to fail in profile mode")
	}
}

func TestNativeProfileFilterGroup_PhraseFilter_NoTreasureKey(t *testing.T) {
	bytesVal := makeMsgpackBytesVal(t, map[string]interface{}{
		"WordIndex": map[string]interface{}{"hello": []interface{}{int64(1)}},
	})
	treasures := map[string]treasure.Treasure{
		"Content": newTreasureWithBytes(bytesVal),
	}

	group := &hydrapb.FilterGroup{
		Logic: hydrapb.FilterLogic_AND,
		PhraseFilters: []*hydrapb.PhraseFilter{
			{BytesFieldPath: "WordIndex", Words: []string{"hello"}},
		},
	}

	if evaluateNativeProfileFilterGroup(treasures, group) {
		t.Error("expected phrase filter without TreasureKey to fail in profile mode")
	}
}

func TestNativeProfileFilterGroup_PhraseFilter_MissingTreasure(t *testing.T) {
	treasures := map[string]treasure.Treasure{
		"Name": newTreasureWithString("Alice"),
	}

	contentKey := "Content"

	group := &hydrapb.FilterGroup{
		Logic: hydrapb.FilterLogic_AND,
		PhraseFilters: []*hydrapb.PhraseFilter{
			{BytesFieldPath: "WordIndex", Words: []string{"hello"}, Negate: false, TreasureKey: &contentKey},
		},
	}

	if evaluateNativeProfileFilterGroup(treasures, group) {
		t.Error("expected non-negated phrase filter to fail when Treasure is missing")
	}

	groupNeg := &hydrapb.FilterGroup{
		Logic: hydrapb.FilterLogic_AND,
		PhraseFilters: []*hydrapb.PhraseFilter{
			{BytesFieldPath: "WordIndex", Words: []string{"hello"}, Negate: true, TreasureKey: &contentKey},
		},
	}

	if !evaluateNativeProfileFilterGroup(treasures, groupNeg) {
		t.Error("expected negated phrase filter to match when Treasure is missing")
	}
}

// --- Profile ForKey Integration Tests ---

func TestNativeProfileForKey_BoolFilter(t *testing.T) {
	treasures := map[string]treasure.Treasure{
		"IsHttps": newTreasureWithBool(true),
		"Name":    newTreasureWithString("example.com"),
	}

	key := "IsHttps"
	group := &hydrapb.FilterGroup{
		Logic: hydrapb.FilterLogic_AND,
		Filters: []*hydrapb.TreasureFilter{
			{Operator: hydrapb.Relational_EQUAL, CompareValue: &hydrapb.TreasureFilter_BoolVal{BoolVal: hydrapb.Boolean_TRUE}, TreasureKey: &key},
		},
	}

	if !evaluateNativeProfileFilterGroup(treasures, group) {
		t.Error("expected FilterBool(EQUAL, true).ForKey('IsHttps') to match")
	}
}

func TestNativeProfileForKey_Int32Filter(t *testing.T) {
	treasures := map[string]treasure.Treasure{
		"Engine": newTreasureWithInt32(10),
	}

	key := "Engine"
	group := &hydrapb.FilterGroup{
		Logic: hydrapb.FilterLogic_AND,
		Filters: []*hydrapb.TreasureFilter{
			{Operator: hydrapb.Relational_EQUAL, CompareValue: &hydrapb.TreasureFilter_Int32Val{Int32Val: 10}, TreasureKey: &key},
		},
	}

	if !evaluateNativeProfileFilterGroup(treasures, group) {
		t.Error("expected FilterInt32(EQUAL, 10).ForKey('Engine') to match")
	}
}

func TestNativeProfileForKey_BytesFieldHasKey(t *testing.T) {
	bytesVal := makeMsgpackBytesVal(t, map[string]interface{}{
		"analytics.google.com": true,
		"cdn.example.com":      true,
	})
	treasures := map[string]treasure.Treasure{
		"PluginDomains": newTreasureWithBytes(bytesVal),
	}

	key := "PluginDomains"
	emptyPath := ""
	group := &hydrapb.FilterGroup{
		Logic: hydrapb.FilterLogic_AND,
		Filters: []*hydrapb.TreasureFilter{
			{
				Operator:       hydrapb.Relational_HAS_KEY,
				CompareValue:   &hydrapb.TreasureFilter_StringVal{StringVal: "analytics.google.com"},
				BytesFieldPath: &emptyPath,
				TreasureKey:    &key,
			},
		},
	}

	if !evaluateNativeProfileFilterGroup(treasures, group) {
		t.Error("expected HAS_KEY to match for existing key")
	}

	groupMiss := &hydrapb.FilterGroup{
		Logic: hydrapb.FilterLogic_AND,
		Filters: []*hydrapb.TreasureFilter{
			{
				Operator:       hydrapb.Relational_HAS_KEY,
				CompareValue:   &hydrapb.TreasureFilter_StringVal{StringVal: "nonexistent.com"},
				BytesFieldPath: &emptyPath,
				TreasureKey:    &key,
			},
		},
	}

	if evaluateNativeProfileFilterGroup(treasures, groupMiss) {
		t.Error("expected HAS_KEY for non-existent key to NOT match")
	}
}

func TestNativeProfileForKey_PhraseFilter(t *testing.T) {
	bytesVal := makeMsgpackBytesVal(t, map[string]interface{}{
		"wordpress": []interface{}{int64(0), int64(5)},
		"plugin":    []interface{}{int64(1), int64(6)},
		"install":   []interface{}{int64(2)},
	})
	treasures := map[string]treasure.Treasure{
		"WordPositions": newTreasureWithBytes(bytesVal),
	}

	key := "WordPositions"
	group := &hydrapb.FilterGroup{
		Logic: hydrapb.FilterLogic_AND,
		PhraseFilters: []*hydrapb.PhraseFilter{
			{BytesFieldPath: "", Words: []string{"wordpress"}, TreasureKey: &key},
		},
	}

	if !evaluateNativeProfileFilterGroup(treasures, group) {
		t.Error("expected single word phrase filter to match")
	}

	groupPhrase := &hydrapb.FilterGroup{
		Logic: hydrapb.FilterLogic_AND,
		PhraseFilters: []*hydrapb.PhraseFilter{
			{BytesFieldPath: "", Words: []string{"wordpress", "plugin"}, TreasureKey: &key},
		},
	}

	if !evaluateNativeProfileFilterGroup(treasures, groupPhrase) {
		t.Error("expected multi-word phrase filter to match consecutive positions")
	}

	groupMiss := &hydrapb.FilterGroup{
		Logic: hydrapb.FilterLogic_AND,
		PhraseFilters: []*hydrapb.PhraseFilter{
			{BytesFieldPath: "", Words: []string{"nonexistent"}, TreasureKey: &key},
		},
	}

	if evaluateNativeProfileFilterGroup(treasures, groupMiss) {
		t.Error("expected phrase filter for non-existent word to NOT match")
	}
}

func TestNativeProfileForKey_CombinedAND(t *testing.T) {
	pluginBytesVal := makeMsgpackBytesVal(t, map[string]interface{}{
		"analytics.google.com": true,
	})
	wordBytesVal := makeMsgpackBytesVal(t, map[string]interface{}{
		"wordpress": []interface{}{int64(0)},
	})

	treasures := map[string]treasure.Treasure{
		"IsHttps":       newTreasureWithBool(true),
		"Engine":        newTreasureWithInt32(10),
		"PluginDomains": newTreasureWithBytes(pluginBytesVal),
		"WordPositions": newTreasureWithBytes(wordBytesVal),
	}

	boolKey := "IsHttps"
	intKey := "Engine"
	mapKey := "PluginDomains"
	wordKey := "WordPositions"
	emptyPath := ""

	group := &hydrapb.FilterGroup{
		Logic: hydrapb.FilterLogic_AND,
		Filters: []*hydrapb.TreasureFilter{
			{Operator: hydrapb.Relational_EQUAL, CompareValue: &hydrapb.TreasureFilter_BoolVal{BoolVal: hydrapb.Boolean_TRUE}, TreasureKey: &boolKey},
			{Operator: hydrapb.Relational_EQUAL, CompareValue: &hydrapb.TreasureFilter_Int32Val{Int32Val: 10}, TreasureKey: &intKey},
			{Operator: hydrapb.Relational_HAS_KEY, CompareValue: &hydrapb.TreasureFilter_StringVal{StringVal: "analytics.google.com"}, BytesFieldPath: &emptyPath, TreasureKey: &mapKey},
		},
		PhraseFilters: []*hydrapb.PhraseFilter{
			{BytesFieldPath: "", Words: []string{"wordpress"}, TreasureKey: &wordKey},
		},
	}

	if !evaluateNativeProfileFilterGroup(treasures, group) {
		t.Error("expected combined AND group with bool, int32, HAS_KEY, and phrase to all match")
	}
}

// --- Utility Tests ---

func TestExtractFieldByPath_EmptyPath(t *testing.T) {
	m := map[string]interface{}{
		"key1": "value1",
		"key2": int64(42),
	}

	result := extractFieldByPath(m, "")
	if result == nil {
		t.Fatal("extractFieldByPath with empty path should return root map, got nil")
	}

	resultMap, ok := result.(map[string]interface{})
	if !ok {
		t.Fatalf("extractFieldByPath with empty path should return map, got %T", result)
	}

	if resultMap["key1"] != "value1" {
		t.Error("root map should contain key1=value1")
	}
}

func TestExtractFieldByPath_NestedPath(t *testing.T) {
	m := map[string]interface{}{
		"Address": map[string]interface{}{
			"City": "Budapest",
		},
	}

	result := extractFieldByPath(m, "Address.City")
	if result != "Budapest" {
		t.Errorf("expected 'Budapest', got %v", result)
	}
}

// --- Vector Filter Tests ---

func makeNormalizedVector(vals ...float32) []interface{} {
	var normF float32
	for _, v := range vals {
		normF += v * v
	}
	invNorm := float32(1.0 / math.Sqrt(float64(normF)))
	result := make([]interface{}, len(vals))
	for i, v := range vals {
		result[i] = float64(v * invNorm)
	}
	return result
}

func normalizeFloat32(vals []float32) []float32 {
	var norm float32
	for _, v := range vals {
		norm += v * v
	}
	if norm == 0 {
		return vals
	}
	invNorm := float32(1.0 / math.Sqrt(float64(norm)))
	result := make([]float32, len(vals))
	for i, v := range vals {
		result[i] = v * invNorm
	}
	return result
}

func TestNativeVectorFilter_ExactMatch(t *testing.T) {
	vec := makeNormalizedVector(1.0, 0.0, 0.0, 0.0)
	bytesVal := makeMsgpackBytesVal(t, map[string]interface{}{"Embedding": vec})
	tr := newTreasureWithBytes(bytesVal)

	queryVec := normalizeFloat32([]float32{1.0, 0.0, 0.0, 0.0})

	vf := &hydrapb.VectorFilter{
		BytesFieldPath: "Embedding",
		QueryVector:    queryVec,
		MinSimilarity:  0.99,
	}

	if !evaluateNativeVectorFilter(tr, vf) {
		t.Error("expected identical normalized vectors to have similarity ~1.0")
	}
}

func TestNativeVectorFilter_Orthogonal(t *testing.T) {
	storedVec := makeNormalizedVector(1.0, 0.0, 0.0, 0.0)
	bytesVal := makeMsgpackBytesVal(t, map[string]interface{}{"Embedding": storedVec})
	tr := newTreasureWithBytes(bytesVal)

	queryVec := normalizeFloat32([]float32{0.0, 1.0, 0.0, 0.0})

	vf := &hydrapb.VectorFilter{
		BytesFieldPath: "Embedding",
		QueryVector:    queryVec,
		MinSimilarity:  0.01,
	}

	if evaluateNativeVectorFilter(tr, vf) {
		t.Error("expected orthogonal vectors to NOT match (similarity ~0.0)")
	}
}

func TestNativeVectorFilter_BelowThreshold(t *testing.T) {
	storedVec := makeNormalizedVector(1.0, 0.5, 0.0, 0.0)
	bytesVal := makeMsgpackBytesVal(t, map[string]interface{}{"Embedding": storedVec})
	tr := newTreasureWithBytes(bytesVal)

	queryVec := normalizeFloat32([]float32{0.5, 1.0, 0.0, 0.0})

	vf := &hydrapb.VectorFilter{
		BytesFieldPath: "Embedding",
		QueryVector:    queryVec,
		MinSimilarity:  0.99,
	}

	if evaluateNativeVectorFilter(tr, vf) {
		t.Error("expected vectors to NOT match with high threshold")
	}
}

func TestNativeVectorFilter_AboveThreshold(t *testing.T) {
	storedVec := makeNormalizedVector(1.0, 0.9, 0.1, 0.0)
	bytesVal := makeMsgpackBytesVal(t, map[string]interface{}{"Embedding": storedVec})
	tr := newTreasureWithBytes(bytesVal)

	queryVec := normalizeFloat32([]float32{1.0, 0.8, 0.2, 0.0})

	vf := &hydrapb.VectorFilter{
		BytesFieldPath: "Embedding",
		QueryVector:    queryVec,
		MinSimilarity:  0.90,
	}

	if !evaluateNativeVectorFilter(tr, vf) {
		t.Error("expected similar vectors to match with reasonable threshold")
	}
}

func TestNativeVectorFilter_DimensionMismatch(t *testing.T) {
	storedVec := makeNormalizedVector(1.0, 0.0, 0.0)
	bytesVal := makeMsgpackBytesVal(t, map[string]interface{}{"Embedding": storedVec})
	tr := newTreasureWithBytes(bytesVal)

	queryVec := normalizeFloat32([]float32{1.0, 0.0, 0.0, 0.0})

	vf := &hydrapb.VectorFilter{
		BytesFieldPath: "Embedding",
		QueryVector:    queryVec,
		MinSimilarity:  0.5,
	}

	if evaluateNativeVectorFilter(tr, vf) {
		t.Error("expected dimension mismatch to NOT match")
	}
}

func TestNativeVectorFilter_NilBytesVal(t *testing.T) {
	tr := newEmptyTreasure()

	queryVec := normalizeFloat32([]float32{1.0, 0.0, 0.0, 0.0})

	vf := &hydrapb.VectorFilter{
		BytesFieldPath: "Embedding",
		QueryVector:    queryVec,
		MinSimilarity:  0.5,
	}

	if evaluateNativeVectorFilter(tr, vf) {
		t.Error("expected empty content to NOT match")
	}
}

func TestNativeVectorFilter_EmptyVector(t *testing.T) {
	bytesVal := makeMsgpackBytesVal(t, map[string]interface{}{"Embedding": []interface{}{}})
	tr := newTreasureWithBytes(bytesVal)

	queryVec := normalizeFloat32([]float32{1.0, 0.0, 0.0, 0.0})

	vf := &hydrapb.VectorFilter{
		BytesFieldPath: "Embedding",
		QueryVector:    queryVec,
		MinSimilarity:  0.5,
	}

	if evaluateNativeVectorFilter(tr, vf) {
		t.Error("expected empty stored vector to NOT match")
	}
}

func TestNativeVectorFilter_GobEncoded(t *testing.T) {
	tr := newTreasureWithBytes([]byte{0x01, 0x02, 0x03, 0x04})

	queryVec := normalizeFloat32([]float32{1.0, 0.0, 0.0, 0.0})

	vf := &hydrapb.VectorFilter{
		BytesFieldPath: "Embedding",
		QueryVector:    queryVec,
		MinSimilarity:  0.5,
	}

	if evaluateNativeVectorFilter(tr, vf) {
		t.Error("expected GOB-encoded data to NOT match vector filter")
	}
}

func TestNativeVectorFilter_NestedPath(t *testing.T) {
	storedVec := makeNormalizedVector(1.0, 0.0, 0.0, 0.0)
	bytesVal := makeMsgpackBytesVal(t, map[string]interface{}{
		"Metadata": map[string]interface{}{
			"Vector": storedVec,
		},
	})
	tr := newTreasureWithBytes(bytesVal)

	queryVec := normalizeFloat32([]float32{1.0, 0.0, 0.0, 0.0})

	vf := &hydrapb.VectorFilter{
		BytesFieldPath: "Metadata.Vector",
		QueryVector:    queryVec,
		MinSimilarity:  0.99,
	}

	if !evaluateNativeVectorFilter(tr, vf) {
		t.Error("expected nested path vector filter to match")
	}
}

func TestNativeVectorFilter_MissingField(t *testing.T) {
	bytesVal := makeMsgpackBytesVal(t, map[string]interface{}{"Category": "business"})
	tr := newTreasureWithBytes(bytesVal)

	queryVec := normalizeFloat32([]float32{1.0, 0.0, 0.0, 0.0})

	vf := &hydrapb.VectorFilter{
		BytesFieldPath: "Embedding",
		QueryVector:    queryVec,
		MinSimilarity:  0.5,
	}

	if evaluateNativeVectorFilter(tr, vf) {
		t.Error("expected missing vector field to NOT match")
	}
}

func TestNativeVectorFilter_NonNumericArray(t *testing.T) {
	bytesVal := makeMsgpackBytesVal(t, map[string]interface{}{
		"Embedding": []interface{}{"not", "a", "vector"},
	})
	tr := newTreasureWithBytes(bytesVal)

	queryVec := normalizeFloat32([]float32{1.0, 0.0, 0.0, 0.0})

	vf := &hydrapb.VectorFilter{
		BytesFieldPath: "Embedding",
		QueryVector:    queryVec,
		MinSimilarity:  0.5,
	}

	if evaluateNativeVectorFilter(tr, vf) {
		t.Error("expected non-numeric array to NOT match vector filter")
	}
}

func TestNativeVectorFilter_NilFilter(t *testing.T) {
	tr := newEmptyTreasure()

	if !evaluateNativeVectorFilter(tr, nil) {
		t.Error("expected nil vector filter to pass")
	}
}

func TestNativeVectorFilter_EmptyQueryVector(t *testing.T) {
	tr := newEmptyTreasure()

	vf := &hydrapb.VectorFilter{
		BytesFieldPath: "Embedding",
		QueryVector:    []float32{},
		MinSimilarity:  0.5,
	}

	if !evaluateNativeVectorFilter(tr, vf) {
		t.Error("expected empty query vector to pass (no filtering)")
	}
}

func TestNativeVectorFilter_CombinedWithOtherFilters_AND(t *testing.T) {
	storedVec := makeNormalizedVector(1.0, 0.0, 0.0, 0.0)
	bytesVal := makeMsgpackBytesVal(t, map[string]interface{}{
		"Embedding": storedVec,
		"Category":  "business",
	})
	tr := newTreasureWithBytes(bytesVal)

	queryVec := normalizeFloat32([]float32{1.0, 0.0, 0.0, 0.0})
	categoryPath := "Category"

	group := &hydrapb.FilterGroup{
		Logic: hydrapb.FilterLogic_AND,
		Filters: []*hydrapb.TreasureFilter{
			{
				Operator:       hydrapb.Relational_EQUAL,
				CompareValue:   &hydrapb.TreasureFilter_StringVal{StringVal: "business"},
				BytesFieldPath: &categoryPath,
			},
		},
		VectorFilters: []*hydrapb.VectorFilter{
			{
				BytesFieldPath: "Embedding",
				QueryVector:    queryVec,
				MinSimilarity:  0.99,
			},
		},
	}

	if !evaluateNativeFilterGroup(tr, group) {
		t.Error("expected AND group to match (Category==business AND vector match)")
	}

	group.VectorFilters[0].MinSimilarity = 1.01
	if evaluateNativeFilterGroup(tr, group) {
		t.Error("expected AND group to NOT match with impossible vector threshold")
	}
}

func TestNativeVectorFilter_CombinedWithOtherFilters_OR(t *testing.T) {
	storedVec := makeNormalizedVector(1.0, 0.0, 0.0, 0.0)
	bytesVal := makeMsgpackBytesVal(t, map[string]interface{}{
		"Embedding": storedVec,
		"Score":     int64(99),
	})
	tr := newTreasureWithBytes(bytesVal)

	queryVec := normalizeFloat32([]float32{1.0, 0.0, 0.0, 0.0})
	scorePath := "Score"

	group := &hydrapb.FilterGroup{
		Logic: hydrapb.FilterLogic_OR,
		Filters: []*hydrapb.TreasureFilter{
			{
				Operator:       hydrapb.Relational_EQUAL,
				CompareValue:   &hydrapb.TreasureFilter_Int64Val{Int64Val: 42},
				BytesFieldPath: &scorePath,
			},
		},
		VectorFilters: []*hydrapb.VectorFilter{
			{
				BytesFieldPath: "Embedding",
				QueryVector:    queryVec,
				MinSimilarity:  0.99,
			},
		},
	}

	if !evaluateNativeFilterGroup(tr, group) {
		t.Error("expected OR group to match (Score!=42 but vector matches)")
	}
}

// --- Profile mode vector filter tests ---

func TestNativeProfileVectorFilter_Match(t *testing.T) {
	storedVec := makeNormalizedVector(1.0, 0.0, 0.0, 0.0)
	bytesVal := makeMsgpackBytesVal(t, map[string]interface{}{"Embedding": storedVec})

	treasures := map[string]treasure.Treasure{
		"MainProfile": newTreasureWithBytes(bytesVal),
	}

	queryVec := normalizeFloat32([]float32{1.0, 0.0, 0.0, 0.0})
	key := "MainProfile"

	group := &hydrapb.FilterGroup{
		Logic: hydrapb.FilterLogic_AND,
		VectorFilters: []*hydrapb.VectorFilter{
			{
				BytesFieldPath: "Embedding",
				QueryVector:    queryVec,
				MinSimilarity:  0.99,
				TreasureKey:    &key,
			},
		},
	}

	if !evaluateNativeProfileFilterGroup(treasures, group) {
		t.Error("expected profile vector filter to match")
	}
}

func TestNativeProfileVectorFilter_MissingTreasure(t *testing.T) {
	treasures := map[string]treasure.Treasure{
		"Name": newTreasureWithString("test"),
	}

	queryVec := normalizeFloat32([]float32{1.0, 0.0, 0.0, 0.0})
	key := "MainProfile"

	group := &hydrapb.FilterGroup{
		Logic: hydrapb.FilterLogic_AND,
		VectorFilters: []*hydrapb.VectorFilter{
			{
				BytesFieldPath: "Embedding",
				QueryVector:    queryVec,
				MinSimilarity:  0.5,
				TreasureKey:    &key,
			},
		},
	}

	if evaluateNativeProfileFilterGroup(treasures, group) {
		t.Error("expected vector filter to fail when Treasure is missing")
	}
}

func TestNativeProfileVectorFilter_NoTreasureKey(t *testing.T) {
	bytesVal := makeMsgpackBytesVal(t, map[string]interface{}{"Embedding": makeNormalizedVector(1.0, 0.0)})
	treasures := map[string]treasure.Treasure{
		"MainProfile": newTreasureWithBytes(bytesVal),
	}

	queryVec := normalizeFloat32([]float32{1.0, 0.0})

	group := &hydrapb.FilterGroup{
		Logic: hydrapb.FilterLogic_AND,
		VectorFilters: []*hydrapb.VectorFilter{
			{
				BytesFieldPath: "Embedding",
				QueryVector:    queryVec,
				MinSimilarity:  0.5,
			},
		},
	}

	if evaluateNativeProfileFilterGroup(treasures, group) {
		t.Error("expected vector filter without TreasureKey to fail in profile mode")
	}
}

// --- Dot product unit tests ---

func TestDotProduct_IdenticalNormalized(t *testing.T) {
	v := normalizeFloat32([]float32{1.0, 2.0, 3.0, 4.0})
	result := dotProduct(v, v)
	if result < 0.999 || result > 1.001 {
		t.Errorf("dot product of identical normalized vectors should be ~1.0, got %f", result)
	}
}

func TestDotProduct_Orthogonal(t *testing.T) {
	a := normalizeFloat32([]float32{1.0, 0.0, 0.0})
	b := normalizeFloat32([]float32{0.0, 1.0, 0.0})
	result := dotProduct(a, b)
	if result < -0.001 || result > 0.001 {
		t.Errorf("dot product of orthogonal vectors should be ~0.0, got %f", result)
	}
}

func TestDotProduct_Opposite(t *testing.T) {
	a := normalizeFloat32([]float32{1.0, 0.0, 0.0})
	b := normalizeFloat32([]float32{-1.0, 0.0, 0.0})
	result := dotProduct(a, b)
	if result < -1.001 || result > -0.999 {
		t.Errorf("dot product of opposite vectors should be ~-1.0, got %f", result)
	}
}

func TestDotProduct_Empty(t *testing.T) {
	result := dotProduct([]float32{}, []float32{})
	if result != 0 {
		t.Errorf("dot product of empty vectors should be 0, got %f", result)
	}
}

// --- toFloat32Slice tests ---

func TestToFloat32Slice_Float64(t *testing.T) {
	input := []interface{}{float64(1.0), float64(2.0), float64(3.0)}
	result := toFloat32Slice(input)
	if len(result) != 3 {
		t.Fatalf("expected 3 elements, got %d", len(result))
	}
	if result[0] != 1.0 || result[1] != 2.0 || result[2] != 3.0 {
		t.Errorf("unexpected values: %v", result)
	}
}

func TestToFloat32Slice_MixedNumeric(t *testing.T) {
	input := []interface{}{int64(1), uint8(2), float32(3.5)}
	result := toFloat32Slice(input)
	if len(result) != 3 {
		t.Fatalf("expected 3 elements, got %d", len(result))
	}
}

func TestToFloat32Slice_NonNumeric(t *testing.T) {
	input := []interface{}{float64(1.0), "not-a-number", float64(3.0)}
	result := toFloat32Slice(input)
	if result != nil {
		t.Error("expected nil for array with non-numeric element")
	}
}

func TestToFloat32Slice_Nil(t *testing.T) {
	result := toFloat32Slice(nil)
	if result != nil {
		t.Error("expected nil for nil input")
	}
}

func TestToFloat32Slice_NotArray(t *testing.T) {
	result := toFloat32Slice("not-an-array")
	if result != nil {
		t.Error("expected nil for non-array input")
	}
}

// --- GeoDistance tests ---

func makeGeoTreasure(t *testing.T, lat, lng float64) treasure.Treasure {
	t.Helper()
	bytesVal := makeMsgpackBytesVal(t, map[string]interface{}{
		"geo_latitude":  lat,
		"geo_longitude": lng,
	})
	return newTreasureWithBytes(bytesVal)
}

func geoFilter(refLat, refLng, radiusKm float64, mode hydrapb.GeoDistanceMode_Type) *hydrapb.GeoDistanceFilter {
	return &hydrapb.GeoDistanceFilter{
		LatFieldPath: "geo_latitude",
		LngFieldPath: "geo_longitude",
		RefLatitude:  refLat,
		RefLongitude: refLng,
		RadiusKm:     radiusKm,
		Mode:         mode,
	}
}

func TestGeoDistance_InsideRadius(t *testing.T) {
	// Budapest → Székesfehérvár ~60km
	tr := makeGeoTreasure(t, 47.1860, 18.4221)
	gf := geoFilter(47.4979, 19.0402, 100.0, hydrapb.GeoDistanceMode_INSIDE)
	if !evaluateNativeGeoDistanceFilter(tr, gf) {
		t.Error("expected Székesfehérvár (~60km) to be inside 100km radius from Budapest")
	}
}

func TestGeoDistance_OutsideRadius(t *testing.T) {
	// Budapest → Székesfehérvár ~60km, but radius is only 30km
	tr := makeGeoTreasure(t, 47.1860, 18.4221)
	gf := geoFilter(47.4979, 19.0402, 30.0, hydrapb.GeoDistanceMode_INSIDE)
	if evaluateNativeGeoDistanceFilter(tr, gf) {
		t.Error("expected Székesfehérvár (~60km) to NOT be inside 30km radius from Budapest")
	}
}

func TestGeoDistance_OutsideMode(t *testing.T) {
	// Budapest → Debrecen ~194km, OUTSIDE 150km → match
	tr := makeGeoTreasure(t, 47.5316, 21.6273)
	gf := geoFilter(47.4979, 19.0402, 150.0, hydrapb.GeoDistanceMode_OUTSIDE)
	if !evaluateNativeGeoDistanceFilter(tr, gf) {
		t.Error("expected Debrecen (~194km) to be outside 150km radius from Budapest")
	}
}

func TestGeoDistance_OutsideMode_TooClose(t *testing.T) {
	// Budapest → Székesfehérvár ~60km, OUTSIDE 200km → no match
	tr := makeGeoTreasure(t, 47.1860, 18.4221)
	gf := geoFilter(47.4979, 19.0402, 200.0, hydrapb.GeoDistanceMode_OUTSIDE)
	if evaluateNativeGeoDistanceFilter(tr, gf) {
		t.Error("expected Székesfehérvár (~60km) to NOT be outside 200km radius from Budapest")
	}
}

func TestGeoDistance_NullIsland(t *testing.T) {
	tr := makeGeoTreasure(t, 0, 0)
	// INSIDE mode
	gf := geoFilter(47.4979, 19.0402, 50000.0, hydrapb.GeoDistanceMode_INSIDE)
	if evaluateNativeGeoDistanceFilter(tr, gf) {
		t.Error("expected Null Island (0,0) to be excluded in INSIDE mode")
	}
	// OUTSIDE mode
	gf2 := geoFilter(47.4979, 19.0402, 1.0, hydrapb.GeoDistanceMode_OUTSIDE)
	if evaluateNativeGeoDistanceFilter(tr, gf2) {
		t.Error("expected Null Island (0,0) to be excluded in OUTSIDE mode")
	}
}

func TestGeoDistance_MissingField(t *testing.T) {
	bytesVal := makeMsgpackBytesVal(t, map[string]interface{}{"other_field": 42})
	tr := newTreasureWithBytes(bytesVal)
	gf := geoFilter(47.4979, 19.0402, 50.0, hydrapb.GeoDistanceMode_INSIDE)
	if evaluateNativeGeoDistanceFilter(tr, gf) {
		t.Error("expected false when lat/lng fields are missing")
	}
}

func TestGeoDistance_NonMsgpack(t *testing.T) {
	tr := newTreasureWithString("not bytes")
	gf := geoFilter(47.4979, 19.0402, 50.0, hydrapb.GeoDistanceMode_INSIDE)
	if evaluateNativeGeoDistanceFilter(tr, gf) {
		t.Error("expected false for non-byte-array content")
	}
}

func TestGeoDistance_Band(t *testing.T) {
	// Budapest → Győr ~120km, band 50–150km → match
	tr := makeGeoTreasure(t, 47.6875, 17.6504)
	group := &hydrapb.FilterGroup{
		Logic: hydrapb.FilterLogic_AND,
		GeoDistanceFilters: []*hydrapb.GeoDistanceFilter{
			geoFilter(47.4979, 19.0402, 50.0, hydrapb.GeoDistanceMode_OUTSIDE),
			geoFilter(47.4979, 19.0402, 150.0, hydrapb.GeoDistanceMode_INSIDE),
		},
	}
	if !evaluateNativeFilterGroup(tr, group) {
		t.Error("expected Győr (~120km) to be within 50–150km band from Budapest")
	}

	// Budapest → Székesfehérvár ~60km, band 100–200km → no match (too close)
	tr2 := makeGeoTreasure(t, 47.1860, 18.4221)
	group2 := &hydrapb.FilterGroup{
		Logic: hydrapb.FilterLogic_AND,
		GeoDistanceFilters: []*hydrapb.GeoDistanceFilter{
			geoFilter(47.4979, 19.0402, 100.0, hydrapb.GeoDistanceMode_OUTSIDE),
			geoFilter(47.4979, 19.0402, 200.0, hydrapb.GeoDistanceMode_INSIDE),
		},
	}
	if evaluateNativeFilterGroup(tr2, group2) {
		t.Error("expected Székesfehérvár (~60km) to NOT be within 100–200km band")
	}
}

func TestGeoDistance_SamePoint(t *testing.T) {
	tr := makeGeoTreasure(t, 47.4979, 19.0402)
	gf := geoFilter(47.4979, 19.0402, 1.0, hydrapb.GeoDistanceMode_INSIDE)
	if !evaluateNativeGeoDistanceFilter(tr, gf) {
		t.Error("expected same point to be inside any radius")
	}
}

func TestGeoDistance_LargeDistance(t *testing.T) {
	// Budapest → Sydney ~15000km
	tr := makeGeoTreasure(t, -33.8688, 151.2093)
	gf := geoFilter(47.4979, 19.0402, 16000.0, hydrapb.GeoDistanceMode_INSIDE)
	if !evaluateNativeGeoDistanceFilter(tr, gf) {
		t.Error("expected Sydney to be inside 16000km radius from Budapest")
	}
	gf2 := geoFilter(47.4979, 19.0402, 14000.0, hydrapb.GeoDistanceMode_INSIDE)
	if evaluateNativeGeoDistanceFilter(tr, gf2) {
		t.Error("expected Sydney to NOT be inside 14000km radius from Budapest")
	}
}

func TestGeoDistance_NegativeCoords(t *testing.T) {
	// Buenos Aires (southern hemisphere)
	tr := makeGeoTreasure(t, -34.6037, -58.3816)
	gf := geoFilter(-34.6037, -58.3816, 1.0, hydrapb.GeoDistanceMode_INSIDE)
	if !evaluateNativeGeoDistanceFilter(tr, gf) {
		t.Error("expected same point with negative coords to match")
	}
}

func TestGeoDistance_NilFilter(t *testing.T) {
	tr := makeGeoTreasure(t, 47.4979, 19.0402)
	if !evaluateNativeGeoDistanceFilter(tr, nil) {
		t.Error("expected nil filter to pass (no filtering)")
	}
}

func TestGeoDistance_ZeroRadius(t *testing.T) {
	tr := makeGeoTreasure(t, 47.4979, 19.0402)
	gf := geoFilter(47.4979, 19.0402, 0.0, hydrapb.GeoDistanceMode_INSIDE)
	if !evaluateNativeGeoDistanceFilter(tr, gf) {
		t.Error("expected exact same point to match with 0 radius")
	}

	// Slightly different point with 0 radius → no match
	tr2 := makeGeoTreasure(t, 47.498, 19.0402)
	if evaluateNativeGeoDistanceFilter(tr2, gf) {
		t.Error("expected different point to NOT match with 0 radius")
	}
}

func TestGeoDistance_CombinedWithOtherFilters(t *testing.T) {
	bytesVal := makeMsgpackBytesVal(t, map[string]interface{}{
		"geo_latitude":  47.1860,
		"geo_longitude": 18.4221,
		"Category":      "business",
	})
	tr := newTreasureWithBytes(bytesVal)

	categoryPath := "Category"
	group := &hydrapb.FilterGroup{
		Logic: hydrapb.FilterLogic_AND,
		Filters: []*hydrapb.TreasureFilter{
			{
				Operator:       hydrapb.Relational_EQUAL,
				BytesFieldPath: &categoryPath,
				CompareValue:   &hydrapb.TreasureFilter_StringVal{StringVal: "business"},
			},
		},
		GeoDistanceFilters: []*hydrapb.GeoDistanceFilter{
			geoFilter(47.4979, 19.0402, 100.0, hydrapb.GeoDistanceMode_INSIDE),
		},
	}
	if !evaluateNativeFilterGroup(tr, group) {
		t.Error("expected combined filter (category + geo) to match")
	}

	// Wrong category → no match despite geo match
	group.Filters[0].CompareValue = &hydrapb.TreasureFilter_StringVal{StringVal: "sports"}
	if evaluateNativeFilterGroup(tr, group) {
		t.Error("expected combined filter to NOT match when category doesn't match")
	}
}

func TestGeoDistance_ProfileMode(t *testing.T) {
	bytesVal := makeMsgpackBytesVal(t, map[string]interface{}{
		"geo_latitude":  47.1860,
		"geo_longitude": 18.4221,
	})
	tr := newTreasureWithBytes(bytesVal)
	key := "Location"
	treasures := map[string]treasure.Treasure{key: tr}

	gf := geoFilter(47.4979, 19.0402, 100.0, hydrapb.GeoDistanceMode_INSIDE)
	gf.TreasureKey = &key

	if !evaluateNativeProfileGeoDistanceFilter(treasures, gf) {
		t.Error("expected profile geo filter to match")
	}

	// Missing treasure key
	missingKey := "NonExistent"
	gf2 := geoFilter(47.4979, 19.0402, 100.0, hydrapb.GeoDistanceMode_INSIDE)
	gf2.TreasureKey = &missingKey
	if evaluateNativeProfileGeoDistanceFilter(treasures, gf2) {
		t.Error("expected profile geo filter to fail for missing treasure key")
	}
}

func TestGeoDistance_NestedFieldPath(t *testing.T) {
	bytesVal := makeMsgpackBytesVal(t, map[string]interface{}{
		"Location": map[string]interface{}{
			"Lat": 47.1860,
			"Lng": 18.4221,
		},
	})
	tr := newTreasureWithBytes(bytesVal)

	gf := &hydrapb.GeoDistanceFilter{
		LatFieldPath: "Location.Lat",
		LngFieldPath: "Location.Lng",
		RefLatitude:  47.4979,
		RefLongitude: 19.0402,
		RadiusKm:     100.0,
		Mode:         hydrapb.GeoDistanceMode_INSIDE,
	}
	if !evaluateNativeGeoDistanceFilter(tr, gf) {
		t.Error("expected nested field path geo filter to match")
	}
}

// --- Haversine unit tests ---

func TestHaversine_KnownDistance(t *testing.T) {
	// Budapest → Vienna ~215km
	dist := haversineDistance(47.4979, 19.0402, 48.2082, 16.3738)
	if math.Abs(dist-215.0) > 10.0 {
		t.Errorf("Budapest→Vienna expected ~215km, got %.1fkm", dist)
	}
}

func TestHaversine_SamePoint(t *testing.T) {
	dist := haversineDistance(47.4979, 19.0402, 47.4979, 19.0402)
	if dist != 0 {
		t.Errorf("expected 0 distance for same point, got %f", dist)
	}
}

func TestHaversine_Antipodal(t *testing.T) {
	// Antipodal points should be ~20015km (half Earth circumference)
	dist := haversineDistance(0, 0, 0, 180)
	if math.Abs(dist-20015.0) > 100.0 {
		t.Errorf("expected ~20015km for antipodal points, got %.1fkm", dist)
	}
}

func TestBoundingBox_Contains(t *testing.T) {
	bb := newGeoBoundingBox(47.4979, 19.0402, 50.0)
	// Budapest itself should be inside
	if !bb.contains(47.4979, 19.0402) {
		t.Error("expected center point to be inside bounding box")
	}
	// Far away point should be outside
	if bb.contains(40.0, 19.0) {
		t.Error("expected far point to be outside bounding box")
	}
}

// --- Benchmark ---

func BenchmarkHaversine(b *testing.B) {
	for b.Loop() {
		haversineDistance(47.4979, 19.0402, 47.1860, 18.4221)
	}
}

func BenchmarkBoundingBox(b *testing.B) {
	bb := newGeoBoundingBox(47.4979, 19.0402, 50.0)
	for b.Loop() {
		bb.contains(47.1860, 18.4221)
	}
}

func BenchmarkGeoDistanceFilter(b *testing.B) {
	data := map[string]interface{}{
		"geo_latitude":  47.1860,
		"geo_longitude": 18.4221,
	}
	encoded, _ := msgpack.Marshal(data)
	bytesVal := append([]byte{msgpackMagic0, msgpackMagic1}, encoded...)
	tr := newTreasureWithBytes(bytesVal)
	gf := geoFilter(47.4979, 19.0402, 100.0, hydrapb.GeoDistanceMode_INSIDE)

	for b.Loop() {
		evaluateNativeGeoDistanceFilter(tr, gf)
	}
}

// =============================================================================
// Slice filter tests
// =============================================================================

func makeSliceTreasure(t *testing.T, data map[string]interface{}) treasure.Treasure {
	t.Helper()
	return newTreasureWithBytes(makeMsgpackBytesVal(t, data))
}

func sliceFilterGroup(op hydrapb.Relational_Operator, path string, cv interface{}) *hydrapb.FilterGroup {
	f := &hydrapb.TreasureFilter{
		Operator:       op,
		BytesFieldPath: &path,
	}
	switch v := cv.(type) {
	case int8:
		f.CompareValue = &hydrapb.TreasureFilter_Int8Val{Int8Val: int32(v)}
	case int32:
		f.CompareValue = &hydrapb.TreasureFilter_Int32Val{Int32Val: v}
	case int64:
		f.CompareValue = &hydrapb.TreasureFilter_Int64Val{Int64Val: v}
	case string:
		f.CompareValue = &hydrapb.TreasureFilter_StringVal{StringVal: v}
	}
	return &hydrapb.FilterGroup{
		Logic:   hydrapb.FilterLogic_AND,
		Filters: []*hydrapb.TreasureFilter{f},
	}
}

// --- SLICE_CONTAINS tests ---

func TestSliceContainsInt8_Found(t *testing.T) {
	tr := makeSliceTreasure(t, map[string]interface{}{"Funcs": []interface{}{int8(1), int8(7), int8(2)}})
	fg := sliceFilterGroup(hydrapb.Relational_SLICE_CONTAINS, "Funcs", int8(7))
	if !evaluateNativeFilterGroup(tr, fg) {
		t.Error("expected SLICE_CONTAINS to find int8(7)")
	}
}

func TestSliceContainsInt8_NotFound(t *testing.T) {
	tr := makeSliceTreasure(t, map[string]interface{}{"Funcs": []interface{}{int8(1), int8(7), int8(2)}})
	fg := sliceFilterGroup(hydrapb.Relational_SLICE_CONTAINS, "Funcs", int8(5))
	if evaluateNativeFilterGroup(tr, fg) {
		t.Error("expected SLICE_CONTAINS to not find int8(5)")
	}
}

func TestSliceContainsInt32_Found(t *testing.T) {
	tr := makeSliceTreasure(t, map[string]interface{}{"IDs": []interface{}{int32(100), int32(200)}})
	fg := sliceFilterGroup(hydrapb.Relational_SLICE_CONTAINS, "IDs", int32(200))
	if !evaluateNativeFilterGroup(tr, fg) {
		t.Error("expected SLICE_CONTAINS to find int32(200)")
	}
}

func TestSliceContainsInt64_Found(t *testing.T) {
	tr := makeSliceTreasure(t, map[string]interface{}{"BigIDs": []interface{}{int64(999999)}})
	fg := sliceFilterGroup(hydrapb.Relational_SLICE_CONTAINS, "BigIDs", int64(999999))
	if !evaluateNativeFilterGroup(tr, fg) {
		t.Error("expected SLICE_CONTAINS to find int64")
	}
}

func TestSliceContainsString_Found(t *testing.T) {
	tr := makeSliceTreasure(t, map[string]interface{}{"Providers": []interface{}{"Barion", "PayPal"}})
	fg := sliceFilterGroup(hydrapb.Relational_SLICE_CONTAINS, "Providers", "Barion")
	if !evaluateNativeFilterGroup(tr, fg) {
		t.Error("expected SLICE_CONTAINS to find 'Barion'")
	}
}

func TestSliceContainsString_NotFound(t *testing.T) {
	tr := makeSliceTreasure(t, map[string]interface{}{"Providers": []interface{}{"Barion", "PayPal"}})
	fg := sliceFilterGroup(hydrapb.Relational_SLICE_CONTAINS, "Providers", "Stripe")
	if evaluateNativeFilterGroup(tr, fg) {
		t.Error("expected SLICE_CONTAINS to not find 'Stripe'")
	}
}

func TestSliceContainsString_CaseSensitive(t *testing.T) {
	tr := makeSliceTreasure(t, map[string]interface{}{"Providers": []interface{}{"Barion"}})
	fg := sliceFilterGroup(hydrapb.Relational_SLICE_CONTAINS, "Providers", "barion")
	if evaluateNativeFilterGroup(tr, fg) {
		t.Error("SLICE_CONTAINS exact match should be case-sensitive")
	}
}

func TestSliceContains_EmptySlice(t *testing.T) {
	tr := makeSliceTreasure(t, map[string]interface{}{"Funcs": []interface{}{}})
	fg := sliceFilterGroup(hydrapb.Relational_SLICE_CONTAINS, "Funcs", int8(1))
	if evaluateNativeFilterGroup(tr, fg) {
		t.Error("expected SLICE_CONTAINS on empty slice to return false")
	}
}

func TestSliceContains_NilField(t *testing.T) {
	tr := makeSliceTreasure(t, map[string]interface{}{"Other": "value"})
	fg := sliceFilterGroup(hydrapb.Relational_SLICE_CONTAINS, "Funcs", int8(1))
	if evaluateNativeFilterGroup(tr, fg) {
		t.Error("expected SLICE_CONTAINS on missing field to return false")
	}
}

func TestSliceContains_NotASlice(t *testing.T) {
	tr := makeSliceTreasure(t, map[string]interface{}{"Funcs": "not-a-slice"})
	fg := sliceFilterGroup(hydrapb.Relational_SLICE_CONTAINS, "Funcs", int8(1))
	if evaluateNativeFilterGroup(tr, fg) {
		t.Error("expected SLICE_CONTAINS on non-slice to return false")
	}
}

// --- SLICE_NOT_CONTAINS tests ---

func TestSliceNotContainsInt8_Found(t *testing.T) {
	tr := makeSliceTreasure(t, map[string]interface{}{"Funcs": []interface{}{int8(1), int8(7), int8(2)}})
	fg := sliceFilterGroup(hydrapb.Relational_SLICE_NOT_CONTAINS, "Funcs", int8(7))
	if evaluateNativeFilterGroup(tr, fg) {
		t.Error("expected SLICE_NOT_CONTAINS to return false when value IS in slice")
	}
}

func TestSliceNotContainsInt8_NotFound(t *testing.T) {
	tr := makeSliceTreasure(t, map[string]interface{}{"Funcs": []interface{}{int8(1), int8(7), int8(2)}})
	fg := sliceFilterGroup(hydrapb.Relational_SLICE_NOT_CONTAINS, "Funcs", int8(5))
	if !evaluateNativeFilterGroup(tr, fg) {
		t.Error("expected SLICE_NOT_CONTAINS to return true when value is NOT in slice")
	}
}

func TestSliceNotContainsString_NotFound(t *testing.T) {
	tr := makeSliceTreasure(t, map[string]interface{}{"Providers": []interface{}{"Barion", "PayPal"}})
	fg := sliceFilterGroup(hydrapb.Relational_SLICE_NOT_CONTAINS, "Providers", "Stripe")
	if !evaluateNativeFilterGroup(tr, fg) {
		t.Error("expected SLICE_NOT_CONTAINS to return true for absent value")
	}
}

func TestSliceNotContains_EmptySlice(t *testing.T) {
	tr := makeSliceTreasure(t, map[string]interface{}{"Funcs": []interface{}{}})
	fg := sliceFilterGroup(hydrapb.Relational_SLICE_NOT_CONTAINS, "Funcs", int8(1))
	if !evaluateNativeFilterGroup(tr, fg) {
		t.Error("expected SLICE_NOT_CONTAINS on empty slice to return true")
	}
}

func TestSliceNotContains_NilField(t *testing.T) {
	tr := makeSliceTreasure(t, map[string]interface{}{"Other": "value"})
	fg := sliceFilterGroup(hydrapb.Relational_SLICE_NOT_CONTAINS, "Funcs", int8(1))
	if !evaluateNativeFilterGroup(tr, fg) {
		t.Error("expected SLICE_NOT_CONTAINS on missing field to return true")
	}
}

// --- SLICE_CONTAINS_SUBSTRING tests ---

func TestSliceContainsSubstring_Found(t *testing.T) {
	tr := makeSliceTreasure(t, map[string]interface{}{
		"Activities": []interface{}{"custom tattoo design", "piercing services"},
	})
	fg := sliceFilterGroup(hydrapb.Relational_SLICE_CONTAINS_SUBSTRING, "Activities", "tattoo")
	if !evaluateNativeFilterGroup(tr, fg) {
		t.Error("expected SLICE_CONTAINS_SUBSTRING to find 'tattoo'")
	}
}

func TestSliceContainsSubstring_CaseInsensitive(t *testing.T) {
	tr := makeSliceTreasure(t, map[string]interface{}{
		"Activities": []interface{}{"custom tattoo design"},
	})
	fg := sliceFilterGroup(hydrapb.Relational_SLICE_CONTAINS_SUBSTRING, "Activities", "TATTOO")
	if !evaluateNativeFilterGroup(tr, fg) {
		t.Error("expected SLICE_CONTAINS_SUBSTRING to be case-insensitive")
	}
}

func TestSliceContainsSubstring_NotFound(t *testing.T) {
	tr := makeSliceTreasure(t, map[string]interface{}{
		"Activities": []interface{}{"custom tattoo design", "piercing"},
	})
	fg := sliceFilterGroup(hydrapb.Relational_SLICE_CONTAINS_SUBSTRING, "Activities", "laser")
	if evaluateNativeFilterGroup(tr, fg) {
		t.Error("expected SLICE_CONTAINS_SUBSTRING to not find 'laser'")
	}
}

func TestSliceContainsSubstring_EmptySlice(t *testing.T) {
	tr := makeSliceTreasure(t, map[string]interface{}{"Activities": []interface{}{}})
	fg := sliceFilterGroup(hydrapb.Relational_SLICE_CONTAINS_SUBSTRING, "Activities", "tattoo")
	if evaluateNativeFilterGroup(tr, fg) {
		t.Error("expected SLICE_CONTAINS_SUBSTRING on empty slice to return false")
	}
}

func TestSliceContainsSubstring_PartialMatch(t *testing.T) {
	tr := makeSliceTreasure(t, map[string]interface{}{
		"Activities": []interface{}{"body-art studio"},
	})
	fg := sliceFilterGroup(hydrapb.Relational_SLICE_CONTAINS_SUBSTRING, "Activities", "art")
	if !evaluateNativeFilterGroup(tr, fg) {
		t.Error("expected SLICE_CONTAINS_SUBSTRING to find partial match 'art'")
	}
}

// --- SLICE_NOT_CONTAINS_SUBSTRING tests ---

func TestSliceNotContainsSubstring_Found(t *testing.T) {
	tr := makeSliceTreasure(t, map[string]interface{}{
		"Activities": []interface{}{"custom tattoo design"},
	})
	fg := sliceFilterGroup(hydrapb.Relational_SLICE_NOT_CONTAINS_SUBSTRING, "Activities", "tattoo")
	if evaluateNativeFilterGroup(tr, fg) {
		t.Error("expected SLICE_NOT_CONTAINS_SUBSTRING to return false when substring is present")
	}
}

func TestSliceNotContainsSubstring_NotFound(t *testing.T) {
	tr := makeSliceTreasure(t, map[string]interface{}{
		"Activities": []interface{}{"custom tattoo design"},
	})
	fg := sliceFilterGroup(hydrapb.Relational_SLICE_NOT_CONTAINS_SUBSTRING, "Activities", "laser")
	if !evaluateNativeFilterGroup(tr, fg) {
		t.Error("expected SLICE_NOT_CONTAINS_SUBSTRING to return true when substring is absent")
	}
}

// --- #len pseudo-field tests ---

func TestSliceLen_GreaterThan_Pass(t *testing.T) {
	tr := makeSliceTreasure(t, map[string]interface{}{
		"Contacts": []interface{}{"a", "b", "c"},
	})
	path := "Contacts.#len"
	fg := sliceFilterGroup(hydrapb.Relational_GREATER_THAN, path, int32(0))
	if !evaluateNativeFilterGroup(tr, fg) {
		t.Error("expected #len > 0 to pass for 3-element slice")
	}
}

func TestSliceLen_GreaterThan_Fail(t *testing.T) {
	tr := makeSliceTreasure(t, map[string]interface{}{
		"Contacts": []interface{}{},
	})
	path := "Contacts.#len"
	fg := sliceFilterGroup(hydrapb.Relational_GREATER_THAN, path, int32(0))
	if evaluateNativeFilterGroup(tr, fg) {
		t.Error("expected #len > 0 to fail for empty slice")
	}
}

func TestSliceLen_Equal_Pass(t *testing.T) {
	tr := makeSliceTreasure(t, map[string]interface{}{
		"Sectors": []interface{}{int8(1), int8(6), int8(3)},
	})
	path := "Sectors.#len"
	fg := sliceFilterGroup(hydrapb.Relational_EQUAL, path, int32(3))
	if !evaluateNativeFilterGroup(tr, fg) {
		t.Error("expected #len == 3 to pass")
	}
}

func TestSliceLen_Equal_Fail(t *testing.T) {
	tr := makeSliceTreasure(t, map[string]interface{}{
		"Sectors": []interface{}{int8(1), int8(6), int8(3)},
	})
	path := "Sectors.#len"
	fg := sliceFilterGroup(hydrapb.Relational_EQUAL, path, int32(5))
	if evaluateNativeFilterGroup(tr, fg) {
		t.Error("expected #len == 5 to fail for 3-element slice")
	}
}

func TestSliceLen_NilField(t *testing.T) {
	tr := makeSliceTreasure(t, map[string]interface{}{"Other": "x"})
	path := "Missing.#len"
	fg := sliceFilterGroup(hydrapb.Relational_GREATER_THAN, path, int32(0))
	if evaluateNativeFilterGroup(tr, fg) {
		t.Error("expected #len on missing field to return false")
	}
}

func TestSliceLen_LessThan(t *testing.T) {
	tr := makeSliceTreasure(t, map[string]interface{}{
		"Items": []interface{}{"a", "b"},
	})
	path := "Items.#len"
	fg := sliceFilterGroup(hydrapb.Relational_LESS_THAN, path, int32(5))
	if !evaluateNativeFilterGroup(tr, fg) {
		t.Error("expected #len < 5 to pass for 2-element slice")
	}
}

func TestSliceLen_MapLen(t *testing.T) {
	tr := makeSliceTreasure(t, map[string]interface{}{
		"Meta": map[string]interface{}{"a": 1, "b": 2, "c": 3},
	})
	path := "Meta.#len"
	fg := sliceFilterGroup(hydrapb.Relational_EQUAL, path, int32(3))
	if !evaluateNativeFilterGroup(tr, fg) {
		t.Error("expected #len == 3 for map with 3 keys")
	}
}

// --- [*] wildcard (NestedSliceAny) tests ---

func makeContactsTreasure(t *testing.T, contacts []map[string]interface{}) treasure.Treasure {
	t.Helper()
	contactSlice := make([]interface{}, len(contacts))
	for i, c := range contacts {
		contactSlice[i] = c
	}
	return makeSliceTreasure(t, map[string]interface{}{"Contacts": contactSlice})
}

func TestNestedSliceAny_IsNotEmpty_Pass(t *testing.T) {
	tr := makeContactsTreasure(t, []map[string]interface{}{
		{"Email": "john@example.com", "Role": "CEO"},
		{"Email": "", "Role": "CTO"},
	})
	path := "Contacts[*].Email"
	f := &hydrapb.TreasureFilter{
		Operator:       hydrapb.Relational_IS_NOT_EMPTY,
		BytesFieldPath: &path,
		CompareValue:   &hydrapb.TreasureFilter_StringVal{StringVal: ""},
	}
	fg := &hydrapb.FilterGroup{Logic: hydrapb.FilterLogic_AND, Filters: []*hydrapb.TreasureFilter{f}}
	if !evaluateNativeFilterGroup(tr, fg) {
		t.Error("expected [*].Email IS_NOT_EMPTY to pass when at least one has email")
	}
}

func TestNestedSliceAny_IsNotEmpty_Fail(t *testing.T) {
	tr := makeContactsTreasure(t, []map[string]interface{}{
		{"Email": "", "Role": "CEO"},
		{"Email": "", "Role": "CTO"},
	})
	path := "Contacts[*].Email"
	f := &hydrapb.TreasureFilter{
		Operator:       hydrapb.Relational_IS_NOT_EMPTY,
		BytesFieldPath: &path,
		CompareValue:   &hydrapb.TreasureFilter_StringVal{StringVal: ""},
	}
	fg := &hydrapb.FilterGroup{Logic: hydrapb.FilterLogic_AND, Filters: []*hydrapb.TreasureFilter{f}}
	if evaluateNativeFilterGroup(tr, fg) {
		t.Error("expected [*].Email IS_NOT_EMPTY to fail when all emails are empty")
	}
}

func TestNestedSliceAny_Equal_Pass(t *testing.T) {
	tr := makeContactsTreasure(t, []map[string]interface{}{
		{"Email": "john@example.com", "Role": "CEO"},
		{"Email": "jane@example.com", "Role": "CTO"},
	})
	path := "Contacts[*].Role"
	fg := sliceFilterGroup(hydrapb.Relational_EQUAL, path, "CEO")
	if !evaluateNativeFilterGroup(tr, fg) {
		t.Error("expected [*].Role == CEO to pass")
	}
}

func TestNestedSliceAny_Equal_Fail(t *testing.T) {
	tr := makeContactsTreasure(t, []map[string]interface{}{
		{"Email": "john@example.com", "Role": "Developer"},
		{"Email": "jane@example.com", "Role": "CTO"},
	})
	path := "Contacts[*].Role"
	fg := sliceFilterGroup(hydrapb.Relational_EQUAL, path, "CEO")
	if evaluateNativeFilterGroup(tr, fg) {
		t.Error("expected [*].Role == CEO to fail when no CEO exists")
	}
}

func TestNestedSliceAny_Contains_Pass(t *testing.T) {
	tr := makeContactsTreasure(t, []map[string]interface{}{
		{"Email": "john@company.com"},
		{"Email": "jane@gmail.com"},
	})
	path := "Contacts[*].Email"
	fg := sliceFilterGroup(hydrapb.Relational_CONTAINS, path, "@company.com")
	if !evaluateNativeFilterGroup(tr, fg) {
		t.Error("expected [*].Email CONTAINS @company.com to pass")
	}
}

func TestNestedSliceAny_EmptySlice(t *testing.T) {
	tr := makeSliceTreasure(t, map[string]interface{}{"Contacts": []interface{}{}})
	path := "Contacts[*].Email"
	f := &hydrapb.TreasureFilter{
		Operator:       hydrapb.Relational_IS_NOT_EMPTY,
		BytesFieldPath: &path,
		CompareValue:   &hydrapb.TreasureFilter_StringVal{StringVal: ""},
	}
	fg := &hydrapb.FilterGroup{Logic: hydrapb.FilterLogic_AND, Filters: []*hydrapb.TreasureFilter{f}}
	if evaluateNativeFilterGroup(tr, fg) {
		t.Error("expected [*] on empty slice to return false for IS_NOT_EMPTY")
	}
}

// --- Compound filter tests ---

func TestCompound_SliceContainsAND_GeoDistance(t *testing.T) {
	tr := makeSliceTreasure(t, map[string]interface{}{
		"SiteFunctions": []interface{}{int8(1), int8(7)},
		"geo_latitude":  47.497,
		"geo_longitude": 19.040,
	})
	slicePath := "SiteFunctions"
	sliceF := &hydrapb.TreasureFilter{
		Operator:       hydrapb.Relational_SLICE_CONTAINS,
		BytesFieldPath: &slicePath,
		CompareValue:   &hydrapb.TreasureFilter_Int8Val{Int8Val: 7},
	}
	gf := geoFilter(47.497, 19.040, 50.0, hydrapb.GeoDistanceMode_INSIDE)
	fg := &hydrapb.FilterGroup{
		Logic:              hydrapb.FilterLogic_AND,
		Filters:            []*hydrapb.TreasureFilter{sliceF},
		GeoDistanceFilters: []*hydrapb.GeoDistanceFilter{gf},
	}
	if !evaluateNativeFilterGroup(tr, fg) {
		t.Error("expected slice contains 7 AND geo inside to pass")
	}
}

func TestCompound_SliceContainsOR(t *testing.T) {
	tr := makeSliceTreasure(t, map[string]interface{}{
		"Sectors": []interface{}{int8(6)},
	})
	path := "Sectors"
	f1 := &hydrapb.TreasureFilter{
		Operator:       hydrapb.Relational_SLICE_CONTAINS,
		BytesFieldPath: &path,
		CompareValue:   &hydrapb.TreasureFilter_Int8Val{Int8Val: 1},
	}
	f2 := &hydrapb.TreasureFilter{
		Operator:       hydrapb.Relational_SLICE_CONTAINS,
		BytesFieldPath: &path,
		CompareValue:   &hydrapb.TreasureFilter_Int8Val{Int8Val: 6},
	}
	fg := &hydrapb.FilterGroup{
		Logic:   hydrapb.FilterLogic_OR,
		Filters: []*hydrapb.TreasureFilter{f1, f2},
	}
	if !evaluateNativeFilterGroup(tr, fg) {
		t.Error("expected OR(contains 1, contains 6) to pass when slice has 6")
	}
}

func TestCompound_SliceContainsAND_NotContainsSubstring(t *testing.T) {
	tr := makeSliceTreasure(t, map[string]interface{}{
		"Activities": []interface{}{"custom tattoo design", "body art"},
	})
	actPath := "Activities"
	f1 := &hydrapb.TreasureFilter{
		Operator:       hydrapb.Relational_SLICE_CONTAINS_SUBSTRING,
		BytesFieldPath: &actPath,
		CompareValue:   &hydrapb.TreasureFilter_StringVal{StringVal: "tattoo"},
	}
	f2 := &hydrapb.TreasureFilter{
		Operator:       hydrapb.Relational_SLICE_NOT_CONTAINS_SUBSTRING,
		BytesFieldPath: &actPath,
		CompareValue:   &hydrapb.TreasureFilter_StringVal{StringVal: "permanent makeup"},
	}
	fg := &hydrapb.FilterGroup{
		Logic:   hydrapb.FilterLogic_AND,
		Filters: []*hydrapb.TreasureFilter{f1, f2},
	}
	if !evaluateNativeFilterGroup(tr, fg) {
		t.Error("expected tattoo AND NOT permanent makeup to pass")
	}
}

func TestCompound_SliceLenAND_SliceContains(t *testing.T) {
	tr := makeSliceTreasure(t, map[string]interface{}{
		"Contacts":  []interface{}{"a@b.com"},
		"Providers": []interface{}{"Barion", "PayPal"},
	})
	lenPath := "Contacts.#len"
	provPath := "Providers"
	f1 := &hydrapb.TreasureFilter{
		Operator:       hydrapb.Relational_GREATER_THAN,
		BytesFieldPath: &lenPath,
		CompareValue:   &hydrapb.TreasureFilter_Int32Val{Int32Val: 0},
	}
	f2 := &hydrapb.TreasureFilter{
		Operator:       hydrapb.Relational_SLICE_CONTAINS,
		BytesFieldPath: &provPath,
		CompareValue:   &hydrapb.TreasureFilter_StringVal{StringVal: "Barion"},
	}
	fg := &hydrapb.FilterGroup{
		Logic:   hydrapb.FilterLogic_AND,
		Filters: []*hydrapb.TreasureFilter{f1, f2},
	}
	if !evaluateNativeFilterGroup(tr, fg) {
		t.Error("expected #len > 0 AND contains Barion to pass")
	}
}

func TestCompound_NestedAnyAND_SliceContains(t *testing.T) {
	contactSlice := []interface{}{
		map[string]interface{}{"Email": "john@example.com", "Role": "CEO"},
	}
	tr := makeSliceTreasure(t, map[string]interface{}{
		"Contacts":      contactSlice,
		"SiteFunctions": []interface{}{int8(7)},
	})
	emailPath := "Contacts[*].Email"
	funcPath := "SiteFunctions"
	f1 := &hydrapb.TreasureFilter{
		Operator:       hydrapb.Relational_IS_NOT_EMPTY,
		BytesFieldPath: &emailPath,
		CompareValue:   &hydrapb.TreasureFilter_StringVal{StringVal: ""},
	}
	f2 := &hydrapb.TreasureFilter{
		Operator:       hydrapb.Relational_SLICE_CONTAINS,
		BytesFieldPath: &funcPath,
		CompareValue:   &hydrapb.TreasureFilter_Int8Val{Int8Val: 7},
	}
	fg := &hydrapb.FilterGroup{
		Logic:   hydrapb.FilterLogic_AND,
		Filters: []*hydrapb.TreasureFilter{f1, f2},
	}
	if !evaluateNativeFilterGroup(tr, fg) {
		t.Error("expected nested email IS_NOT_EMPTY AND slice contains 7 to pass")
	}
}

// --- Slice filter benchmarks ---

func BenchmarkSliceContainsInt8_Small(b *testing.B) {
	data := map[string]interface{}{"Funcs": []interface{}{int8(1), int8(3), int8(5), int8(7), int8(9)}}
	encoded, _ := msgpack.Marshal(data)
	bytesVal := append([]byte{msgpackMagic0, msgpackMagic1}, encoded...)
	tr := newTreasureWithBytes(bytesVal)
	path := "Funcs"
	f := &hydrapb.TreasureFilter{
		Operator:       hydrapb.Relational_SLICE_CONTAINS,
		BytesFieldPath: &path,
		CompareValue:   &hydrapb.TreasureFilter_Int8Val{Int8Val: 7},
	}
	for b.Loop() {
		evaluateNativeBytesFieldFilter(tr, f)
	}
}

func BenchmarkSliceContainsString_Small(b *testing.B) {
	data := map[string]interface{}{"P": []interface{}{"Barion", "PayPal", "Stripe", "Square", "Klarna"}}
	encoded, _ := msgpack.Marshal(data)
	bytesVal := append([]byte{msgpackMagic0, msgpackMagic1}, encoded...)
	tr := newTreasureWithBytes(bytesVal)
	path := "P"
	f := &hydrapb.TreasureFilter{
		Operator:       hydrapb.Relational_SLICE_CONTAINS,
		BytesFieldPath: &path,
		CompareValue:   &hydrapb.TreasureFilter_StringVal{StringVal: "Klarna"},
	}
	for b.Loop() {
		evaluateNativeBytesFieldFilter(tr, f)
	}
}

func BenchmarkSliceContainsSubstring(b *testing.B) {
	acts := make([]interface{}, 10)
	for i := range acts {
		acts[i] = "some long activity description number " + string(rune('A'+i))
	}
	acts[7] = "custom tattoo design"
	data := map[string]interface{}{"A": acts}
	encoded, _ := msgpack.Marshal(data)
	bytesVal := append([]byte{msgpackMagic0, msgpackMagic1}, encoded...)
	tr := newTreasureWithBytes(bytesVal)
	path := "A"
	f := &hydrapb.TreasureFilter{
		Operator:       hydrapb.Relational_SLICE_CONTAINS_SUBSTRING,
		BytesFieldPath: &path,
		CompareValue:   &hydrapb.TreasureFilter_StringVal{StringVal: "tattoo"},
	}
	for b.Loop() {
		evaluateNativeBytesFieldFilter(tr, f)
	}
}

func BenchmarkSliceLen(b *testing.B) {
	data := map[string]interface{}{"C": []interface{}{"a", "b", "c"}}
	encoded, _ := msgpack.Marshal(data)
	bytesVal := append([]byte{msgpackMagic0, msgpackMagic1}, encoded...)
	tr := newTreasureWithBytes(bytesVal)
	path := "C.#len"
	f := &hydrapb.TreasureFilter{
		Operator:       hydrapb.Relational_GREATER_THAN,
		BytesFieldPath: &path,
		CompareValue:   &hydrapb.TreasureFilter_Int32Val{Int32Val: 0},
	}
	for b.Loop() {
		evaluateNativeBytesFieldFilter(tr, f)
	}
}

func BenchmarkNestedSliceAny(b *testing.B) {
	contacts := []interface{}{
		map[string]interface{}{"Email": "a@x.com", "Role": "Dev"},
		map[string]interface{}{"Email": "b@x.com", "Role": "CEO"},
		map[string]interface{}{"Email": "", "Role": "CTO"},
	}
	data := map[string]interface{}{"C": contacts}
	encoded, _ := msgpack.Marshal(data)
	bytesVal := append([]byte{msgpackMagic0, msgpackMagic1}, encoded...)
	tr := newTreasureWithBytes(bytesVal)
	path := "C[*].Email"
	f := &hydrapb.TreasureFilter{
		Operator:       hydrapb.Relational_IS_NOT_EMPTY,
		BytesFieldPath: &path,
		CompareValue:   &hydrapb.TreasureFilter_StringVal{StringVal: ""},
	}
	for b.Loop() {
		evaluateNativeBytesFieldFilter(tr, f)
	}
}
