package msgpackpatch

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------- Multi-op atomicity ----------

func TestApply_MultiOp_AppliedInOrder(t *testing.T) {
	blob := mustEncode(t, map[string]any{
		"counter": int32(10),
		"tags":    []any{"a"},
	})
	out, err := Apply(blob, []Op{
		{Kind: OpInc, Path: "counter", Value: encVal(t, int32(5))},
		{Kind: OpAppend, Path: "tags[]", Value: encVal(t, "b")},
		{Kind: OpSet, Path: "status", Value: encVal(t, "ok")},
	})
	require.NoError(t, err)

	got := decodeMap(t, out)
	assert.EqualValues(t, 15, got["counter"])
	assert.Equal(t, []any{"a", "b"}, got["tags"])
	assert.Equal(t, "ok", got["status"])
}

func TestApply_MultiOp_FailureMidwayDiscardsAll(t *testing.T) {
	blob := mustEncode(t, map[string]any{
		"counter": int32(10),
		"name":    "alice",
	})
	original := bytes.Clone(blob)

	// Second op fails (INC on string field) — must not commit the first SET.
	_, err := Apply(blob, []Op{
		{Kind: OpSet, Path: "counter", Value: encVal(t, int32(99))},
		{Kind: OpInc, Path: "name", Value: encVal(t, int8(1))},
	})
	require.Error(t, err)
	assert.True(t, bytes.Equal(blob, original), "input blob must not be mutated")
}

// ---------- ApplyWithCondition ----------

func TestApplyWithCondition_NilConditionAlwaysApplies(t *testing.T) {
	blob := mustEncode(t, map[string]any{"a": int8(1)})
	out, err := ApplyWithCondition(blob, []Op{
		{Kind: OpSet, Path: "a", Value: encVal(t, int8(2))},
	}, nil)
	require.NoError(t, err)
	got := decodeMap(t, out)
	assert.EqualValues(t, 2, got["a"])
}

func TestApplyWithCondition_EqualMet(t *testing.T) {
	blob := mustEncode(t, map[string]any{"owner": "alice", "n": int8(0)})
	out, err := ApplyWithCondition(blob, []Op{
		{Kind: OpSet, Path: "n", Value: encVal(t, int8(5))},
	}, &Condition{Path: "owner", Op: CondEqual, Threshold: encVal(t, "alice")})
	require.NoError(t, err)
	got := decodeMap(t, out)
	assert.EqualValues(t, 5, got["n"])
}

func TestApplyWithCondition_EqualNotMet(t *testing.T) {
	blob := mustEncode(t, map[string]any{"owner": "alice", "n": int8(0)})
	original := bytes.Clone(blob)
	_, err := ApplyWithCondition(blob, []Op{
		{Kind: OpSet, Path: "n", Value: encVal(t, int8(5))},
	}, &Condition{Path: "owner", Op: CondEqual, Threshold: encVal(t, "bob")})
	assert.ErrorIs(t, err, ErrConditionNotMet)
	assert.True(t, bytes.Equal(blob, original))
}

func TestApplyWithCondition_NotEqual(t *testing.T) {
	blob := mustEncode(t, map[string]any{"owner": "alice"})

	_, err := ApplyWithCondition(blob, []Op{
		{Kind: OpSet, Path: "owner", Value: encVal(t, "bob")},
	}, &Condition{Path: "owner", Op: CondNotEqual, Threshold: encVal(t, "alice")})
	assert.ErrorIs(t, err, ErrConditionNotMet)

	_, err = ApplyWithCondition(blob, []Op{
		{Kind: OpSet, Path: "owner", Value: encVal(t, "bob")},
	}, &Condition{Path: "owner", Op: CondNotEqual, Threshold: encVal(t, "carol")})
	require.NoError(t, err)
}

func TestApplyWithCondition_NumericComparators(t *testing.T) {
	blob := mustEncode(t, map[string]any{"v": int32(10)})

	cases := []struct {
		name   string
		op     CondOp
		thr    int32
		expect bool
	}{
		{"GT met", CondGreaterThan, 5, true},
		{"GT not met (equal)", CondGreaterThan, 10, false},
		{"GT not met (less)", CondGreaterThan, 20, false},
		{"GTE met (greater)", CondGreaterThanOrEqual, 5, true},
		{"GTE met (equal)", CondGreaterThanOrEqual, 10, true},
		{"GTE not met", CondGreaterThanOrEqual, 11, false},
		{"LT met", CondLessThan, 20, true},
		{"LT not met (equal)", CondLessThan, 10, false},
		{"LTE met (equal)", CondLessThanOrEqual, 10, true},
		{"LTE not met", CondLessThanOrEqual, 9, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ApplyWithCondition(blob, []Op{
				{Kind: OpSet, Path: "v", Value: encVal(t, int32(99))},
			}, &Condition{Path: "v", Op: tc.op, Threshold: encVal(t, tc.thr)})
			if tc.expect {
				assert.NoError(t, err)
			} else {
				assert.ErrorIs(t, err, ErrConditionNotMet)
			}
		})
	}
}

func TestApplyWithCondition_Exists(t *testing.T) {
	blob := mustEncode(t, map[string]any{"a": int8(1)})

	_, err := ApplyWithCondition(blob, []Op{
		{Kind: OpSet, Path: "a", Value: encVal(t, int8(2))},
	}, &Condition{Path: "a", Op: CondExists})
	require.NoError(t, err)

	_, err = ApplyWithCondition(blob, []Op{
		{Kind: OpSet, Path: "missing", Value: encVal(t, int8(1))},
	}, &Condition{Path: "nope", Op: CondExists})
	assert.ErrorIs(t, err, ErrConditionNotMet)
}

func TestApplyWithCondition_NotExists(t *testing.T) {
	blob := mustEncode(t, map[string]any{"a": int8(1)})

	_, err := ApplyWithCondition(blob, []Op{
		{Kind: OpSet, Path: "b", Value: encVal(t, int8(2))},
	}, &Condition{Path: "b", Op: CondNotExists})
	require.NoError(t, err)

	_, err = ApplyWithCondition(blob, []Op{
		{Kind: OpSet, Path: "a", Value: encVal(t, int8(2))},
	}, &Condition{Path: "a", Op: CondNotExists})
	assert.ErrorIs(t, err, ErrConditionNotMet)
}

func TestApplyWithCondition_NestedPath(t *testing.T) {
	blob := mustEncode(t, map[string]any{
		"profile": map[string]any{
			"role": "admin",
		},
	})
	_, err := ApplyWithCondition(blob, []Op{
		{Kind: OpSet, Path: "profile.role", Value: encVal(t, "user")},
	}, &Condition{Path: "profile.role", Op: CondEqual, Threshold: encVal(t, "admin")})
	require.NoError(t, err)
}

func TestApplyWithCondition_TypeMismatchOnComparator(t *testing.T) {
	// GT against a non-numeric field is a type mismatch.
	blob := mustEncode(t, map[string]any{"name": "alice"})
	_, err := ApplyWithCondition(blob, []Op{
		{Kind: OpSet, Path: "name", Value: encVal(t, "bob")},
	}, &Condition{Path: "name", Op: CondGreaterThan, Threshold: encVal(t, int8(1))})
	assert.ErrorIs(t, err, ErrTypeMismatch)
}
