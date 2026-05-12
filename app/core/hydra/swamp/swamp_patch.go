package swamp

import (
	"errors"
	"time"

	"github.com/hydraide/hydraide/app/core/hydra/swamp/treasure"
	"github.com/hydraide/hydraide/app/core/hydra/swamp/treasure/guard"
	"github.com/hydraide/hydraide/app/core/hydra/swamp/treasure/msgpackpatch"
)

// PatchFieldsStatus categorizes the per-key outcome of a PatchFields call.
// It mirrors the wire-level PatchResult.StatusCode values defined in
// hydraide.proto so the gateway can map directly without translation.
type PatchFieldsStatus uint8

const (
	// PatchStatusPatched indicates ops were applied on an existing treasure.
	PatchStatusPatched PatchFieldsStatus = iota

	// PatchStatusCreated indicates the treasure did not exist; CreateIfNotExist
	// was true so a new treasure was seeded and patched.
	PatchStatusCreated

	// PatchStatusKeyNotFound indicates the treasure did not exist and
	// CreateIfNotExist was false.
	PatchStatusKeyNotFound

	// PatchStatusConditionNotMet indicates the patch's Condition evaluated
	// to false; ops were not applied.
	PatchStatusConditionNotMet

	// PatchStatusFieldNotFound is reserved for ops that strictly require an
	// existing field; currently all ops auto-create or no-op as documented.
	PatchStatusFieldNotFound

	// PatchStatusTypeMismatch indicates an op or condition crossed a type
	// boundary (e.g. INC on a string field, MERGE on a non-map target, or
	// the existing treasure value is not a ByteArray).
	PatchStatusTypeMismatch

	// PatchStatusPathInvalid indicates a malformed path or an unresolvable
	// structural reference (e.g. array index out of range).
	PatchStatusPathInvalid

	// PatchStatusEncodingNotSupported indicates the existing treasure
	// ByteArray is not msgpack-encoded (e.g. raw bytes, or GOB-encoded).
	PatchStatusEncodingNotSupported

	// PatchStatusInternalError is a catch-all for unexpected failures.
	PatchStatusInternalError

	// PatchStatusCapExceeded indicates the patch was rejected because
	// applying it would have pushed the count of records matching
	// Cap.Filter above Cap.MaxMatching in the affected swamp. Ops were
	// not applied; the request can be retried after the matching count
	// drops below the budget.
	PatchStatusCapExceeded
)

// PatchFieldsMeta selects which timestamp/identity fields the server should
// stamp on the patched treasure.
type PatchFieldsMeta struct {
	// SetUpdatedAt: when true, ModifiedAt is stamped to the current time.
	SetUpdatedAt bool

	// SetUpdatedBy is recorded as ModifiedBy when non-empty.
	SetUpdatedBy string

	// SetCreatedAt: when true and the treasure is created in this call,
	// CreatedAt is stamped to the current time. Ignored on existing
	// treasures.
	SetCreatedAt bool

	// SetCreatedBy is recorded as CreatedBy when non-empty and the treasure
	// is created in this call.
	SetCreatedBy string

	// SetExpiredAt sets ExpiredAt on the patched treasure when non-zero.
	// Applied to both newly-created and existing treasures, so callers can
	// attach or slide a TTL through a patch without rewriting the body.
	// Ignored when ClearExpiredAt is true.
	SetExpiredAt time.Time

	// ClearExpiredAt: when true, ExpiredAt is reset to "never expires" on
	// the patched treasure. Takes precedence over SetExpiredAt when both
	// are set.
	ClearExpiredAt bool
}

// PatchFieldsOptions controls per-key patch behavior.
type PatchFieldsOptions struct {
	// CreateIfNotExist: when true, missing keys are created with an empty
	// msgpack map (or InitialMsgpackOnCreate, if set) before ops apply.
	CreateIfNotExist bool

	// InitialMsgpackOnCreate is an optional seed body for newly-created
	// treasures. Must be a msgpack-encoded map (no magic prefix); the
	// server adds the prefix when storing. Non-map seeds yield
	// PatchStatusTypeMismatch.
	InitialMsgpackOnCreate []byte

	// Meta selects metadata fields to stamp on the patched treasure.
	Meta *PatchFieldsMeta

	// CapPredicate, when non-nil, enforces a Cap quota check on this
	// per-key patch. The predicate receives a raw msgpack body (no magic
	// prefix) and returns true when the body matches Cap.Filter.
	// PatchFields invokes it on the pre body and the simulated post
	// body to apply the (pre, post) four-cell rule (see Cap proto
	// docs). The caller is responsible for tracking the running
	// (no→yes) budget across a batch via CapBudgetLeft, and for
	// decoding the body inside the predicate.
	//
	// The engine layer does not import msgpack — the predicate
	// closure (built at the gateway from the Cap.Filter wire form)
	// owns decoding.
	CapPredicate func(rawMsgpackBody []byte) bool

	// CapBudgetLeft is the remaining (no→yes) budget for this batch.
	// PatchFields decrements *CapBudgetLeft when it accepts a (no, yes)
	// transition. A (no, yes) transition with *CapBudgetLeft <= 0 is
	// rejected as PatchStatusCapExceeded and *CapBudgetLeft is not
	// modified. Ignored when CapPredicate is nil.
	CapBudgetLeft *int32
}

