package msgpackpatch

import (
	"errors"
	"fmt"
)

// OpKind identifies a mutation operation.
type OpKind uint8

const (
	OpSet OpKind = iota
	OpDelete
	OpInc
	OpAppend
	OpPrepend
	OpRemoveAt
	OpRemoveVal
	OpMerge
)

// Op is a single mutation step.
//
// Value carries a pre-encoded msgpack blob holding the value to splice in.
// SET requires Value. DELETE ignores Value.
type Op struct {
	Kind  OpKind
	Path  string
	Value []byte
}

// ErrInvalidOp indicates a structurally invalid op (e.g. SET without Value).
var ErrInvalidOp = errors.New("msgpackpatch: invalid op")

// Apply applies ops in order to blob and returns a new msgpack blob with all
// mutations applied. blob is treated read-only.
//
// Atomicity: if any op fails, the original blob is returned untouched (the
// caller receives only an error and never a partially-mutated result). Ops
// applied so far are discarded.
//
// With zero ops, Apply is a no-op clone (round-trip through parse+serialize).
func Apply(blob []byte, ops []Op) ([]byte, error) {
	skel, err := Parse(blob)
	if err != nil {
		return nil, err
	}
	for i, op := range ops {
		path, err := ParsePath(op.Path)
		if err != nil {
			return nil, fmt.Errorf("op %d (%s): %w", i, opName(op.Kind), err)
		}
		if err := applyOp(skel, blob, op, path); err != nil {
			return nil, fmt.Errorf("op %d (%s): %w", i, opName(op.Kind), err)
		}
	}
	return skel.Serialize(blob)
}

func applyOp(skel *Skeleton, orig []byte, op Op, path *Path) error {
	switch op.Kind {
	case OpSet:
		return applySet(skel, op, path)
	case OpDelete:
		return applyDelete(skel, path)
	case OpInc:
		return applyInc(skel, orig, op, path)
	case OpAppend:
		return applyAppend(skel, op, path, false)
	case OpPrepend:
		return applyAppend(skel, op, path, true)
	case OpRemoveAt:
		return applyRemoveAt(skel, path)
	case OpRemoveVal:
		return applyRemoveVal(skel, orig, op, path)
	case OpMerge:
		return applyMerge(skel, op, path)
	default:
		return fmt.Errorf("%w: unknown kind %d", ErrInvalidOp, op.Kind)
	}
}

func opName(k OpKind) string {
	switch k {
	case OpSet:
		return "SET"
	case OpDelete:
		return "DELETE"
	case OpInc:
		return "INC"
	case OpAppend:
		return "APPEND"
	case OpPrepend:
		return "PREPEND"
	case OpRemoveAt:
		return "REMOVE_AT"
	case OpRemoveVal:
		return "REMOVE_VAL"
	case OpMerge:
		return "MERGE"
	}
	return fmt.Sprintf("OpKind(%d)", k)
}

// applySet implements SET semantics:
//   - Existing target: replace wholesale with op.Value bytes.
//   - Missing target with existing parent: insert.
//   - Missing intermediate path segments: auto-create empty maps.
//
// The final segment must be SegField. SegIndex on a missing target is
// invalid (no sparse arrays); SegAppend belongs to the APPEND op.
func applySet(skel *Skeleton, op Op, path *Path) error {
	if len(op.Value) == 0 {
		return fmt.Errorf("%w: SET requires Value", ErrInvalidOp)
	}

	cur, err := path.Resolve(skel)
	if err != nil {
		return err
	}

	// Case 1: Target exists — replace wholesale.
	if cur.Target != nil {
		replaceWithLeaf(cur.Target, op.Value)
		return nil
	}

	// Final-segment kind constrains what we can do for missing targets.
	finalSeg := path.Segments[len(path.Segments)-1]
	if finalSeg.Kind != SegField {
		return fmt.Errorf("%w: SET final segment must be a field name", ErrPathInvalid)
	}

	// Case 2: Final field missing under existing parent (no intermediates).
	if cur.MissingAt == len(path.Segments)-1 {
		if cur.Parent.Kind != KindMap {
			return fmt.Errorf("%w: SET parent is not a map", ErrTypeMismatch)
		}
		cur.Parent.MapFields = append(cur.Parent.MapFields, MapField{
			Key:   finalSeg.Field,
			Value: newLeaf(op.Value),
		})
		return nil
	}

	// Case 3: Intermediate missing — auto-create empty-map chain.
	parent := cur.Parent
	for i := cur.MissingAt; i < len(path.Segments)-1; i++ {
		seg := path.Segments[i]
		if seg.Kind != SegField {
			return fmt.Errorf("%w: cannot auto-create non-field segment", ErrPathInvalid)
		}
		newMap := &Skeleton{Kind: KindMap}
		parent.MapFields = append(parent.MapFields, MapField{Key: seg.Field, Value: newMap})
		parent = newMap
	}
	parent.MapFields = append(parent.MapFields, MapField{
		Key:   finalSeg.Field,
		Value: newLeaf(op.Value),
	})
	return nil
}

// applyDelete removes the target if it exists; missing target is a no-op.
func applyDelete(skel *Skeleton, path *Path) error {
	cur, err := path.Resolve(skel)
	if err != nil {
		return err
	}
	if cur.Target == nil {
		return nil
	}
	switch cur.Final.Kind {
	case SegField:
		idx := cur.TargetIdx
		cur.Parent.MapFields = append(cur.Parent.MapFields[:idx], cur.Parent.MapFields[idx+1:]...)
	case SegIndex:
		idx := cur.TargetIdx
		cur.Parent.ArrayItems = append(cur.Parent.ArrayItems[:idx], cur.Parent.ArrayItems[idx+1:]...)
	case SegAppend:
		return fmt.Errorf("%w: DELETE cannot target append marker", ErrPathInvalid)
	}
	return nil
}

func newLeaf(raw []byte) *Skeleton {
	return &Skeleton{Kind: KindLeaf, RawBytes: raw, LeafCode: raw[0]}
}

func replaceWithLeaf(target *Skeleton, raw []byte) {
	target.Kind = KindLeaf
	target.RawBytes = raw
	target.LeafCode = raw[0]
	target.MapFields = nil
	target.ArrayItems = nil
	// LeafStart/LeafEnd become irrelevant once RawBytes is set.
}
