package gateway

import (
	"context"
	"sync"
	"testing"
	"time"

	hydrapb "github.com/hydraide/hydraide/sdk/go/hydraidego/v3/hydraidepbgo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// seedExpiredViaGateway creates a treasure under the gateway's swamp and
// stamps a past ExpiredAt by patching meta. Sequence:
//  1. Seed the body via PatchTreasures with CreateIfNotExist.
//  2. Slide ExpiredAt to a past time via a second PatchTreasures call
//     (Meta.SetExpiredAt). The gateway path is what test code uses, so
//     we drive it the same way an e2e caller would.
func seedExpiredViaGateway(t *testing.T, rig *gatewayPatchTestRig, key string, fields map[string]any, expiredAgo time.Duration) {
	t.Helper()
	ops := make([]*hydrapb.PatchOp, 0, len(fields))
	for k, v := range fields {
		ops = append(ops, &hydrapb.PatchOp{
			Op:    hydrapb.PatchOp_SET,
			Path:  k,
			Value: encMsgpack(t, v),
		})
	}
	_, err := rig.gw.PatchTreasures(context.Background(), &hydrapb.PatchTreasuresRequest{
		IslandID:         rig.islandID,
		SwampName:        rig.swampName,
		CreateIfNotExist: true,
		Patches:          []*hydrapb.TreasurePatch{{Key: key, Ops: ops}},
		Meta: &hydrapb.PatchMeta{
			SetExpiredAt: timestamppb.New(time.Now().UTC().Add(-expiredAgo)),
		},
	})
	require.NoError(t, err)
}

// ---------- D.1 — empty SwampName rejected ----------

func TestGatewayPatchExpired_EmptySwampNameRejected(t *testing.T) {
	rig := newGatewayPatchTestRig(t, "gw-patch-exp-1", "empty-name", "any")
	_, err := rig.gw.PatchExpiredTreasures(context.Background(), &hydrapb.PatchExpiredTreasuresRequest{
		IslandID:  rig.islandID,
		SwampName: "",
		HowMany:   10,
		Ops: []*hydrapb.PatchOp{
			{Op: hydrapb.PatchOp_SET, Path: "x", Value: encMsgpack(t, int8(1))},
		},
	})
	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.InvalidArgument, st.Code())
}

// ---------- D.2 — no ops + nil meta rejected ----------

func TestGatewayPatchExpired_EmptyOpsAndNilMetaRejected(t *testing.T) {
	rig := newGatewayPatchTestRig(t, "gw-patch-exp-2", "no-op", "no-meta")
	_, err := rig.gw.PatchExpiredTreasures(context.Background(), &hydrapb.PatchExpiredTreasuresRequest{
		IslandID:  rig.islandID,
		SwampName: rig.swampName,
		HowMany:   10,
		// no Ops, no Meta
	})
	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.InvalidArgument, st.Code())
}

// ---------- D.3 — missing swamp returns empty, not an error ----------

func TestGatewayPatchExpired_MissingSwampReturnsEmpty(t *testing.T) {
	rig := newGatewayPatchTestRig(t, "gw-patch-exp-3", "missing", "swamp")
	resp, err := rig.gw.PatchExpiredTreasures(context.Background(), &hydrapb.PatchExpiredTreasuresRequest{
		IslandID:  rig.islandID,
		SwampName: rig.swampName,
		HowMany:   10,
		Ops: []*hydrapb.PatchOp{
			{Op: hydrapb.PatchOp_SET, Path: "x", Value: encMsgpack(t, int8(1))},
		},
	})
	require.NoError(t, err, "missing swamp must not surface as gRPC error")
	assert.Empty(t, resp.GetPatched())
}

// ---------- D.4 — happy path: ops + meta slide ExpiredAt ----------

func TestGatewayPatchExpired_HappyPathSlide(t *testing.T) {
	rig := newGatewayPatchTestRig(t, "gw-patch-exp-4", "happy", "slide")
	for _, k := range []string{"a", "b", "c"} {
		seedExpiredViaGateway(t, rig, k, map[string]any{"x": int8(0)}, time.Hour)
	}

	newExp := time.Now().UTC().Add(24 * time.Hour)
	resp, err := rig.gw.PatchExpiredTreasures(context.Background(), &hydrapb.PatchExpiredTreasuresRequest{
		IslandID:  rig.islandID,
		SwampName: rig.swampName,
		HowMany:   10,
		Ops: []*hydrapb.PatchOp{
			{Op: hydrapb.PatchOp_SET, Path: "x", Value: encMsgpack(t, int8(1))},
		},
		Meta: &hydrapb.PatchMeta{
			SetExpiredAt: timestamppb.New(newExp),
			SetUpdatedAt: true,
		},
	})
	require.NoError(t, err)
	require.Equal(t, 3, len(resp.GetPatched()))
	for _, p := range resp.GetPatched() {
		assert.Equal(t, hydrapb.PatchResult_PATCHED, p.GetStatus())
		require.NotNil(t, p.ExpiredAt)
		assert.WithinDuration(t, newExp, p.GetExpiredAt().AsTime(), time.Second)
		// Body echo carries the patched msgpack body (no magic prefix).
		assert.NotEmpty(t, p.GetNewMsgpack())
	}

	// Second call returns nothing — every entry has a future ExpiredAt.
	resp2, err := rig.gw.PatchExpiredTreasures(context.Background(), &hydrapb.PatchExpiredTreasuresRequest{
		IslandID:  rig.islandID,
		SwampName: rig.swampName,
		HowMany:   10,
		Ops: []*hydrapb.PatchOp{
			{Op: hydrapb.PatchOp_SET, Path: "x", Value: encMsgpack(t, int8(9))},
		},
	})
	require.NoError(t, err)
	assert.Empty(t, resp2.GetPatched())
}

