package gateway

import (
	"context"
	"fmt"

	"github.com/hydraide/hydraide/app/core/hydra/swamp"
	hydrapb "github.com/hydraide/hydraide/sdk/go/hydraidego/v3/hydraidepbgo"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// PatchExpiredTreasures handles the in-place patch-of-expired RPC. It
// selects up to HowMany expired treasures from the named swamp under
// the beacon mu (so concurrent callers see disjoint subsets), applies
// the request's Ops + Meta to each one under its per-key guard, and
// returns the per-treasure outcomes in selection order.
//
// Per-treasure business outcomes (CONDITION_NOT_MET, TYPE_MISMATCH,
// ENCODING_NOT_SUPPORTED, etc.) are surfaced as PatchResult.StatusCode
// entries in the response. The gRPC error return is reserved for
// transport-level / configuration failures.
func (g Gateway) PatchExpiredTreasures(ctx context.Context, in *hydrapb.PatchExpiredTreasuresRequest) (*hydrapb.PatchExpiredTreasuresResponse, error) {

	g.ZeusInterface.GetSafeops().LockSystem()
	defer g.ZeusInterface.GetSafeops().UnlockSystem()

	defer handlePanic()

	if in.GetSwampName() == "" {
		return nil, status.Error(codes.InvalidArgument, "SwampName cannot be empty")
	}

	hasOps := len(in.GetOps()) > 0
	hasMeta := in.GetMeta() != nil
	if !hasOps && !hasMeta {
		return nil, status.Error(codes.InvalidArgument, "PatchExpiredTreasures requires non-empty Ops or non-nil Meta")
	}

	// Missing swamp → empty result, not an error. Mirrors
	// ShiftExpiredTreasures behaviour: callers commonly poll a swamp
	// that may not exist yet, and a FailedPrecondition here would
	// force every caller to handle that code separately.
	swampName, err := checkSwampName(g.ZeusInterface, in.GetIslandID(), in.GetSwampName(), true)
	if err != nil {
		if st, ok := status.FromError(err); ok && st.Code() == codes.FailedPrecondition {
			return &hydrapb.PatchExpiredTreasuresResponse{Patched: nil}, nil
		}
		return nil, err
	}

	hydraInterface := g.ZeusInterface.GetHydra()
	swampObj, err := hydraInterface.SummonSwamp(ctx, in.GetIslandID(), swampName)
	if err != nil {
		return nil, status.Error(codes.Internal, fmt.Sprintf("internal server error in hydra: %s", err.Error()))
	}
	swampObj.BeginVigil()
	defer swampObj.CeaseVigil()

	ops, opsErr := protoOpsToMsgpackpatchOps(in.GetOps())
	if opsErr != nil {
		return nil, status.Error(codes.InvalidArgument, fmt.Sprintf("invalid Ops: %s", opsErr.Error()))
	}
	cond := protoCondToMsgpackpatchCond(in.GetCondition())
	meta := protoMetaToSwampMeta(in.GetMeta())

	entries, perr := swampObj.PatchExpired(in.GetHowMany(), ops, cond, meta)
	if perr != nil {
		return nil, status.Error(codes.Internal, fmt.Sprintf("PatchExpired failed: %s", perr.Error()))
	}

	patched := make([]*hydrapb.PatchedExpiredTreasure, 0, len(entries))
	for _, e := range entries {
		out := &hydrapb.PatchedExpiredTreasure{
			Key:    e.Key,
			Status: hydrapb.PatchResult_StatusCode(e.Status),
		}
		if e.Error != "" {
			out.Error = protoStr(e.Error)
		}
		if e.Status == swamp.PatchStatusPatched && len(e.NewMsgpack) > 0 {
			out.NewMsgpack = e.NewMsgpack
		}
		if !e.ExpiredAt.IsZero() {
			out.ExpiredAt = timestamppb.New(e.ExpiredAt)
		}
		patched = append(patched, out)
	}

	return &hydrapb.PatchExpiredTreasuresResponse{Patched: patched}, nil
}

// PatchExpiredTreasuresMany dispatches a batch of PatchExpiredTreasures
// requests against multiple swamps on the same server in a single RPC.
// Each request is processed independently under its own per-swamp guard;
// a per-swamp error becomes a populated Error field on the matching
// response entry, leaving the rest of the batch unaffected.
//
// The response order matches the input order. The system lock is
// acquired once for the entire batch — the meaning aligns with the
// single-swamp counterpart, where each in-process operation runs while
// the system lock is held.
func (g Gateway) PatchExpiredTreasuresMany(ctx context.Context, in *hydrapb.PatchExpiredTreasuresManyRequest) (*hydrapb.PatchExpiredTreasuresManyResponse, error) {

	g.ZeusInterface.GetSafeops().LockSystem()
	defer g.ZeusInterface.GetSafeops().UnlockSystem()

	defer handlePanic()

	requests := in.GetRequests()
	if len(requests) == 0 {
		return &hydrapb.PatchExpiredTreasuresManyResponse{Responses: nil}, nil
	}

	responses := make([]*hydrapb.PatchExpiredTreasuresManyEntry, 0, len(requests))
	for _, req := range requests {
		entry := patchExpiredOneSwamp(ctx, g, req)
		responses = append(responses, entry)
	}
	return &hydrapb.PatchExpiredTreasuresManyResponse{Responses: responses}, nil
}

// patchExpiredOneSwamp runs the per-swamp body of PatchExpiredTreasures
// for a single request inside a Many batch. It mirrors the single-RPC
// handler's logic but converts swamp-level failures (missing name,
// missing ops, summon failure) into a populated Error field on the
// response entry instead of a top-level gRPC error, so the rest of the
// batch can complete.
func patchExpiredOneSwamp(ctx context.Context, g Gateway, in *hydrapb.PatchExpiredTreasuresRequest) *hydrapb.PatchExpiredTreasuresManyEntry {
	if in == nil {
		return &hydrapb.PatchExpiredTreasuresManyEntry{Error: protoStr("nil request")}
	}
	if in.GetSwampName() == "" {
		return &hydrapb.PatchExpiredTreasuresManyEntry{Error: protoStr("SwampName cannot be empty")}
	}

	hasOps := len(in.GetOps()) > 0
	hasMeta := in.GetMeta() != nil
	if !hasOps && !hasMeta {
		return &hydrapb.PatchExpiredTreasuresManyEntry{Error: protoStr("PatchExpiredTreasures requires non-empty Ops or non-nil Meta")}
	}

	swampName, err := checkSwampName(g.ZeusInterface, in.GetIslandID(), in.GetSwampName(), true)
	if err != nil {
		if st, ok := status.FromError(err); ok && st.Code() == codes.FailedPrecondition {
			// Missing swamp → empty Patched, no error. Same as the
			// single-RPC handler's behaviour: callers commonly poll a
			// swamp that may not exist yet.
			return &hydrapb.PatchExpiredTreasuresManyEntry{Patched: nil}
		}
		return &hydrapb.PatchExpiredTreasuresManyEntry{Error: protoStr(err.Error())}
	}

	hydraInterface := g.ZeusInterface.GetHydra()
	swampObj, err := hydraInterface.SummonSwamp(ctx, in.GetIslandID(), swampName)
	if err != nil {
		return &hydrapb.PatchExpiredTreasuresManyEntry{Error: protoStr(fmt.Sprintf("summon swamp: %s", err.Error()))}
	}
	swampObj.BeginVigil()
	defer swampObj.CeaseVigil()

	ops, opsErr := protoOpsToMsgpackpatchOps(in.GetOps())
	if opsErr != nil {
		return &hydrapb.PatchExpiredTreasuresManyEntry{Error: protoStr(fmt.Sprintf("invalid Ops: %s", opsErr.Error()))}
	}
	cond := protoCondToMsgpackpatchCond(in.GetCondition())
	meta := protoMetaToSwampMeta(in.GetMeta())

	entries, perr := swampObj.PatchExpired(in.GetHowMany(), ops, cond, meta)
	if perr != nil {
		return &hydrapb.PatchExpiredTreasuresManyEntry{Error: protoStr(fmt.Sprintf("PatchExpired failed: %s", perr.Error()))}
	}

	patched := make([]*hydrapb.PatchedExpiredTreasure, 0, len(entries))
	for _, e := range entries {
		out := &hydrapb.PatchedExpiredTreasure{
			Key:    e.Key,
			Status: hydrapb.PatchResult_StatusCode(e.Status),
		}
		if e.Error != "" {
			out.Error = protoStr(e.Error)
		}
		if e.Status == swamp.PatchStatusPatched && len(e.NewMsgpack) > 0 {
			out.NewMsgpack = e.NewMsgpack
		}
		if !e.ExpiredAt.IsZero() {
			out.ExpiredAt = timestamppb.New(e.ExpiredAt)
		}
		patched = append(patched, out)
	}
	return &hydrapb.PatchExpiredTreasuresManyEntry{Patched: patched}
}
