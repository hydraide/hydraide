package msgpackpatch

import (
	"bytes"
	"fmt"
)

// applyRemoveAt removes the array element at a fixed index. The path's final
// segment must be SegIndex (e.g. "Tags[3]"). Out-of-range indices are reported
// as ErrPathInvalid (not silently ignored).
func applyRemoveAt(skel *Skeleton, path *Path) error {
	finalSeg := path.Segments[len(path.Segments)-1]
	if finalSeg.Kind != SegIndex {
		return fmt.Errorf("%w: REMOVE_AT requires an [index] in the path", ErrPathInvalid)
	}
	cur, err := path.Resolve(skel)
	if err != nil {
		return err
	}
	if cur.Target == nil {
		return fmt.Errorf("%w: REMOVE_AT target missing", ErrPathInvalid)
	}
	idx := cur.TargetIdx
	cur.Parent.ArrayItems = append(cur.Parent.ArrayItems[:idx], cur.Parent.ArrayItems[idx+1:]...)
	return nil
}

// applyRemoveVal removes the first array element whose msgpack-encoded bytes
// equal op.Value. Path must point to an array. Missing field / value-not-found
// are silent no-ops.
func applyRemoveVal(skel *Skeleton, orig []byte, op Op, path *Path) error {
	if len(op.Value) == 0 {
		return fmt.Errorf("%w: REMOVE_VAL requires Value", ErrInvalidOp)
	}
	cur, err := path.Resolve(skel)
	if err != nil {
		return err
	}
	if cur.Target == nil {
		// Missing field — nothing to remove.
		return nil
	}
	if cur.Target.Kind != KindArray {
		return fmt.Errorf("%w: REMOVE_VAL target is not an array", ErrTypeMismatch)
	}
	for i, item := range cur.Target.ArrayItems {
		if item.Kind != KindLeaf {
			continue
		}
		raw := leafBytes(item, orig)
		if bytes.Equal(raw, op.Value) {
			cur.Target.ArrayItems = append(cur.Target.ArrayItems[:i], cur.Target.ArrayItems[i+1:]...)
			return nil
		}
	}
	return nil
}
