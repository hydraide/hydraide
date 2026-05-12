package hydraidego

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"

	hydraidepbgo "github.com/hydraide/hydraide/sdk/go/hydraidego/v3/hydraidepbgo"
	"github.com/hydraide/hydraide/sdk/go/hydraidego/v3/name"
	"github.com/vmihailenco/msgpack/v5"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// CatalogPatchExpiredIteratorFunc is invoked once per patched treasure
// returned by CatalogPatchExpired. The model is a fresh instance of the
// caller's model template, populated with the post-patch state (Key,
// per-field values from the patched body, and the new ExpiredAt). The
// status is included so callers can branch on outcomes other than
// PATCHED (e.g. CONDITION_NOT_MET, TYPE_MISMATCH).
//
// Returning a non-nil error from the iterator stops further iteration
// and bubbles up as the error returned by CatalogPatchExpired.
type CatalogPatchExpiredIteratorFunc func(model any, status PatchStatus) error

// PatchExpiredOps is a builder for CatalogPatchExpired. It accumulates
// a single op-set, an optional condition, and an optional metadata
// stamp — all of which are applied to every selected expired treasure
// in the same RPC.
//
// The builder is not bound to a key: PatchExpired derives its targets
// server-side via the swamp's expiration-time index. This is the only
// reason it exists as a separate builder from PatchBuilder; everything
// else mirrors that surface.
type PatchExpiredOps struct {
	ops         []*hydraidepbgo.PatchOp
	cond        *hydraidepbgo.PatchCondition
	meta        *hydraidepbgo.PatchMeta
	cap         *Cap
	filters     *FilterGroup
	encodeError error
}

// WithCap attaches a server-enforced quota check to this patch. The
// server pre-counts records matching cap.Filter and patches at most
// (cap.MaxMatching - currentMatching) selected treasures, regardless of
// HowMany. cap == nil clears any previously set Cap.
//
// Cap is opt-in: a patch without WithCap has byte-identical behaviour to
// previous releases.
//
// Validation (rejected with ErrCodeInvalidModel at the call site):
//   - cap.MaxMatching must be > 0.
//   - cap.Filter is required when cap is non-nil.
func (b *PatchExpiredOps) WithCap(cap *Cap) *PatchExpiredOps {
	if b.encodeError != nil {
		return b
	}
	b.cap = cap
	return b
}

// WithFilters narrows the candidate set before HowMany / Cap budget
// arithmetic is applied. Records that fail the predicate are not patched
// and do not count toward HowMany or Cap. Use this to scope PatchExpired
// to a sub-population sharing the same expiration index (e.g. per-ASN,
// per-tenant claim queues).
//
// Symmetric to ShiftRequest.Filters. filters == nil clears any previously
// set Filters and reverts to "every expired treasure is a candidate".
func (b *PatchExpiredOps) WithFilters(filters *FilterGroup) *PatchExpiredOps {
	if b.encodeError != nil {
		return b
	}
	b.filters = filters
	return b
}

// NewPatchExpiredOps returns an empty op accumulator.
func NewPatchExpiredOps() *PatchExpiredOps {
	return &PatchExpiredOps{}
}

// Set appends a SET op applied to every selected treasure.
func (b *PatchExpiredOps) Set(path string, value any) *PatchExpiredOps {
	return b.appendOp(hydraidepbgo.PatchOp_SET, path, value)
}

// Inc appends an INC op. delta must be a numeric Go value matching the
// target field's msgpack class.
func (b *PatchExpiredOps) Inc(path string, delta any) *PatchExpiredOps {
	return b.appendOp(hydraidepbgo.PatchOp_INC, path, delta)
}

// Append appends an APPEND op. path must end in "[]".
func (b *PatchExpiredOps) Append(path string, value any) *PatchExpiredOps {
	return b.appendOp(hydraidepbgo.PatchOp_APPEND, path, value)
}

// Prepend appends a PREPEND op. path must end in "[]".
func (b *PatchExpiredOps) Prepend(path string, value any) *PatchExpiredOps {
	return b.appendOp(hydraidepbgo.PatchOp_PREPEND, path, value)
}

