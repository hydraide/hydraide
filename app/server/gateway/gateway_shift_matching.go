package gateway

import (
	"context"
	"fmt"
	"time"

	"github.com/hydraide/hydraide/app/core/hydra/swamp"
	"github.com/hydraide/hydraide/app/core/hydra/swamp/treasure"
	hydrapb "github.com/hydraide/hydraide/sdk/go/hydraidego/v3/hydraidepbgo"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// ShiftMatchingTreasures is the parametric generalisation of
// ShiftExpiredTreasures: it atomically selects, removes, and returns up
// to HowMany treasures from a single swamp, ordered by IndexType +
// OrderType, narrowed by optional Filters, optionally bounded by Cap.
// Selection + deletion run under the beacon mu, so concurrent callers
// receive disjoint subsets.
//
// Per-swamp business outcomes (cap exhausted → CapReached=true, no
// matches → empty Treasures) are surfaced on the response. The gRPC
// error return is reserved for transport / configuration failures.
func (g Gateway) ShiftMatchingTreasures(ctx context.Context, in *hydrapb.ShiftMatchingTreasuresRequest) (*hydrapb.ShiftMatchingTreasuresResponse, error) {

	g.ZeusInterface.GetSafeops().LockSystem()
	defer g.ZeusInterface.GetSafeops().UnlockSystem()

	defer handlePanic()

	treasures, capReached, err := shiftMatchingOneSwamp(ctx, g, in)
	if err != nil {
		return nil, err
	}
	return &hydrapb.ShiftMatchingTreasuresResponse{Treasures: treasures, CapReached: capReached}, nil
}

// ShiftMatchingTreasuresMany dispatches a batch of ShiftMatchingTreasures
// requests against multiple swamps on the same server. Each entry runs
// independently; per-swamp failures populate the matching response
// entry's Error field instead of aborting the batch.
func (g Gateway) ShiftMatchingTreasuresMany(ctx context.Context, in *hydrapb.ShiftMatchingTreasuresManyRequest) (*hydrapb.ShiftMatchingTreasuresManyResponse, error) {
	g.ZeusInterface.GetSafeops().LockSystem()
	defer g.ZeusInterface.GetSafeops().UnlockSystem()

	defer handlePanic()

	requests := in.GetRequests()
	if len(requests) == 0 {
		return &hydrapb.ShiftMatchingTreasuresManyResponse{Responses: nil}, nil
	}

	responses := make([]*hydrapb.ShiftMatchingTreasuresManyEntry, 0, len(requests))
	for _, req := range requests {
		entry := &hydrapb.ShiftMatchingTreasuresManyEntry{}
		if req == nil {
			s := "nil request"
			entry.Error = &s
			responses = append(responses, entry)
			continue
		}
		treasures, capReached, err := shiftMatchingOneSwamp(ctx, g, req)
		if err != nil {
			s := err.Error()
			entry.Error = &s
		} else {
			entry.Treasures = treasures
			entry.CapReached = capReached
		}
		responses = append(responses, entry)
	}
	return &hydrapb.ShiftMatchingTreasuresManyResponse{Responses: responses}, nil
}

// shiftMatchingOneSwamp runs the per-swamp body of ShiftMatchingTreasures
// and is shared between the single-RPC handler and the Many-RPC handler.
// Returns a typed gRPC error for swamp-level failures.
func shiftMatchingOneSwamp(ctx context.Context, g Gateway, in *hydrapb.ShiftMatchingTreasuresRequest) ([]*hydrapb.Treasure, bool, error) {
	if in.GetSwampName() == "" {
		return nil, false, status.Error(codes.InvalidArgument, "SwampName cannot be empty")
	}

	// Validate Cap early — before the missing-swamp branch — so a malformed
	// Cap on an unseeded swamp is still surfaced as InvalidArgument rather
	// than silently swallowed.
	capPred, capMax, capErr := buildCapPredicate(in.GetCap())
	if capErr != nil {
		return nil, false, status.Error(codes.InvalidArgument, capErr.Error())
	}

	// Missing swamp → empty result, not an error. Mirrors
	// ShiftExpiredTreasures behaviour: callers commonly poll a swamp
	// that may not exist yet.
	swampName, err := checkSwampName(g.ZeusInterface, in.GetIslandID(), in.GetSwampName(), true)
	if err != nil {
		if st, ok := status.FromError(err); ok && st.Code() == codes.FailedPrecondition {
			return nil, false, nil
		}
		return nil, false, err
	}

	hydraInterface := g.ZeusInterface.GetHydra()

	swampInterface, err := hydraInterface.SummonSwamp(ctx, in.GetIslandID(), swampName)
	if err != nil {
		return nil, false, status.Error(codes.Internal, fmt.Sprintf("internal server error in hydra: %s", err.Error()))
	}
	swampInterface.BeginVigil()
	defer swampInterface.CeaseVigil()

	beaconType := inputIndexTypeToBeaconType(in.GetIndexType())
	order := inputOrderTypeToBeaconOrderType(in.GetOrderType())

	howMany := in.GetHowMany()
	maxResults := in.GetMaxResults()
	// HowMany == 0 means "all matching" (still bounded by MaxResults / Cap).
	// Engine treats howMany<=0 as no-op, so substitute a large sentinel.
	if howMany == 0 {
		howMany = 1000000000
	}
	if maxResults > 0 && maxResults < howMany {
		howMany = maxResults
	}

	fromTime, toTime := parseOptionalTimestamps(in.GetFromTime(), in.GetToTime())
	predicate, predErr := buildShiftMatchingPredicate(swampInterface, beaconType, in.GetFilters(), fromTime, toTime)
	if predErr != nil {
		return nil, false, status.Error(codes.InvalidArgument, predErr.Error())
	}

	treasures, capReached, err := swampInterface.CloneAndDeleteMatchingTreasures(beaconType, order, howMany, predicate, capPred, capMax)
	if err != nil {
		return nil, false, status.Error(codes.Internal, fmt.Sprintf("hydra error: %s", err.Error()))
	}

	response := make([]*hydrapb.Treasure, 0, len(treasures))
	for _, t := range treasures {
		out := &hydrapb.Treasure{}
		treasureToKeyValuePair(t, out)
		response = append(response, out)
	}
	return response, capReached, nil
}

// buildShiftMatchingPredicate composes the per-treasure selection
// predicate from a FilterGroup and optional time-range bounds.
//
// Time bounds (FromTime / ToTime) apply only on time-based indexes
// (creation, update, expiration). On non-time indexes they are silently
// ignored — the existing GetTreasuresByBeacon API does the same.
//
// When filters resolve to a bucket-eligible plan, the residual is
// composed with a candidate-key fast-reject. The engine still walks
// the full beacon under its mu (preserving the disjoint-subset
// guarantee across concurrent callers) but body-decode only fires
// for the small candidate set.
func buildShiftMatchingPredicate(sw swamp.Swamp, beaconType swamp.BeaconType, filters *hydrapb.FilterGroup, fromTime, toTime *time.Time) (func(treasure.Treasure) bool, error) {
	getTs := timestampExtractorFor(beaconType)
	hasTimeBounds := (fromTime != nil || toTime != nil) && getTs != nil
	hasFilters := filters != nil

	if !hasFilters {
		if !hasTimeBounds {
			return func(_ treasure.Treasure) bool { return true }, nil
		}
		fromNano, toNano := timeBoundsNanos(fromTime, toTime)
		return func(t treasure.Treasure) bool {
			return inTimeRange(getTs(t), fromNano, toNano)
		}, nil
	}

	// Filter present — plan it.
	plan := PlanFilter(filters)
	filterEval := filters
	var keySet map[string]struct{}
	if plan.Mode != PlanModeBypass {
		candidates := collectBucketCandidates(sw, plan.Hints)
		keySet = candidateKeySet(candidates)
		filterEval = plan.Residual
	}

	if !hasTimeBounds {
		return func(t treasure.Treasure) bool {
			if keySet != nil {
				if _, in := keySet[t.GetKey()]; !in {
					return false
				}
			}
			return evaluateNativeFilterGroup(t, filterEval)
		}, nil
	}
	fromNano, toNano := timeBoundsNanos(fromTime, toTime)
	return func(t treasure.Treasure) bool {
		if !inTimeRange(getTs(t), fromNano, toNano) {
			return false
		}
		if keySet != nil {
			if _, in := keySet[t.GetKey()]; !in {
				return false
			}
		}
		return evaluateNativeFilterGroup(t, filterEval)
	}, nil
}

// timestampExtractorFor returns the treasure-timestamp getter that
// matches a time-based BeaconType, or nil for non-time indexes.
func timestampExtractorFor(b swamp.BeaconType) func(treasure.Treasure) int64 {
	switch b {
	case swamp.BeaconTypeCreationTime:
		return func(t treasure.Treasure) int64 { return t.GetCreatedAt() }
	case swamp.BeaconTypeUpdateTime:
		return func(t treasure.Treasure) int64 { return t.GetModifiedAt() }
	case swamp.BeaconTypeExpirationTime:
		return func(t treasure.Treasure) int64 { return t.GetExpirationTime() }
	default:
		return nil
	}
}

// timeBoundsNanos converts *time.Time bounds into UnixNano sentinels.
// A nil bound becomes the open-interval sentinel (math.MinInt64 /
// math.MaxInt64), so the predicate evaluation stays branch-free.
func timeBoundsNanos(from, to *time.Time) (int64, int64) {
	const maxInt64 = int64(^uint64(0) >> 1)
	const minInt64 = -maxInt64 - 1
	fromNano := minInt64
	toNano := maxInt64
	if from != nil {
		fromNano = from.UTC().UnixNano()
	}
	if to != nil {
		toNano = to.UTC().UnixNano()
	}
	return fromNano, toNano
}

// inTimeRange applies [fromNano, toNano) half-open semantics, matching
// the existing GetTreasuresByBeacon range behaviour.
func inTimeRange(ts, fromNano, toNano int64) bool {
	return ts >= fromNano && ts < toNano
}
