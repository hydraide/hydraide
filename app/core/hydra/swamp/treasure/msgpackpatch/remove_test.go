package msgpackpatch

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------- REMOVE_AT ----------

func TestApply_REMOVE_AT_ValidIndex(t *testing.T) {
	blob := mustEncode(t, map[string]any{
		"tags": []any{"a", "b", "c", "d"},
	})
	out, err := Apply(blob, []Op{
		{Kind: OpRemoveAt, Path: "tags[1]"},
	})
	require.NoError(t, err)

	got := decodeMap(t, out)
	tags := got["tags"].([]any)
	assert.Equal(t, []any{"a", "c", "d"}, tags)
}

func TestApply_REMOVE_AT_NegativeIndex(t *testing.T) {
	blob := mustEncode(t, map[string]any{
		"tags": []any{"a", "b", "c"},
	})
	out, err := Apply(blob, []Op{
		{Kind: OpRemoveAt, Path: "tags[-1]"},
	})
	require.NoError(t, err)

	got := decodeMap(t, out)
	tags := got["tags"].([]any)
	assert.Equal(t, []any{"a", "b"}, tags)
}

func TestApply_REMOVE_AT_OutOfRangeFails(t *testing.T) {
	blob := mustEncode(t, map[string]any{
		"tags": []any{"a"},
	})
	_, err := Apply(blob, []Op{
		{Kind: OpRemoveAt, Path: "tags[5]"},
	})
	assert.ErrorIs(t, err, ErrPathInvalid)
}

func TestApply_REMOVE_AT_NonArrayFails(t *testing.T) {
	blob := mustEncode(t, map[string]any{"name": "alice"})
	_, err := Apply(blob, []Op{
		{Kind: OpRemoveAt, Path: "name[0]"},
	})
	assert.ErrorIs(t, err, ErrTypeMismatch)
}

func TestApply_REMOVE_AT_RequiresIndex(t *testing.T) {
	blob := mustEncode(t, map[string]any{"tags": []any{"a"}})
	_, err := Apply(blob, []Op{
		{Kind: OpRemoveAt, Path: "tags"},
	})
	assert.ErrorIs(t, err, ErrPathInvalid)
}

// ---------- REMOVE_VAL ----------

func TestApply_REMOVE_VAL_FirstMatch(t *testing.T) {
	blob := mustEncode(t, map[string]any{
		"tags": []any{"a", "b", "a", "c"},
	})
	out, err := Apply(blob, []Op{
		{Kind: OpRemoveVal, Path: "tags", Value: encVal(t, "a")},
	})
	require.NoError(t, err)

	got := decodeMap(t, out)
	tags := got["tags"].([]any)
	// First "a" removed; second "a" kept.
	assert.Equal(t, []any{"b", "a", "c"}, tags)
}

func TestApply_REMOVE_VAL_NotPresentIsNoOp(t *testing.T) {
	blob := mustEncode(t, map[string]any{
		"tags": []any{"a", "b"},
	})
	out, err := Apply(blob, []Op{
		{Kind: OpRemoveVal, Path: "tags", Value: encVal(t, "z")},
	})
	require.NoError(t, err)

	got := decodeMap(t, out)
	tags := got["tags"].([]any)
	assert.Equal(t, []any{"a", "b"}, tags)
}

func TestApply_REMOVE_VAL_PreservesRemainingTypes(t *testing.T) {
	blob := mustEncode(t, map[string]any{
		"nums": []any{int8(1), int8(2), int8(3)},
	})
	out, err := Apply(blob, []Op{
		{Kind: OpRemoveVal, Path: "nums", Value: encVal(t, int8(2))},
	})
	require.NoError(t, err)

	skel, err := Parse(out)
	require.NoError(t, err)
	for _, f := range skel.MapFields {
		if f.Key == "nums" {
			require.Equal(t, KindArray, f.Value.Kind)
			require.Len(t, f.Value.ArrayItems, 2)
			for i, item := range f.Value.ArrayItems {
				assert.Equal(t, codeInt8, item.LeafCode, "item %d should remain int8", i)
			}
		}
	}
}

func TestApply_REMOVE_VAL_NonArrayFails(t *testing.T) {
	blob := mustEncode(t, map[string]any{"name": "alice"})
	_, err := Apply(blob, []Op{
		{Kind: OpRemoveVal, Path: "name", Value: encVal(t, "alice")},
	})
	assert.ErrorIs(t, err, ErrTypeMismatch)
}

func TestApply_REMOVE_VAL_RequiresValue(t *testing.T) {
	blob := mustEncode(t, map[string]any{"tags": []any{"a"}})
	_, err := Apply(blob, []Op{
		{Kind: OpRemoveVal, Path: "tags", Value: nil},
	})
	assert.ErrorIs(t, err, ErrInvalidOp)
}

func TestApply_REMOVE_VAL_MissingFieldIsNoOp(t *testing.T) {
	blob := mustEncode(t, map[string]any{"name": "alice"})
	out, err := Apply(blob, []Op{
		{Kind: OpRemoveVal, Path: "tags", Value: encVal(t, "x")},
	})
	require.NoError(t, err)

	// "tags" was never present; the patch is a no-op.
	got := decodeMap(t, out)
	assert.NotContains(t, got, "tags")
}
