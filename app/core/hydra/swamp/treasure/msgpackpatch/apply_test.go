package msgpackpatch

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vmihailenco/msgpack/v5"
)

// decodeMap unmarshals a msgpack blob into map[string]any for assertions.
func decodeMap(t *testing.T, blob []byte) map[string]any {
	t.Helper()
	var got map[string]any
	require.NoError(t, msgpack.Unmarshal(blob, &got))
	return got
}

// encVal marshals one value into a stand-alone msgpack blob (the wire form
// used for Op.Value).
func encVal(t *testing.T, v any) []byte {
	t.Helper()
	b, err := msgpack.Marshal(v)
	require.NoError(t, err)
	return b
}

// ---------- SET ----------

func TestApply_SET_TopLevelExisting(t *testing.T) {
	blob := mustEncode(t, map[string]any{"name": "alice", "age": int8(30)})
	out, err := Apply(blob, []Op{
		{Kind: OpSet, Path: "name", Value: encVal(t, "bob")},
	})
	require.NoError(t, err)

	got := decodeMap(t, out)
	assert.Equal(t, "bob", got["name"])
	// age preserved exactly (int8 stays int8).
	assert.EqualValues(t, 30, got["age"])
}

func TestApply_SET_TopLevelNew(t *testing.T) {
	blob := mustEncode(t, map[string]any{"name": "alice"})
	out, err := Apply(blob, []Op{
		{Kind: OpSet, Path: "city", Value: encVal(t, "Budapest")},
	})
	require.NoError(t, err)

	got := decodeMap(t, out)
	assert.Equal(t, "alice", got["name"])
	assert.Equal(t, "Budapest", got["city"])
	assert.Len(t, got, 2)
}

func TestApply_SET_NestedExisting(t *testing.T) {
	blob := mustEncode(t, map[string]any{
		"address": map[string]any{
			"city":   "Budapest",
			"street": "Andrassy",
		},
	})
	out, err := Apply(blob, []Op{
		{Kind: OpSet, Path: "address.city", Value: encVal(t, "Debrecen")},
	})
	require.NoError(t, err)

	got := decodeMap(t, out)
	addr := got["address"].(map[string]any)
	assert.Equal(t, "Debrecen", addr["city"])
	assert.Equal(t, "Andrassy", addr["street"])
}

func TestApply_SET_NestedAutoCreate(t *testing.T) {
	blob := mustEncode(t, map[string]any{"name": "alice"})
	out, err := Apply(blob, []Op{
		{Kind: OpSet, Path: "address.city.zip", Value: encVal(t, "1011")},
	})
	require.NoError(t, err)

	got := decodeMap(t, out)
	assert.Equal(t, "alice", got["name"])
	addr := got["address"].(map[string]any)
	city := addr["city"].(map[string]any)
	assert.Equal(t, "1011", city["zip"])
}

func TestApply_SET_PreservesOtherFieldTypes(t *testing.T) {
	type Payload struct {
		I8  int8    `msgpack:"i8"`
		I64 int64   `msgpack:"i64"`
		F64 float64 `msgpack:"f64"`
		S   string  `msgpack:"s"`
		B   bool    `msgpack:"b"`
	}
	original := Payload{I8: -1, I64: -5_000_000_000, F64: 3.14159, S: "hi", B: true}
	blob := mustEncode(t, original)

	// Patch only the string field.
	out, err := Apply(blob, []Op{
		{Kind: OpSet, Path: "s", Value: encVal(t, "ho")},
	})
	require.NoError(t, err)

	var decoded Payload
	require.NoError(t, msgpack.Unmarshal(out, &decoded))
	assert.Equal(t, "ho", decoded.S)
	assert.Equal(t, original.I8, decoded.I8)
	assert.Equal(t, original.I64, decoded.I64)
	assert.Equal(t, original.F64, decoded.F64)
	assert.Equal(t, original.B, decoded.B)
}

func TestApply_SET_ArrayIndexExisting(t *testing.T) {
	blob := mustEncode(t, map[string]any{
		"tags": []any{"a", "b", "c"},
	})
	out, err := Apply(blob, []Op{
		{Kind: OpSet, Path: "tags[1]", Value: encVal(t, "X")},
	})
	require.NoError(t, err)

	got := decodeMap(t, out)
	tags := got["tags"].([]any)
	assert.Equal(t, []any{"a", "X", "c"}, tags)
}

