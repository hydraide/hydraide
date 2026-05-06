package msgpackpatch

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vmihailenco/msgpack/v5"
)

// mustEncode marshals v and fails the test on error.
func mustEncode(t *testing.T, v any) []byte {
	t.Helper()
	b, err := msgpack.Marshal(v)
	require.NoError(t, err)
	return b
}

// rangeBytes returns the byte slice covered by a leaf skeleton.
func rangeBytes(blob []byte, s *Skeleton) []byte {
	return blob[s.LeafStart:s.LeafEnd]
}

func TestParse_FlatMap(t *testing.T) {
	blob := mustEncode(t, map[string]any{
		"name":   "alice",
		"age":    int8(30),
		"active": true,
	})

	skel, err := Parse(blob)
	require.NoError(t, err)
	require.NotNil(t, skel)
	assert.Equal(t, KindMap, skel.Kind)
	assert.Len(t, skel.MapFields, 3)

	// Order preservation: msgpack.Marshal of map[string]any sorts keys
	// alphabetically (vmihailenco/v5 behavior). We verify by looking up.
	got := map[string]*Skeleton{}
	for _, f := range skel.MapFields {
		got[f.Key] = f.Value
	}

	require.Contains(t, got, "name")
	assert.Equal(t, KindLeaf, got["name"].Kind)
	assert.Equal(t, "alice", string(rangeBytes(blob, got["name"])[1:])) // strip fixstr header

	require.Contains(t, got, "age")
	assert.Equal(t, KindLeaf, got["age"].Kind)
	assert.Equal(t, codeInt8, got["age"].LeafCode)

	require.Contains(t, got, "active")
	assert.Equal(t, KindLeaf, got["active"].Kind)
	assert.Equal(t, codeTrue, got["active"].LeafCode)
}

func TestParse_NestedMap(t *testing.T) {
	blob := mustEncode(t, map[string]any{
		"outer": map[string]any{
			"inner": "deep",
		},
	})

	skel, err := Parse(blob)
	require.NoError(t, err)
	require.Equal(t, KindMap, skel.Kind)
	require.Len(t, skel.MapFields, 1)

	outer := skel.MapFields[0]
	assert.Equal(t, "outer", outer.Key)
	assert.Equal(t, KindMap, outer.Value.Kind)
	require.Len(t, outer.Value.MapFields, 1)

	inner := outer.Value.MapFields[0]
	assert.Equal(t, "inner", inner.Key)
	assert.Equal(t, KindLeaf, inner.Value.Kind)
}

func TestParse_Array(t *testing.T) {
	blob := mustEncode(t, []any{int8(1), "two", true})

	skel, err := Parse(blob)
	require.NoError(t, err)
	require.Equal(t, KindArray, skel.Kind)
	require.Len(t, skel.ArrayItems, 3)

	assert.Equal(t, KindLeaf, skel.ArrayItems[0].Kind)
	assert.Equal(t, KindLeaf, skel.ArrayItems[1].Kind)
	assert.Equal(t, KindLeaf, skel.ArrayItems[2].Kind)
	assert.Equal(t, codeTrue, skel.ArrayItems[2].LeafCode)
}

func TestParse_MapOfArrayOfMap(t *testing.T) {
	blob := mustEncode(t, map[string]any{
		"items": []any{
			map[string]any{"id": int8(1)},
			map[string]any{"id": int8(2)},
		},
	})

	skel, err := Parse(blob)
	require.NoError(t, err)
	require.Equal(t, KindMap, skel.Kind)
	require.Len(t, skel.MapFields, 1)

	items := skel.MapFields[0].Value
	require.Equal(t, KindArray, items.Kind)
	require.Len(t, items.ArrayItems, 2)

	for i, child := range items.ArrayItems {
		require.Equal(t, KindMap, child.Kind, "child %d", i)
		require.Len(t, child.MapFields, 1)
		assert.Equal(t, "id", child.MapFields[0].Key)
	}
}

