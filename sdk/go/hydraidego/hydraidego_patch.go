package hydraidego

import (
	"context"
	"fmt"
	"time"

	hydraidepbgo "github.com/hydraide/hydraide/sdk/go/hydraidego/v3/hydraidepbgo"
	"github.com/hydraide/hydraide/sdk/go/hydraidego/v3/name"
	"github.com/vmihailenco/msgpack/v5"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// PatchStatus mirrors the wire-level PatchResult.StatusCode values from the
// proto definition. Returning the engine's status verbatim lets callers map
// outcomes precisely without requiring a translation layer for every
// possible "did the patch apply?" question.
type PatchStatus int

const (
	// PatchStatusPatched indicates ops were applied to an existing treasure.
	PatchStatusPatched PatchStatus = iota

	// PatchStatusCreated indicates CreateIfNotExist was true, the treasure
	// did not exist, and was created and patched.
	PatchStatusCreated

	// PatchStatusKeyNotFound indicates the treasure did not exist and
	// CreateIfNotExist was false.
	PatchStatusKeyNotFound

	// PatchStatusConditionNotMet indicates the patch's condition evaluated
	// to false; ops were not applied.
	PatchStatusConditionNotMet

	// PatchStatusFieldNotFound is reserved; current ops auto-create or
	// no-op as documented.
	PatchStatusFieldNotFound

	// PatchStatusTypeMismatch indicates an op or condition crossed a type
	// boundary (e.g. INC on a string field, MERGE on a non-map target).
	PatchStatusTypeMismatch

	// PatchStatusPathInvalid indicates a malformed path or an unresolvable
	// structural reference (e.g. array index out of range).
	PatchStatusPathInvalid

	// PatchStatusEncodingNotSupported indicates the existing treasure is
	// not msgpack-encoded (raw bytes, GOB-encoded, etc.).
	PatchStatusEncodingNotSupported

	// PatchStatusInternalError is a catch-all for unexpected server
	// failures. The accompanying error string carries detail.
	PatchStatusInternalError
)

// String implements the Stringer interface for human-readable status logs.
func (s PatchStatus) String() string {
	switch s {
	case PatchStatusPatched:
		return "PATCHED"
	case PatchStatusCreated:
		return "CREATED"
	case PatchStatusKeyNotFound:
		return "KEY_NOT_FOUND"
	case PatchStatusConditionNotMet:
		return "CONDITION_NOT_MET"
	case PatchStatusFieldNotFound:
		return "FIELD_NOT_FOUND"
	case PatchStatusTypeMismatch:
		return "TYPE_MISMATCH"
	case PatchStatusPathInvalid:
		return "PATH_INVALID"
	case PatchStatusEncodingNotSupported:
		return "ENCODING_NOT_SUPPORTED"
	case PatchStatusInternalError:
		return "INTERNAL_ERROR"
	}
	return fmt.Sprintf("PatchStatus(%d)", int(s))
}

// PatchManyRequest is one entry in a multi-key patch batch issued via
// CatalogPatchFieldsMany. The Builder carries the key, ops, optional
// condition, and optional metadata for one treasure.
//
// Build the per-key patch with NewPatchBuilder(key) and the same fluent
// API used by single-key CatalogPatch. The builder is data-only (no
// client / context binding); the batch dispatcher executes the ops on
// the swamp passed to CatalogPatchFieldsMany.
//
// Example:
//
//	requests := []*PatchManyRequest{
//	    {Builder: NewPatchBuilder("alice").
//	        Set("Score", int32(42)).
//	        WithUpdatedAt()},
//	    {Builder: NewPatchBuilder("bob").
//	        Inc("Counter", int32(1)).
//	        IfFieldGreaterThanOrEqual("Counter", int32(0))},
//	}
//	err := h.CatalogPatchFieldsMany(ctx, swamp, requests, iterator)
type PatchManyRequest struct {
	// Builder holds the key + ops + optional cond + optional meta for
	// this single treasure. Use NewPatchBuilder(key) to obtain one.
	Builder *PatchBuilder
}

// PatchManyIteratorFunc is invoked once per result in a CatalogPatchFieldsMany
// response, in the same order as the input requests. Returning a non-nil
// error from the iterator stops further iteration and bubbles up.
type PatchManyIteratorFunc func(key string, status PatchStatus, errMsg string) error

// CatalogPatchField applies a single SET op on a msgpack-encoded Catalog
// treasure. fieldPath uses dot notation for nested fields ("Counters.Views")
// and bracket notation for array indices ("Tags[0]"). value is encoded to
// msgpack via reflection; the field's resulting type code matches the
// canonical encoding for value's Go type (e.g. int8 → msgpack int8).
//
// The treasure is created if it does not exist (CreateIfNotExist is enabled).
func (h *hydraidego) CatalogPatchField(ctx context.Context, swampName name.Name, key, fieldPath string, value any) (PatchStatus, error) {
	encoded, err := encodePatchValue(value)
	if err != nil {
		return PatchStatusInternalError, NewError(ErrCodeInvalidModel, err.Error())
	}
	return h.runPatch(ctx, swampName, key, []*hydraidepbgo.PatchOp{
		{Op: hydraidepbgo.PatchOp_SET, Path: fieldPath, Value: encoded},
	}, nil, true, nil)
}

// CatalogPatchFields applies a SET op per field in a single round-trip.
// All ops execute atomically under the per-key guard, so callers cannot
// observe an intermediate state.
//
// The map's iteration order is not deterministic — paths that overlap (e.g.
// both "Foo" and "Foo.Bar") will produce undefined ordering and should be
// avoided. Use the builder API (CatalogPatch) when ordering matters.
func (h *hydraidego) CatalogPatchFields(ctx context.Context, swampName name.Name, key string, fields map[string]any) (PatchStatus, error) {
	if len(fields) == 0 {
		return PatchStatusInternalError, NewError(ErrCodeInvalidModel, "fields map is empty")
	}
	ops := make([]*hydraidepbgo.PatchOp, 0, len(fields))
	for path, v := range fields {
		encoded, err := encodePatchValue(v)
		if err != nil {
			return PatchStatusInternalError, NewError(ErrCodeInvalidModel, fmt.Sprintf("encode field %q: %v", path, err))
		}
		ops = append(ops, &hydraidepbgo.PatchOp{
			Op:    hydraidepbgo.PatchOp_SET,
			Path:  path,
			Value: encoded,
		})
	}
	return h.runPatch(ctx, swampName, key, ops, nil, true, nil)
}

// CatalogPatchFieldsMany dispatches a batch of per-key patches in a single
// PatchTreasures RPC. Each request carries a PatchBuilder with the key,
// ops, optional condition, and optional metadata for one treasure. Ops
// run in declaration order under the per-key guard. The iterator (if
// non-nil) is invoked once per result in request order. Returning an
// error from the iterator stops iteration.
//
// All requests are dispatched against the same swampName. CreateIfNotExist
// is honored per-builder via NoCreate(); by default each builder creates
// missing treasures.
//
// When a builder carries Meta (WithUpdatedAt / WithExpiredAt / etc.), the
// per-key Meta fully replaces any request-level Meta on that key (no
// field-level merge). For a batch where every key shares the same Meta,
// set it on each builder — there is no batch-level Meta knob on this API.
func (h *hydraidego) CatalogPatchFieldsMany(ctx context.Context, swampName name.Name, requests []*PatchManyRequest, iterator PatchManyIteratorFunc) error {
	if len(requests) == 0 {
		return nil
	}
	patches := make([]*hydraidepbgo.TreasurePatch, 0, len(requests))
	var createIfNotExist bool
	for i, req := range requests {
		if req == nil || req.Builder == nil {
			return NewError(ErrCodeInvalidModel, fmt.Sprintf("request %d: nil request or nil Builder", i))
		}
		b := req.Builder
		if b.encodeError != nil {
			return NewError(ErrCodeInvalidModel, fmt.Sprintf("request %d (%q): %v", i, b.key, b.encodeError))
		}
		if b.key == "" {
			return NewError(ErrCodeInvalidModel, fmt.Sprintf("request %d: builder has empty key", i))
		}
		if len(b.ops) == 0 && b.meta == nil {
			return NewError(ErrCodeInvalidModel, fmt.Sprintf("request %d (%q): builder has no ops and no meta", i, b.key))
		}
		// CreateIfNotExist is a request-level wire knob; require all
		// builders in one batch to agree to keep the semantics
		// per-builder explicit. Mixed flags would silently apply the OR
		// of the two, which is a footgun.
		if i == 0 {
			createIfNotExist = b.create
		} else if b.create != createIfNotExist {
			return NewError(ErrCodeInvalidModel, fmt.Sprintf("request %d (%q): NoCreate flag differs from request 0; split the batch", i, b.key))
		}
		patches = append(patches, &hydraidepbgo.TreasurePatch{
			Key:       b.key,
			Ops:       b.ops,
			Condition: b.cond,
			Meta:      b.meta,
		})
	}

	resp, err := h.client.GetServiceClient(swampName).PatchTreasures(ctx, &hydraidepbgo.PatchTreasuresRequest{
		IslandID:         swampName.GetIslandID(h.client.GetAllIslands()),
		SwampName:        swampName.Get(),
		Patches:          patches,
		CreateIfNotExist: createIfNotExist,
	})
	if err != nil {
		return translatePatchGRPCError(err)
	}

	if iterator == nil {
		return nil
	}
	for _, r := range resp.GetResults() {
		if err := iterator(r.GetKey(), PatchStatus(r.GetStatus()), r.GetError()); err != nil {
			return err
		}
	}
	return nil
}

// runPatch issues a single-key PatchTreasures RPC and returns the per-key
// status. cond and meta are optional; createIfNotExist is forwarded onto
// the request.
func (h *hydraidego) runPatch(
	ctx context.Context,
	swampName name.Name,
	key string,
	ops []*hydraidepbgo.PatchOp,
	cond *hydraidepbgo.PatchCondition,
	createIfNotExist bool,
	meta *hydraidepbgo.PatchMeta,
) (PatchStatus, error) {
	if len(ops) == 0 {
		return PatchStatusInternalError, NewError(ErrCodeInvalidModel, "ops list is empty")
	}
	resp, err := h.client.GetServiceClient(swampName).PatchTreasures(ctx, &hydraidepbgo.PatchTreasuresRequest{
		IslandID:         swampName.GetIslandID(h.client.GetAllIslands()),
		SwampName:        swampName.Get(),
		CreateIfNotExist: createIfNotExist,
		Meta:             meta,
		Patches: []*hydraidepbgo.TreasurePatch{
			{Key: key, Ops: ops, Condition: cond},
		},
	})
	if err != nil {
		return PatchStatusInternalError, translatePatchGRPCError(err)
	}
	if len(resp.GetResults()) == 0 {
		return PatchStatusInternalError, NewError(ErrCodeUnknown, "empty PatchTreasures response")
	}
	r := resp.GetResults()[0]
	st := PatchStatus(r.GetStatus())
	// Per-key statuses are NOT errors — they describe an outcome the caller
	// is expected to handle (e.g. CONDITION_NOT_MET, KEY_NOT_FOUND, even
	// TYPE_MISMATCH which often signals a typed client bug). The error
	// return is reserved for transport-level failures, which are caught
	// above via translatePatchGRPCError. INTERNAL_ERROR is the only
	// per-key status that maps back to a non-nil error so callers writing
	// `if err != nil` still notice unexpected server-side failures.
	if st == PatchStatusInternalError && r.GetError() != "" {
		return st, NewError(ErrCodeInternalDatabaseError, r.GetError())
	}
	return st, nil
}

// encodePatchValue marshals an arbitrary Go value into a stand-alone msgpack
// blob suitable for use as PatchOp.Value or PatchCondition.Threshold. The
// type code of the resulting blob mirrors the Go type's canonical msgpack
// encoding (vmihailenco/msgpack/v5 maps int8 → msgpack int8 etc.).
func encodePatchValue(v any) ([]byte, error) {
	if v == nil {
		return nil, fmt.Errorf("patch value cannot be nil")
	}
	return msgpack.Marshal(v)
}

// translatePatchGRPCError maps gRPC error codes onto the SDK's existing
// error vocabulary (mirrors CatalogSave's error translation block).
func translatePatchGRPCError(err error) error {
	if s, ok := status.FromError(err); ok {
		switch s.Code() {
		case codes.Aborted:
			return NewError(ErrorShuttingDown, errorMessageShuttingDown)
		case codes.Unavailable:
			return NewError(ErrCodeConnectionError, errorMessageConnectionError)
		case codes.DeadlineExceeded:
			return NewError(ErrCodeCtxTimeout, errorMessageCtxTimeout)
		case codes.Canceled:
			return NewError(ErrCodeCtxClosedByClient, errorMessageCtxClosedByClient)
		case codes.Internal:
			return NewError(ErrCodeInternalDatabaseError, fmt.Sprintf("%s: %v", errorMessageInternalError, s.Message()))
		case codes.InvalidArgument:
			return NewError(ErrCodeInvalidArgument, s.Message())
		default:
			return NewError(ErrCodeUnknown, fmt.Sprintf("%s: %v", errorMessageUnknown, err))
		}
	}
	return NewError(ErrCodeUnknown, fmt.Sprintf("%s: %v", errorMessageUnknown, err))
}

// ============================================================================
// Builder API
// ============================================================================

// PatchBuilder accumulates ops, an optional condition, and optional
// metadata for one treasure key. The fluent style mirrors common ORM
// idioms; ops are appended in call order, so chained calls produce a
// deterministic op sequence.
//
// Two construction paths exist:
//
//   - CatalogPatch(ctx, swamp, key) returns a builder bound to a client
//     and swamp; call Exec() on it to dispatch a single PatchTreasures RPC.
//
//   - NewPatchBuilder(key) returns a data-only builder (no client / no
//     swamp); pass it inside a PatchManyRequest to CatalogPatchFieldsMany,
//     CatalogPatchManyToMany, etc., where the dispatcher provides the
//     swamp and the gRPC client.
//
// Both paths share the same fluent API.
type PatchBuilder struct {
	h           *hydraidego
	ctx         context.Context
	swampName   name.Name
	key         string
	ops         []*hydraidepbgo.PatchOp
	cond        *hydraidepbgo.PatchCondition
	create      bool
	meta        *hydraidepbgo.PatchMeta
	encodeError error
}

// NewPatchBuilder returns a data-only builder for one treasure key, ready
// to be carried inside a PatchManyRequest in batch APIs (e.g.
// CatalogPatchFieldsMany). The builder is not bound to a client or
// context; calling Exec() on it returns an error.
//
// CreateIfNotExist defaults to true; call NoCreate() to disable.
func NewPatchBuilder(key string) *PatchBuilder {
	return &PatchBuilder{
		key:    key,
		create: true,
	}
}

// CatalogPatch returns a new builder bound to (swamp, key) and ready to
// dispatch its accumulated ops via Exec(). CreateIfNotExist defaults to
// true; call NoCreate() to disable.
func (h *hydraidego) CatalogPatch(ctx context.Context, swampName name.Name, key string) *PatchBuilder {
	b := NewPatchBuilder(key)
	b.h = h
	b.ctx = ctx
	b.swampName = swampName
	return b
}

// NoCreate disables the auto-create behavior. Use this when you want a
	// missing treasure to surface as PatchStatusKeyNotFound rather than
	// being created on the fly.
func (b *PatchBuilder) NoCreate() *PatchBuilder {
	b.create = false
	return b
}

// Set appends a SET op.
func (b *PatchBuilder) Set(path string, value any) *PatchBuilder {
	return b.appendOp(hydraidepbgo.PatchOp_SET, path, value)
}

// Delete appends a DELETE op (Value is ignored on the wire).
func (b *PatchBuilder) Delete(path string) *PatchBuilder {
	return b.appendOpRaw(hydraidepbgo.PatchOp_DELETE, path, nil)
}

// Inc appends an INC op. delta must be a numeric Go value whose msgpack
// class (int / uint / float) matches the target field's class.
func (b *PatchBuilder) Inc(path string, delta any) *PatchBuilder {
	return b.appendOp(hydraidepbgo.PatchOp_INC, path, delta)
}

// Append appends an APPEND op. path must end in "[]" (e.g. "Tags[]").
func (b *PatchBuilder) Append(path string, value any) *PatchBuilder {
	return b.appendOp(hydraidepbgo.PatchOp_APPEND, path, value)
}

// Prepend appends a PREPEND op. path must end in "[]".
func (b *PatchBuilder) Prepend(path string, value any) *PatchBuilder {
	return b.appendOp(hydraidepbgo.PatchOp_PREPEND, path, value)
}

// RemoveAt appends a REMOVE_AT op. path must include an [index] segment.
func (b *PatchBuilder) RemoveAt(path string) *PatchBuilder {
	return b.appendOpRaw(hydraidepbgo.PatchOp_REMOVE_AT, path, nil)
}

// RemoveVal appends a REMOVE_VAL op. value is matched byte-wise against
// array elements; the first match is removed.
func (b *PatchBuilder) RemoveVal(path string, value any) *PatchBuilder {
	return b.appendOp(hydraidepbgo.PatchOp_REMOVE_VAL, path, value)
}

// Merge appends a MERGE op. value must be a Go map / struct that encodes
// to a msgpack map; conflicting keys overwrite the target's matching
// fields, others are appended.
func (b *PatchBuilder) Merge(path string, value any) *PatchBuilder {
	return b.appendOp(hydraidepbgo.PatchOp_MERGE, path, value)
}

// IfFieldEquals adds a CondEqual condition.
func (b *PatchBuilder) IfFieldEquals(path string, value any) *PatchBuilder {
	return b.setCondition(hydraidepbgo.PatchCondition_EQUAL, path, value)
}

// IfFieldNotEquals adds a CondNotEqual condition.
func (b *PatchBuilder) IfFieldNotEquals(path string, value any) *PatchBuilder {
	return b.setCondition(hydraidepbgo.PatchCondition_NOT_EQUAL, path, value)
}

// IfFieldGreaterThan adds a CondGreaterThan condition.
func (b *PatchBuilder) IfFieldGreaterThan(path string, value any) *PatchBuilder {
	return b.setCondition(hydraidepbgo.PatchCondition_GREATER_THAN, path, value)
}

// IfFieldGreaterThanOrEqual adds a CondGreaterThanOrEqual condition.
func (b *PatchBuilder) IfFieldGreaterThanOrEqual(path string, value any) *PatchBuilder {
	return b.setCondition(hydraidepbgo.PatchCondition_GREATER_THAN_OR_EQUAL, path, value)
}

// IfFieldLessThan adds a CondLessThan condition.
func (b *PatchBuilder) IfFieldLessThan(path string, value any) *PatchBuilder {
	return b.setCondition(hydraidepbgo.PatchCondition_LESS_THAN, path, value)
}

// IfFieldLessThanOrEqual adds a CondLessThanOrEqual condition.
func (b *PatchBuilder) IfFieldLessThanOrEqual(path string, value any) *PatchBuilder {
	return b.setCondition(hydraidepbgo.PatchCondition_LESS_THAN_OR_EQUAL, path, value)
}

// IfFieldExists adds a CondExists condition.
func (b *PatchBuilder) IfFieldExists(path string) *PatchBuilder {
	if b.encodeError != nil {
		return b
	}
	b.cond = &hydraidepbgo.PatchCondition{Path: path, Operator: hydraidepbgo.PatchCondition_EXISTS}
	return b
}

// IfFieldNotExists adds a CondNotExists condition.
func (b *PatchBuilder) IfFieldNotExists(path string) *PatchBuilder {
	if b.encodeError != nil {
		return b
	}
	b.cond = &hydraidepbgo.PatchCondition{Path: path, Operator: hydraidepbgo.PatchCondition_NOT_EXISTS}
	return b
}

// WithUpdatedAt stamps ModifiedAt = now on the patched treasure.
func (b *PatchBuilder) WithUpdatedAt() *PatchBuilder {
	if b.meta == nil {
		b.meta = &hydraidepbgo.PatchMeta{}
	}
	b.meta.SetUpdatedAt = true
	return b
}

// WithUpdatedBy stamps ModifiedBy = userID on the patched treasure.
func (b *PatchBuilder) WithUpdatedBy(userID string) *PatchBuilder {
	if b.meta == nil {
		b.meta = &hydraidepbgo.PatchMeta{}
	}
	b.meta.SetUpdatedBy = &userID
	return b
}

// WithExpiredAt stamps ExpiredAt = expireAt on the patched treasure (both
// newly-created and existing). Use this to attach a TTL at patch time, or to
// slide an existing TTL without rewriting the body. A zero time.Time clears
// any existing ExpiredAt — equivalent to WithoutExpiredAt().
func (b *PatchBuilder) WithExpiredAt(expireAt time.Time) *PatchBuilder {
	if b.meta == nil {
		b.meta = &hydraidepbgo.PatchMeta{}
	}
	if expireAt.IsZero() {
		b.meta.ClearExpiredAt = true
		b.meta.SetExpiredAt = nil
		return b
	}
	b.meta.SetExpiredAt = timestamppb.New(expireAt)
	b.meta.ClearExpiredAt = false
	return b
}

// WithoutExpiredAt resets ExpiredAt to "never expires" on the patched
// treasure. Wins over a previous WithExpiredAt call on the same builder.
func (b *PatchBuilder) WithoutExpiredAt() *PatchBuilder {
	if b.meta == nil {
		b.meta = &hydraidepbgo.PatchMeta{}
	}
	b.meta.ClearExpiredAt = true
	b.meta.SetExpiredAt = nil
	return b
}

// Exec dispatches the accumulated patch as a single RPC. Only valid on
// builders returned by CatalogPatch (which bind a client, context, and
// swamp). Builders created via NewPatchBuilder are data-only and must be
// dispatched through a batch API instead.
func (b *PatchBuilder) Exec() (PatchStatus, error) {
	if b.encodeError != nil {
		return PatchStatusInternalError, NewError(ErrCodeInvalidModel, b.encodeError.Error())
	}
	if b.h == nil {
		return PatchStatusInternalError, NewError(ErrCodeInvalidModel, "PatchBuilder is not bound to a client; use CatalogPatch(ctx, swamp, key) for single-key dispatch, or pass it via PatchManyRequest to a batch API")
	}
	return b.h.runPatch(b.ctx, b.swampName, b.key, b.ops, b.cond, b.create, b.meta)
}

// appendOp encodes value to msgpack and appends a typed op.
func (b *PatchBuilder) appendOp(kind hydraidepbgo.PatchOp_Kind, path string, value any) *PatchBuilder {
	if b.encodeError != nil {
		return b
	}
	encoded, err := encodePatchValue(value)
	if err != nil {
		b.encodeError = fmt.Errorf("encode op %s path %q: %w", kind, path, err)
		return b
	}
	return b.appendOpRaw(kind, path, encoded)
}

// appendOpRaw appends an op without encoding (DELETE / REMOVE_AT case).
func (b *PatchBuilder) appendOpRaw(kind hydraidepbgo.PatchOp_Kind, path string, raw []byte) *PatchBuilder {
	op := &hydraidepbgo.PatchOp{Op: kind, Path: path}
	if raw != nil {
		op.Value = raw
	}
	b.ops = append(b.ops, op)
	return b
}

// setCondition encodes value to msgpack and replaces the builder's condition.
func (b *PatchBuilder) setCondition(opCode hydraidepbgo.PatchCondition_Op, path string, value any) *PatchBuilder {
	if b.encodeError != nil {
		return b
	}
	encoded, err := encodePatchValue(value)
	if err != nil {
		b.encodeError = fmt.Errorf("encode condition path %q: %w", path, err)
		return b
	}
	b.cond = &hydraidepbgo.PatchCondition{
		Path:      path,
		Operator:  opCode,
		Threshold: encoded,
	}
	return b
}
