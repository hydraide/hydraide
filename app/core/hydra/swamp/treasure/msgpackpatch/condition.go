package msgpackpatch

import (
	"bytes"
	"errors"
	"fmt"

	"github.com/vmihailenco/msgpack/v5"
)

// CondOp identifies a Condition comparator.
type CondOp uint8

const (
	CondEqual CondOp = iota
	CondNotEqual
	CondGreaterThan
	CondGreaterThanOrEqual
	CondLessThan
	CondLessThanOrEqual
	CondExists
	CondNotExists
)

// Condition is a pre-condition evaluated against the current blob before any
// op is applied. If the condition does not hold, ErrConditionNotMet is
// returned and the blob remains untouched.
//
// Threshold is a pre-encoded msgpack value; it is ignored for Exists /
// NotExists. For string/byte fields, comparisons are byte-wise lexicographic.
// For numeric fields, comparisons are class-aware (int / uint / float).
type Condition struct {
	Path      string
	Op        CondOp
	Threshold []byte
}

// ErrConditionNotMet signals a Condition that evaluated to false. This is a
// distinct sentinel so the gateway can map it to CONDITION_NOT_MET status
// without confusing it with a structural error.
var ErrConditionNotMet = errors.New("msgpackpatch: condition not met")

// ApplyWithCondition is Apply plus an optional pre-condition evaluated before
// any op runs. If cond is nil, it behaves exactly like Apply.
func ApplyWithCondition(blob []byte, ops []Op, cond *Condition) ([]byte, error) {
	skel, err := Parse(blob)
	if err != nil {
		return nil, err
	}
	if cond != nil {
		if err := evaluateCondition(skel, blob, cond); err != nil {
			return nil, err
		}
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

func evaluateCondition(skel *Skeleton, orig []byte, cond *Condition) error {
	path, err := ParsePath(cond.Path)
	if err != nil {
		return fmt.Errorf("condition: %w", err)
	}
	cur, err := path.Resolve(skel)
	// ErrPathInvalid (e.g. out-of-range index) propagates; missing fields
	// surface via Cursor{Target=nil}.
	if err != nil && !errors.Is(err, ErrTypeMismatch) {
		return fmt.Errorf("condition: %w", err)
	}

	exists := err == nil && cur != nil && cur.Target != nil && cur.Target.Kind == KindLeaf

	switch cond.Op {
	case CondExists:
		if !exists {
			return ErrConditionNotMet
		}
		return nil
	case CondNotExists:
		if exists {
			return ErrConditionNotMet
		}
		return nil
	}

	// Comparators all require an existing leaf target.
	if err != nil {
		return fmt.Errorf("condition: %w", err)
	}
	if !exists {
		return ErrConditionNotMet
	}

	raw := leafBytes(cur.Target, orig)
	cmp, err := compareLeafBytes(raw, cond.Threshold)
	if err != nil {
		return fmt.Errorf("condition: %w", err)
	}

	met := false
	switch cond.Op {
	case CondEqual:
		met = cmp == 0
	case CondNotEqual:
		met = cmp != 0
	case CondGreaterThan:
		met = cmp > 0
	case CondGreaterThanOrEqual:
		met = cmp >= 0
	case CondLessThan:
		met = cmp < 0
	case CondLessThanOrEqual:
		met = cmp <= 0
	default:
		return fmt.Errorf("%w: unknown condition op %d", ErrInvalidOp, cond.Op)
	}
	if !met {
		return ErrConditionNotMet
	}
	return nil
}

// compareLeafBytes returns -1 / 0 / 1 for a < b, a == b, a > b. Numeric
// comparisons are class-aware; string / bytes use byte-wise comparison; bool
// uses canonical false<true ordering. Cross-class comparisons (numeric vs
// non-numeric, or int vs float) return ErrTypeMismatch.
func compareLeafBytes(a, b []byte) (int, error) {
	if len(a) == 0 || len(b) == 0 {
		return 0, fmt.Errorf("%w: empty operand", ErrInvalidMsgpack)
	}
	ai, au, af, ac, err := readNumericLeaf(a)
	if err != nil {
		return 0, err
	}
	bi, bu, bf, bc, err := readNumericLeaf(b)
	if err != nil {
		return 0, err
	}
	if ac != classNone || bc != classNone {
		if ac != bc {
			return 0, fmt.Errorf("%w: numeric class mismatch", ErrTypeMismatch)
		}
		switch ac {
		case classInt:
			return cmpInt64(ai, bi), nil
		case classUint:
			return cmpUint64(au, bu), nil
		case classFloat:
			return cmpFloat64(af, bf), nil
		}
	}

	// Non-numeric: try string/bytes/bool via msgpack.Unmarshal.
	var av, bv any
	if err := msgpack.Unmarshal(a, &av); err != nil {
		return 0, fmt.Errorf("%w: %v", ErrInvalidMsgpack, err)
	}
	if err := msgpack.Unmarshal(b, &bv); err != nil {
		return 0, fmt.Errorf("%w: %v", ErrInvalidMsgpack, err)
	}
	switch x := av.(type) {
	case string:
		y, ok := bv.(string)
		if !ok {
			return 0, fmt.Errorf("%w: string vs %T", ErrTypeMismatch, bv)
		}
		return bytes.Compare([]byte(x), []byte(y)), nil
	case []byte:
		y, ok := bv.([]byte)
		if !ok {
			return 0, fmt.Errorf("%w: bytes vs %T", ErrTypeMismatch, bv)
		}
		return bytes.Compare(x, y), nil
	case bool:
		y, ok := bv.(bool)
		if !ok {
			return 0, fmt.Errorf("%w: bool vs %T", ErrTypeMismatch, bv)
		}
		return cmpBool(x, y), nil
	}
	// Fallback for nil / containers: byte-wise comparison only meaningful for
	// equality. Containers are intentionally unsupported by comparators.
	if bytes.Equal(a, b) {
		return 0, nil
	}
	return 0, fmt.Errorf("%w: unsupported leaf type for comparison", ErrTypeMismatch)
}

func cmpInt64(a, b int64) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

func cmpUint64(a, b uint64) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

func cmpFloat64(a, b float64) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

func cmpBool(a, b bool) int {
	switch {
	case a == b:
		return 0
	case !a && b:
		return -1
	}
	return 1
}
