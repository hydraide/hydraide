package msgpackpatch

import (
	"fmt"
)

// applyAppend / applyPrepend insert op.Value as a new element into the array
// at path. Path must end in SegAppend ("Tags[]") OR in a SegField that targets
// a missing array (auto-create). Intermediate maps are auto-created as needed.
//
// Both ops share the same skeleton walk; only the insertion position differs.
func applyAppend(skel *Skeleton, op Op, path *Path, prepend bool) error {
	if len(op.Value) == 0 {
		return fmt.Errorf("%w: APPEND/PREPEND requires Value", ErrInvalidOp)
	}

	cur, err := path.Resolve(skel)
	if err != nil {
		return err
	}

	finalSeg := path.Segments[len(path.Segments)-1]

	// Case 1: existing array reachable via SegAppend on existing parent.
	if finalSeg.Kind == SegAppend && cur.Parent.Kind == KindArray && cur.MissingAt < 0 {
		insertIntoArray(cur.Parent, op.Value, prepend)
		return nil
	}

	// Case 2: existing field targets an array (path = "Tags[]"). The Resolver
	// already verified Parent==array; nothing more to check.
	if finalSeg.Kind == SegAppend && cur.Parent.Kind == KindArray {
		insertIntoArray(cur.Parent, op.Value, prepend)
		return nil
	}

	// Case 3: missing path — must end in SegAppend, otherwise the caller used
	// a syntax that doesn't make sense for APPEND.
	if finalSeg.Kind != SegAppend {
		return fmt.Errorf("%w: APPEND path must end with [] marker", ErrPathInvalid)
	}

	// Walk and create any missing intermediate maps. The final segment is
	// SegAppend, and the segment immediately before it is the field that
	// holds the (to-be-created) array.
	parent := cur.Parent
	if cur.MissingAt < 0 {
		// The resolver returned a SegAppend cursor; if Parent is not an array
		// here, it's a type mismatch (existing non-array field).
		return fmt.Errorf("%w: APPEND target is not an array", ErrTypeMismatch)
	}

	// path.Segments[len-1] is SegAppend, len-2 must be the array field name.
	if len(path.Segments) < 2 {
		return fmt.Errorf("%w: APPEND path must include a field before []", ErrPathInvalid)
	}
	arrFieldIdx := len(path.Segments) - 2
	for i := cur.MissingAt; i < arrFieldIdx; i++ {
		seg := path.Segments[i]
		if seg.Kind != SegField {
			return fmt.Errorf("%w: cannot auto-create non-field segment", ErrPathInvalid)
		}
		newMap := &Skeleton{Kind: KindMap}
		parent.MapFields = append(parent.MapFields, MapField{Key: seg.Field, Value: newMap})
		parent = newMap
	}
	arrSeg := path.Segments[arrFieldIdx]
	if arrSeg.Kind != SegField {
		return fmt.Errorf("%w: APPEND array slot must be a field", ErrPathInvalid)
	}
	if parent.Kind != KindMap {
		return fmt.Errorf("%w: APPEND parent is not a map", ErrTypeMismatch)
	}
	// If the field already exists, it must be an array.
	if idx := findField(parent, arrSeg.Field); idx >= 0 {
		existing := parent.MapFields[idx].Value
		if existing.Kind != KindArray {
			return fmt.Errorf("%w: APPEND target is not an array", ErrTypeMismatch)
		}
		insertIntoArray(existing, op.Value, prepend)
		return nil
	}
	newArr := &Skeleton{Kind: KindArray, ArrayItems: []*Skeleton{newLeaf(append([]byte(nil), op.Value...))}}
	parent.MapFields = append(parent.MapFields, MapField{Key: arrSeg.Field, Value: newArr})
	return nil
}

func insertIntoArray(arr *Skeleton, raw []byte, prepend bool) {
	leaf := newLeaf(append([]byte(nil), raw...))
	if prepend {
		arr.ArrayItems = append([]*Skeleton{leaf}, arr.ArrayItems...)
		return
	}
	arr.ArrayItems = append(arr.ArrayItems, leaf)
}
