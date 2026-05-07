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
