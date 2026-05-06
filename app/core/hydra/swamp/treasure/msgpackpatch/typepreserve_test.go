package msgpackpatch

import (
	"bytes"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vmihailenco/msgpack/v5"
)

// AllPrimitives exercises every msgpack primitive type the SDK is expected to
// emit. The regression suite mutates one field at a time and verifies that
// every other field is byte-identical to the original encoding (the splice
// invariant).
type AllPrimitives struct {
	B    bool      `msgpack:"b"`
	I8   int8      `msgpack:"i8"`
	I16  int16     `msgpack:"i16"`
	I32  int32     `msgpack:"i32"`
	I64  int64     `msgpack:"i64"`
	U8   uint8     `msgpack:"u8"`
	U16  uint16    `msgpack:"u16"`
	U32  uint32    `msgpack:"u32"`
	U64  uint64    `msgpack:"u64"`
	F32  float32   `msgpack:"f32"`
	F64  float64   `msgpack:"f64"`
	S    string    `msgpack:"s"`
	Bin  []byte    `msgpack:"bin"`
	T    time.Time `msgpack:"t"`
	IS   []int     `msgpack:"is"`
	MSI  map[string]int `msgpack:"msi"`
	Ptr  *string        `msgpack:"ptr"`
	Nil  *string        `msgpack:"nil"`
	Nest InnerStruct    `msgpack:"nest"`
}

type InnerStruct struct {
	Label string `msgpack:"label"`
	Count int32  `msgpack:"count"`
}

func sample() AllPrimitives {
	s := "hello"
	return AllPrimitives{
		B:    true,
		I8:   -1,
		I16:  -300,
		I32:  -70_000,
		I64:  -5_000_000_000,
		U8:   200,
		U16:  60_000,
		U32:  4_000_000_000,
		U64:  9_000_000_000_000_000_000,
		F32:  3.14,
		F64:  2.718281828,
		S:    "hi",
		Bin:  []byte{0x01, 0x02, 0x03},
		T:    time.Date(2026, 5, 6, 12, 0, 0, 0, time.UTC),
		IS:   []int{1, 2, 3},
		MSI:  map[string]int{"x": 1, "y": 2},
		Ptr:  &s,
		Nil:  nil,
		Nest: InnerStruct{Label: "lab", Count: 42},
	}
}

// extractField parses two blobs and returns the raw encoded byte slice for
// the named top-level field in each. Equality of those slices proves the
// splice preserved the field bit-for-bit.
func extractField(t *testing.T, blob []byte, name string) []byte {
	t.Helper()
	skel, err := Parse(blob)
	require.NoError(t, err)
	for _, f := range skel.MapFields {
		if f.Key == name {
			if f.Value.RawBytes != nil {
				return f.Value.RawBytes
			}
			// Snapshot the slice so mutations to the underlying blob don't
			// invalidate later comparisons.
			return bytes.Clone(blob[f.Value.LeafStart:f.Value.LeafEnd])
		}
	}
	t.Fatalf("field %q not found", name)
	return nil
}

func TestTypePreservation_AllPrimitives(t *testing.T) {
	original := sample()
	blob, err := msgpack.Marshal(original)
	require.NoError(t, err)

	// We patch the string field "s" only.
	patched, err := Apply(blob, []Op{
		{Kind: OpSet, Path: "s", Value: encVal(t, "ho")},
	})
	require.NoError(t, err)

	// Every non-mutated top-level field's encoded bytes must be identical.
	keepers := []string{
		"b", "i8", "i16", "i32", "i64",
		"u8", "u16", "u32", "u64",
		"f32", "f64",
		"bin", "t", "is", "msi", "ptr", "nil", "nest",
	}
	for _, name := range keepers {
		orig := extractField(t, blob, name)
		got := extractField(t, patched, name)
		assert.True(t, bytes.Equal(orig, got),
			"field %q encoding drifted: orig=%x got=%x", name, orig, got)
	}

	// Decoded struct round-trip preserves every field except the patched one.
	var decoded AllPrimitives
	require.NoError(t, msgpack.Unmarshal(patched, &decoded))
	assert.Equal(t, "ho", decoded.S)

	expected := original
	expected.S = "ho"
	assertPrimitivesEqual(t, expected, decoded)
}

// assertPrimitivesEqual compares two AllPrimitives field-by-field. time.Time
// is compared with the Equal() method rather than struct equality (which
// trips on the *Location pointer after a marshal round-trip).
func assertPrimitivesEqual(t *testing.T, want, got AllPrimitives) {
	t.Helper()
	assert.Equal(t, want.B, got.B, "B")
	assert.Equal(t, want.I8, got.I8, "I8")
	assert.Equal(t, want.I16, got.I16, "I16")
	assert.Equal(t, want.I32, got.I32, "I32")
	assert.Equal(t, want.I64, got.I64, "I64")
	assert.Equal(t, want.U8, got.U8, "U8")
	assert.Equal(t, want.U16, got.U16, "U16")
	assert.Equal(t, want.U32, got.U32, "U32")
	assert.Equal(t, want.U64, got.U64, "U64")
	assert.InDelta(t, want.F32, got.F32, 1e-6, "F32")
	assert.InDelta(t, want.F64, got.F64, 1e-12, "F64")
	assert.Equal(t, want.S, got.S, "S")
	assert.Equal(t, want.Bin, got.Bin, "Bin")
	assert.True(t, want.T.Equal(got.T), "T: want=%v got=%v", want.T, got.T)
	assert.Equal(t, want.IS, got.IS, "IS")
	assert.Equal(t, want.MSI, got.MSI, "MSI")
	if want.Ptr == nil {
		assert.Nil(t, got.Ptr, "Ptr")
	} else {
		require.NotNil(t, got.Ptr, "Ptr")
		assert.Equal(t, *want.Ptr, *got.Ptr, "Ptr deref")
	}
	assert.Equal(t, want.Nil, got.Nil, "Nil")
	assert.Equal(t, want.Nest, got.Nest, "Nest")
}

