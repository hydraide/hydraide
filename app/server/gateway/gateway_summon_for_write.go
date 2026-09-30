package gateway

import (
	"context"
	"fmt"

	"github.com/hydraide/hydraide/app/core/hydra"
	"github.com/hydraide/hydraide/app/core/hydra/swamp"
	"github.com/hydraide/hydraide/app/name"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// summonForWriteMaxAttempts bounds the re-summon loop below. Two attempts are
// already enough in theory — the second summon builds a brand new instance
// that nobody is evicting — so a third exists purely so a pathologically
// unlucky caller is not failed on a technicality. An unbounded loop is not an
// option: if something ever kept swamps closing immediately, it would spin
// forever instead of surfacing the problem.
const summonForWriteMaxAttempts = 3

// summonSwampForWrite summons a swamp for a MUTATING operation and hands it
// back with an active vigil already held. The returned release func must be
// deferred by the caller; it releases that vigil.
//
// Why writes need this and reads do not:
//
// SummonSwamp asks the resident instance IsClosing() before handing it over,
// which also bumps the idle clock so the evictor leaves the swamp alone while
// the caller travels to BeginVigil(). But the vigil only exists AFTER the
// caller takes it, so between the summon returning and BeginVigil() the swamp
// is protected by a clock bump rather than by a vigil. A swamp that is torn
// down inside that gap — by the idle evictor, or by a concurrent Destroy()
// which also flips the closing flag — leaves the caller holding an orphaned
// instance: already flushed, writer goroutine already stopped, and no longer
// in hydra's registry. A write into it is accepted, reported as successful,
// and silently never reaches disk.
//
// This helper closes that gap by taking the vigil and only then asking whether
// the instance is still alive. IsClosing() answers under the same closeMutex
// the evictor uses for its decision, so the two cannot interleave: either we
// observe closing==1 and discard this instance, or the evictor observes our
// vigil and leaves it open. A discarded instance costs one re-summon, which
// returns a freshly built swamp read from disk.
//
// Read paths deliberately keep the plain SummonSwamp + BeginVigil shape: a
// read against an orphaned instance returns the data it was loaded with, which
// is correct, merely stale by moments. Destroy paths must NOT use this helper
// at all — Destroy() waits for vigils to drain, so entering it with a vigil in
// hand would deadlock until the timeout.
func summonSwampForWrite(ctx context.Context, hydraInterface hydra.Hydra, islandID uint64, swampName name.Name) (swamp.Swamp, func(), error) {

	for attempt := 0; attempt < summonForWriteMaxAttempts; attempt++ {

		swampInterface, err := hydraInterface.SummonSwamp(ctx, islandID, swampName)
		if err != nil {
			return nil, nil, hydraErrorStatus(err)
		}

		// Take the vigil FIRST, then verify. Doing it the other way round is
		// exactly the race this helper exists to avoid.
		swampInterface.BeginVigil()

		if !swampInterface.IsClosing() {
			return swampInterface, swampInterface.CeaseVigil, nil
		}

		// The instance is on its way out and our write would be lost in it.
		// Drop the vigil so the teardown can finish, then summon again — the
		// next summon waits for the close and builds a fresh instance.
		swampInterface.CeaseVigil()

	}

	return nil, nil, status.Error(codes.Unavailable,
		fmt.Sprintf("swamp %s kept closing across %d summon attempts, write not applied",
			swampName.Get(), summonForWriteMaxAttempts))

}
