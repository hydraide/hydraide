package gateway

import (
	"context"
	"testing"
	"time"

	"github.com/hydraide/hydraide/app/core/filesystem"
	"github.com/hydraide/hydraide/app/core/settings"
	"github.com/hydraide/hydraide/app/core/zeus"
	"github.com/hydraide/hydraide/app/name"
	hydrapb "github.com/hydraide/hydraide/sdk/go/hydraidego/hydraidepbgo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vmihailenco/msgpack/v5"
)

// gatewayPatchTestRig spins up a real Zeus + Hydra wired Gateway so the
// PatchTreasures handler can be exercised end-to-end without standing up
// the gRPC layer.
type gatewayPatchTestRig struct {
	gw            Gateway
	swampName     string
	islandID      uint64
	settings      settings.Settings
	teardownHooks []func()
}

func (r *gatewayPatchTestRig) close() {
	for _, fn := range r.teardownHooks {
		fn()
	}
}

func newGatewayPatchTestRig(t *testing.T, sanctuary, realm, swampN string) *gatewayPatchTestRig {
	t.Helper()
	const (
		maxDepth        = 3
		maxFolderPerLvl = 2000
	)
	settingsInterface := settings.New(maxDepth, maxFolderPerLvl)
	settingsInterface.RegisterPattern(
		name.New().Sanctuary(sanctuary).Realm("*").Swamp("*"),
		false, 1,
		&settings.FileSystemSettings{WriteIntervalSec: 1, MaxFileSizeByte: 8192},
	)
	fsInterface := filesystem.New()
	zeusInterface := zeus.New(settingsInterface, fsInterface)
	zeusInterface.StartHydra()

	gw := Gateway{
		SettingsInterface:     settingsInterface,
		ZeusInterface:         zeusInterface,
		DefaultCloseAfterIdle: 1,
		DefaultWriteInterval:  1,
		DefaultFileSize:       8192,
	}

	swampName := name.New().Sanctuary(sanctuary).Realm(realm).Swamp(swampN).Get()
	rig := &gatewayPatchTestRig{
		gw:        gw,
		swampName: swampName,
		islandID:  1,
		settings:  settingsInterface,
	}
	t.Cleanup(rig.close)
	return rig
}

// encMsgpack marshals v to a stand-alone msgpack blob.
func encMsgpack(t *testing.T, v any) []byte {
	t.Helper()
	b, err := msgpack.Marshal(v)
	require.NoError(t, err)
	return b
}

// ---------- C.1 — single key, single op ----------

func TestGatewayPatch_SingleKeySingleOp(t *testing.T) {
	rig := newGatewayPatchTestRig(t, "gw-patch-1", "single-key", "single-op")

	resp, err := rig.gw.PatchTreasures(context.Background(), &hydrapb.PatchTreasuresRequest{
		IslandID:         rig.islandID,
		SwampName:        rig.swampName,
		CreateIfNotExist: true,
		Patches: []*hydrapb.TreasurePatch{
			{
				Key: "alice",
				Ops: []*hydrapb.PatchOp{
					{Op: hydrapb.PatchOp_SET, Path: "name", Value: encMsgpack(t, "alice")},
				},
			},
		},
	})
	require.NoError(t, err)
	require.Len(t, resp.GetResults(), 1)

	got := resp.GetResults()[0]
	assert.Equal(t, "alice", got.GetKey())
	assert.Equal(t, hydrapb.PatchResult_CREATED, got.GetStatus())
}

// ---------- C.2 — multi-key batch preserves order ----------

func TestGatewayPatch_MultiKeyBatchOrder(t *testing.T) {
	rig := newGatewayPatchTestRig(t, "gw-patch-2", "multi", "batch")

	keys := []string{"k1", "k2", "k3", "k4"}
	patches := make([]*hydrapb.TreasurePatch, 0, len(keys))
	for _, k := range keys {
		patches = append(patches, &hydrapb.TreasurePatch{
			Key: k,
			Ops: []*hydrapb.PatchOp{
				{Op: hydrapb.PatchOp_SET, Path: "k", Value: encMsgpack(t, k)},
			},
		})
	}
	resp, err := rig.gw.PatchTreasures(context.Background(), &hydrapb.PatchTreasuresRequest{
		IslandID:         rig.islandID,
		SwampName:        rig.swampName,
		CreateIfNotExist: true,
		Patches:          patches,
	})
	require.NoError(t, err)
	require.Len(t, resp.GetResults(), len(keys))

	for i, k := range keys {
		assert.Equal(t, k, resp.GetResults()[i].GetKey(), "result %d", i)
		assert.Equal(t, hydrapb.PatchResult_CREATED, resp.GetResults()[i].GetStatus())
	}
}

