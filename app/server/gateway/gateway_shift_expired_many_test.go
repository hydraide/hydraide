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

// ---------- ShiftExpiredTreasuresMany (R2-7) ----------

// seedExpiredTreasureForShift creates a treasure under swampName and
// stamps a past ExpiredAt by patching meta. Used by R2-7 tests.
func seedExpiredTreasureForShift(t *testing.T, rig *gatewayPatchTestRig, swampName, key string, ago time.Duration) {
	t.Helper()
	_, err := rig.gw.PatchTreasures(context.Background(), &hydrapb.PatchTreasuresRequest{
		IslandID:         rig.islandID,
		SwampName:        swampName,
		CreateIfNotExist: true,
		Patches: []*hydrapb.TreasurePatch{
			{Key: key, Ops: []*hydrapb.PatchOp{{Op: hydrapb.PatchOp_SET, Path: "x", Value: encMsgpack(t, int8(1))}}},
		},
		Meta: &hydrapb.PatchMeta{SetExpiredAt: timestamppb.New(time.Now().UTC().Add(-ago))},
	})
	require.NoError(t, err)
}

func TestGatewayShiftExpiredMany_HappyPathPerSwampSelection(t *testing.T) {
	rig := newGatewayPatchTestRig(t, "gw-shift-many", "happy", "any")

	swampA := "gw-shift-many/swamp/a"
	swampB := "gw-shift-many/swamp/b"

	for i := 0; i < 4; i++ {
		seedExpiredTreasureForShift(t, rig, swampA, gatewayKey(i), time.Hour)
	}
	for i := 0; i < 2; i++ {
		seedExpiredTreasureForShift(t, rig, swampB, gatewayKey(i), time.Hour)
	}

	resp, err := rig.gw.ShiftExpiredTreasuresMany(context.Background(), &hydrapb.ShiftExpiredTreasuresManyRequest{
		Requests: []*hydrapb.ShiftExpiredTreasuresRequest{
			{IslandID: rig.islandID, SwampName: swampA, HowMany: 4},
			{IslandID: rig.islandID, SwampName: swampB, HowMany: 5},
		},
	})
	require.NoError(t, err)
	require.Len(t, resp.GetResponses(), 2)

	assert.Empty(t, resp.GetResponses()[0].GetError())
	assert.Len(t, resp.GetResponses()[0].GetTreasures(), 4, "swamp A")

	assert.Empty(t, resp.GetResponses()[1].GetError())
	assert.Len(t, resp.GetResponses()[1].GetTreasures(), 2, "swamp B (only 2 expired, HowMany=5 caps higher)")
}

func TestGatewayShiftExpiredMany_PerSwampErrorIsolation(t *testing.T) {
	rig := newGatewayPatchTestRig(t, "gw-shift-many-err", "isolation", "any")

	good := "gw-shift-many-err/swamp/good"
	for i := 0; i < 2; i++ {
		seedExpiredTreasureForShift(t, rig, good, gatewayKey(i), time.Hour)
	}

	resp, err := rig.gw.ShiftExpiredTreasuresMany(context.Background(), &hydrapb.ShiftExpiredTreasuresManyRequest{
		Requests: []*hydrapb.ShiftExpiredTreasuresRequest{
			// Bad: never-created swamp; the single ShiftExpired handler
			// surfaces a FailedPrecondition for missing swamps, which
			// the Many handler converts to a per-entry Error.
			{IslandID: rig.islandID, SwampName: "gw-shift-many-err/swamp/never-created", HowMany: 5},
			// Good.
			{IslandID: rig.islandID, SwampName: good, HowMany: 5},
		},
	})
	require.NoError(t, err, "per-swamp errors must not surface as gRPC error")
	require.Len(t, resp.GetResponses(), 2)

	assert.NotEmpty(t, resp.GetResponses()[0].GetError())
	assert.Empty(t, resp.GetResponses()[0].GetTreasures())

	assert.Empty(t, resp.GetResponses()[1].GetError())
	assert.Len(t, resp.GetResponses()[1].GetTreasures(), 2)
}

func TestGatewayShiftExpiredMany_EmptyRequestsIsNoop(t *testing.T) {
	rig := newGatewayPatchTestRig(t, "gw-shift-many-empty", "empty", "any")
	resp, err := rig.gw.ShiftExpiredTreasuresMany(context.Background(), &hydrapb.ShiftExpiredTreasuresManyRequest{})
	require.NoError(t, err)
	assert.Empty(t, resp.GetResponses())
}