func TestParse_AllPrimitiveTypes(t *testing.T) {
	type S struct {
		B   bool    `msgpack:"b"`
		I8  int8    `msgpack:"i8"`
		I16 int16   `msgpack:"i16"`
		I32 int32   `msgpack:"i32"`
		I64 int64   `msgpack:"i64"`
		U8  uint8   `msgpack:"u8"`
		U16 uint16  `msgpack:"u16"`
		U32 uint32  `msgpack:"u32"`
		U64 uint64  `msgpack:"u64"`
		F32 float32 `msgpack:"f32"`
		F64 float64 `msgpack:"f64"`
		S   string  `msgpack:"s"`
		Bin []byte  `msgpack:"bin"`
	}
	blob := mustEncode(t, S{
		B: true, I8: -1, I16: -300, I32: -70000, I64: -5_000_000_000,
		U8: 200, U16: 60000, U32: 4_000_000_000, U64: 9_000_000_000_000_000_000,
		F32: 3.14, F64: 2.718281828, S: "hi", Bin: []byte{0x01, 0x02},
	})

	skel, err := Parse(blob)
	require.NoError(t, err)
	require.Equal(t, KindMap, skel.Kind)
	require.Len(t, skel.MapFields, 13)

	for _, f := range skel.MapFields {
		assert.Equal(t, KindLeaf, f.Value.Kind, "field %s", f.Key)
		// Each leaf should occupy at least one byte.
		assert.Greater(t, f.Value.LeafEnd, f.Value.LeafStart, "field %s", f.Key)
	}
}

func TestParse_EmptyMap(t *testing.T) {
	blob := mustEncode(t, map[string]any{})
	skel, err := Parse(blob)
	require.NoError(t, err)
	assert.Equal(t, KindMap, skel.Kind)
	assert.Empty(t, skel.MapFields)
}

func TestParse_EmptyArray(t *testing.T) {
	blob := mustEncode(t, []any{})
	skel, err := Parse(blob)
	require.NoError(t, err)
	assert.Equal(t, KindArray, skel.Kind)
	assert.Empty(t, skel.ArrayItems)
}

func TestParse_Malformed(t *testing.T) {
	t.Run("empty blob", func(t *testing.T) {
		_, err := Parse(nil)
		assert.ErrorIs(t, err, ErrInvalidMsgpack)
	})

	t.Run("truncated map", func(t *testing.T) {
		// fixmap header claiming 2 entries but no body.
		_, err := Parse([]byte{0x82})
		assert.ErrorIs(t, err, ErrInvalidMsgpack)
	})

	t.Run("trailing bytes", func(t *testing.T) {
		blob := mustEncode(t, map[string]any{"a": int8(1)})
		blob = append(blob, 0xff, 0xff)
		_, err := Parse(blob)
		assert.ErrorIs(t, err, ErrInvalidMsgpack)
	})

	t.Run("non-string key", func(t *testing.T) {
		// Build by hand: fixmap{1} key=int8(0x00) value=int8(0x01)
		blob := []byte{0x81, 0x00, 0x01}
		_, err := Parse(blob)
		assert.True(t, errors.Is(err, ErrNonStringKey),
			"expected ErrNonStringKey, got %v", err)
	})
}

// Type preservation invariant: leaf byte ranges, when concatenated in order
// alongside reconstructed container headers, must reproduce the original blob
// exactly (this is what enables zero-drift splice).
//
// Here we just verify each leaf's byte range round-trips through msgpack.
func TestParse_LeafBytesAreSelfContained(t *testing.T) {
	blob := mustEncode(t, map[string]any{
		"i8":  int8(-7),
		"f64": float64(1.5),
		"s":   "hello",
	})
	skel, err := Parse(blob)
	require.NoError(t, err)

	for _, f := range skel.MapFields {
		leaf := rangeBytes(blob, f.Value)
		var got any
		require.NoError(t, msgpack.Unmarshal(leaf, &got),
			"leaf %s bytes %x should be valid standalone msgpack", f.Key, leaf)
	}
}
