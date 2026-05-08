package hydraidego

import (
	"context"
	"testing"
	"time"

	hydraidepbgo "github.com/hydraide/hydraide/sdk/go/hydraidego/v3/hydraidepbgo"
	"github.com/hydraide/hydraide/sdk/go/hydraidego/v3/name"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vmihailenco/msgpack/v5"
	"google.golang.org/protobuf/types/known/timestamppb"
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

func TestBuilder_WithExpiredAt(t *testing.T) {
	want := time.Now().Add(2 * time.Hour).UTC()
	b := newBuilderForTest().WithExpiredAt(want)

	require.NotNil(t, b.meta)
	require.NotNil(t, b.meta.GetSetExpiredAt())
	assert.Equal(t, want.UnixNano(), b.meta.GetSetExpiredAt().AsTime().UnixNano())
	assert.False(t, b.meta.GetClearExpiredAt())
}

func TestBuilder_WithExpiredAtZeroClearsTTL(t *testing.T) {
	b := newBuilderForTest().
		WithExpiredAt(time.Now().Add(1 * time.Hour)).
		WithExpiredAt(time.Time{})

	require.NotNil(t, b.meta)
	assert.Nil(t, b.meta.GetSetExpiredAt(), "zero time must clear SetExpiredAt")
	assert.True(t, b.meta.GetClearExpiredAt())
}

func TestBuilder_WithoutExpiredAt(t *testing.T) {
	b := newBuilderForTest().
		WithExpiredAt(time.Now().Add(1 * time.Hour)).
		WithoutExpiredAt()

	require.NotNil(t, b.meta)
	assert.Nil(t, b.meta.GetSetExpiredAt())
	assert.True(t, b.meta.GetClearExpiredAt(), "WithoutExpiredAt must win over a prior WithExpiredAt")
}

func TestBuilder_MetaCombined(t *testing.T) {
	want := time.Now().Add(30 * time.Minute).UTC()
	b := newBuilderForTest().
		WithUpdatedAt().
		WithUpdatedBy("alice").
		WithExpiredAt(want)

	require.NotNil(t, b.meta)
	assert.True(t, b.meta.GetSetUpdatedAt())
	assert.Equal(t, "alice", b.meta.GetSetUpdatedBy())
	require.NotNil(t, b.meta.GetSetExpiredAt())
	assert.Equal(t, want.UnixNano(), b.meta.GetSetExpiredAt().AsTime().UnixNano())
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

// ---------- NewPatchBuilder + PatchManyRequest (R2-2) ----------

// TestNewPatchBuilder_DataOnlyHasNoClient verifies that a builder
// returned by NewPatchBuilder is data-only (no client / no swamp), so
// Exec() refuses to dispatch.
func TestNewPatchBuilder_DataOnlyHasNoClient(t *testing.T) {
	b := NewPatchBuilder("k").Set("X", int8(1))
	require.Nil(t, b.h, "data-only builder must not be bound to a client")
	require.Nil(t, b.ctx, "data-only builder must not carry a context")
	require.True(t, b.create, "CreateIfNotExist defaults to true")

	_, err := b.Exec()
	require.Error(t, err, "Exec on a data-only builder must fail")
}

// TestNewPatchBuilder_NoCreateFlag flips the create flag.
func TestNewPatchBuilder_NoCreateFlag(t *testing.T) {
	b := NewPatchBuilder("k").NoCreate().Set("X", int8(1))
	require.False(t, b.create)
}

// TestNewPatchBuilder_OpsAndCondAndMetaCarried verifies the data-only
// builder accumulates ops, condition, and meta the same way as a bound
// builder.
func TestNewPatchBuilder_OpsAndCondAndMetaCarried(t *testing.T) {
	exp := time.Now().Add(time.Hour).UTC()
	b := NewPatchBuilder("k").
		Inc("Counter", int32(1)).
		Set("Name", "alice").
		IfFieldGreaterThanOrEqual("Counter", int32(0)).
		WithExpiredAt(exp).
		WithUpdatedAt()

	require.Len(t, b.ops, 2)
	assert.Equal(t, hydraidepbgo.PatchOp_INC, b.ops[0].GetOp())
	assert.Equal(t, hydraidepbgo.PatchOp_SET, b.ops[1].GetOp())
	require.NotNil(t, b.cond)
	assert.Equal(t, hydraidepbgo.PatchCondition_GREATER_THAN_OR_EQUAL, b.cond.GetOperator())
	require.NotNil(t, b.meta)
	assert.True(t, b.meta.GetSetUpdatedAt())
	require.NotNil(t, b.meta.GetSetExpiredAt())
}

// ---------- CatalogShiftExpiredManyFromMany validation (R2-7) ----------

func TestShiftExpiredManyFromMany_NilRequest(t *testing.T) {
	h := &hydraidego{}
	requests := []*ShiftExpiredManyFromManyRequest{nil}
	err := h.CatalogShiftExpiredManyFromMany(context.Background(), requests, struct{}{}, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "is nil")
}

func TestShiftExpiredManyFromMany_EmptyRequestsIsNoop(t *testing.T) {
	h := &hydraidego{}
	err := h.CatalogShiftExpiredManyFromMany(context.Background(), nil, struct{}{}, nil)
	require.NoError(t, err)
}

// ---------- CatalogPatchManyToMany validation (R2-4) ----------

func TestPatchManyToMany_NilRequest(t *testing.T) {
	h := &hydraidego{}
	requests := []*CatalogPatchManyToManyRequest{nil}
	err := h.CatalogPatchManyToMany(context.Background(), requests, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "is nil")
}

func TestPatchManyToMany_EmptyPatches(t *testing.T) {
	h := &hydraidego{}
	swamp := name.New().Sanctuary("s").Realm("r").Swamp("a")
	requests := []*CatalogPatchManyToManyRequest{{SwampName: swamp}}
	err := h.CatalogPatchManyToMany(context.Background(), requests, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Patches is empty")
}

func TestPatchManyToMany_NilBuilder(t *testing.T) {
	h := &hydraidego{}
	swamp := name.New().Sanctuary("s").Realm("r").Swamp("a")
	requests := []*CatalogPatchManyToManyRequest{
		{SwampName: swamp, Patches: []*PatchManyRequest{nil}},
	}
	err := h.CatalogPatchManyToMany(context.Background(), requests, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "nil Builder")
}

func TestPatchManyToMany_EmptyBuilderRejected(t *testing.T) {
	h := &hydraidego{}
	swamp := name.New().Sanctuary("s").Realm("r").Swamp("a")
	requests := []*CatalogPatchManyToManyRequest{
		{SwampName: swamp, Patches: []*PatchManyRequest{{Builder: NewPatchBuilder("k")}}},
	}
	err := h.CatalogPatchManyToMany(context.Background(), requests, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no ops and no meta")
}

func TestPatchManyToMany_MixedNoCreateRejected(t *testing.T) {
	h := &hydraidego{}
	swamp := name.New().Sanctuary("s").Realm("r").Swamp("a")
	requests := []*CatalogPatchManyToManyRequest{
		{SwampName: swamp, Patches: []*PatchManyRequest{
			{Builder: NewPatchBuilder("a").Set("x", int8(1))},
			{Builder: NewPatchBuilder("b").NoCreate().Set("x", int8(2))},
		}},
	}
	err := h.CatalogPatchManyToMany(context.Background(), requests, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "NoCreate flag differs")
}

func TestPatchManyToMany_EmptyRequestsIsNoop(t *testing.T) {
	h := &hydraidego{}
	err := h.CatalogPatchManyToMany(context.Background(), nil, nil)
	require.NoError(t, err)
}

// ---------- CatalogPatchExpiredManyFromMany validation (R2-3) ----------

func TestPatchExpiredManyFromMany_NilRequest(t *testing.T) {
	h := &hydraidego{}
	requests := []*PatchExpiredManyFromManyRequest{nil}
	err := h.CatalogPatchExpiredManyFromMany(context.Background(), requests, struct{}{}, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "is nil")
}

func TestPatchExpiredManyFromMany_NilBuilder(t *testing.T) {
	h := &hydraidego{}
	requests := []*PatchExpiredManyFromManyRequest{{HowMany: 1}}
	err := h.CatalogPatchExpiredManyFromMany(context.Background(), requests, struct{}{}, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Builder is required")
}

func TestPatchExpiredManyFromMany_EmptyBuilder(t *testing.T) {
	h := &hydraidego{}
	requests := []*PatchExpiredManyFromManyRequest{{Builder: NewPatchExpiredOps()}}
	err := h.CatalogPatchExpiredManyFromMany(context.Background(), requests, struct{}{}, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "at least one op or non-nil meta")
}

func TestPatchExpiredManyFromMany_EmptyRequestsIsNoop(t *testing.T) {
	h := &hydraidego{}
	err := h.CatalogPatchExpiredManyFromMany(context.Background(), nil, struct{}{}, nil)
	require.NoError(t, err)
}

func TestPatchExpiredManyFromMany_BuilderEncodeError(t *testing.T) {
	h := &hydraidego{}
	// Triggering an encode error on the expired-ops builder requires
	// passing a value that cannot be msgpack-encoded; use nil which the
	// encoder rejects.
	bad := NewPatchExpiredOps().Set("x", nil)
	requests := []*PatchExpiredManyFromManyRequest{{Builder: bad}}
	err := h.CatalogPatchExpiredManyFromMany(context.Background(), requests, struct{}{}, nil)
	require.Error(t, err)
}

// ---------- PatchExpiredOps builder ----------

func TestPatchExpiredOps_AccumulatesOpsInOrder(t *testing.T) {
	b := NewPatchExpiredOps().
		Set("a", int8(1)).
		Inc("b", int8(2)).
		Append("c[]", "x").
		Prepend("d[]", "y").
		Delete("e").
		RemoveAt("f[3]").
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
	// DELETE / REMOVE_AT must have no Value on the wire.
	assert.Empty(t, b.ops[4].GetValue())
	assert.Empty(t, b.ops[5].GetValue())
}

func TestPatchExpiredOps_ConditionRoundTrip(t *testing.T) {
	b := NewPatchExpiredOps().
		Set("x", int8(1)).
		IfFieldLessThan("ExpireAt", time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	require.NotNil(t, b.cond)
	assert.Equal(t, hydraidepbgo.PatchCondition_LESS_THAN, b.cond.GetOperator())
	assert.Equal(t, "ExpireAt", b.cond.GetPath())
	assert.NotEmpty(t, b.cond.GetThreshold())
}

func TestPatchExpiredOps_ConditionExistsHasNoThreshold(t *testing.T) {
	b := NewPatchExpiredOps().
		Set("x", int8(1)).
		IfFieldExists("ClaimedBy")
	require.NotNil(t, b.cond)
	assert.Equal(t, hydraidepbgo.PatchCondition_EXISTS, b.cond.GetOperator())
	assert.Empty(t, b.cond.GetThreshold(), "EXISTS must not carry a threshold")
}

func TestPatchExpiredOps_WithExpiredAtSetsMeta(t *testing.T) {
	want := time.Date(2026, 5, 8, 12, 0, 0, 0, time.UTC)
	b := NewPatchExpiredOps().Set("x", int8(1)).WithExpiredAt(want)
	require.NotNil(t, b.meta)
	require.NotNil(t, b.meta.GetSetExpiredAt())
	assert.Equal(t, want.UnixNano(), b.meta.GetSetExpiredAt().AsTime().UnixNano())
	assert.False(t, b.meta.GetClearExpiredAt())
}

func TestPatchExpiredOps_WithExpiredAtZeroClears(t *testing.T) {
	b := NewPatchExpiredOps().Set("x", int8(1)).WithExpiredAt(time.Time{})
	require.NotNil(t, b.meta)
	assert.Nil(t, b.meta.GetSetExpiredAt())
	assert.True(t, b.meta.GetClearExpiredAt(), "zero time must clear")
}

func TestPatchExpiredOps_WithoutExpiredAtBeatsWith(t *testing.T) {
	want := time.Date(2026, 5, 8, 12, 0, 0, 0, time.UTC)
	b := NewPatchExpiredOps().
		Set("x", int8(1)).
		WithExpiredAt(want).
		WithoutExpiredAt()
	require.NotNil(t, b.meta)
	assert.Nil(t, b.meta.GetSetExpiredAt())
	assert.True(t, b.meta.GetClearExpiredAt())
}

func TestPatchExpiredOps_WithUpdatedAtAndBy(t *testing.T) {
	b := NewPatchExpiredOps().Set("x", int8(1)).WithUpdatedAt().WithUpdatedBy("alice")
	require.NotNil(t, b.meta)
	assert.True(t, b.meta.GetSetUpdatedAt())
	assert.Equal(t, "alice", b.meta.GetSetUpdatedBy())
}

func TestPatchExpiredOps_EncodeErrorShortCircuits(t *testing.T) {
	b := NewPatchExpiredOps().Set("x", nil).Inc("y", int8(1))
	require.NotNil(t, b.encodeError)
	assert.Empty(t, b.ops, "ops must not append after encode error")
}

// ---------- populateCatalogModelFromPatchedExpired ----------

type testCatalogModel struct {
	Key       string    `hydraide:"key"`
	ExpireAt  time.Time `hydraide:"expireAt"`
	Counter   int8
	ClaimedBy string
}

func TestPopulateCatalogModelFromPatchedExpired_Body(t *testing.T) {
	body, err := msgpack.Marshal(map[string]any{
		"Counter":   int8(7),
		"ClaimedBy": "worker-A",
	})
	require.NoError(t, err)

	exp := time.Date(2026, 5, 8, 12, 0, 0, 0, time.UTC)
	entry := &hydraidepbgo.PatchedExpiredTreasure{
		Key:        "k-1",
		Status:     hydraidepbgo.PatchResult_PATCHED,
		NewMsgpack: body,
		ExpiredAt:  timestamppb.New(exp),
	}

	var m testCatalogModel
	require.NoError(t, populateCatalogModelFromPatchedExpired(entry, &m))

	assert.Equal(t, "k-1", m.Key)
	assert.True(t, m.ExpireAt.Equal(exp), "got=%v want=%v", m.ExpireAt, exp)
	assert.Equal(t, int8(7), m.Counter)
	assert.Equal(t, "worker-A", m.ClaimedBy)
}

func TestPopulateCatalogModelFromPatchedExpired_NoBody(t *testing.T) {
	exp := time.Date(2026, 5, 8, 12, 0, 0, 0, time.UTC)
	entry := &hydraidepbgo.PatchedExpiredTreasure{
		Key:       "k-2",
		Status:    hydraidepbgo.PatchResult_CONDITION_NOT_MET,
		ExpiredAt: timestamppb.New(exp),
	}
	var m testCatalogModel
	require.NoError(t, populateCatalogModelFromPatchedExpired(entry, &m))
	assert.Equal(t, "k-2", m.Key)
	assert.True(t, m.ExpireAt.Equal(exp))
	assert.Equal(t, int8(0), m.Counter, "no body → defaults")
}

// Map-body Catalogs use hydraide:"FieldName" tags as the wire keys, so the
// patch decoder must look up by tag value rather than by Go field name.
func TestPopulateCatalogModelFromPatchedExpired_MapBodyTagged(t *testing.T) {
	type Tagged struct {
		Key       string    `hydraide:"key"`
		ASN       string    `hydraide:"ASN"`
		ClaimedBy string    `hydraide:"ClaimedBy"`
		ClaimedAt time.Time `hydraide:"ClaimedAt"`
		ExpireAt  time.Time `hydraide:"expireAt"`
	}
	when := time.Date(2026, 5, 8, 9, 0, 0, 0, time.UTC)
	body, err := msgpack.Marshal(map[string]any{
		"ASN":       "AS42",
		"ClaimedBy": "worker-9",
		"ClaimedAt": when,
	})
	require.NoError(t, err)

	exp := time.Date(2026, 5, 8, 12, 0, 0, 0, time.UTC)
	entry := &hydraidepbgo.PatchedExpiredTreasure{
		Key:        "k-1",
		Status:     hydraidepbgo.PatchResult_PATCHED,
		NewMsgpack: body,
		ExpiredAt:  timestamppb.New(exp),
	}

	var m Tagged
	require.NoError(t, populateCatalogModelFromPatchedExpired(entry, &m))
	assert.Equal(t, "k-1", m.Key)
	assert.Equal(t, "AS42", m.ASN)
	assert.Equal(t, "worker-9", m.ClaimedBy)
	assert.True(t, m.ClaimedAt.Equal(when))
	assert.True(t, m.ExpireAt.Equal(exp))
}

func TestPopulateCatalogModelFromPatchedExpired_NonStructRejected(t *testing.T) {
	entry := &hydraidepbgo.PatchedExpiredTreasure{Key: "x"}
	var s string
	err := populateCatalogModelFromPatchedExpired(entry, &s)
	assert.Error(t, err)
}