// Delete appends a DELETE op (Value is ignored on the wire).
func (b *PatchExpiredOps) Delete(path string) *PatchExpiredOps {
	return b.appendOpRaw(hydraidepbgo.PatchOp_DELETE, path, nil)
}

// RemoveAt appends a REMOVE_AT op. path must include an [index] segment.
func (b *PatchExpiredOps) RemoveAt(path string) *PatchExpiredOps {
	return b.appendOpRaw(hydraidepbgo.PatchOp_REMOVE_AT, path, nil)
}

// RemoveVal appends a REMOVE_VAL op.
func (b *PatchExpiredOps) RemoveVal(path string, value any) *PatchExpiredOps {
	return b.appendOp(hydraidepbgo.PatchOp_REMOVE_VAL, path, value)
}

// Merge appends a MERGE op.
func (b *PatchExpiredOps) Merge(path string, value any) *PatchExpiredOps {
	return b.appendOp(hydraidepbgo.PatchOp_MERGE, path, value)
}

// IfFieldEquals adds a CondEqual condition.
func (b *PatchExpiredOps) IfFieldEquals(path string, value any) *PatchExpiredOps {
	return b.setCondition(hydraidepbgo.PatchCondition_EQUAL, path, value)
}

// IfFieldNotEquals adds a CondNotEqual condition.
func (b *PatchExpiredOps) IfFieldNotEquals(path string, value any) *PatchExpiredOps {
	return b.setCondition(hydraidepbgo.PatchCondition_NOT_EQUAL, path, value)
}

// IfFieldGreaterThan adds a CondGreaterThan condition.
func (b *PatchExpiredOps) IfFieldGreaterThan(path string, value any) *PatchExpiredOps {
	return b.setCondition(hydraidepbgo.PatchCondition_GREATER_THAN, path, value)
}

// IfFieldGreaterThanOrEqual adds a CondGreaterThanOrEqual condition.
func (b *PatchExpiredOps) IfFieldGreaterThanOrEqual(path string, value any) *PatchExpiredOps {
	return b.setCondition(hydraidepbgo.PatchCondition_GREATER_THAN_OR_EQUAL, path, value)
}

// IfFieldLessThan adds a CondLessThan condition.
func (b *PatchExpiredOps) IfFieldLessThan(path string, value any) *PatchExpiredOps {
	return b.setCondition(hydraidepbgo.PatchCondition_LESS_THAN, path, value)
}

// IfFieldLessThanOrEqual adds a CondLessThanOrEqual condition.
func (b *PatchExpiredOps) IfFieldLessThanOrEqual(path string, value any) *PatchExpiredOps {
	return b.setCondition(hydraidepbgo.PatchCondition_LESS_THAN_OR_EQUAL, path, value)
}

// IfFieldExists adds a CondExists condition.
func (b *PatchExpiredOps) IfFieldExists(path string) *PatchExpiredOps {
	if b.encodeError != nil {
		return b
	}
	b.cond = &hydraidepbgo.PatchCondition{Path: path, Operator: hydraidepbgo.PatchCondition_EXISTS}
	return b
}

// IfFieldNotExists adds a CondNotExists condition.
func (b *PatchExpiredOps) IfFieldNotExists(path string) *PatchExpiredOps {
	if b.encodeError != nil {
		return b
	}
	b.cond = &hydraidepbgo.PatchCondition{Path: path, Operator: hydraidepbgo.PatchCondition_NOT_EXISTS}
	return b
}

// WithUpdatedAt stamps ModifiedAt = now on patched treasures.
func (b *PatchExpiredOps) WithUpdatedAt() *PatchExpiredOps {
	if b.meta == nil {
		b.meta = &hydraidepbgo.PatchMeta{}
	}
	b.meta.SetUpdatedAt = true
	return b
}

// WithUpdatedBy stamps ModifiedBy = userID on patched treasures.
func (b *PatchExpiredOps) WithUpdatedBy(userID string) *PatchExpiredOps {
	if b.meta == nil {
		b.meta = &hydraidepbgo.PatchMeta{}
	}
	b.meta.SetUpdatedBy = &userID
	return b
}

