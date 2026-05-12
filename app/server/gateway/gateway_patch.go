package gateway

import (
	"context"
	"fmt"

	"github.com/hydraide/hydraide/app/core/hydra/swamp"
	"github.com/hydraide/hydraide/app/core/hydra/swamp/treasure"
	"github.com/hydraide/hydraide/app/core/hydra/swamp/treasure/msgpackpatch"
	hydrapb "github.com/hydraide/hydraide/sdk/go/hydraidego/v3/hydraidepbgo"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// PatchTreasures handles the multi-key structural-mutation RPC. Each patch
// targets a single key under the named swamp; ops within one patch run
// atomically under the per-key guard, but cross-key atomicity is not
// provided. Per-key business outcomes (KEY_NOT_FOUND, CONDITION_NOT_MET,
// TYPE_MISMATCH, etc.) are surfaced as PatchResult.StatusCode entries —
// the gRPC error return is reserved for transport-level / configuration
// failures (missing swamp name, unsummonable swamp).
func (g Gateway) PatchTreasures(ctx context.Context, in *hydrapb.PatchTreasuresRequest) (*hydrapb.PatchTreasuresResponse, error) {
	g.ZeusInterface.GetSafeops().LockSystem()
	defer g.ZeusInterface.GetSafeops().UnlockSystem()

	defer handlePanic()

	results, capReached, swampErr := patchTreasuresOneSwamp(ctx, g, in)
	if swampErr != nil {
		return nil, swampErr
	}
	return &hydrapb.PatchTreasuresResponse{Results: results, CapReached: capReached}, nil
}

// patchTreasuresOneSwamp runs the per-swamp body of PatchTreasures and
// is shared between the single-RPC handler and the Many-RPC handler.
// Returns a typed gRPC error for swamp-level failures (the single-RPC
// handler propagates it; the Many handler converts it to a per-entry
// Error field).
func patchTreasuresOneSwamp(ctx context.Context, g Gateway, in *hydrapb.PatchTreasuresRequest) ([]*hydrapb.PatchResult, bool, error) {
	if in.GetSwampName() == "" {
		return nil, false, status.Error(codes.InvalidArgument, "SwampName cannot be empty")
	}

	if len(in.GetPatches()) == 0 {
		// Empty batch is a valid no-op.
		return nil, false, nil
	}

	// Cap validation runs before swamp summon so a malformed Cap is
	// surfaced as InvalidArgument regardless of swamp existence.
	bodyCapPred, bodyCapMax, capErr := buildBodyCapPredicate(in.GetCap())
	if capErr != nil {
		return nil, false, status.Error(codes.InvalidArgument, capErr.Error())
	}

	// We do not require the swamp to pre-exist; SummonSwamp creates it.
	// CheckSwampName with checkExist=false validates the name format only.
	swampName, err := checkSwampName(g.ZeusInterface, in.GetIslandID(), in.GetSwampName(), false)
	if err != nil {
		return nil, false, err
	}

	hydraInterface := g.ZeusInterface.GetHydra()
	swampObj, err := hydraInterface.SummonSwamp(ctx, in.GetIslandID(), swampName)
	if err != nil {
		return nil, false, status.Error(codes.Internal, fmt.Sprintf("internal server error in hydra: %s", err.Error()))
	}
	swampObj.BeginVigil()
	defer swampObj.CeaseVigil()

	// Cap path: count currently-matching records ONCE under capMu (via
	// CountMatchingTreasures, which itself takes the beacon RLock).
	// PatchFields-level capMu acquisition would re-enter the same mutex
	// per-key; instead, the gateway holds capMu for the whole batch
	// (matching the engine atomicity model used by PatchExpired).
	var capPredicate func([]byte) bool
	var budgetLeft int32
	capReached := false
	if bodyCapPred != nil {
		// Engine predicate: takes raw msgpack body bytes.
		capPredicate = func(rawBody []byte) bool {
			decoded, decErr := decodeMsgpackMapForCap(rawBody)
			if decErr != nil {
				return false
			}
			return bodyCapPred(decoded)
		}
		// Same per-treasure predicate, but feeding the live treasure's
		// current body to capCountTreasures.
		treasurePredicate := func(t treasureForCount) bool {
			raw, err := t.GetContentByteArray()
			if err != nil || len(raw) < 2 {
				return false
			}
			return capPredicate(raw[2:])
		}
		currentMatching, lockHolder := capPreCount(swampObj, treasurePredicate)
		// Hold capMu for the whole batch so concurrent Cap-bearing flows
		// observe consistent budget arithmetic.
		defer lockHolder()
		budgetLeft = bodyCapMax - currentMatching
		if budgetLeft < 0 {
			budgetLeft = 0
		}
	}

	requestMeta := protoMetaToSwampMeta(in.GetMeta())
	baseOpts := swamp.PatchFieldsOptions{
		CreateIfNotExist:       in.GetCreateIfNotExist(),
		InitialMsgpackOnCreate: in.GetInitialMsgpackOnCreate(),
		Meta:                   requestMeta,
		CapPredicate:           capPredicate,
		CapBudgetLeft:          &budgetLeft,
	}

	results := make([]*hydrapb.PatchResult, 0, len(in.GetPatches()))
	for _, patch := range in.GetPatches() {
		ops, opsErr := protoOpsToMsgpackpatchOps(patch.GetOps())
		if opsErr != nil {
			results = append(results, &hydrapb.PatchResult{
				Key:    patch.GetKey(),
				Status: hydrapb.PatchResult_PATH_INVALID,
				Error:  protoStr(opsErr.Error()),
			})
			continue
		}
		cond := protoCondToMsgpackpatchCond(patch.GetCondition())

		// Per-key Meta fully replaces the request-level Meta on this patch
		// (no field-level merge). When the patch carries no Meta of its own,
		// the request-level Meta applies.
		opts := baseOpts
		if patch.Meta != nil {
			opts.Meta = protoMetaToSwampMeta(patch.GetMeta())
		}

		res, perr := swampObj.PatchFields(patch.GetKey(), ops, cond, opts)
		if perr != nil {
			// Internal error surfaces as INTERNAL_ERROR status, not as a
			// gRPC-level error: we still want to return per-key results
			// for the rest of the batch.
			results = append(results, &hydrapb.PatchResult{
				Key:    patch.GetKey(),
				Status: hydrapb.PatchResult_INTERNAL_ERROR,
				Error:  protoStr(perr.Error()),
			})
			continue
		}

		if res.Status == swamp.PatchStatusCapExceeded {
			capReached = true
		}

		out := &hydrapb.PatchResult{
			Key:    patch.GetKey(),
			Status: hydrapb.PatchResult_StatusCode(res.Status),
		}
		if res.Error != "" {
			out.Error = protoStr(res.Error)
		}
		// NewMsgpack is reserved for a future opt-in; leaving unset.
		results = append(results, out)
	}

	return results, capReached, nil
}

// treasureForCount is the minimal Treasure-shaped subset capPreCount
// needs: a getter for the raw ByteArray body.
type treasureForCount interface {
	GetContentByteArray() ([]byte, error)
}

// capPreCount acquires the swamp's capMu, counts treasures matching the
// predicate, and returns the count plus a release function the caller
// must defer. Holding capMu for the whole batch keeps the running
// (no→yes) budget exact relative to concurrent Cap-bearing flows.
func capPreCount(swampObj swamp.Swamp, predicate func(treasureForCount) bool) (int32, func()) {
	// We need to count over s.beaconKey. The swamp interface exposes
	// CountMatchingTreasures which takes a treasure.Treasure predicate;
	// adapt our treasureForCount predicate to that signature.
	adapted := func(t treasure.Treasure) bool {
		return predicate(t)
	}
	count := swampObj.CountMatchingTreasures(adapted)
	// Cap-bearing patch flows serialise on swamp.capMu — but the swamp
	// interface does not expose it directly. Acquire it via the
	// public LockCapMu / UnlockCapMu accessors added on the swamp
	// interface so the gateway can hold it for the whole batch.
	swampObj.LockCapMu()
	return count, swampObj.UnlockCapMu
}

// PatchTreasuresMany dispatches a batch of PatchTreasures requests
// against multiple swamps on the same server in a single RPC. Each
// entry runs independently; per-swamp failures populate the matching
// response entry's Error field instead of aborting the batch.
func (g Gateway) PatchTreasuresMany(ctx context.Context, in *hydrapb.PatchTreasuresManyRequest) (*hydrapb.PatchTreasuresManyResponse, error) {
	g.ZeusInterface.GetSafeops().LockSystem()
	defer g.ZeusInterface.GetSafeops().UnlockSystem()

	defer handlePanic()

	requests := in.GetRequests()
	if len(requests) == 0 {
		return &hydrapb.PatchTreasuresManyResponse{Responses: nil}, nil
	}

	responses := make([]*hydrapb.PatchTreasuresManyEntry, 0, len(requests))
	for _, req := range requests {
		entry := &hydrapb.PatchTreasuresManyEntry{}
		if req == nil {
			entry.Error = protoStr("nil request")
			responses = append(responses, entry)
			continue
		}
		results, capReached, swampErr := patchTreasuresOneSwamp(ctx, g, req)
		if swampErr != nil {
			entry.Error = protoStr(swampErr.Error())
		} else {
			entry.Results = results
			entry.CapReached = capReached
		}
		responses = append(responses, entry)
	}
	return &hydrapb.PatchTreasuresManyResponse{Responses: responses}, nil
}

// protoOpsToMsgpackpatchOps converts the wire-level op list to the core
// engine's typed op list. The Kind enum values are intentionally aligned
// (SET=0..MERGE=7), so this is a direct cast plus a path/value copy.
func protoOpsToMsgpackpatchOps(in []*hydrapb.PatchOp) ([]msgpackpatch.Op, error) {
	if len(in) == 0 {
		return nil, nil
	}
	out := make([]msgpackpatch.Op, 0, len(in))
	for i, op := range in {
		if op == nil {
			return nil, fmt.Errorf("op %d is nil", i)
		}
		out = append(out, msgpackpatch.Op{
			Kind:  msgpackpatch.OpKind(op.GetOp()),
			Path:  op.GetPath(),
			Value: op.GetValue(),
		})
	}
	return out, nil
}

// protoCondToMsgpackpatchCond converts the wire-level condition to the
// engine's typed condition. Returns nil when in is nil.
func protoCondToMsgpackpatchCond(in *hydrapb.PatchCondition) *msgpackpatch.Condition {
	if in == nil {
		return nil
	}
	return &msgpackpatch.Condition{
		Path:      in.GetPath(),
		Op:        msgpackpatch.CondOp(in.GetOperator()),
		Threshold: in.GetThreshold(),
	}
}

// protoMetaToSwampMeta converts the wire-level metadata setup to the swamp
// helper struct. Returns nil when in is nil.
func protoMetaToSwampMeta(in *hydrapb.PatchMeta) *swamp.PatchFieldsMeta {
	if in == nil {
		return nil
	}
	out := &swamp.PatchFieldsMeta{
		SetUpdatedAt:   in.GetSetUpdatedAt(),
		SetUpdatedBy:   in.GetSetUpdatedBy(),
		SetCreatedAt:   in.GetSetCreatedAt(),
		SetCreatedBy:   in.GetSetCreatedBy(),
		ClearExpiredAt: in.GetClearExpiredAt(),
	}
	if exp := in.GetSetExpiredAt(); exp != nil {
		out.SetExpiredAt = exp.AsTime()
	}
	return out
}

// protoStr returns a *string with the given content, mirroring the proto
// optional-string idiom.
func protoStr(s string) *string {
	return &s
}
