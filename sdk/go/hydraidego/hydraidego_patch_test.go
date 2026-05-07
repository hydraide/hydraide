package hydraidego

import (
	"context"
	"testing"
	"time"

	hydraidepbgo "github.com/hydraide/hydraide/sdk/go/hydraidego/v3/hydraidepbgo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vmihailenco/msgpack/v5"
)

// These unit tests cover the SDK's local logic (encoding, builder
// accumulation, status mapping). End-to-end round-trip behavior is
// covered separately under the e2e build tag.

// ---------- encodePatchValue ----------

func TestEncodePatchValue_PrimitiveTypeCodes(t *testing.T) {
	cases := []struct {
		name     string
		value    any
		wantCode byte
	}{
		{"int8", int8(-1), 0xd0},
		{"int16", int16(-300), 0xd1},
		{"int32", int32(-70000), 0xd2},
		{"int64", int64(-5_000_000_000), 0xd3},
		{"uint8", uint8(200), 0xcc},
		{"uint16", uint16(60000), 0xcd},
		{"uint32", uint32(4_000_000_000), 0xce},
		{"uint64", uint64(9_000_000_000_000_000_000), 0xcf},
		{"float32", float32(3.14), 0xca},
		{"float64", float64(2.718281828), 0xcb},
		{"true", true, 0xc3},
		{"false", false, 0xc2},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, err := encodePatchValue(tc.value)
			require.NoError(t, err)
			require.NotEmpty(t, out)
			assert.Equal(t, tc.wantCode, out[0], "leading code for %s", tc.name)
		})
	}
}

func TestEncodePatchValue_StringRoundTrip(t *testing.T) {
	out, err := encodePatchValue("hello")
	require.NoError(t, err)

	var got string
	require.NoError(t, msgpack.Unmarshal(out, &got))
	assert.Equal(t, "hello", got)
}

func TestEncodePatchValue_NilRejected(t *testing.T) {
	_, err := encodePatchValue(nil)
	assert.Error(t, err)
}

func TestEncodePatchValue_TimePreservesExtensionEncoding(t *testing.T) {
	now := time.Date(2026, 5, 6, 12, 0, 0, 0, time.UTC)
	out, err := encodePatchValue(now)
	require.NoError(t, err)

	// vmihailenco encodes time.Time as a msgpack ext type. Verify it round-trips.
	var got time.Time
	require.NoError(t, msgpack.Unmarshal(out, &got))
	assert.True(t, got.Equal(now))
}

// ---------- PatchStatus.String ----------

func TestPatchStatus_String(t *testing.T) {
	cases := map[PatchStatus]string{
		PatchStatusPatched:              "PATCHED",
		PatchStatusCreated:              "CREATED",
		PatchStatusKeyNotFound:          "KEY_NOT_FOUND",
		PatchStatusConditionNotMet:      "CONDITION_NOT_MET",
		PatchStatusFieldNotFound:        "FIELD_NOT_FOUND",
		PatchStatusTypeMismatch:         "TYPE_MISMATCH",
		PatchStatusPathInvalid:          "PATH_INVALID",
		PatchStatusEncodingNotSupported: "ENCODING_NOT_SUPPORTED",
		PatchStatusInternalError:        "INTERNAL_ERROR",
	}
	for st, want := range cases {
		assert.Equal(t, want, st.String())
	}
}

// ---------- PatchStatus / wire enum alignment ----------

// The whole point of having PatchStatus values mirror the wire codes is so
// the gateway and SDK can map by direct cast. Lock this in with a guard
// test — if either side reorders, this fails loudly.
func TestPatchStatus_AlignsWithWireEnum(t *testing.T) {
	require.Equal(t, int(hydraidepbgo.PatchResult_PATCHED), int(PatchStatusPatched))
	require.Equal(t, int(hydraidepbgo.PatchResult_CREATED), int(PatchStatusCreated))
	require.Equal(t, int(hydraidepbgo.PatchResult_KEY_NOT_FOUND), int(PatchStatusKeyNotFound))
	require.Equal(t, int(hydraidepbgo.PatchResult_CONDITION_NOT_MET), int(PatchStatusConditionNotMet))
	require.Equal(t, int(hydraidepbgo.PatchResult_FIELD_NOT_FOUND), int(PatchStatusFieldNotFound))
	require.Equal(t, int(hydraidepbgo.PatchResult_TYPE_MISMATCH), int(PatchStatusTypeMismatch))
	require.Equal(t, int(hydraidepbgo.PatchResult_PATH_INVALID), int(PatchStatusPathInvalid))
	require.Equal(t, int(hydraidepbgo.PatchResult_ENCODING_NOT_SUPPORTED), int(PatchStatusEncodingNotSupported))
	require.Equal(t, int(hydraidepbgo.PatchResult_INTERNAL_ERROR), int(PatchStatusInternalError))
}

