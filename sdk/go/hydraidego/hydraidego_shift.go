package hydraidego

import (
	"context"
	"fmt"
	"reflect"
	"time"

	hydraidepbgo "github.com/hydraide/hydraide/sdk/go/hydraidego/v3/hydraidepbgo"
	"github.com/hydraide/hydraide/sdk/go/hydraidego/v3/name"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// Cap is a server-enforced quota check that bounds the post-operation
// number of records matching Filter in the affected swamp to ≤ MaxMatching.
// Used by CatalogShift, CatalogPatchExpired, and (Phase B) every explicit-
// key Patch surface.
//
// The cap is pre-enforced under the same per-swamp guard as the mutation:
// the server counts records matching Filter and only proceeds with
// mutations that respect the budget. No race window exists between count
// and claim.
//
// Filter is required when Cap is set; MaxMatching must be > 0.
type Cap struct {
	// Filter selects records that count toward the quota. Typically
	// "claimed and lease still valid" or similar in-flight status.
	Filter *FilterGroup

	// MaxMatching is the post-operation upper bound on the matching
	// count. The server claims at most (MaxMatching - currentMatching)
	// records on a Cap-bearing call.
	MaxMatching int32
}

// ShiftRequest carries the parameters of a single-swamp atomic shift.
// All Treasures matching the selection (IndexType + Filters + time
// bounds), in the requested order, up to HowMany / MaxResults / Cap
// budget, are atomically removed from the swamp and streamed to the
// caller's iterator.
type ShiftRequest struct {
	// IndexType selects the ordering / range axis: key, value (typed),
	// createdAt, updatedAt, or expiredAt.
	IndexType IndexType

	// IndexOrder controls ascending vs descending walk over IndexType.
	IndexOrder IndexOrder

	// HowMany caps the result count. 0 means "all matching" (still
	// bounded by MaxResults / Cap).
	HowMany int32

	// MaxResults is a hard cap defended at the engine. 0 means "fall
	// back to HowMany". Set this in production to protect a swamp from
	// accidental drain.
	MaxResults int32

	// Filters narrows selection inside IndexType. nil = no filter.
	Filters *FilterGroup

	// FromTime / ToTime are inclusive lower / exclusive upper bounds
	// for time-based indexes (creation, update, expiration). Ignored
	// for non-time indexes.
	FromTime *time.Time
	ToTime   *time.Time

	// Cap is an optional server-enforced quota check. nil = no quota
	// enforcement. See Cap for semantics.
	Cap *Cap
}

// ShiftResult carries the request-level outcome of a Cap-bearing call.
// CapReached is true when the cap budget (rather than HowMany) bounded
// the result count. Always false when ShiftRequest.Cap is nil.
type ShiftResult struct {
	CapReached bool
}

// CatalogShiftIteratorFunc is invoked once per Treasure shifted out of
// the swamp by CatalogShift. The model is a fresh instance of the
// caller's model template, populated from the deleted Treasure.
//
// Returning a non-nil error from the iterator stops iteration and
// bubbles up as the error returned by CatalogShift.
type CatalogShiftIteratorFunc func(model any) error

// CatalogShift atomically selects, removes, and returns up to HowMany
// Treasures from a single swamp, ordered by IndexType + IndexOrder,
// optionally narrowed by Filters, optionally bounded by Cap. Selection
// and deletion happen under one per-swamp guard, so concurrent callers
// receive disjoint subsets.
//
// CatalogShift is the parametric generalisation of CatalogShiftExpired:
// passing IndexExpirationTime + an ExpiredAt<now filter reproduces the
// legacy behaviour byte-for-byte.
//
// Common patterns:
//   - FIFO / LIFO scan-claim queue: IndexCreationTime + filter on status.
//   - Top-K consumer: IndexValueInt32 / Float64, DESC, HowMany=K.
//   - Bounded claim: any IndexType + Cap to enforce per-swamp concurrency.
//
// Returns the request-level ShiftResult (currently just CapReached) so
// callers can backoff vs poll on cap exhaustion. The error return is
// reserved for transport / configuration failures.
func (h *hydraidego) CatalogShift(ctx context.Context, swampName name.Name, req *ShiftRequest, model any, iterator CatalogShiftIteratorFunc) (*ShiftResult, error) {
	if req == nil {
		return nil, NewError(ErrCodeInvalidModel, "CatalogShift requires a non-nil ShiftRequest")
	}
	wire, validateErr := buildShiftMatchingRequest(req, swampName, h.client.GetAllIslands())
	if validateErr != nil {
		return nil, validateErr
	}

	resp, err := h.client.GetServiceClient(swampName).ShiftMatchingTreasures(ctx, wire)
	if err != nil {
		return nil, errorHandler(err)
	}

	if iterator != nil {
		for _, t := range resp.GetTreasures() {
			if !t.IsExist {
				continue
			}
			modelValue := reflect.New(reflect.TypeOf(model)).Interface()
			if convErr := convertProtoTreasureToCatalogModel(t, modelValue); convErr != nil {
				return nil, NewError(ErrCodeInvalidModel, convErr.Error())
			}
			if iterErr := iterator(modelValue); iterErr != nil {
				return nil, iterErr
			}
		}
	}

	return &ShiftResult{CapReached: resp.GetCapReached()}, nil
}

// ShiftManyFromManyRequest is one entry in a multi-swamp shift batch
// issued via CatalogShiftManyFromMany. Each entry carries one swamp
// name plus its own ShiftRequest (IndexType, filters, Cap, etc. — the
// shape can vary per swamp).
type ShiftManyFromManyRequest struct {
	SwampName name.Name
	Request   *ShiftRequest
}

// ShiftManyFromManyResult is the per-swamp outcome of a
// CatalogShiftManyFromMany call. CapReached mirrors ShiftResult; SwampErr
// is non-nil for swamp-level failures (missing swamp on a strict swamp,
// summon failure, invalid filter).
type ShiftManyFromManyResult struct {
	SwampName  name.Name
	CapReached bool
	SwampErr   error
}

// CatalogShiftManyFromManyIteratorFunc is invoked once per Treasure
// across all swamps in the batch. swampName identifies which swamp
// produced the model.
type CatalogShiftManyFromManyIteratorFunc func(swampName name.Name, model any) error

// CatalogShiftManyFromMany dispatches a multi-swamp shift batch. Each
// request claims up to its HowMany Treasures from its swamp under the
// swamp's beacon mu and streams them to the iterator. Per-swamp failures
// (missing swamp, summon failure) populate the returned results' SwampErr
// rather than aborting the batch.
//
// Requests are grouped by destination server (consistent hashing on
// SwampName); one ShiftMatchingTreasuresMany RPC is sent per server.
//
// Returns one ShiftManyFromManyResult per input request, in the same
// order, regardless of success / failure. The function-level error is
// reserved for transport / wire-format failures.
func (h *hydraidego) CatalogShiftManyFromMany(ctx context.Context, requests []*ShiftManyFromManyRequest, model any, iterator CatalogShiftManyFromManyIteratorFunc) ([]*ShiftManyFromManyResult, error) {
	if len(requests) == 0 {
		return nil, nil
	}

	results := make([]*ShiftManyFromManyResult, len(requests))
	wireRequests := make([]*hydraidepbgo.ShiftMatchingTreasuresRequest, len(requests))
	for i, req := range requests {
		results[i] = &ShiftManyFromManyResult{}
		if req == nil {
			return nil, NewError(ErrCodeInvalidModel, fmt.Sprintf("request %d is nil", i))
		}
		if req.Request == nil {
			return nil, NewError(ErrCodeInvalidModel, fmt.Sprintf("request %d: Request is required", i))
		}
		results[i].SwampName = req.SwampName
		wire, validateErr := buildShiftMatchingRequest(req.Request, req.SwampName, h.client.GetAllIslands())
		if validateErr != nil {
			return nil, validateErr
		}
		wireRequests[i] = wire
	}

	type serverGroup struct {
		client  hydraidepbgo.HydraideServiceClient
		indices []int
	}
	groups := make(map[string]*serverGroup)
	for i, req := range requests {
		clientAndHost := h.client.GetServiceClientAndHost(req.SwampName)
		g, ok := groups[clientAndHost.Host]
		if !ok {
			g = &serverGroup{client: clientAndHost.GrpcClient}
			groups[clientAndHost.Host] = g
		}
		g.indices = append(g.indices, i)
	}

	for _, g := range groups {
		batch := make([]*hydraidepbgo.ShiftMatchingTreasuresRequest, 0, len(g.indices))
		for _, idx := range g.indices {
			batch = append(batch, wireRequests[idx])
		}
		resp, err := g.client.ShiftMatchingTreasuresMany(ctx, &hydraidepbgo.ShiftMatchingTreasuresManyRequest{
			Requests: batch,
		})
		if err != nil {
			return results, errorHandler(err)
		}
		entries := resp.GetResponses()
		if len(entries) != len(g.indices) {
			return results, NewError(ErrCodeInternalDatabaseError, fmt.Sprintf("ShiftMatchingTreasuresMany: server returned %d entries for %d requests", len(entries), len(g.indices)))
		}

		for k, idx := range g.indices {
			entry := entries[k]
			results[idx].CapReached = entry.GetCapReached()
			if errMsg := entry.GetError(); errMsg != "" {
				results[idx].SwampErr = NewError(ErrCodeInternalDatabaseError, errMsg)
				continue
			}
			if iterator == nil {
				continue
			}
			for _, t := range entry.GetTreasures() {
				if !t.IsExist {
					continue
				}
				modelValue := reflect.New(reflect.TypeOf(model)).Interface()
				if convErr := convertProtoTreasureToCatalogModel(t, modelValue); convErr != nil {
					return results, NewError(ErrCodeInvalidModel, convErr.Error())
				}
				if iterErr := iterator(requests[idx].SwampName, modelValue); iterErr != nil {
					return results, iterErr
				}
			}
		}
	}
	return results, nil
}

// buildShiftMatchingRequest converts an SDK ShiftRequest into the wire
// message. Validates Cap, encodes filters, applies time bounds.
func buildShiftMatchingRequest(req *ShiftRequest, swampName name.Name, allIslands uint64) (*hydraidepbgo.ShiftMatchingTreasuresRequest, error) {
	wireCap, capErr := buildWireCap(req.Cap)
	if capErr != nil {
		return nil, capErr
	}
	wire := &hydraidepbgo.ShiftMatchingTreasuresRequest{
		IslandID:   swampName.GetIslandID(allIslands),
		SwampName:  swampName.Get(),
		IndexType:  convertIndexTypeToProtoIndexType(req.IndexType),
		OrderType:  convertOrderTypeToProtoOrderType(req.IndexOrder),
		HowMany:    req.HowMany,
		MaxResults: req.MaxResults,
		Cap:        wireCap,
	}
	if req.Filters != nil {
		wire.Filters = convertFilterGroupToProto(req.Filters)
	}
	if req.FromTime != nil {
		wire.FromTime = timestamppb.New(req.FromTime.UTC())
	}
	if req.ToTime != nil {
		wire.ToTime = timestamppb.New(req.ToTime.UTC())
	}
	return wire, nil
}

// buildWireCap validates and converts an SDK Cap into the wire form.
// Returns (nil, nil) when cap is absent — Cap is opt-in.
//
// Rejection rules (must match the server-side validation in
// gateway_cap.go):
//   - Cap with nil Filter → ErrCodeInvalidModel.
//   - Cap with MaxMatching <= 0 → ErrCodeInvalidModel.
func buildWireCap(cap *Cap) (*hydraidepbgo.Cap, error) {
	if cap == nil {
		return nil, nil
	}
	if cap.MaxMatching <= 0 {
		return nil, NewError(ErrCodeInvalidModel, "Cap.MaxMatching must be > 0")
	}
	if cap.Filter == nil {
		return nil, NewError(ErrCodeInvalidModel, "Cap.Filter is required when Cap is set")
	}
	return &hydraidepbgo.Cap{
		Filter:      convertFilterGroupToProto(cap.Filter),
		MaxMatching: cap.MaxMatching,
	}, nil
}