// Patch every numeric type in turn and verify the type tag is preserved
// (e.g. patching "i8" with int8(99) keeps a single-byte int8 encoding,
// not a widened int64).
func TestTypePreservation_NumericFieldsKeepTheirCodes(t *testing.T) {
	type N struct {
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
	}
	original := N{I8: -1, I16: -300, I32: -70000, I64: -5_000_000_000,
		U8: 200, U16: 60000, U32: 4_000_000_000, U64: 9_000_000_000_000_000_000,
		F32: 3.14, F64: 2.718}
	blob, err := msgpack.Marshal(original)
	require.NoError(t, err)

	cases := []struct {
		field    string
		newValue any
	}{
		{"i8", int8(7)},
		{"i16", int16(-32000)},
		{"i32", int32(123456)},
		{"i64", int64(-1_000_000_000_000)},
		{"u8", uint8(7)},
		{"u16", uint16(50_000)},
		{"u32", uint32(3_000_000_000)},
		{"u64", uint64(8_000_000_000_000_000_000)},
		{"f32", float32(1.5)},
		{"f64", float64(9.99)},
	}
	for _, tc := range cases {
		t.Run(tc.field, func(t *testing.T) {
			patched, err := Apply(blob, []Op{
				{Kind: OpSet, Path: tc.field, Value: encVal(t, tc.newValue)},
			})
			require.NoError(t, err)

			var decoded N
			require.NoError(t, msgpack.Unmarshal(patched, &decoded))

			// Unchanged fields stay equal to original.
			expected := original
			switch tc.field {
			case "i8":
				expected.I8 = tc.newValue.(int8)
			case "i16":
				expected.I16 = tc.newValue.(int16)
			case "i32":
				expected.I32 = tc.newValue.(int32)
			case "i64":
				expected.I64 = tc.newValue.(int64)
			case "u8":
				expected.U8 = tc.newValue.(uint8)
			case "u16":
				expected.U16 = tc.newValue.(uint16)
			case "u32":
				expected.U32 = tc.newValue.(uint32)
			case "u64":
				expected.U64 = tc.newValue.(uint64)
			case "f32":
				expected.F32 = tc.newValue.(float32)
			case "f64":
				expected.F64 = tc.newValue.(float64)
			}
			assert.Equal(t, expected, decoded)
		})
	}
}

// Mutating a deep nested field must not disturb sibling field encodings.
func TestTypePreservation_NestedSiblingsUnchanged(t *testing.T) {
	original := sample()
	blob, err := msgpack.Marshal(original)
	require.NoError(t, err)

	patched, err := Apply(blob, []Op{
		{Kind: OpSet, Path: "nest.label", Value: encVal(t, "newlab")},
	})
	require.NoError(t, err)

	var decoded AllPrimitives
	require.NoError(t, msgpack.Unmarshal(patched, &decoded))
	assert.Equal(t, "newlab", decoded.Nest.Label)
	assert.Equal(t, original.Nest.Count, decoded.Nest.Count)

	// Every other top-level field bytes must match.
	keepers := []string{
		"b", "i8", "i16", "i32", "i64",
		"u8", "u16", "u32", "u64", "f32", "f64",
		"s", "bin", "t", "is", "msi", "ptr", "nil",
	}
	for _, name := range keepers {
		orig := extractField(t, blob, name)
		got := extractField(t, patched, name)
		assert.True(t, bytes.Equal(orig, got), "field %q drifted", name)
	}
}

// Time preservation: the time.Time msgpack ext encoding survives unrelated patches.
func TestTypePreservation_TimeStaysExtension(t *testing.T) {
	original := sample()
	blob, err := msgpack.Marshal(original)
	require.NoError(t, err)

	tBytesBefore := extractField(t, blob, "t")

	patched, err := Apply(blob, []Op{
		{Kind: OpSet, Path: "b", Value: encVal(t, false)},
	})
	require.NoError(t, err)

	tBytesAfter := extractField(t, patched, "t")
	assert.True(t, bytes.Equal(tBytesBefore, tBytesAfter),
		"time.Time encoding drifted: before=%x after=%x", tBytesBefore, tBytesAfter)
}

// Round-trip through Apply with no ops must not lose any field.
func TestTypePreservation_NoOpRoundTrip(t *testing.T) {
	original := sample()
	blob, err := msgpack.Marshal(original)
	require.NoError(t, err)

	out, err := Apply(blob, nil)
	require.NoError(t, err)

	var decoded AllPrimitives
	require.NoError(t, msgpack.Unmarshal(out, &decoded))
	assertPrimitivesEqual(t, original, decoded)
}