// PatchFieldsResult carries the per-key outcome.
type PatchFieldsResult struct {
	// Status is the outcome code.
	Status PatchFieldsStatus

	// Error carries a free-form description for non-success statuses.
	Error string

	// NewMsgpack is the unwrapped (no magic prefix) post-patch msgpack body.
	// Populated on PATCHED / CREATED outcomes for callers that need to
	// echo it back to clients; nil otherwise.
	NewMsgpack []byte
}

// msgpackMagic0/1 mirror the SDK's wire-level prefix on msgpack-encoded
// ByteArray values. These two bytes precede the actual msgpack body.
const (
	patchMsgpackMagic0 byte = 0xC7
	patchMsgpackMagic1 byte = 0x00
)

// emptyMapMsgpack is the canonical encoding of an empty msgpack map (fixmap
// with zero entries). Used as the default seed for CreateIfNotExist.
var emptyMapMsgpack = []byte{0x80}

// PatchFields applies field-level mutations to a msgpack-encoded ByteArray
// treasure value at key. Ops execute in order under the per-key guard, so
// concurrent callers on the same key serialize via the existing FIFO queue.
//
// On TYPE_MISMATCH / PATH_INVALID / ENCODING_NOT_SUPPORTED / CONDITION_NOT_MET
// the function returns (result, nil) — these are per-key business outcomes,
// not server errors. The error return is reserved for unexpected internal
// failures.
func (s *swamp) PatchFields(key string, ops []msgpackpatch.Op, condition *msgpackpatch.Condition, opts PatchFieldsOptions) (PatchFieldsResult, error) {
	// Best-effort early exit: if the treasure does not exist and we are not allowed to create
	// one, there is no point taking a guard. The in-guard re-check below is the actual
	// correctness guarantee against the TOCTOU race between beaconKey.Get and CreateTreasure.
	if !opts.CreateIfNotExist && s.beaconKey.Get(key) == nil {
		return PatchFieldsResult{Status: PatchStatusKeyNotFound}, nil
	}

	// If we may create the treasure, validate the seed up front so we never park an unusable
	// in-flight treasure in creatingTreasures (it would leak there until the swamp closes).
	seed := opts.InitialMsgpackOnCreate
	if len(seed) == 0 {
		seed = emptyMapMsgpack
	}
	if opts.CreateIfNotExist {
		if _, err := msgpackpatch.Parse(seed); err != nil {
			return PatchFieldsResult{
				Status: PatchStatusTypeMismatch,
				Error:  "InitialMsgpackOnCreate: " + err.Error(),
			}, nil
		}
	}

	// Get-or-create the treasure outside the guard, then decide new-vs-existing inside the
	// guard. Without the in-guard re-check, a concurrent goroutine that finishes its create+
	// patch in the window between the early beaconKey.Get and CreateTreasure would let this
	// caller silently overwrite the patched body with the seed.
	treasureObj := s.beaconKey.Get(key)
	createdNew := false
	if treasureObj == nil {
		treasureObj = s.CreateTreasure(key)
		createdNew = true
	}

	guardID := treasureObj.StartTreasureGuard(true)
	defer treasureObj.ReleaseTreasureGuard(guardID)

	saved := false
	defer func() {
		// If we obtained a fresh in-flight treasure but never persisted it (condition failed,
		// patch errored, content type was wrong), drop it from the creatingTreasures tracker
		// so it does not accumulate. Save() already cleans up on the success path.
		if createdNew && !saved {
			s.creatingTreasures.Delete(key)
		}
	}()

	var (
		inputBody []byte
		isCreate  bool
	)
	switch treasureObj.GetContentType() {
	case treasure.ContentTypeVoid:
		if !opts.CreateIfNotExist {
			// The early exit lost a race against a concurrent delete. Report KeyNotFound.
			return PatchFieldsResult{Status: PatchStatusKeyNotFound}, nil
		}
		inputBody = seed
		isCreate = true
	case treasure.ContentTypeByteArray:
		raw, err := treasureObj.GetContentByteArray()
		if err != nil {
			return PatchFieldsResult{Status: PatchStatusInternalError, Error: err.Error()}, nil
		}
		if len(raw) < 2 || raw[0] != patchMsgpackMagic0 || raw[1] != patchMsgpackMagic1 {
			return PatchFieldsResult{
				Status: PatchStatusEncodingNotSupported,
				Error:  "treasure ByteArray is not msgpack-encoded (missing magic prefix)",
			}, nil
		}
		inputBody = raw[2:]
	default:
		return PatchFieldsResult{
			Status: PatchStatusTypeMismatch,
			Error:  "treasure is not a ByteArray",
		}, nil
	}

	out, err := msgpackpatch.ApplyWithCondition(inputBody, ops, condition)
	if err != nil {
		return PatchFieldsResult{Status: classifyPatchError(err), Error: err.Error()}, nil
	}

	// Cap pre/post check: only the (no, yes) transition consumes budget;
	// all other transitions proceed unchanged (idempotent / shrinking /
	// untouched). The caller (gateway) has already pre-counted matching
	// records and holds capMu, so the running CapBudgetLeft observed by
	// concurrent batches is consistent.
	if opts.CapPredicate != nil {
		preMatched := false
		if !isCreate {
			preMatched = opts.CapPredicate(inputBody)
		}
		postMatched := opts.CapPredicate(out)
		if !preMatched && postMatched {
			if opts.CapBudgetLeft == nil || *opts.CapBudgetLeft <= 0 {
				return PatchFieldsResult{Status: PatchStatusCapExceeded}, nil
			}
			*opts.CapBudgetLeft--
		}
	}

	treasureObj.SetContentByteArray(guardID, wrapMsgpackBody(out))
	applyPatchMeta(treasureObj, guardID, opts.Meta, isCreate)
	treasureObj.Save(guardID)
	saved = true

	if isCreate {
		return PatchFieldsResult{Status: PatchStatusCreated, NewMsgpack: out}, nil
	}
	return PatchFieldsResult{Status: PatchStatusPatched, NewMsgpack: out}, nil
}

