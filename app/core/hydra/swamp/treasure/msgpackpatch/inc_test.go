package msgpackpatch

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vmihailenco/msgpack/v5"
)

// rawTypeCode returns the leading msgpack type code at the given map key.
// It re-parses out via Parse to avoid coupling tests to the encoder details.
func rawTypeCode(t *testing.T, blob []byte, key string) byte {
	t.Helper()
	skel, err := Parse(blob)
	require.NoError(t, err)
	for _, f := range skel.MapFields {
		if f.Key == key {
			require.Equal(t, KindLeaf, f.Value.Kind)
			return f.Value.LeafCode
		}
	}
	t.Fatalf("key %q not found", key)
	return 0
}

func TestApply_INC_Int8KeepsCode(t *testing.T) {
	blob := mustEncode(t, map[string]any{"n": int8(10)})
	out, err := Apply(blob, []Op{
		{Kind: OpInc, Path: "n", Value: encVal(t, int8(3))},
	})
	require.NoError(t, err)

	assert.Equal(t, codeInt8, rawTypeCode(t, out, "n"))
	got := decodeMap(t, out)
	assert.EqualValues(t, 13, got["n"])
}

func TestApply_INC_Int16KeepsCode(t *testing.T) {
	blob := mustEncode(t, map[string]any{"n": int16(1000)})
	out, err := Apply(blob, []Op{
		{Kind: OpInc, Path: "n", Value: encVal(t, int16(50))},
	})
	require.NoError(t, err)

	assert.Equal(t, codeInt16, rawTypeCode(t, out, "n"))
	got := decodeMap(t, out)
	assert.EqualValues(t, 1050, got["n"])
}

func TestApply_INC_Int32KeepsCode(t *testing.T) {
	blob := mustEncode(t, map[string]any{"n": int32(70000)})
	out, err := Apply(blob, []Op{
		{Kind: OpInc, Path: "n", Value: encVal(t, int32(5))},
	})
	require.NoError(t, err)

	assert.Equal(t, codeInt32, rawTypeCode(t, out, "n"))
	got := decodeMap(t, out)
	assert.EqualValues(t, 70005, got["n"])
}

func TestApply_INC_Int64KeepsCode(t *testing.T) {
	blob := mustEncode(t, map[string]any{"n": int64(5_000_000_000)})
	out, err := Apply(blob, []Op{
		{Kind: OpInc, Path: "n", Value: encVal(t, int64(7))},
	})
	require.NoError(t, err)

	assert.Equal(t, codeInt64, rawTypeCode(t, out, "n"))
	got := decodeMap(t, out)
	assert.EqualValues(t, 5_000_000_007, got["n"])
}

func TestApply_INC_Uint8KeepsCode(t *testing.T) {
	blob := mustEncode(t, map[string]any{"n": uint8(200)})
	out, err := Apply(blob, []Op{
		{Kind: OpInc, Path: "n", Value: encVal(t, uint8(5))},
	})
	require.NoError(t, err)

	assert.Equal(t, codeUint8, rawTypeCode(t, out, "n"))
	got := decodeMap(t, out)
	assert.EqualValues(t, 205, got["n"])
}

func TestApply_INC_Uint16KeepsCode(t *testing.T) {
	blob := mustEncode(t, map[string]any{"n": uint16(60000)})
	out, err := Apply(blob, []Op{
		{Kind: OpInc, Path: "n", Value: encVal(t, uint16(7))},
	})
	require.NoError(t, err)

	assert.Equal(t, codeUint16, rawTypeCode(t, out, "n"))
	got := decodeMap(t, out)
	assert.EqualValues(t, 60007, got["n"])
}

func TestApply_INC_Uint32KeepsCode(t *testing.T) {
	blob := mustEncode(t, map[string]any{"n": uint32(4_000_000_000)})
	out, err := Apply(blob, []Op{
		{Kind: OpInc, Path: "n", Value: encVal(t, uint32(11))},
	})
	require.NoError(t, err)

	assert.Equal(t, codeUint32, rawTypeCode(t, out, "n"))
	got := decodeMap(t, out)
	assert.EqualValues(t, 4_000_000_011, got["n"])
}

func TestApply_INC_Uint64KeepsCode(t *testing.T) {
	blob := mustEncode(t, map[string]any{"n": uint64(9_000_000_000_000_000_000)})
	out, err := Apply(blob, []Op{
		{Kind: OpInc, Path: "n", Value: encVal(t, uint64(13))},
	})
	require.NoError(t, err)

	assert.Equal(t, codeUint64, rawTypeCode(t, out, "n"))
	got := decodeMap(t, out)
	assert.EqualValues(t, uint64(9_000_000_000_000_000_013), got["n"])
}

