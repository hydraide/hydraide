package gateway

import (
	"sort"
	"time"

	hydra "github.com/hydraide/hydraide/app/core/hydra/swamp"
	"github.com/hydraide/hydraide/app/core/hydra/swamp/treasure"
)

// bucketExecPreconditions reports whether a planner-routed execution
// can faithfully reproduce the legacy beacon-walk behaviour for the
// given request shape.
//
// v1 supports sorting and time-range filtering on the four time-based
// beacons (key, creation-, update-, expiration-time). Value-axis
// sorting (BeaconTypeValueInt64 etc.) is not yet supported through the
// bucket path because each Treasure's "value" field varies by content
// type; we fall back to Bypass for those.
func bucketExecPreconditions(beaconType hydra.BeaconType) bool {
	switch beaconType {
	case hydra.BeaconTypeKey,
		hydra.BeaconTypeCreationTime,
		hydra.BeaconTypeUpdateTime,
		hydra.BeaconTypeExpirationTime:
		return true
	}
	return false
}

// collectBucketCandidates resolves every hint against the swamp and
// returns the deduplicated union, in arbitrary order.
func collectBucketCandidates(sw hydra.Swamp, hints []BucketHint) []treasure.Treasure {
	if len(hints) == 0 {
		return nil
	}
	if len(hints) == 1 {
		h := hints[0]
		switch h.Op {
		case HintEqual:
			if len(h.Values) == 0 {
				return nil
			}
			return sw.LookupByBucketEqual(h.FieldPath, h.Values[0])
		case HintIn:
			return sw.LookupByBucketIn(h.FieldPath, h.Values)
		}
		return nil
	}
	// OrUnion: dedupe across hints by treasure key.
	seen := make(map[string]struct{}, 32)
	out := make([]treasure.Treasure, 0, 32)
	for _, h := range hints {
		var hits []treasure.Treasure
		switch h.Op {
		case HintEqual:
			if len(h.Values) > 0 {
				hits = sw.LookupByBucketEqual(h.FieldPath, h.Values[0])
			}
		case HintIn:
			hits = sw.LookupByBucketIn(h.FieldPath, h.Values)
		}
		for _, t := range hits {
			k := t.GetKey()
			if _, dup := seen[k]; dup {
				continue
			}
			seen[k] = struct{}{}
			out = append(out, t)
		}
	}
	return out
}

// applyTimeRange filters candidates by the beacon-type's time field
// against [fromTime, toTime]. Either or both bounds may be nil.
func applyTimeRange(candidates []treasure.Treasure, beaconType hydra.BeaconType, fromTime, toTime *time.Time) []treasure.Treasure {
	if fromTime == nil && toTime == nil {
		return candidates
	}
	var fromNs, toNs int64
	if fromTime != nil {
		fromNs = fromTime.UnixNano()
	}
	if toTime != nil {
		toNs = toTime.UnixNano()
	}
	out := candidates[:0]
	for _, t := range candidates {
		ts := beaconTimeOf(t, beaconType)
		if fromTime != nil && ts < fromNs {
			continue
		}
		if toTime != nil && ts > toNs {
			continue
		}
		out = append(out, t)
	}
	return out
}

func beaconTimeOf(t treasure.Treasure, beaconType hydra.BeaconType) int64 {
	switch beaconType {
	case hydra.BeaconTypeCreationTime:
		return t.GetCreatedAt()
	case hydra.BeaconTypeUpdateTime:
		return t.GetModifiedAt()
	case hydra.BeaconTypeExpirationTime:
		return t.GetExpirationTime()
	}
	return 0
}

// sortCandidates sorts in place by the beacon axis. Key sort uses the
// Treasure's string key; the time beacons compare the corresponding
// timestamp. Ascending unless order indicates descending.
func sortCandidates(candidates []treasure.Treasure, beaconType hydra.BeaconType, order hydra.BeaconOrder) {
	desc := order == hydra.IndexOrderDesc
	less := func(i, j int) bool { return false }
	switch beaconType {
	case hydra.BeaconTypeKey:
		less = func(i, j int) bool {
			if desc {
				return candidates[i].GetKey() > candidates[j].GetKey()
			}
			return candidates[i].GetKey() < candidates[j].GetKey()
		}
	case hydra.BeaconTypeCreationTime, hydra.BeaconTypeUpdateTime, hydra.BeaconTypeExpirationTime:
		less = func(i, j int) bool {
			a := beaconTimeOf(candidates[i], beaconType)
			b := beaconTimeOf(candidates[j], beaconType)
			if desc {
				return a > b
			}
			return a < b
		}
	default:
		return
	}
	sort.SliceStable(candidates, less)
}

// candidateKeySet builds a lookup-friendly key set from a candidate
// slice. Used by the cap-bearing flows (PatchExpired, ShiftMatching)
// which keep the engine's beacon-walk-based atomicity primitive and
// rely on a wrapped selectionPredicate to fast-reject non-candidates.
func candidateKeySet(candidates []treasure.Treasure) map[string]struct{} {
	if len(candidates) == 0 {
		return nil
	}
	out := make(map[string]struct{}, len(candidates))
	for _, t := range candidates {
		out[t.GetKey()] = struct{}{}
	}
	return out
}

// applyFromLimit returns a sub-slice of candidates respecting the
// from-offset and limit (limit <= 0 means "no upper bound").
func applyFromLimit(candidates []treasure.Treasure, from int32, limit int32) []treasure.Treasure {
	if from < 0 {
		from = 0
	}
	if int(from) >= len(candidates) {
		return nil
	}
	candidates = candidates[from:]
	if limit > 0 && int(limit) < len(candidates) {
		candidates = candidates[:limit]
	}
	return candidates
}
