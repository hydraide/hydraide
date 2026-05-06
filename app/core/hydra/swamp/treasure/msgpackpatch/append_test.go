package msgpackpatch

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------- APPEND ----------

func TestApply_APPEND_ExistingArray(t *testing.T) {
	blob := mustEncode(t, map[string]any{
		"tags": []any{"a", "b"},
	})
	out, err := Apply(blob, []Op{
		{Kind: OpAppend, Path: "tags[]", Value: encVal(t, "c")},
	})
	require.NoError(t, err)

	got := decodeMap(t, out)
	tags := got["tags"].([]any)
	assert.Equal(t, []any{"a", "b", "c"}, tags)
}

func TestApply_APPEND_MissingFieldCreatesArray(t *testing.T) {
	blob := mustEncode(t, map[string]any{"name": "alice"})
	out, err := Apply(blob, []Op{
		{Kind: OpAppend, Path: "tags[]", Value: encVal(t, "x")},
	})
	require.NoError(t, err)

	got := decodeMap(t, out)
	tags := got["tags"].([]any)
	assert.Equal(t, []any{"x"}, tags)
}

func TestApply_APPEND_NestedAutoCreate(t *testing.T) {
	blob := mustEncode(t, map[string]any{"name": "alice"})
	out, err := Apply(blob, []Op{
		{Kind: OpAppend, Path: "stats.events[]", Value: encVal(t, "boot")},
	})
	require.NoError(t, err)

	got := decodeMap(t, out)
	stats := got["stats"].(map[string]any)
	events := stats["events"].([]any)
	assert.Equal(t, []any{"boot"}, events)
}

func TestApply_APPEND_OnNonArrayFails(t *testing.T) {
	blob := mustEncode(t, map[string]any{"name": "alice"})
	_, err := Apply(blob, []Op{
		{Kind: OpAppend, Path: "name[]", Value: encVal(t, "x")},
	})
	assert.ErrorIs(t, err, ErrTypeMismatch)
}

func TestApply_APPEND_PreservesExistingItemTypes(t *testing.T) {
	blob := mustEncode(t, map[string]any{
		"nums": []any{int8(1), int8(2)},
	})
	out, err := Apply(blob, []Op{
		{Kind: OpAppend, Path: "nums[]", Value: encVal(t, int8(3))},
	})
	require.NoError(t, err)

	skel, err := Parse(out)
	require.NoError(t, err)
	for _, f := range skel.MapFields {
		if f.Key == "nums" {
			require.Equal(t, KindArray, f.Value.Kind)
			require.Len(t, f.Value.ArrayItems, 3)
			for i, item := range f.Value.ArrayItems {
				assert.Equal(t, codeInt8, item.LeafCode, "item %d should stay int8", i)
			}
		}
	}
}

func TestApply_APPEND_RequiresValue(t *testing.T) {
	blob := mustEncode(t, map[string]any{"tags": []any{"a"}})
	_, err := Apply(blob, []Op{
		{Kind: OpAppend, Path: "tags[]", Value: nil},
	})
	assert.ErrorIs(t, err, ErrInvalidOp)
}

// ---------- PREPEND ----------

func TestApply_PREPEND_ExistingArray(t *testing.T) {
	blob := mustEncode(t, map[string]any{
		"tags": []any{"b", "c"},
	})
	out, err := Apply(blob, []Op{
		{Kind: OpPrepend, Path: "tags[]", Value: encVal(t, "a")},
	})
	require.NoError(t, err)

	got := decodeMap(t, out)
	tags := got["tags"].([]any)
	assert.Equal(t, []any{"a", "b", "c"}, tags)
}

func TestApply_PREPEND_MissingFieldCreatesArray(t *testing.T) {
	blob := mustEncode(t, map[string]any{})
	out, err := Apply(blob, []Op{
		{Kind: OpPrepend, Path: "tags[]", Value: encVal(t, "first")},
	})
	require.NoError(t, err)

	got := decodeMap(t, out)
	tags := got["tags"].([]any)
	assert.Equal(t, []any{"first"}, tags)
}

func TestApply_PREPEND_OnNonArrayFails(t *testing.T) {
	blob := mustEncode(t, map[string]any{"name": "alice"})
	_, err := Apply(blob, []Op{
		{Kind: OpPrepend, Path: "name[]", Value: encVal(t, "x")},
	})
	assert.ErrorIs(t, err, ErrTypeMismatch)
}