// ---------- D.5 — Condition: per-treasure CONDITION_NOT_MET ----------

func TestGatewayPatchExpired_ConditionNotMetRetainsEntry(t *testing.T) {
	rig := newGatewayPatchTestRig(t, "gw-patch-exp-5", "cond", "gate")
	seedExpiredViaGateway(t, rig, "free", map[string]any{"claimedBy": ""}, time.Hour)
	seedExpiredViaGateway(t, rig, "taken", map[string]any{"claimedBy": "X"}, time.Hour)

	resp, err := rig.gw.PatchExpiredTreasures(context.Background(), &hydrapb.PatchExpiredTreasuresRequest{
		IslandID:  rig.islandID,
		SwampName: rig.swampName,
		HowMany:   10,
		Ops: []*hydrapb.PatchOp{
			{Op: hydrapb.PatchOp_SET, Path: "claimedBy", Value: encMsgpack(t, "Y")},
		},
		Meta: &hydrapb.PatchMeta{SetExpiredAt: timestamppb.New(time.Now().UTC().Add(time.Hour))},
		Condition: &hydrapb.PatchCondition{
			Path:      "claimedBy",
			Operator:  hydrapb.PatchCondition_EQUAL,
			Threshold: encMsgpack(t, ""),
		},
	})
	require.NoError(t, err)
	require.Equal(t, 2, len(resp.GetPatched()))

	statuses := map[string]hydrapb.PatchResult_StatusCode{}
	for _, p := range resp.GetPatched() {
		statuses[p.GetKey()] = p.GetStatus()
	}
	assert.Equal(t, hydrapb.PatchResult_PATCHED, statuses["free"])
	assert.Equal(t, hydrapb.PatchResult_CONDITION_NOT_MET, statuses["taken"])

	// "taken" must still be expired-visible on a follow-up call (it
	// retained its original past ExpiredAt).
	resp2, err := rig.gw.PatchExpiredTreasures(context.Background(), &hydrapb.PatchExpiredTreasuresRequest{
		IslandID:  rig.islandID,
		SwampName: rig.swampName,
		HowMany:   10,
		Ops: []*hydrapb.PatchOp{
			{Op: hydrapb.PatchOp_SET, Path: "claimedBy", Value: encMsgpack(t, "Z")},
		},
		Meta: &hydrapb.PatchMeta{SetExpiredAt: timestamppb.New(time.Now().UTC().Add(time.Hour))},
	})
	require.NoError(t, err)
	require.Equal(t, 1, len(resp2.GetPatched()))
	assert.Equal(t, "taken", resp2.GetPatched()[0].GetKey())
}

// ---------- D.6 — concurrent disjoint subsets across the gateway ----------

func TestGatewayPatchExpired_ConcurrentDisjoint(t *testing.T) {
	const total = 60
	rig := newGatewayPatchTestRig(t, "gw-patch-exp-6", "concurrent", "claim")
	for i := 0; i < total; i++ {
		seedExpiredViaGateway(t, rig, gatewayKey(i), map[string]any{"claimedBy": ""}, time.Hour)
	}

	const goroutines = 4
	const batch = 15
	var (
		mu   sync.Mutex
		seen = make(map[string]int)
		wg   sync.WaitGroup
	)
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			resp, err := rig.gw.PatchExpiredTreasures(context.Background(), &hydrapb.PatchExpiredTreasuresRequest{
				IslandID:  rig.islandID,
				SwampName: rig.swampName,
				HowMany:   batch,
				Ops: []*hydrapb.PatchOp{
					{Op: hydrapb.PatchOp_SET, Path: "claimedBy", Value: encMsgpack(t, gatewayWorkerID(workerID))},
				},
				Meta: &hydrapb.PatchMeta{SetExpiredAt: timestamppb.New(time.Now().UTC().Add(24 * time.Hour))},
			})
			require.NoError(t, err)
			mu.Lock()
			defer mu.Unlock()
			for _, p := range resp.GetPatched() {
				seen[p.GetKey()]++
			}
		}(g)
	}
	wg.Wait()

	assert.Equal(t, goroutines*batch, sumValues(seen))
	for k, n := range seen {
		assert.Equal(t, 1, n, "key %s claimed %d times", k, n)
	}
}

func gatewayKey(i int) string         { return "k-" + intToFixed(i) }
func gatewayWorkerID(i int) string    { return "worker-" + intToFixed(i) }
func intToFixed(i int) string         { return string(rune('0' + (i / 100 % 10))) + string(rune('0' + (i / 10 % 10))) + string(rune('0' + (i % 10))) }
func sumValues(m map[string]int) int  { s := 0; for _, v := range m { s += v }; return s }
