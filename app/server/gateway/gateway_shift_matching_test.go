package gateway

import (
	"context"
	"testing"
	"time"

	hydrapb "github.com/hydraide/hydraide/sdk/go/hydraidego/v3/hydraidepbgo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// seedTreasureWithBody creates a treasure under swampName via PatchTreasures
// with the given msgpack ops. Test-only helper for the ShiftMatching +
// Cap gateway suite.
func seedTreasureWithBody(t *testing.T, rig *gatewayPatchTestRig, swampName, key string, status string, expiredAgo time.Duration) {
	t.Helper()
	_, err := rig.gw.PatchTreasures(context.Background(), &hydrapb.PatchTreasuresRequest{
		IslandID:         rig.islandID,
		SwampName:        swampName,
		CreateIfNotExist: true,
		Patches: []*hydrapb.TreasurePatch{
			{Key: key, Ops: []*hydrapb.PatchOp{
				{Op: hydrapb.PatchOp_SET, Path: "status", Value: encMsgpack(t, status)},
			}},
		},
		Meta: &hydrapb.PatchMeta{SetExpiredAt: timestamppb.New(time.Now().UTC().Add(-expiredAgo))},
	})
	require.NoError(t, err)
}

// ---------- M.1 — ShiftMatchingTreasures: IndexKey ASC, no filter ----------

func TestGatewayShiftMatching_KeyAscBasic(t *testing.T) {
	rig := newGatewayPatchTestRig(t, "gw-shift-match", "basic", "any")
	swamp := "gw-shift-match/basic/any"

	for i := 0; i < 5; i++ {
		seedTreasureWithBody(t, rig, swamp, gatewayKey(i), "pending", time.Hour)
	}

	resp, err := rig.gw.ShiftMatchingTreasures(context.Background(), &hydrapb.ShiftMatchingTreasuresRequest{
		IslandID:  rig.islandID,
		SwampName: swamp,
		IndexType: hydrapb.IndexType_KEY,
		OrderType: hydrapb.OrderType_ASC,
		HowMany:   3,
	})
	require.NoError(t, err)
	assert.False(t, resp.GetCapReached())
	require.Len(t, resp.GetTreasures(), 3)
	for i, tr := range resp.GetTreasures() {
		assert.Equal(t, gatewayKey(i), tr.GetKey())
	}
}

// ---------- M.2 — ShiftMatchingTreasures with FilterGroup narrows selection ----------

func TestGatewayShiftMatching_FilterNarrowsSelection(t *testing.T) {
	rig := newGatewayPatchTestRig(t, "gw-shift-match", "filter", "any")
	swamp := "gw-shift-match/filter/any"

	// 3 "pending" + 2 "done" treasures.
	for i := 0; i < 3; i++ {
		seedTreasureWithBody(t, rig, swamp, gatewayKey(i), "pending", time.Hour)
	}
	for i := 3; i < 5; i++ {
		seedTreasureWithBody(t, rig, swamp, gatewayKey(i), "done", time.Hour)
	}

	filters := &hydrapb.FilterGroup{
		Logic: hydrapb.FilterLogic_AND,
		Filters: []*hydrapb.TreasureFilter{
			{
				BytesFieldPath: protoStr("status"),
				Operator:       hydrapb.Relational_EQUAL,
				CompareValue:   &hydrapb.TreasureFilter_StringVal{StringVal: "pending"},
			},
		},
	}

	resp, err := rig.gw.ShiftMatchingTreasures(context.Background(), &hydrapb.ShiftMatchingTreasuresRequest{
		IslandID:  rig.islandID,
		SwampName: swamp,
		IndexType: hydrapb.IndexType_KEY,
		OrderType: hydrapb.OrderType_ASC,
		HowMany:   100,
		Filters:   filters,
	})
	require.NoError(t, err)
	assert.False(t, resp.GetCapReached())
	require.Len(t, resp.GetTreasures(), 3, "only 'pending' must be shifted")
}

// ---------- M.3 — Cap budget bounds result ----------

func TestGatewayShiftMatching_CapBoundsResult(t *testing.T) {
	rig := newGatewayPatchTestRig(t, "gw-shift-match", "cap", "any")
	swamp := "gw-shift-match/cap/any"

	for i := 0; i < 10; i++ {
		seedTreasureWithBody(t, rig, swamp, gatewayKey(i), "pending", time.Hour)
	}

	// Cap.Filter counts "claimed" records (none exist) → budget=3.
	capFilter := &hydrapb.FilterGroup{
		Logic: hydrapb.FilterLogic_AND,
		Filters: []*hydrapb.TreasureFilter{
			{
				BytesFieldPath: protoStr("status"),
				Operator:       hydrapb.Relational_EQUAL,
				CompareValue:   &hydrapb.TreasureFilter_StringVal{StringVal: "claimed"},
			},
		},
	}

	resp, err := rig.gw.ShiftMatchingTreasures(context.Background(), &hydrapb.ShiftMatchingTreasuresRequest{
		IslandID:  rig.islandID,
		SwampName: swamp,
		IndexType: hydrapb.IndexType_KEY,
		OrderType: hydrapb.OrderType_ASC,
		HowMany:   100,
		Cap:       &hydrapb.Cap{Filter: capFilter, MaxMatching: 3},
	})
	require.NoError(t, err)
	assert.True(t, resp.GetCapReached())
	assert.Len(t, resp.GetTreasures(), 3)
}

// ---------- M.4 — Cap.Filter nil with non-nil Cap is rejected ----------

func TestGatewayShiftMatching_RejectsCapWithoutFilter(t *testing.T) {
	rig := newGatewayPatchTestRig(t, "gw-shift-match", "bad-cap", "any")
	swamp := "gw-shift-match/bad-cap/any"

	_, err := rig.gw.ShiftMatchingTreasures(context.Background(), &hydrapb.ShiftMatchingTreasuresRequest{
		IslandID:  rig.islandID,
		SwampName: swamp,
		IndexType: hydrapb.IndexType_KEY,
		OrderType: hydrapb.OrderType_ASC,
		HowMany:   1,
		Cap:       &hydrapb.Cap{MaxMatching: 5}, // missing Filter
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Cap.Filter is required")
}

// ---------- M.5 — Cap.MaxMatching <= 0 is rejected ----------

func TestGatewayShiftMatching_RejectsZeroCapMax(t *testing.T) {
	rig := newGatewayPatchTestRig(t, "gw-shift-match", "zero-cap", "any")
	swamp := "gw-shift-match/zero-cap/any"

	_, err := rig.gw.ShiftMatchingTreasures(context.Background(), &hydrapb.ShiftMatchingTreasuresRequest{
		IslandID:  rig.islandID,
		SwampName: swamp,
		IndexType: hydrapb.IndexType_KEY,
		OrderType: hydrapb.OrderType_ASC,
		HowMany:   1,
		Cap: &hydrapb.Cap{
			Filter:      &hydrapb.FilterGroup{Logic: hydrapb.FilterLogic_AND},
			MaxMatching: 0,
		},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "MaxMatching must be > 0")
}

// ---------- M.6 — ShiftMatchingTreasuresMany batch ----------

func TestGatewayShiftMatchingMany_PerSwampIsolation(t *testing.T) {
	rig := newGatewayPatchTestRig(t, "gw-shift-match-many", "iso", "any")
	swampA := "gw-shift-match-many/iso/a"
	swampB := "gw-shift-match-many/iso/b"

	for i := 0; i < 3; i++ {
		seedTreasureWithBody(t, rig, swampA, gatewayKey(i), "pending", time.Hour)
	}
	for i := 0; i < 2; i++ {
		seedTreasureWithBody(t, rig, swampB, gatewayKey(i), "pending", time.Hour)
	}

	resp, err := rig.gw.ShiftMatchingTreasuresMany(context.Background(), &hydrapb.ShiftMatchingTreasuresManyRequest{
		Requests: []*hydrapb.ShiftMatchingTreasuresRequest{
			{IslandID: rig.islandID, SwampName: swampA, IndexType: hydrapb.IndexType_KEY, OrderType: hydrapb.OrderType_ASC, HowMany: 100},
			{IslandID: rig.islandID, SwampName: swampB, IndexType: hydrapb.IndexType_KEY, OrderType: hydrapb.OrderType_ASC, HowMany: 100},
		},
	})
	require.NoError(t, err)
	require.Len(t, resp.GetResponses(), 2)
	assert.Empty(t, resp.GetResponses()[0].GetError())
	assert.Len(t, resp.GetResponses()[0].GetTreasures(), 3)
	assert.Empty(t, resp.GetResponses()[1].GetError())
	assert.Len(t, resp.GetResponses()[1].GetTreasures(), 2)
}

// ---------- M.7 — PatchExpired Cap exhausted via gateway ----------

func TestGatewayPatchExpired_CapExhausted(t *testing.T) {
	rig := newGatewayPatchTestRig(t, "gw-pe-cap", "exhausted", "any")
	swamp := "gw-pe-cap/exhausted/any"

	// 5 already-"claimed" expired records (preset cap-filter state).
	for i := 0; i < 5; i++ {
		seedTreasureWithBody(t, rig, swamp, gatewayKey(i), "claimed", time.Hour)
	}

	capFilter := &hydrapb.FilterGroup{
		Logic: hydrapb.FilterLogic_AND,
		Filters: []*hydrapb.TreasureFilter{
			{
				BytesFieldPath: protoStr("status"),
				Operator:       hydrapb.Relational_EQUAL,
				CompareValue:   &hydrapb.TreasureFilter_StringVal{StringVal: "claimed"},
			},
		},
	}

	resp, err := rig.gw.PatchExpiredTreasures(context.Background(), &hydrapb.PatchExpiredTreasuresRequest{
		IslandID:  rig.islandID,
		SwampName: swamp,
		HowMany:   100,
		Ops: []*hydrapb.PatchOp{
			{Op: hydrapb.PatchOp_SET, Path: "status", Value: encMsgpack(t, "later")},
		},
		Meta: &hydrapb.PatchMeta{SetExpiredAt: timestamppb.New(time.Now().UTC().Add(time.Hour))},
		Cap:  &hydrapb.Cap{Filter: capFilter, MaxMatching: 3},
	})
	require.NoError(t, err)
	assert.True(t, resp.GetCapReached())
	assert.Empty(t, resp.GetPatched(), "cap budget < currentMatching ⇒ no patches")
}

// ---------- M.8 — PatchExpired without Cap is regression twin ----------

func TestGatewayPatchExpired_NoCap_RegressionTwin(t *testing.T) {
	rig := newGatewayPatchTestRig(t, "gw-pe-cap", "twin", "any")
	swamp := "gw-pe-cap/twin/any"

	for i := 0; i < 4; i++ {
		seedTreasureWithBody(t, rig, swamp, gatewayKey(i), "pending", time.Hour)
	}

	resp, err := rig.gw.PatchExpiredTreasures(context.Background(), &hydrapb.PatchExpiredTreasuresRequest{
		IslandID:  rig.islandID,
		SwampName: swamp,
		HowMany:   100,
		Ops: []*hydrapb.PatchOp{
			{Op: hydrapb.PatchOp_SET, Path: "status", Value: encMsgpack(t, "done")},
		},
		Meta: &hydrapb.PatchMeta{SetExpiredAt: timestamppb.New(time.Now().UTC().Add(time.Hour))},
	})
	require.NoError(t, err)
	assert.False(t, resp.GetCapReached(), "nil Cap → CapReached must be false")
	assert.Len(t, resp.GetPatched(), 4)
}
