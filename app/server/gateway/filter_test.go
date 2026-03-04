package gateway

import (
	"testing"
	"time"

	hydrapb "github.com/hydraide/hydraide/generated/hydraidepbgo"
	"github.com/vmihailenco/msgpack/v5"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// --- Helper: create msgpack-encoded BytesVal with magic prefix ---

func makeMsgpackBytesVal(t *testing.T, data map[string]interface{}) []byte {
	t.Helper()
	encoded, err := msgpack.Marshal(data)
	if err != nil {
		t.Fatalf("msgpack.Marshal failed: %v", err)
	}
	return append([]byte{msgpackMagic0, msgpackMagic1}, encoded...)
}

// --- Timestamp Filter Tests ---

func TestCompareTimestamp_AllOperators(t *testing.T) {
	earlier := timestamppb.New(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC))
	middle := timestamppb.New(time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC))
	later := timestamppb.New(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))

	tests := []struct {
		name     string
		actual   *timestamppb.Timestamp
		op       hydrapb.Relational_Operator
		ref      *timestamppb.Timestamp
		expected bool
	}{
		{"equal_same", middle, hydrapb.Relational_EQUAL, middle, true},
		{"equal_different", middle, hydrapb.Relational_EQUAL, later, false},
		{"not_equal_different", middle, hydrapb.Relational_NOT_EQUAL, later, true},
		{"not_equal_same", middle, hydrapb.Relational_NOT_EQUAL, middle, false},
		{"gt_true", later, hydrapb.Relational_GREATER_THAN, earlier, true},
		{"gt_false", earlier, hydrapb.Relational_GREATER_THAN, later, false},
		{"gt_equal", middle, hydrapb.Relational_GREATER_THAN, middle, false},
		{"gte_greater", later, hydrapb.Relational_GREATER_THAN_OR_EQUAL, earlier, true},
		{"gte_equal", middle, hydrapb.Relational_GREATER_THAN_OR_EQUAL, middle, true},
		{"gte_less", earlier, hydrapb.Relational_GREATER_THAN_OR_EQUAL, later, false},
		{"lt_true", earlier, hydrapb.Relational_LESS_THAN, later, true},
		{"lt_false", later, hydrapb.Relational_LESS_THAN, earlier, false},
		{"lt_equal", middle, hydrapb.Relational_LESS_THAN, middle, false},
		{"lte_less", earlier, hydrapb.Relational_LESS_THAN_OR_EQUAL, later, true},
		{"lte_equal", middle, hydrapb.Relational_LESS_THAN_OR_EQUAL, middle, true},
		{"lte_greater", later, hydrapb.Relational_LESS_THAN_OR_EQUAL, earlier, false},
		{"nil_actual", nil, hydrapb.Relational_EQUAL, middle, false},
		{"nil_ref", middle, hydrapb.Relational_EQUAL, nil, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := compareTimestamp(tt.actual, tt.op, tt.ref)
			if result != tt.expected {
				t.Errorf("compareTimestamp() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestTimestampFilter_CreatedAt(t *testing.T) {
	now := time.Now()
	cutoff := now.Add(-24 * time.Hour)

	treasure := &hydrapb.Treasure{
		Key:       "test1",
		CreatedAt: timestamppb.New(now),
	}

	filter := &hydrapb.TreasureFilter{
		Operator: hydrapb.Relational_GREATER_THAN,
		CompareValue: &hydrapb.TreasureFilter_CreatedAtVal{
			CreatedAtVal: timestamppb.New(cutoff),
		},
	}

	if !evaluateSingleFilter(treasure, filter) {
		t.Error("expected CreatedAt filter to match (now > 24h ago)")
	}

	// Should not match with LT
	filter.Operator = hydrapb.Relational_LESS_THAN
	if evaluateSingleFilter(treasure, filter) {
		t.Error("expected CreatedAt filter NOT to match (now < 24h ago)")
	}
}

func TestTimestampFilter_UpdatedAt(t *testing.T) {
	ts := time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC)
	treasure := &hydrapb.Treasure{
		Key:       "test2",
		UpdatedAt: timestamppb.New(ts),
	}

	filter := &hydrapb.TreasureFilter{
		Operator: hydrapb.Relational_EQUAL,
		CompareValue: &hydrapb.TreasureFilter_UpdatedAtVal{
			UpdatedAtVal: timestamppb.New(ts),
		},
	}

	if !evaluateSingleFilter(treasure, filter) {
		t.Error("expected UpdatedAt filter to match (same timestamp)")
	}
}

func TestTimestampFilter_ExpiredAt(t *testing.T) {
	past := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	now := time.Now()

	treasure := &hydrapb.Treasure{
		Key:       "test3",
		ExpiredAt: timestamppb.New(past),
	}

	filter := &hydrapb.TreasureFilter{
		Operator: hydrapb.Relational_LESS_THAN,
		CompareValue: &hydrapb.TreasureFilter_ExpiredAtVal{
			ExpiredAtVal: timestamppb.New(now),
		},
	}

	if !evaluateSingleFilter(treasure, filter) {
		t.Error("expected ExpiredAt filter to match (past < now)")
	}
}

func TestTimestampFilter_IsEmpty(t *testing.T) {
	// Treasure with no ExpiredAt
	treasure := &hydrapb.Treasure{
		Key:       "no-expiry",
		CreatedAt: timestamppb.Now(),
	}

	filter := &hydrapb.TreasureFilter{
		Operator: hydrapb.Relational_IS_EMPTY,
		CompareValue: &hydrapb.TreasureFilter_ExpiredAtVal{
			ExpiredAtVal: timestamppb.Now(),
		},
	}

	if !evaluateSingleFilter(treasure, filter) {
		t.Error("expected IS_EMPTY to be true for nil ExpiredAt")
	}

	// CreatedAt IS_NOT_EMPTY
	filter2 := &hydrapb.TreasureFilter{
		Operator: hydrapb.Relational_IS_NOT_EMPTY,
		CompareValue: &hydrapb.TreasureFilter_CreatedAtVal{
			CreatedAtVal: timestamppb.Now(),
		},
	}

	if !evaluateSingleFilter(treasure, filter2) {
		t.Error("expected IS_NOT_EMPTY to be true for non-nil CreatedAt")
	}
}

func TestTimestampFilter_NilTreasureTimestamp(t *testing.T) {
	treasure := &hydrapb.Treasure{Key: "empty"}

	filter := &hydrapb.TreasureFilter{
		Operator: hydrapb.Relational_GREATER_THAN,
		CompareValue: &hydrapb.TreasureFilter_CreatedAtVal{
			CreatedAtVal: timestamppb.Now(),
		},
	}

	if evaluateSingleFilter(treasure, filter) {
		t.Error("expected filter to NOT match when treasure CreatedAt is nil")
	}
}

// --- HAS_KEY / HAS_NOT_KEY Tests ---

func TestHasKey_KeyExists(t *testing.T) {
	treasure := &hydrapb.Treasure{
		Key: "user1",
		BytesVal: makeMsgpackBytesVal(t, map[string]interface{}{
			"UserData": map[string]interface{}{
				"email": "user@example.com",
				"name":  "John",
			},
		}),
	}

	fieldPath := "UserData"
	filter := &hydrapb.TreasureFilter{
		Operator:       hydrapb.Relational_HAS_KEY,
		CompareValue:   &hydrapb.TreasureFilter_StringVal{StringVal: "email"},
		BytesFieldPath: &fieldPath,
	}

	if !evaluateSingleFilter(treasure, filter) {
		t.Error("expected HAS_KEY to match for existing key 'email'")
	}
}

func TestHasKey_KeyNotExists(t *testing.T) {
	treasure := &hydrapb.Treasure{
		Key: "user2",
		BytesVal: makeMsgpackBytesVal(t, map[string]interface{}{
			"UserData": map[string]interface{}{
				"name": "John",
			},
		}),
	}

	fieldPath := "UserData"
	filter := &hydrapb.TreasureFilter{
		Operator:       hydrapb.Relational_HAS_KEY,
		CompareValue:   &hydrapb.TreasureFilter_StringVal{StringVal: "email"},
		BytesFieldPath: &fieldPath,
	}

	if evaluateSingleFilter(treasure, filter) {
		t.Error("expected HAS_KEY NOT to match for non-existing key 'email'")
	}
}

func TestHasNotKey_KeyExists(t *testing.T) {
	treasure := &hydrapb.Treasure{
		Key: "user3",
		BytesVal: makeMsgpackBytesVal(t, map[string]interface{}{
			"UserData": map[string]interface{}{
				"email": "user@example.com",
			},
		}),
	}

	fieldPath := "UserData"
	filter := &hydrapb.TreasureFilter{
		Operator:       hydrapb.Relational_HAS_NOT_KEY,
		CompareValue:   &hydrapb.TreasureFilter_StringVal{StringVal: "email"},
		BytesFieldPath: &fieldPath,
	}

	if evaluateSingleFilter(treasure, filter) {
		t.Error("expected HAS_NOT_KEY NOT to match for existing key 'email'")
	}
}

func TestHasNotKey_KeyNotExists(t *testing.T) {
	treasure := &hydrapb.Treasure{
		Key: "user4",
		BytesVal: makeMsgpackBytesVal(t, map[string]interface{}{
			"UserData": map[string]interface{}{
				"name": "John",
			},
		}),
	}

	fieldPath := "UserData"
	filter := &hydrapb.TreasureFilter{
		Operator:       hydrapb.Relational_HAS_NOT_KEY,
		CompareValue:   &hydrapb.TreasureFilter_StringVal{StringVal: "email"},
		BytesFieldPath: &fieldPath,
	}

	if !evaluateSingleFilter(treasure, filter) {
		t.Error("expected HAS_NOT_KEY to match for non-existing key 'email'")
	}
}

func TestHasKey_NotAMap(t *testing.T) {
	treasure := &hydrapb.Treasure{
		Key: "user5",
		BytesVal: makeMsgpackBytesVal(t, map[string]interface{}{
			"UserData": "not-a-map",
		}),
	}

	fieldPath := "UserData"
	filter := &hydrapb.TreasureFilter{
		Operator:       hydrapb.Relational_HAS_KEY,
		CompareValue:   &hydrapb.TreasureFilter_StringVal{StringVal: "email"},
		BytesFieldPath: &fieldPath,
	}

	if evaluateSingleFilter(treasure, filter) {
		t.Error("expected HAS_KEY NOT to match when field is not a map")
	}
}

func TestHasKey_NilBytesVal(t *testing.T) {
	treasure := &hydrapb.Treasure{
		Key:      "user6",
		BytesVal: nil,
	}

	fieldPath := "UserData"
	filter := &hydrapb.TreasureFilter{
		Operator:       hydrapb.Relational_HAS_KEY,
		CompareValue:   &hydrapb.TreasureFilter_StringVal{StringVal: "email"},
		BytesFieldPath: &fieldPath,
	}

	if evaluateSingleFilter(treasure, filter) {
		t.Error("expected HAS_KEY NOT to match when BytesVal is nil")
	}
}

func TestHasNotKey_NilBytesVal(t *testing.T) {
	treasure := &hydrapb.Treasure{
		Key:      "user7",
		BytesVal: nil,
	}

	fieldPath := "UserData"
	filter := &hydrapb.TreasureFilter{
		Operator:       hydrapb.Relational_HAS_NOT_KEY,
		CompareValue:   &hydrapb.TreasureFilter_StringVal{StringVal: "email"},
		BytesFieldPath: &fieldPath,
	}

	// When BytesVal is nil, evaluateBytesFieldFilter falls through to
	// the IS_EMPTY check: op == HAS_NOT_KEY != IS_EMPTY → false
	// This is correct: we can't determine key non-existence without data
	if evaluateSingleFilter(treasure, filter) {
		t.Error("expected HAS_NOT_KEY NOT to match when BytesVal is nil (no data to inspect)")
	}
}

// --- PhraseFilter Tests ---

func TestPhraseFilter_ConsecutiveWords(t *testing.T) {
	// "altalanos szerzodesi feltetelek" at positions 5,6,7
	treasure := &hydrapb.Treasure{
		Key: "doc1",
		BytesVal: makeMsgpackBytesVal(t, map[string]interface{}{
			"WordIndex": map[string]interface{}{
				"altalanos":   []interface{}{int64(5), int64(20)},
				"szerzodesi":  []interface{}{int64(6), int64(30)},
				"feltetelek":  []interface{}{int64(7), int64(40)},
			},
		}),
	}

	pf := &hydrapb.PhraseFilter{
		BytesFieldPath: "WordIndex",
		Words:          []string{"altalanos", "szerzodesi", "feltetelek"},
		Negate:         false,
	}

	if !evaluatePhraseFilter(treasure, pf) {
		t.Error("expected phrase filter to match consecutive positions 5,6,7")
	}
}

func TestPhraseFilter_NonConsecutiveWords(t *testing.T) {
	// Words exist but not at consecutive positions
	treasure := &hydrapb.Treasure{
		Key: "doc2",
		BytesVal: makeMsgpackBytesVal(t, map[string]interface{}{
			"WordIndex": map[string]interface{}{
				"altalanos":   []interface{}{int64(5), int64(20)},
				"szerzodesi":  []interface{}{int64(8), int64(30)},  // gap: 5→8 (not 6)
				"feltetelek":  []interface{}{int64(12), int64(40)}, // gap: 8→12 (not 9)
			},
		}),
	}

	pf := &hydrapb.PhraseFilter{
		BytesFieldPath: "WordIndex",
		Words:          []string{"altalanos", "szerzodesi", "feltetelek"},
		Negate:         false,
	}

	if evaluatePhraseFilter(treasure, pf) {
		t.Error("expected phrase filter NOT to match non-consecutive positions")
	}
}

func TestPhraseFilter_Negated(t *testing.T) {
	// Consecutive words found, but Negate=true → should NOT match
	treasure := &hydrapb.Treasure{
		Key: "doc3",
		BytesVal: makeMsgpackBytesVal(t, map[string]interface{}{
			"WordIndex": map[string]interface{}{
				"hello": []interface{}{int64(1)},
				"world": []interface{}{int64(2)},
			},
		}),
	}

	pf := &hydrapb.PhraseFilter{
		BytesFieldPath: "WordIndex",
		Words:          []string{"hello", "world"},
		Negate:         true,
	}

	if evaluatePhraseFilter(treasure, pf) {
		t.Error("expected negated phrase filter NOT to match when phrase is found")
	}
}

func TestPhraseFilter_NegatedNoMatch(t *testing.T) {
	// Non-consecutive words + Negate=true → SHOULD match
	treasure := &hydrapb.Treasure{
		Key: "doc4",
		BytesVal: makeMsgpackBytesVal(t, map[string]interface{}{
			"WordIndex": map[string]interface{}{
				"hello": []interface{}{int64(1)},
				"world": []interface{}{int64(5)}, // not consecutive
			},
		}),
	}

	pf := &hydrapb.PhraseFilter{
		BytesFieldPath: "WordIndex",
		Words:          []string{"hello", "world"},
		Negate:         true,
	}

	if !evaluatePhraseFilter(treasure, pf) {
		t.Error("expected negated phrase filter to match when phrase is NOT found")
	}
}

func TestPhraseFilter_SingleWord(t *testing.T) {
	treasure := &hydrapb.Treasure{
		Key: "doc5",
		BytesVal: makeMsgpackBytesVal(t, map[string]interface{}{
			"WordIndex": map[string]interface{}{
				"hello": []interface{}{int64(1), int64(5), int64(10)},
			},
		}),
	}

	pf := &hydrapb.PhraseFilter{
		BytesFieldPath: "WordIndex",
		Words:          []string{"hello"},
		Negate:         false,
	}

	if !evaluatePhraseFilter(treasure, pf) {
		t.Error("expected single-word phrase filter to match")
	}
}

func TestPhraseFilter_MissingWord(t *testing.T) {
	treasure := &hydrapb.Treasure{
		Key: "doc6",
		BytesVal: makeMsgpackBytesVal(t, map[string]interface{}{
			"WordIndex": map[string]interface{}{
				"hello": []interface{}{int64(1)},
			},
		}),
	}

	pf := &hydrapb.PhraseFilter{
		BytesFieldPath: "WordIndex",
		Words:          []string{"hello", "missing"},
		Negate:         false,
	}

	if evaluatePhraseFilter(treasure, pf) {
		t.Error("expected phrase filter NOT to match when a word is missing from index")
	}
}

func TestPhraseFilter_MultipleOccurrences(t *testing.T) {
	// "hello world" appears at positions (3,4) even though there are other occurrences
	treasure := &hydrapb.Treasure{
		Key: "doc7",
		BytesVal: makeMsgpackBytesVal(t, map[string]interface{}{
			"WordIndex": map[string]interface{}{
				"hello": []interface{}{int64(1), int64(3), int64(10)},
				"world": []interface{}{int64(4), int64(8), int64(15)},
			},
		}),
	}

	pf := &hydrapb.PhraseFilter{
		BytesFieldPath: "WordIndex",
		Words:          []string{"hello", "world"},
		Negate:         false,
	}

	if !evaluatePhraseFilter(treasure, pf) {
		t.Error("expected phrase filter to find consecutive occurrence at positions 3,4")
	}
}

func TestPhraseFilter_EmptyPositions(t *testing.T) {
	treasure := &hydrapb.Treasure{
		Key: "doc8",
		BytesVal: makeMsgpackBytesVal(t, map[string]interface{}{
			"WordIndex": map[string]interface{}{
				"hello": []interface{}{},
				"world": []interface{}{int64(1)},
			},
		}),
	}

	pf := &hydrapb.PhraseFilter{
		BytesFieldPath: "WordIndex",
		Words:          []string{"hello", "world"},
		Negate:         false,
	}

	if evaluatePhraseFilter(treasure, pf) {
		t.Error("expected phrase filter NOT to match when a word has empty position list")
	}
}

func TestPhraseFilter_EmptyWords(t *testing.T) {
	treasure := &hydrapb.Treasure{
		Key:      "doc9",
		BytesVal: makeMsgpackBytesVal(t, map[string]interface{}{"WordIndex": map[string]interface{}{}}),
	}

	pf := &hydrapb.PhraseFilter{
		BytesFieldPath: "WordIndex",
		Words:          []string{},
		Negate:         false,
	}

	if !evaluatePhraseFilter(treasure, pf) {
		t.Error("expected empty words phrase filter to pass (vacuously true)")
	}
}

func TestPhraseFilter_NilBytesVal(t *testing.T) {
	treasure := &hydrapb.Treasure{Key: "doc10"}

	pf := &hydrapb.PhraseFilter{
		BytesFieldPath: "WordIndex",
		Words:          []string{"hello"},
		Negate:         false,
	}

	if evaluatePhraseFilter(treasure, pf) {
		t.Error("expected phrase filter NOT to match when BytesVal is nil")
	}
}

func TestPhraseFilter_NilBytesValNegated(t *testing.T) {
	treasure := &hydrapb.Treasure{Key: "doc11"}

	pf := &hydrapb.PhraseFilter{
		BytesFieldPath: "WordIndex",
		Words:          []string{"hello"},
		Negate:         true,
	}

	if !evaluatePhraseFilter(treasure, pf) {
		t.Error("expected negated phrase filter to match when BytesVal is nil (phrase can't exist)")
	}
}

// --- FilterGroup integration tests ---

func TestPhraseFilter_InFilterGroup_AND(t *testing.T) {
	treasure := &hydrapb.Treasure{
		Key:      "doc-and",
		Int32Val: int32Ptr(42),
		BytesVal: makeMsgpackBytesVal(t, map[string]interface{}{
			"WordIndex": map[string]interface{}{
				"hello": []interface{}{int64(1)},
				"world": []interface{}{int64(2)},
			},
		}),
	}

	group := &hydrapb.FilterGroup{
		Logic: hydrapb.FilterLogic_AND,
		Filters: []*hydrapb.TreasureFilter{
			{
				Operator:     hydrapb.Relational_EQUAL,
				CompareValue: &hydrapb.TreasureFilter_Int32Val{Int32Val: 42},
			},
		},
		PhraseFilters: []*hydrapb.PhraseFilter{
			{
				BytesFieldPath: "WordIndex",
				Words:          []string{"hello", "world"},
			},
		},
	}

	if !evaluateFilterGroup(treasure, group) {
		t.Error("expected AND group to match (Int32Val==42 AND phrase found)")
	}

	// Change Int32Val to not match
	treasure.Int32Val = int32Ptr(99)
	if evaluateFilterGroup(treasure, group) {
		t.Error("expected AND group NOT to match (Int32Val!=42)")
	}
}

func TestPhraseFilter_InFilterGroup_OR(t *testing.T) {
	treasure := &hydrapb.Treasure{
		Key:      "doc-or",
		Int32Val: int32Ptr(99), // doesn't match filter
		BytesVal: makeMsgpackBytesVal(t, map[string]interface{}{
			"WordIndex": map[string]interface{}{
				"hello": []interface{}{int64(1)},
				"world": []interface{}{int64(2)},
			},
		}),
	}

	group := &hydrapb.FilterGroup{
		Logic: hydrapb.FilterLogic_OR,
		Filters: []*hydrapb.TreasureFilter{
			{
				Operator:     hydrapb.Relational_EQUAL,
				CompareValue: &hydrapb.TreasureFilter_Int32Val{Int32Val: 42},
			},
		},
		PhraseFilters: []*hydrapb.PhraseFilter{
			{
				BytesFieldPath: "WordIndex",
				Words:          []string{"hello", "world"},
			},
		},
	}

	if !evaluateFilterGroup(treasure, group) {
		t.Error("expected OR group to match (Int32Val!=42 but phrase found)")
	}
}

// --- Helper functions ---

func int32Ptr(v int32) *int32 { return &v }