// wrapMsgpackBody prepends the msgpack magic prefix to a raw msgpack body.
func wrapMsgpackBody(body []byte) []byte {
	out := make([]byte, 2+len(body))
	out[0] = patchMsgpackMagic0
	out[1] = patchMsgpackMagic1
	copy(out[2:], body)
	return out
}

// classifyPatchError maps a msgpackpatch sentinel error onto a PatchFieldsStatus.
func classifyPatchError(err error) PatchFieldsStatus {
	switch {
	case errors.Is(err, msgpackpatch.ErrConditionNotMet):
		return PatchStatusConditionNotMet
	case errors.Is(err, msgpackpatch.ErrTypeMismatch):
		return PatchStatusTypeMismatch
	case errors.Is(err, msgpackpatch.ErrPathInvalid):
		return PatchStatusPathInvalid
	case errors.Is(err, msgpackpatch.ErrInvalidOp):
		return PatchStatusPathInvalid
	case errors.Is(err, msgpackpatch.ErrInvalidMsgpack), errors.Is(err, msgpackpatch.ErrNonStringKey):
		return PatchStatusEncodingNotSupported
	}
	return PatchStatusInternalError
}

// applyPatchMeta stamps the requested metadata fields on treasureObj.
// onCreate gates the SetCreated* fields so they only apply on newly-created
// treasures.
func applyPatchMeta(treasureObj treasure.Treasure, guardID guard.ID, meta *PatchFieldsMeta, onCreate bool) {
	if meta == nil {
		return
	}
	now := time.Now().UTC()
	if meta.SetUpdatedAt {
		treasureObj.SetModifiedAt(guardID, now)
	}
	if meta.SetUpdatedBy != "" {
		treasureObj.SetModifiedBy(guardID, meta.SetUpdatedBy)
	}
	if onCreate {
		if meta.SetCreatedAt {
			treasureObj.SetCreatedAt(guardID, now)
		}
		if meta.SetCreatedBy != "" {
			treasureObj.SetCreatedBy(guardID, meta.SetCreatedBy)
		}
	}
	// ExpiredAt applies to both newly-created and existing treasures.
	// ClearExpiredAt takes precedence so callers can unambiguously reset
	// the TTL even if SetExpiredAt is also populated.
	if meta.ClearExpiredAt {
		treasureObj.SetExpirationTime(guardID, time.Time{})
	} else if !meta.SetExpiredAt.IsZero() {
		treasureObj.SetExpirationTime(guardID, meta.SetExpiredAt)
	}
}
