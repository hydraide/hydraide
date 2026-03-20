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