// ---------- C.3 — failing key does not stop batch ----------

func TestGatewayPatch_FailingKeyDoesNotStopBatch(t *testing.T) {
	rig := newGatewayPatchTestRig(t, "gw-patch-3", "failing", "batch")

	// Seed k2 with a non-msgpack ByteArray that should yield ENCODING_NOT_SUPPORTED.
	hydraInterface := rig.gw.ZeusInterface.GetHydra()
	loaded := name.Load(rig.swampName)
	swampObj, err := hydraInterface.SummonSwamp(context.Background(), rig.islandID, loaded)
	require.NoError(t, err)
	swampObj.BeginVigil()
	tr := swampObj.CreateTreasure("k2")
	gid := tr.StartTreasureGuard(true)
	tr.SetContentByteArray(gid, []byte{0x01, 0x02, 0x03}) // no magic prefix
	tr.Save(gid)
	tr.ReleaseTreasureGuard(gid)
	swampObj.CeaseVigil()

	resp, err := rig.gw.PatchTreasures(context.Background(), &hydrapb.PatchTreasuresRequest{
		IslandID:         rig.islandID,
		SwampName:        rig.swampName,
		CreateIfNotExist: true,
		Patches: []*hydrapb.TreasurePatch{
			{Key: "k1", Ops: []*hydrapb.PatchOp{{Op: hydrapb.PatchOp_SET, Path: "x", Value: encMsgpack(t, int8(1))}}},
			{Key: "k2", Ops: []*hydrapb.PatchOp{{Op: hydrapb.PatchOp_SET, Path: "x", Value: encMsgpack(t, int8(2))}}},
			{Key: "k3", Ops: []*hydrapb.PatchOp{{Op: hydrapb.PatchOp_SET, Path: "x", Value: encMsgpack(t, int8(3))}}},
		},
	})
	require.NoError(t, err)
	require.Len(t, resp.GetResults(), 3)

	assert.Equal(t, hydrapb.PatchResult_CREATED, resp.GetResults()[0].GetStatus(), "k1")
	assert.Equal(t, hydrapb.PatchResult_ENCODING_NOT_SUPPORTED, resp.GetResults()[1].GetStatus(), "k2")
	assert.Equal(t, hydrapb.PatchResult_CREATED, resp.GetResults()[2].GetStatus(), "k3")
}

// ---------- C.4 — empty patches returns empty results ----------

func TestGatewayPatch_EmptyPatches(t *testing.T) {
	rig := newGatewayPatchTestRig(t, "gw-patch-4", "empty", "patches")

	resp, err := rig.gw.PatchTreasures(context.Background(), &hydrapb.PatchTreasuresRequest{
		IslandID:  rig.islandID,
		SwampName: rig.swampName,
		Patches:   nil,
	})
	require.NoError(t, err)
	assert.Empty(t, resp.GetResults())
}

// ---------- Validation ----------

func TestGatewayPatch_EmptySwampNameRejected(t *testing.T) {
	rig := newGatewayPatchTestRig(t, "gw-patch-val", "swamp-name", "validation")

	_, err := rig.gw.PatchTreasures(context.Background(), &hydrapb.PatchTreasuresRequest{
		IslandID:  rig.islandID,
		SwampName: "",
		Patches: []*hydrapb.TreasurePatch{
			{Key: "k", Ops: []*hydrapb.PatchOp{{Op: hydrapb.PatchOp_SET, Path: "x", Value: encMsgpack(t, int8(1))}}},
		},
	})
	require.Error(t, err)
}

// ---------- Condition wire-up ----------

