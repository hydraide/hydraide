package msgpackpatch

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestApply_MERGE_Shallow(t *testing.T) {
	blob := mustEncode(t, map[string]any{
		"profile": map[string]any{
			"name": "alice",
			"age":  int8(30),
		},
	})
	patch := mustEncode(t, map[string]any{
		"age":  int8(31),
		"city": "Budapest",
	})
	out, err := Apply(blob, []Op{
		{Kind: OpMerge, Path: "profile", Value: patch},
	})
	require.NoError(t, err)

	got := decodeMap(t, out)
	prof := got["profile"].(map[string]any)
	assert.Equal(t, "alice", prof["name"])
	assert.EqualValues(t, 31, prof["age"])
	assert.Equal(t, "Budapest", prof["city"])
}

func TestApply_MERGE_PreservesNonConflictingTypes(t *testing.T) {
	type Profile struct {
		Name string `msgpack:"name"`
		Age  int8   `msgpack:"age"`
		Code int32  `msgpack:"code"`
	}
	blob := mustEncode(t, map[string]any{
		"profile": Profile{Name: "alice", Age: 30, Code: 70000},
	})
	patch := mustEncode(t, map[string]any{
		"name": "bob",
	})
	out, err := Apply(blob, []Op{
		{Kind: OpMerge, Path: "profile", Value: patch},
	})
	require.NoError(t, err)

	skel, err := Parse(out)
	require.NoError(t, err)
	var profSkel *Skeleton
	for _, f := range skel.MapFields {
		if f.Key == "profile" {
			profSkel = f.Value
		}
	}
	require.NotNil(t, profSkel)
	for _, f := range profSkel.MapFields {
		switch f.Key {
		case "age":
			assert.Equal(t, codeInt8, f.Value.LeafCode, "age must remain int8")
		case "code":
			assert.Equal(t, codeInt32, f.Value.LeafCode, "code must remain int32")
		}
	}
}

func TestApply_MERGE_MissingPathCreatesMap(t *testing.T) {
	blob := mustEncode(t, map[string]any{"name": "alice"})
	patch := mustEncode(t, map[string]any{
		"city": "Budapest",
	})
	out, err := Apply(blob, []Op{
		{Kind: OpMerge, Path: "address", Value: patch},
	})
	require.NoError(t, err)

	got := decodeMap(t, out)
	addr := got["address"].(map[string]any)
	assert.Equal(t, "Budapest", addr["city"])
}

func TestApply_MERGE_NonMapTargetFails(t *testing.T) {
	blob := mustEncode(t, map[string]any{"name": "alice"})
	patch := mustEncode(t, map[string]any{"x": int8(1)})
	_, err := Apply(blob, []Op{
		{Kind: OpMerge, Path: "name", Value: patch},
	})
	assert.ErrorIs(t, err, ErrTypeMismatch)
}

func TestApply_MERGE_NonMapValueFails(t *testing.T) {
	blob := mustEncode(t, map[string]any{
		"profile": map[string]any{"name": "alice"},
	})
	_, err := Apply(blob, []Op{
		{Kind: OpMerge, Path: "profile", Value: encVal(t, "not a map")},
	})
	assert.ErrorIs(t, err, ErrTypeMismatch)
}

func TestApply_MERGE_RequiresValue(t *testing.T) {
	blob := mustEncode(t, map[string]any{
		"profile": map[string]any{"name": "alice"},
	})
	_, err := Apply(blob, []Op{
		{Kind: OpMerge, Path: "profile", Value: nil},
	})
	assert.ErrorIs(t, err, ErrInvalidOp)
}

func TestApply_MERGE_NestedAutoCreate(t *testing.T) {
	blob := mustEncode(t, map[string]any{"name": "alice"})
	patch := mustEncode(t, map[string]any{"city": "Budapest"})
	out, err := Apply(blob, []Op{
		{Kind: OpMerge, Path: "addr.home", Value: patch},
	})
	require.NoError(t, err)

	got := decodeMap(t, out)
	home := got["addr"].(map[string]any)["home"].(map[string]any)
	assert.Equal(t, "Budapest", home["city"])
}