// WithExpiredAt stamps the new ExpiredAt on every patched treasure.
// This is the typical knob for queue-claim flows: pass a future time
// and that timestamp doubles as the worker's lease deadline and the
// recovery trigger if the worker crashes (the next caller picks up
// the entry once the lease elapses).
//
// A zero time.Time clears the existing ExpiredAt — equivalent to
// WithoutExpiredAt().
func (b *PatchExpiredOps) WithExpiredAt(expireAt time.Time) *PatchExpiredOps {
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

// WithoutExpiredAt resets ExpiredAt to "never expires" on patched
// treasures. The patched treasures will not appear in subsequent
// PatchExpired calls.
func (b *PatchExpiredOps) WithoutExpiredAt() *PatchExpiredOps {
	if b.meta == nil {
		b.meta = &hydraidepbgo.PatchMeta{}
	}
	b.meta.ClearExpiredAt = true
	b.meta.SetExpiredAt = nil
	return b
}

// CatalogPatchExpired performs the in-place patch-of-expired RPC.
//
// Selection is server-side, atomic across concurrent callers (each
// caller observes a disjoint subset of the expired treasures). The
// builder's ops + condition + meta are applied to every selected
// treasure under its per-key guard. The iterator (when non-nil) is
// invoked once per patched (or attempted-patched) treasure with a
// freshly-instantiated model populated from the post-patch state.
//
// Behaviour:
//   - howMany == 0 → all currently-expired treasures.
//   - Empty ops + nil meta is a programmer error and returns
//     ErrCodeInvalidModel.
//   - The iterator receives only entries the server returned. With
//     a non-nil condition, CONDITION_NOT_MET entries reach the
//     iterator with status PatchStatusConditionNotMet — model
//     contents reflect the unchanged state (best-effort; the body
//     is not echoed back when the condition fails).
//   - Returning a non-nil error from the iterator stops iteration
//     and bubbles up.
//
// Common use cases:
//   - Crash-safe queue claims (claim by sliding ExpiredAt forward).
//   - Bulk TTL slides on expired entries (empty ops, meta only).
//   - Periodic recheck scheduling driven by ExpiredAt.
func (h *hydraidego) CatalogPatchExpired(
	ctx context.Context,
	swampName name.Name,
	howMany int32,
	model any,
	iterator CatalogPatchExpiredIteratorFunc,
	builder *PatchExpiredOps,
) error {
	_, err := h.CatalogPatchExpiredWithResult(ctx, swampName, howMany, model, iterator, builder)
	return err
}

// PatchExpiredResult carries the request-level outcome of a Cap-bearing
// PatchExpired call. CapReached is true when the cap budget bounded the
// result count; always false when builder.WithCap was not called.
type PatchExpiredResult struct {
	CapReached bool
}

// CatalogPatchExpiredWithResult is the Cap-aware variant of
// CatalogPatchExpired. It returns the request-level *PatchExpiredResult
// in addition to the per-treasure iterator stream, so callers can
// distinguish "cap budget exhausted, back off" from "no expired records,
// nothing to do" without inspecting the iterator.
//
// Identical semantics to CatalogPatchExpired otherwise. Build the
// builder with .WithCap(*Cap) to attach a quota check; without it,
// behaviour is byte-identical to CatalogPatchExpired and CapReached is
// always false.
func (h *hydraidego) CatalogPatchExpiredWithResult(
	ctx context.Context,
	swampName name.Name,
	howMany int32,
	model any,
	iterator CatalogPatchExpiredIteratorFunc,
	builder *PatchExpiredOps,
) (*PatchExpiredResult, error) {
	if builder == nil {
		return nil, NewError(ErrCodeInvalidModel, "PatchExpiredOps builder is required")
	}
	if builder.encodeError != nil {
		return nil, NewError(ErrCodeInvalidModel, builder.encodeError.Error())
	}
	if len(builder.ops) == 0 && builder.meta == nil {
		return nil, NewError(ErrCodeInvalidModel, "CatalogPatchExpired requires at least one op or a non-nil meta")
	}

	wireCap, capErr := buildWireCap(builder.cap)
	if capErr != nil {
		return nil, capErr
	}

	wireReq := &hydraidepbgo.PatchExpiredTreasuresRequest{
		IslandID:  swampName.GetIslandID(h.client.GetAllIslands()),
		SwampName: swampName.Get(),
		HowMany:   howMany,
		Ops:       builder.ops,
		Meta:      builder.meta,
		Condition: builder.cond,
		Cap:       wireCap,
	}
	if builder.filters != nil {
		wireReq.Filters = convertFilterGroupToProto(builder.filters)
	}
	resp, err := h.client.GetServiceClient(swampName).PatchExpiredTreasures(ctx, wireReq)
	if err != nil {
		return nil, translatePatchGRPCError(err)
	}

	if iterator != nil {
		for _, entry := range resp.GetPatched() {
			modelValue := reflect.New(reflect.TypeOf(model)).Interface()
			if convErr := populateCatalogModelFromPatchedExpired(entry, modelValue); convErr != nil {
				return nil, NewError(ErrCodeInvalidModel, convErr.Error())
			}
			if iterErr := iterator(modelValue, PatchStatus(entry.GetStatus())); iterErr != nil {
				return nil, iterErr
			}
		}
	}
	return &PatchExpiredResult{CapReached: resp.GetCapReached()}, nil
}

// appendOp encodes value to msgpack and appends a typed op.
func (b *PatchExpiredOps) appendOp(kind hydraidepbgo.PatchOp_Kind, path string, value any) *PatchExpiredOps {
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
func (b *PatchExpiredOps) appendOpRaw(kind hydraidepbgo.PatchOp_Kind, path string, raw []byte) *PatchExpiredOps {
	op := &hydraidepbgo.PatchOp{Op: kind, Path: path}
	if raw != nil {
		op.Value = raw
	}
	b.ops = append(b.ops, op)
	return b
}

// setCondition encodes value to msgpack and replaces the builder's condition.
func (b *PatchExpiredOps) setCondition(opCode hydraidepbgo.PatchCondition_Op, path string, value any) *PatchExpiredOps {
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

// populateCatalogModelFromPatchedExpired populates the caller's model
// instance from the response entry. Field assignment:
//   - hydraide:"key" → entry.Key
//   - hydraide:"expireAt" → entry.ExpiredAt (when set)
//   - msgpack body keys (entry.NewMsgpack) → matching struct fields
//     by field name (msgpack default name encoding)
//
// Fields tagged hydraide:"value" are NOT supported here — Patch flows
// operate on msgpack-map bodies, not single-value catalogs. Mixing
// would silently no-op.
func populateCatalogModelFromPatchedExpired(entry *hydraidepbgo.PatchedExpiredTreasure, model any) error {
	v := reflect.ValueOf(model)
	if v.Kind() != reflect.Ptr || v.Elem().Kind() != reflect.Struct {
		return errors.New("model must be a pointer to a struct")
	}

	if body := entry.GetNewMsgpack(); len(body) > 0 {
		// When the model declares map-body fields with hydraide:"FieldName"
		// tags, decode by tag value (so the wire key follows the tag, not
		// the Go field name). For models that rely on Go-field-name matching
		// (no body tags), fall back to direct msgpack.Unmarshal — the
		// vmihailenco decoder matches by Go field name out of the box.
		shape, mapBodyFields, shapeErr := inspectCatalogModel(v.Elem().Type())
		if shapeErr == nil && shape == catalogShapeMapBody {
			if err := decodeMapBodyInto(body, v.Elem(), mapBodyFields); err != nil {
				return fmt.Errorf("decode msgpack body: %w", err)
			}
		} else if err := msgpack.Unmarshal(body, model); err != nil {
			return fmt.Errorf("decode msgpack body: %w", err)
		}
	}

	t := v.Elem().Type()
	for i := 0; i < t.NumField(); i++ {
		tag, ok := t.Field(i).Tag.Lookup(tagHydrAIDE)
		if !ok {
			continue
		}
		switch {
		case strings.Contains(tag, tagKey):
			if v.Elem().Field(i).Kind() == reflect.String {
				v.Elem().Field(i).SetString(entry.GetKey())
			}
		case strings.Contains(tag, tagExpireAt):
			if entry.ExpiredAt != nil {
				field := v.Elem().Field(i)
				if field.Type() == reflect.TypeOf(time.Time{}) {
					field.Set(reflect.ValueOf(entry.GetExpiredAt().AsTime()))
				}
			}
		}
	}
	return nil
}

// PatchExpiredManyFromManyRequest is one entry in a multi-swamp expired-
// patch batch issued via CatalogPatchExpiredManyFromMany. Each entry
// targets one swamp; the batch dispatcher groups entries by destination
// server and sends one PatchExpiredTreasuresMany RPC per server.
type PatchExpiredManyFromManyRequest struct {
	// SwampName addresses one swamp.
	SwampName name.Name

	// HowMany caps the per-swamp result count. 0 means "all currently
	// expired treasures" (matches CatalogPatchExpired and
	// ShiftExpiredTreasures).
	HowMany int32

	// Builder accumulates the ops + condition + meta applied to every
	// selected expired treasure of THIS swamp under its per-key guard.
	// Use NewPatchExpiredOps() and the same fluent API as the
	// single-swamp CatalogPatchExpired.
	Builder *PatchExpiredOps
}

// CatalogPatchExpiredManyFromManyIteratorFunc is invoked once per
// patched (or attempted-patched) treasure across all swamps. The
// swampName argument identifies which entry produced the model. Errors
// returned from the callback stop iteration and bubble up as the result
// of the call.
//
// SwampErr is non-nil for swamp-level failures (missing Ops, summon
// failure). When SwampErr is non-nil the callback fires once for that
// swamp with model == nil and status == PatchStatusInternalError, and
// no per-treasure callbacks follow for it.
type CatalogPatchExpiredManyFromManyIteratorFunc func(swampName name.Name, model any, status PatchStatus, swampErr error) error

// CatalogPatchExpiredManyFromMany dispatches a multi-swamp expired-patch
// batch. Each request claims up to its HowMany expired treasures from
// its swamp under the swamp's beacon mu, applies the builder's ops +
// condition + meta to each one under its per-key guard, and returns the
// per-treasure outcomes to the iterator.
//
// Behaviour:
//   - Requests are grouped by destination server (consistent hashing on
//     SwampName); one PatchExpiredTreasuresMany RPC is sent per server.
//   - Per-swamp failures (missing Ops/Meta, summon failure) are surfaced
//     to the iterator via the swampErr argument; the rest of the batch
//     continues unaffected.
//   - Per-treasure failures (CONDITION_NOT_MET, TYPE_MISMATCH, etc.) are
//     surfaced to the iterator via the status argument, same as the
//     single-swamp CatalogPatchExpired.
//   - Empty requests slice is a valid no-op.
//
// Common use cases:
//   - Combined queue claim across multiple ready swamps in a single RPC
//     (e.g. crawler that pulls 80% of work from a direct queue and 20%
//     from a proxy queue).
//   - Periodic recheck scheduling across TLD-sharded state swamps.
func (h *hydraidego) CatalogPatchExpiredManyFromMany(
	ctx context.Context,
	requests []*PatchExpiredManyFromManyRequest,
	model any,
	iterator CatalogPatchExpiredManyFromManyIteratorFunc,
) error {
	_, err := h.CatalogPatchExpiredManyFromManyWithResults(ctx, requests, model, iterator)
	return err
}

// PatchExpiredManyFromManyResult is the per-swamp outcome of a
// CatalogPatchExpiredManyFromManyWithResults call. CapReached mirrors
// the Cap-bearing single-RPC behaviour; SwampErr is non-nil for swamp-
// level failures (already surfaced to the iterator via swampErr, repeated
// here for callers that prefer the results-slice shape).
type PatchExpiredManyFromManyResult struct {
	SwampName  name.Name
	CapReached bool
	SwampErr   error
}

// CatalogPatchExpiredManyFromManyWithResults is the Cap-aware variant of
// CatalogPatchExpiredManyFromMany. It returns one
// PatchExpiredManyFromManyResult per input request (in input order)
// alongside the per-treasure iterator stream, so callers can act on
// per-swamp CapReached signals (e.g. back off on swamps with exhausted
// budgets, continue claiming on others).
//
// builder.WithCap on individual requests enables per-swamp quota
// enforcement; without it CapReached stays false and the behaviour is
// byte-identical to CatalogPatchExpiredManyFromMany.
func (h *hydraidego) CatalogPatchExpiredManyFromManyWithResults(
	ctx context.Context,
	requests []*PatchExpiredManyFromManyRequest,
	model any,
	iterator CatalogPatchExpiredManyFromManyIteratorFunc,
) ([]*PatchExpiredManyFromManyResult, error) {
	if len(requests) == 0 {
		return nil, nil
	}

	results := make([]*PatchExpiredManyFromManyResult, len(requests))
	// Validate every request up-front so a single bad builder cannot
	// silently corrupt a server-grouped batch.
	wireRequests := make([]*hydraidepbgo.PatchExpiredTreasuresRequest, len(requests))
	for i, req := range requests {
		results[i] = &PatchExpiredManyFromManyResult{}
		if req == nil {
			return nil, NewError(ErrCodeInvalidModel, fmt.Sprintf("request %d is nil", i))
		}
		results[i].SwampName = req.SwampName
		if req.Builder == nil {
			return nil, NewError(ErrCodeInvalidModel, fmt.Sprintf("request %d: Builder is required", i))
		}
		if req.Builder.encodeError != nil {
			return nil, NewError(ErrCodeInvalidModel, fmt.Sprintf("request %d: %v", i, req.Builder.encodeError))
		}
		if len(req.Builder.ops) == 0 && req.Builder.meta == nil {
			return nil, NewError(ErrCodeInvalidModel, fmt.Sprintf("request %d: builder needs at least one op or non-nil meta", i))
		}
		wireCap, capErr := buildWireCap(req.Builder.cap)
		if capErr != nil {
			return nil, capErr
		}
		wireRequests[i] = &hydraidepbgo.PatchExpiredTreasuresRequest{
			IslandID:  req.SwampName.GetIslandID(h.client.GetAllIslands()),
			SwampName: req.SwampName.Get(),
			HowMany:   req.HowMany,
			Ops:       req.Builder.ops,
			Meta:      req.Builder.meta,
			Condition: req.Builder.cond,
			Cap:       wireCap,
		}
		if req.Builder.filters != nil {
			wireRequests[i].Filters = convertFilterGroupToProto(req.Builder.filters)
		}
	}

	// Group request indices by destination server so we can issue one
	// PatchExpiredTreasuresMany RPC per server, then merge responses
	// back to the input order.
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
		batch := make([]*hydraidepbgo.PatchExpiredTreasuresRequest, 0, len(g.indices))
		for _, idx := range g.indices {
			batch = append(batch, wireRequests[idx])
		}
		resp, err := g.client.PatchExpiredTreasuresMany(ctx, &hydraidepbgo.PatchExpiredTreasuresManyRequest{
			Requests: batch,
		})
		if err != nil {
			return results, translatePatchGRPCError(err)
		}

		entries := resp.GetResponses()
		if len(entries) != len(g.indices) {
			return results, NewError(ErrCodeInternalDatabaseError, fmt.Sprintf("PatchExpiredTreasuresMany: server returned %d entries for %d requests", len(entries), len(g.indices)))
		}

		for k, idx := range g.indices {
			swampName := requests[idx].SwampName
			entry := entries[k]
			results[idx].CapReached = entry.GetCapReached()
			if errMsg := entry.GetError(); errMsg != "" {
				results[idx].SwampErr = NewError(ErrCodeInternalDatabaseError, errMsg)
				if iterator != nil {
					if iterErr := iterator(swampName, nil, PatchStatusInternalError, results[idx].SwampErr); iterErr != nil {
						return results, iterErr
					}
				}
				continue
			}
			if iterator == nil {
				continue
			}
			for _, treasure := range entry.GetPatched() {
				modelValue := reflect.New(reflect.TypeOf(model)).Interface()
				if convErr := populateCatalogModelFromPatchedExpired(treasure, modelValue); convErr != nil {
					return results, NewError(ErrCodeInvalidModel, convErr.Error())
				}
				if iterErr := iterator(swampName, modelValue, PatchStatus(treasure.GetStatus()), nil); iterErr != nil {
					return results, iterErr
				}
			}
		}
	}
	return results, nil
}