func TestApply_INC_Float32KeepsCode(t *testing.T) {
	blob := mustEncode(t, map[string]any{"n": float32(1.5)})
	out, err := Apply(blob, []Op{
		{Kind: OpInc, Path: "n", Value: encVal(t, float32(0.25))},
	})
	require.NoError(t, err)

	assert.Equal(t, codeFloat32, rawTypeCode(t, out, "n"))
	got := decodeMap(t, out)
	assert.InDelta(t, 1.75, got["n"], 1e-6)
}

func TestApply_INC_Float64KeepsCode(t *testing.T) {
	blob := mustEncode(t, map[string]any{"n": float64(2.5)})
	out, err := Apply(blob, []Op{
		{Kind: OpInc, Path: "n", Value: encVal(t, float64(0.5))},
	})
	require.NoError(t, err)

	assert.Equal(t, codeFloat64, rawTypeCode(t, out, "n"))
	got := decodeMap(t, out)
	assert.InDelta(t, 3.0, got["n"].(float64), 1e-12)
}

func TestApply_INC_NegativeDelta(t *testing.T) {
	blob := mustEncode(t, map[string]any{"n": int32(100)})
	out, err := Apply(blob, []Op{
		{Kind: OpInc, Path: "n", Value: encVal(t, int32(-30))},
	})
	require.NoError(t, err)

	assert.Equal(t, codeInt32, rawTypeCode(t, out, "n"))
	got := decodeMap(t, out)
	assert.EqualValues(t, 70, got["n"])
}

func TestApply_INC_OverflowWrapsForSigned(t *testing.T) {
	blob := mustEncode(t, map[string]any{"n": int8(127)})
	out, err := Apply(blob, []Op{
		{Kind: OpInc, Path: "n", Value: encVal(t, int8(1))},
	})
	require.NoError(t, err)

	// int8(127)+int8(1) wraps to -128 in native Go.
	assert.Equal(t, codeInt8, rawTypeCode(t, out, "n"))
	var v struct {
		N int8 `msgpack:"n"`
	}
	require.NoError(t, msgpack.Unmarshal(out, &v))
	assert.Equal(t, int8(-128), v.N)
}

func TestApply_INC_MissingFieldUsesDeltaType(t *testing.T) {
	blob := mustEncode(t, map[string]any{"name": "alice"})
	out, err := Apply(blob, []Op{
		{Kind: OpInc, Path: "counter", Value: encVal(t, int16(7))},
	})
	require.NoError(t, err)

	// Missing field is created with the delta's type (int16).
	assert.Equal(t, codeInt16, rawTypeCode(t, out, "counter"))
	got := decodeMap(t, out)
	assert.EqualValues(t, 7, got["counter"])
}

func TestApply_INC_NestedAutoCreate(t *testing.T) {
	blob := mustEncode(t, map[string]any{"name": "alice"})
	out, err := Apply(blob, []Op{
		{Kind: OpInc, Path: "stats.views", Value: encVal(t, int64(1))},
	})
	require.NoError(t, err)

	got := decodeMap(t, out)
	stats := got["stats"].(map[string]any)
	assert.EqualValues(t, 1, stats["views"])
}

func TestApply_INC_OnNonNumericFails(t *testing.T) {
	blob := mustEncode(t, map[string]any{"name": "alice"})
	_, err := Apply(blob, []Op{
		{Kind: OpInc, Path: "name", Value: encVal(t, int8(1))},
	})
	assert.ErrorIs(t, err, ErrTypeMismatch)
}

func TestApply_INC_NonNumericDeltaFails(t *testing.T) {
	blob := mustEncode(t, map[string]any{"n": int8(1)})
	_, err := Apply(blob, []Op{
		{Kind: OpInc, Path: "n", Value: encVal(t, "nope")},
	})
	assert.ErrorIs(t, err, ErrTypeMismatch)
}

func TestApply_INC_RequiresValue(t *testing.T) {
	blob := mustEncode(t, map[string]any{"n": int8(1)})
	_, err := Apply(blob, []Op{
		{Kind: OpInc, Path: "n", Value: nil},
	})
	assert.ErrorIs(t, err, ErrInvalidOp)
}

func TestApply_INC_FloatIntoIntFails(t *testing.T) {
	// Type-class drift is not allowed: applying a float delta on an int field
	// is a type mismatch. The client must use a typed delta matching the
	// target's class (int / uint / float).
	blob := mustEncode(t, map[string]any{"n": int32(10)})
	_, err := Apply(blob, []Op{
		{Kind: OpInc, Path: "n", Value: encVal(t, float64(0.5))},
	})
	assert.ErrorIs(t, err, ErrTypeMismatch)
}
