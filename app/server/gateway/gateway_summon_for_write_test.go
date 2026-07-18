package gateway

import (
	"context"
	"testing"

	"github.com/hydraide/hydraide/app/name"
	"github.com/stretchr/testify/require"
)

// TestSummonSwampForWriteReturnsVigiledInstance pins the contract that every
// mutating gateway handler now depends on: the swamp comes back with a vigil
// ALREADY held, and it is not an instance that is on its way out.
//
// The ordering is the whole point. A handler that summoned first and took the
// vigil afterwards left a gap in which the idle evictor (or a concurrent
// Destroy) could tear the instance down; the handler then wrote into an
// orphan, was told the write succeeded, and the data never reached disk. The
// engine-level half of that race is covered by
// swamp.TestLostWriteOnCloseListenerRace; this test guards the gateway half,
// namely that handlers cannot reintroduce the gap by construction.
func TestSummonSwampForWriteReturnsVigiledInstance(t *testing.T) {

	rig := newStreamVigilRig(t, "summon-for-write", "write-path", "sw-1")
	hydraInterface := rig.gw.ZeusInterface.GetHydra()

	swampObj, releaseVigil, err := summonSwampForWrite(
		context.Background(), hydraInterface, rig.islandID, name.Load(rig.swampName))
	require.NoError(t, err)
	require.NotNil(t, swampObj)
	require.NotNil(t, releaseVigil)

	require.True(t, swampObj.HasActiveVigils(),
		"the write path must receive the swamp with the vigil already held, otherwise the "+
			"summon -> BeginVigil gap is back and a concurrent eviction can swallow the write")
	require.False(t, swampObj.IsClosing(),
		"a closing instance must never be handed to a mutating handler")

	// The returned release func is the caller's only handle on that vigil; if
	// it did not actually release, the swamp could never be evicted again.
	releaseVigil()
	require.False(t, swampObj.HasActiveVigils(),
		"the release func must drop the vigil taken at summon time")

}

// TestSummonSwampForWriteSurvivesAggressiveEviction drives the helper in a
// loop against a swamp configured to be evicted almost immediately, which is
// the shape of the Trendizz queue swamps that first exposed this bug
// (CloseAfterIdle measured in single-digit seconds, constant re-summoning).
// Every attempt must hand back a live, writable instance.
func TestSummonSwampForWriteSurvivesAggressiveEviction(t *testing.T) {

	rig := newStreamVigilRig(t, "summon-for-write", "eviction-churn", "sw-2")
	hydraInterface := rig.gw.ZeusInterface.GetHydra()
	swampName := name.Load(rig.swampName)

	for i := 0; i < 50; i++ {

		swampObj, releaseVigil, err := summonSwampForWrite(
			context.Background(), hydraInterface, rig.islandID, swampName)
		require.NoErrorf(t, err, "iteration %d: summon for write failed", i)
		require.Falsef(t, swampObj.IsClosing(),
			"iteration %d: handler was handed a closing instance", i)

		// A real write, so the treasure has to survive whatever the evictor
		// is doing concurrently.
		key := "churn-key"
		tr := swampObj.CreateTreasure(key)
		require.NotNilf(t, tr, "iteration %d: CreateTreasure returned nil", i)
		gid := tr.StartTreasureGuard(true)
		tr.SetContentString(gid, "value")
		tr.Save(gid)
		tr.ReleaseTreasureGuard(gid)

		require.Truef(t, swampObj.TreasureExists(key),
			"iteration %d: the treasure vanished from the instance it was written to", i)

		releaseVigil()

	}

}
