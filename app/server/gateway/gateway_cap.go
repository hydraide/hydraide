package gateway

import (
	"errors"

	"github.com/hydraide/hydraide/app/core/hydra/swamp/treasure"
	hydrapb "github.com/hydraide/hydraide/sdk/go/hydraidego/v3/hydraidepbgo"
)

// buildCapPredicate converts a proto Cap message into the engine-level
// (predicate, max) pair consumed by ShiftMatching / PatchExpired. Returns
// (nil, 0, nil) when Cap is absent — engine path skips the cap check.
//
// Validation rejects invalid Cap shapes early so the engine layer can
// trust the inputs:
//   - Cap.Filter is required when Cap is set;
//   - Cap.MaxMatching must be > 0.
//
// The predicate closes over the FilterGroup and reuses the same native
// filter evaluator the streaming/read paths use, so Cap semantics align
// 1:1 with Filters elsewhere.
func buildCapPredicate(cap *hydrapb.Cap) (func(treasure.Treasure) bool, int32, error) {
	if cap == nil {
		return nil, 0, nil
	}
	if cap.GetMaxMatching() <= 0 {
		return nil, 0, errors.New("Cap.MaxMatching must be > 0")
	}
	filter := cap.GetFilter()
	if filter == nil {
		return nil, 0, errors.New("Cap.Filter is required when Cap is set")
	}
	predicate := func(t treasure.Treasure) bool {
		return evaluateNativeFilterGroup(t, filter)
	}
	return predicate, cap.GetMaxMatching(), nil
}
