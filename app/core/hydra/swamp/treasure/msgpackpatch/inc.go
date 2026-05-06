package msgpackpatch

import (
	"fmt"
)

// applyInc adds the delta encoded in op.Value to a numeric leaf at the path,
// preserving the target's msgpack type code (int8 stays int8, float32 stays
// float32). Missing intermediates are auto-created as empty maps; a missing
// final field is created with the delta's class and code.
//
// Crossing numeric classes (int <-> uint <-> float) is rejected as a type
// mismatch — clients must send a delta whose class matches the field.
func applyInc(skel *Skeleton, orig []byte, op Op, path *Path) error {
	if len(op.Value) == 0 {
		return fmt.Errorf("%w: INC requires Value", ErrInvalidOp)
	}
	di, du, df, deltaClass, err := readNumericLeaf(op.Value)
	if err != nil {
		return err
	}
	if deltaClass == classNone {
		return fmt.Errorf("%w: INC delta is not numeric", ErrTypeMismatch)
	}

	cur, err := path.Resolve(skel)
	if err != nil {
		return err
	}

	// Case 1: target exists — must be a numeric leaf of matching class.
	if cur.Target != nil {
		if cur.Target.Kind != KindLeaf {
			return fmt.Errorf("%w: INC target is a container", ErrTypeMismatch)
		}
		raw := leafBytes(cur.Target, orig)
		ti, tu, tf, targetClass, err := readNumericLeaf(raw)
		if err != nil {
			return err
		}
		if targetClass != deltaClass {
			return fmt.Errorf("%w: INC class mismatch (target=%d delta=%d)",
				ErrTypeMismatch, targetClass, deltaClass)
		}
		newRaw, err := computeIncBytes(cur.Target.LeafCode, targetClass, ti, tu, tf, di, du, df)
		if err != nil {
			return err
		}
		replaceWithLeaf(cur.Target, newRaw)
		return nil
	}

	// Case 2 / 3: target missing — auto-create using the delta's class+code.
	finalSeg := path.Segments[len(path.Segments)-1]
	if finalSeg.Kind != SegField {
		return fmt.Errorf("%w: INC final segment must be a field name", ErrPathInvalid)
	}

	parent := cur.Parent
	if cur.MissingAt < len(path.Segments)-1 {
		// Auto-create intermediate map chain.
		for i := cur.MissingAt; i < len(path.Segments)-1; i++ {
			seg := path.Segments[i]
			if seg.Kind != SegField {
				return fmt.Errorf("%w: cannot auto-create non-field segment", ErrPathInvalid)
			}
			newMap := &Skeleton{Kind: KindMap}
			parent.MapFields = append(parent.MapFields, MapField{Key: seg.Field, Value: newMap})
			parent = newMap
		}
	} else if parent.Kind != KindMap {
		return fmt.Errorf("%w: INC parent is not a map", ErrTypeMismatch)
	}
	// Use the delta value verbatim as the seed; it already carries the right
	// type code, so no re-encode is necessary.
	parent.MapFields = append(parent.MapFields, MapField{
		Key:   finalSeg.Field,
		Value: newLeaf(append([]byte(nil), op.Value...)),
	})
	return nil
}

// computeIncBytes adds the delta to the target value within the given class
// and returns the re-encoded leaf bytes preserving the original type code.
func computeIncBytes(code byte, class numericClass, ti int64, tu uint64, tf float64, di int64, du uint64, df float64) ([]byte, error) {
	switch class {
	case classInt:
		return encodeIntWithCode(code, ti+di)
	case classUint:
		return encodeUintWithCode(code, tu+du)
	case classFloat:
		return encodeFloatWithCode(code, tf+df)
	}
	return nil, fmt.Errorf("%w: unknown numeric class", ErrTypeMismatch)
}
