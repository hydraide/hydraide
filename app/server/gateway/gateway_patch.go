package gateway

import (
	"context"
	"fmt"

	"github.com/hydraide/hydraide/app/core/hydra/swamp"
	"github.com/hydraide/hydraide/app/core/hydra/swamp/treasure/msgpackpatch"
	hydrapb "github.com/hydraide/hydraide/generated/hydraidepbgo"
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

	if in.GetSwampName() == "" {
		return nil, status.Error(codes.InvalidArgument, "SwampName cannot be empty")
	}

	if len(in.GetPatches()) == 0 {
		// Empty batch is a valid no-op.
		return &hydrapb.PatchTreasuresResponse{Results: nil}, nil
	}

	// We do not require the swamp to pre-exist; SummonSwamp creates it.
	// CheckSwampName with checkExist=false validates the name format only.
	swampName, err := checkSwampName(g.ZeusInterface, in.GetIslandID(), in.GetSwampName(), false)
	if err != nil {
		return nil, err
	}

	hydraInterface := g.ZeusInterface.GetHydra()
	swampObj, err := hydraInterface.SummonSwamp(ctx, in.GetIslandID(), swampName)
	if err != nil {
		return nil, status.Error(codes.Internal, fmt.Sprintf("internal server error in hydra: %s", err.Error()))
	}
	swampObj.BeginVigil()
	defer swampObj.CeaseVigil()

	opts := swamp.PatchFieldsOptions{
		CreateIfNotExist:       in.GetCreateIfNotExist(),
		InitialMsgpackOnCreate: in.GetInitialMsgpackOnCreate(),
		Meta:                   protoMetaToSwampMeta(in.GetMeta()),
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

	return &hydrapb.PatchTreasuresResponse{Results: results}, nil
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
	return &swamp.PatchFieldsMeta{
		SetUpdatedAt: in.GetSetUpdatedAt(),
		SetUpdatedBy: in.GetSetUpdatedBy(),
		SetCreatedAt: in.GetSetCreatedAt(),
		SetCreatedBy: in.GetSetCreatedBy(),
	}
}

// protoStr returns a *string with the given content, mirroring the proto
// optional-string idiom.
func protoStr(s string) *string {
	return &s
}
