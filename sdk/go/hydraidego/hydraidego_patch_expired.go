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
	encodeError error
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
	if builder == nil {
		return NewError(ErrCodeInvalidModel, "PatchExpiredOps builder is required")
	}
	if builder.encodeError != nil {
		return NewError(ErrCodeInvalidModel, builder.encodeError.Error())
	}
	if len(builder.ops) == 0 && builder.meta == nil {
		return NewError(ErrCodeInvalidModel, "CatalogPatchExpired requires at least one op or a non-nil meta")
	}

	resp, err := h.client.GetServiceClient(swampName).PatchExpiredTreasures(ctx, &hydraidepbgo.PatchExpiredTreasuresRequest{
		IslandID:  swampName.GetIslandID(h.client.GetAllIslands()),
		SwampName: swampName.Get(),
		HowMany:   howMany,
		Ops:       builder.ops,
		Meta:      builder.meta,
		Condition: builder.cond,
	})
	if err != nil {
		return translatePatchGRPCError(err)
	}

	if iterator == nil {
		return nil
	}

	for _, entry := range resp.GetPatched() {
		modelValue := reflect.New(reflect.TypeOf(model)).Interface()
		if convErr := populateCatalogModelFromPatchedExpired(entry, modelValue); convErr != nil {
			return NewError(ErrCodeInvalidModel, convErr.Error())
		}
		if iterErr := iterator(modelValue, PatchStatus(entry.GetStatus())); iterErr != nil {
			return iterErr
		}
	}
	return nil
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
		if err := msgpack.Unmarshal(body, model); err != nil {
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