func TestGatewayPatch_ConditionEvaluated(t *testing.T) {
	rig := newGatewayPatchTestRig(t, "gw-patch-cond", "condition", "wireup")

	// Seed an existing treasure.
	_, err := rig.gw.PatchTreasures(context.Background(), &hydrapb.PatchTreasuresRequest{
		IslandID:         rig.islandID,
		SwampName:        rig.swampName,
		CreateIfNotExist: true,
		Patches: []*hydrapb.TreasurePatch{
			{Key: "k", Ops: []*hydrapb.PatchOp{
				{Op: hydrapb.PatchOp_SET, Path: "owner", Value: encMsgpack(t, "alice")},
				{Op: hydrapb.PatchOp_SET, Path: "n", Value: encMsgpack(t, int8(0))},
			}},
		},
	})
	require.NoError(t, err)

	// Patch under a condition that holds.
	resp, err := rig.gw.PatchTreasures(context.Background(), &hydrapb.PatchTreasuresRequest{
		IslandID:  rig.islandID,
		SwampName: rig.swampName,
		Patches: []*hydrapb.TreasurePatch{
			{
				Key:       "k",
				Ops:       []*hydrapb.PatchOp{{Op: hydrapb.PatchOp_SET, Path: "n", Value: encMsgpack(t, int8(7))}},
				Condition: &hydrapb.PatchCondition{Path: "owner", Operator: hydrapb.PatchCondition_EQUAL, Threshold: encMsgpack(t, "alice")},
			},
		},
	})
	require.NoError(t, err)
	assert.Equal(t, hydrapb.PatchResult_PATCHED, resp.GetResults()[0].GetStatus())

	// Patch under a condition that does NOT hold.
	resp, err = rig.gw.PatchTreasures(context.Background(), &hydrapb.PatchTreasuresRequest{
		IslandID:  rig.islandID,
		SwampName: rig.swampName,
		Patches: []*hydrapb.TreasurePatch{
			{
				Key:       "k",
				Ops:       []*hydrapb.PatchOp{{Op: hydrapb.PatchOp_SET, Path: "n", Value: encMsgpack(t, int8(99))}},
				Condition: &hydrapb.PatchCondition{Path: "owner", Operator: hydrapb.PatchCondition_EQUAL, Threshold: encMsgpack(t, "bob")},
			},
		},
	})
	require.NoError(t, err)
	assert.Equal(t, hydrapb.PatchResult_CONDITION_NOT_MET, resp.GetResults()[0].GetStatus())
}

// ---------- Meta wire-up ----------

func TestGatewayPatch_MetaUpdatedAtPropagates(t *testing.T) {
	rig := newGatewayPatchTestRig(t, "gw-patch-meta", "meta", "updatedat")

	// Seed.
	_, err := rig.gw.PatchTreasures(context.Background(), &hydrapb.PatchTreasuresRequest{
		IslandID:         rig.islandID,
		SwampName:        rig.swampName,
		CreateIfNotExist: true,
		Patches: []*hydrapb.TreasurePatch{
			{Key: "k", Ops: []*hydrapb.PatchOp{{Op: hydrapb.PatchOp_SET, Path: "n", Value: encMsgpack(t, int8(0))}}},
		},
	})
	require.NoError(t, err)

	// Read modified-at before.
	hydraInterface := rig.gw.ZeusInterface.GetHydra()
	loaded := name.Load(rig.swampName)
	swampObj, err := hydraInterface.SummonSwamp(context.Background(), rig.islandID, loaded)
	require.NoError(t, err)
	swampObj.BeginVigil()
	tr, err := swampObj.GetTreasure("k")
	require.NoError(t, err)
	before := tr.GetModifiedAt()
	swampObj.CeaseVigil()

	time.Sleep(5 * time.Millisecond)

	// Patch with Meta.SetUpdatedAt + SetUpdatedBy.
	updatedBy := "tester"
	_, err = rig.gw.PatchTreasures(context.Background(), &hydrapb.PatchTreasuresRequest{
		IslandID:  rig.islandID,
		SwampName: rig.swampName,
		Meta:      &hydrapb.PatchMeta{SetUpdatedAt: true, SetUpdatedBy: &updatedBy},
		Patches: []*hydrapb.TreasurePatch{
			{Key: "k", Ops: []*hydrapb.PatchOp{{Op: hydrapb.PatchOp_INC, Path: "n", Value: encMsgpack(t, int8(1))}}},
		},
	})
	require.NoError(t, err)

	swampObj, err = hydraInterface.SummonSwamp(context.Background(), rig.islandID, loaded)
	require.NoError(t, err)
	swampObj.BeginVigil()
	defer swampObj.CeaseVigil()
	tr, err = swampObj.GetTreasure("k")
	require.NoError(t, err)
	assert.Greater(t, tr.GetModifiedAt(), before, "ModifiedAt must advance")
	assert.Equal(t, "tester", tr.GetModifiedBy())
}