// ---------- Builder accumulation ----------

func newBuilderForTest() *PatchBuilder {
	// Constructing the builder directly bypasses the network; we only need
	// a non-nil hydraidego pointer for the chain methods.
	return &PatchBuilder{
		h:      &hydraidego{},
		ctx:    context.Background(),
		create: true,
	}
}

func TestBuilder_OpsAppendInOrder(t *testing.T) {
	b := newBuilderForTest().
		Set("a", int8(1)).
		Inc("b", int8(2)).
		Append("c[]", "x").
		Prepend("d[]", "y").
		Delete("e").
		RemoveAt("f[2]").
		RemoveVal("g", "z").
		Merge("h", map[string]any{"k": int8(1)})

	require.Len(t, b.ops, 8)
	wantKinds := []hydraidepbgo.PatchOp_Kind{
		hydraidepbgo.PatchOp_SET,
		hydraidepbgo.PatchOp_INC,
		hydraidepbgo.PatchOp_APPEND,
		hydraidepbgo.PatchOp_PREPEND,
		hydraidepbgo.PatchOp_DELETE,
		hydraidepbgo.PatchOp_REMOVE_AT,
		hydraidepbgo.PatchOp_REMOVE_VAL,
		hydraidepbgo.PatchOp_MERGE,
	}
	for i, want := range wantKinds {
		assert.Equal(t, want, b.ops[i].GetOp(), "op %d", i)
	}

	// DELETE / REMOVE_AT carry no Value.
	assert.Empty(t, b.ops[4].GetValue(), "DELETE has no Value")
	assert.Empty(t, b.ops[5].GetValue(), "REMOVE_AT has no Value")
	// SET / INC / APPEND / PREPEND / REMOVE_VAL / MERGE all carry encoded Value.
	for _, i := range []int{0, 1, 2, 3, 6, 7} {
		assert.NotEmpty(t, b.ops[i].GetValue(), "op %d should carry Value", i)
	}
}

func TestBuilder_ConditionsLatestWins(t *testing.T) {
	b := newBuilderForTest().
		IfFieldEquals("owner", "alice").
		IfFieldGreaterThan("counter", int32(10))

	require.NotNil(t, b.cond)
	assert.Equal(t, "counter", b.cond.GetPath())
	assert.Equal(t, hydraidepbgo.PatchCondition_GREATER_THAN, b.cond.GetOperator())
	assert.NotEmpty(t, b.cond.GetThreshold())
}

func TestBuilder_ConditionsExistsAndNotExists(t *testing.T) {
	b := newBuilderForTest().IfFieldExists("flag")
	require.NotNil(t, b.cond)
	assert.Equal(t, hydraidepbgo.PatchCondition_EXISTS, b.cond.GetOperator())
	assert.Empty(t, b.cond.GetThreshold(), "EXISTS uses no threshold")

	b = newBuilderForTest().IfFieldNotExists("flag")
	require.NotNil(t, b.cond)
	assert.Equal(t, hydraidepbgo.PatchCondition_NOT_EXISTS, b.cond.GetOperator())
}

func TestBuilder_NoCreate(t *testing.T) {
	b := newBuilderForTest()
	assert.True(t, b.create, "default is create-if-not-exist")
	b.NoCreate()
	assert.False(t, b.create)
}

func TestBuilder_MetaAccumulates(t *testing.T) {
	b := newBuilderForTest().
		WithUpdatedAt().
		WithUpdatedBy("alice")

	require.NotNil(t, b.meta)
	assert.True(t, b.meta.GetSetUpdatedAt())
	assert.Equal(t, "alice", b.meta.GetSetUpdatedBy())
}

func TestBuilder_EncodeErrorShortCircuits(t *testing.T) {
	// Passing nil to Set must surface as an encode error on Exec, not
	// a panic.
	b := newBuilderForTest().Set("x", nil).Inc("y", int8(1))
	require.NotNil(t, b.encodeError)
	// Subsequent ops are silently dropped after the first encode error,
	// to keep the builder safe to chain even when errors occur.
	assert.Len(t, b.ops, 0, "ops not appended after encode error")
}