func TestApply_SET_ReplacesContainerWithLeaf(t *testing.T) {
	blob := mustEncode(t, map[string]any{
		"address": map[string]any{"city": "Budapest"},
	})
	// Replace the whole address subtree with a string.
	out, err := Apply(blob, []Op{
		{Kind: OpSet, Path: "address", Value: encVal(t, "n/a")},
	})
	require.NoError(t, err)

	got := decodeMap(t, out)
	assert.Equal(t, "n/a", got["address"])
}

func TestApply_SET_AutoCreateOnArrayIndex(t *testing.T) {
	// SET on a missing array index is invalid (can't sparse-index).
	blob := mustEncode(t, map[string]any{"name": "alice"})
	_, err := Apply(blob, []Op{
		{Kind: OpSet, Path: "tags[3]", Value: encVal(t, "x")},
	})
	assert.ErrorIs(t, err, ErrPathInvalid)
}

func TestApply_SET_RequiresValue(t *testing.T) {
	blob := mustEncode(t, map[string]any{"a": int8(1)})
	_, err := Apply(blob, []Op{
		{Kind: OpSet, Path: "a", Value: nil},
	})
	assert.ErrorIs(t, err, ErrInvalidOp)
}

// ---------- DELETE ----------

func TestApply_DELETE_TopLevelExisting(t *testing.T) {
	blob := mustEncode(t, map[string]any{
		"a": int8(1), "b": int8(2), "c": int8(3),
	})
	out, err := Apply(blob, []Op{
		{Kind: OpDelete, Path: "b"},
	})
	require.NoError(t, err)

	got := decodeMap(t, out)
	assert.Len(t, got, 2)
	assert.NotContains(t, got, "b")
	assert.EqualValues(t, 1, got["a"])
	assert.EqualValues(t, 3, got["c"])
}

func TestApply_DELETE_NestedField(t *testing.T) {
	blob := mustEncode(t, map[string]any{
		"address": map[string]any{
			"city":   "Budapest",
			"street": "Andrassy",
		},
	})
	out, err := Apply(blob, []Op{
		{Kind: OpDelete, Path: "address.city"},
	})
	require.NoError(t, err)

	got := decodeMap(t, out)
	addr := got["address"].(map[string]any)
	assert.NotContains(t, addr, "city")
	assert.Equal(t, "Andrassy", addr["street"])
}

func TestApply_DELETE_MissingFieldIsNoOp(t *testing.T) {
	blob := mustEncode(t, map[string]any{"a": int8(1)})
	out, err := Apply(blob, []Op{
		{Kind: OpDelete, Path: "missing"},
	})
	require.NoError(t, err)

	got := decodeMap(t, out)
	assert.EqualValues(t, 1, got["a"])
	assert.Len(t, got, 1)
}

func TestApply_DELETE_ArrayIndex(t *testing.T) {
	blob := mustEncode(t, map[string]any{
		"tags": []any{"a", "b", "c", "d"},
	})
	out, err := Apply(blob, []Op{
		{Kind: OpDelete, Path: "tags[1]"},
	})
	require.NoError(t, err)

	got := decodeMap(t, out)
	tags := got["tags"].([]any)
	assert.Equal(t, []any{"a", "c", "d"}, tags)
}

func TestApply_DELETE_EntireNestedMap(t *testing.T) {
	blob := mustEncode(t, map[string]any{
		"keep":   "yes",
		"remove": map[string]any{"x": int8(1), "y": int8(2)},
	})
	out, err := Apply(blob, []Op{
		{Kind: OpDelete, Path: "remove"},
	})
	require.NoError(t, err)

	got := decodeMap(t, out)
	assert.NotContains(t, got, "remove")
	assert.Equal(t, "yes", got["keep"])
}

// ---------- Atomicity ----------

func TestApply_AllOrNothing_OnFailure(t *testing.T) {
	blob := mustEncode(t, map[string]any{"a": int8(1)})
	original := bytes.Clone(blob)

	// Two ops; the second one is invalid (out-of-range array index on a non-array).
	_, err := Apply(blob, []Op{
		{Kind: OpSet, Path: "a", Value: encVal(t, int8(99))},
		{Kind: OpSet, Path: "a[5]", Value: encVal(t, "x")}, // type mismatch
	})
	require.Error(t, err)

	// Original blob must not have been mutated in place.
	assert.True(t, bytes.Equal(blob, original), "input blob must not be mutated")
}

func TestApply_NoOps_ReturnsCopy(t *testing.T) {
	blob := mustEncode(t, map[string]any{"a": int8(1)})
	out, err := Apply(blob, nil)
	require.NoError(t, err)
	// Round-trip through Parse+Serialize should yield equivalent decoded map.
	assert.Equal(t, decodeMap(t, blob), decodeMap(t, out))
}
