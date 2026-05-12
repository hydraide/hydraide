package gateway

import (
	"context"
	"testing"

	hydrapb "github.com/hydraide/hydraide/sdk/go/hydraidego/v3/hydraidepbgo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// statusCapFilter builds a Cap.Filter that matches treasures whose
// msgpack body has status == target. Used by the gateway Cap-on-Patch
// suite.
func statusCapFilter(target string) *hydrapb.FilterGroup {
	return &hydrapb.FilterGroup{
		Logic: hydrapb.FilterLogic_AND,
		Filters: []*hydrapb.TreasureFilter{
			{
				BytesFieldPath: protoStr("status"),
				Operator:       hydrapb.Relational_EQUAL,
				CompareValue:   &hydrapb.TreasureFilter_StringVal{StringVal: target},
			},
		},
	}
}

// ---------- G.1 — Cap budget exhausted mid-batch ----------

func TestGatewayPatchTreasures_Cap_MidBatchExhaustion(t *testing.T) {
	rig := newGatewayPatchTestRig(t, "gw-pt-cap", "mid", "any")
	swamp := "gw-pt-cap/mid/any"

	// Seed 5 "pending" records.
	for i := 0; i < 5; i++ {
		_, err := rig.gw.PatchTreasures(context.Background(), &hydrapb.PatchTreasuresRequest{
			IslandID:         rig.islandID,
			SwampName:        swamp,
			CreateIfNotExist: true,
			Patches: []*hydrapb.TreasurePatch{
				{Key: gatewayKey(i), Ops: []*hydrapb.PatchOp{
					{Op: hydrapb.PatchOp_SET, Path: "status", Value: encMsgpack(t, "pending")},
				}},
			},
		})
		require.NoError(t, err)
	}

	// Now flip status to "claimed" for all 5 in one batch with Cap=2.
	patches := make([]*hydrapb.TreasurePatch, 0, 5)
	for i := 0; i < 5; i++ {
		patches = append(patches, &hydrapb.TreasurePatch{
			Key: gatewayKey(i),
			Ops: []*hydrapb.PatchOp{
				{Op: hydrapb.PatchOp_SET, Path: "status", Value: encMsgpack(t, "claimed")},
			},
		})
	}
	resp, err := rig.gw.PatchTreasures(context.Background(), &hydrapb.PatchTreasuresRequest{
		IslandID:  rig.islandID,
		SwampName: swamp,
		Patches:   patches,
		Cap:       &hydrapb.Cap{Filter: statusCapFilter("claimed"), MaxMatching: 2},
	})
	require.NoError(t, err)
	assert.True(t, resp.GetCapReached())

	patched := 0
	capExceeded := 0
	for _, r := range resp.GetResults() {
		switch r.GetStatus() {
		case hydrapb.PatchResult_PATCHED:
			patched++
		case hydrapb.PatchResult_CAP_EXCEEDED:
			capExceeded++
		}
	}
	assert.Equal(t, 2, patched, "Cap=2, no pre-claimed → exactly 2 patched")
	assert.Equal(t, 3, capExceeded, "remaining 3 rejected as CAP_EXCEEDED")
}

// ---------- G.2 — Cap idempotent re-patch proceeds ----------

func TestGatewayPatchTreasures_Cap_IdempotentReclaim(t *testing.T) {
	rig := newGatewayPatchTestRig(t, "gw-pt-cap", "idemp", "any")
	swamp := "gw-pt-cap/idemp/any"

	// Seed 3 already-"claimed" records.
	for i := 0; i < 3; i++ {
		_, err := rig.gw.PatchTreasures(context.Background(), &hydrapb.PatchTreasuresRequest{
			IslandID:         rig.islandID,
			SwampName:        swamp,
			CreateIfNotExist: true,
			Patches: []*hydrapb.TreasurePatch{
				{Key: gatewayKey(i), Ops: []*hydrapb.PatchOp{
					{Op: hydrapb.PatchOp_SET, Path: "status", Value: encMsgpack(t, "claimed")},
				}},
			},
		})
		require.NoError(t, err)
	}

	// Cap=3, already 3 claimed → budget=0. But (yes,yes) re-patches must
	// still proceed (no count growth).
	resp, err := rig.gw.PatchTreasures(context.Background(), &hydrapb.PatchTreasuresRequest{
		IslandID:  rig.islandID,
		SwampName: swamp,
		Patches: []*hydrapb.TreasurePatch{
			{Key: gatewayKey(0), Ops: []*hydrapb.PatchOp{
				{Op: hydrapb.PatchOp_SET, Path: "status", Value: encMsgpack(t, "claimed")},
			}},
		},
		Cap: &hydrapb.Cap{Filter: statusCapFilter("claimed"), MaxMatching: 3},
	})
	require.NoError(t, err)
	assert.False(t, resp.GetCapReached())
	require.Len(t, resp.GetResults(), 1)
	assert.Equal(t, hydrapb.PatchResult_PATCHED, resp.GetResults()[0].GetStatus())
}

// ---------- G.3 — Cap rejects metadata filter ----------

func TestGatewayPatchTreasures_Cap_RejectsMetadataFilter(t *testing.T) {
	rig := newGatewayPatchTestRig(t, "gw-pt-cap", "meta-reject", "any")
	swamp := "gw-pt-cap/meta-reject/any"

	metaFilter := &hydrapb.FilterGroup{
		Logic: hydrapb.FilterLogic_AND,
		Filters: []*hydrapb.TreasureFilter{
			{
				Operator:     hydrapb.Relational_EQUAL,
				CompareValue: &hydrapb.TreasureFilter_StringVal{StringVal: "x"},
				// No BytesFieldPath → operates on the treasure value itself.
			},
		},
	}

	_, err := rig.gw.PatchTreasures(context.Background(), &hydrapb.PatchTreasuresRequest{
		IslandID:         rig.islandID,
		SwampName:        swamp,
		CreateIfNotExist: true,
		Patches: []*hydrapb.TreasurePatch{
			{Key: gatewayKey(0), Ops: []*hydrapb.PatchOp{
				{Op: hydrapb.PatchOp_SET, Path: "x", Value: encMsgpack(t, "y")},
			}},
		},
		Cap: &hydrapb.Cap{Filter: metaFilter, MaxMatching: 1},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "BytesField filters only")
}

// ---------- G.4 — Cap=nil regression twin ----------

func TestGatewayPatchTreasures_NoCap_RegressionTwin(t *testing.T) {
	rig := newGatewayPatchTestRig(t, "gw-pt-cap", "nocap-twin", "any")
	swamp := "gw-pt-cap/nocap-twin/any"

	resp, err := rig.gw.PatchTreasures(context.Background(), &hydrapb.PatchTreasuresRequest{
		IslandID:         rig.islandID,
		SwampName:        swamp,
		CreateIfNotExist: true,
		Patches: []*hydrapb.TreasurePatch{
			{Key: gatewayKey(0), Ops: []*hydrapb.PatchOp{
				{Op: hydrapb.PatchOp_SET, Path: "status", Value: encMsgpack(t, "claimed")},
			}},
		},
	})
	require.NoError(t, err)
	assert.False(t, resp.GetCapReached(), "nil Cap → CapReached must be false")
	require.Len(t, resp.GetResults(), 1)
	assert.Equal(t, hydrapb.PatchResult_CREATED, resp.GetResults()[0].GetStatus())
}
