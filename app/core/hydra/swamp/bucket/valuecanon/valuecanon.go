// Package valuecanon provides canonical normalization for values extracted
// from msgpack-decoded Treasure bodies. Bucket indexes and filter comparisons
// must agree on equality byte-for-byte; this package is the single source of
// truth for that decision.
//
// The canonical Key collapses Go's numeric tower into three numeric kinds
// (int64, uint64, float64) plus string, bool, and a null sentinel. Equality
// across kinds follows the same promotion rules as the filter package's
// compareOrdered: an int64 and a uint64 compare equal when the int64 is
// non-negative and the magnitudes match; an int64 and a float64 compare
// equal only when the conversion is lossless.
package valuecanon

import (
	"time"
)

// Kind is the canonical type tag for a normalized value.
type Kind uint8

const (
	KindNull Kind = iota
	KindBool
	KindInt64
	KindUint64
	KindFloat64
	KindString
)

// Key is the canonical representation of a value. Only the field matching
// Kind is meaningful; the others are zero. Keys are comparable structs and
// can be used directly as Go map keys.
type Key struct {
	Kind Kind
	I    int64
	U    uint64
	F    float64
	S    string
	B    bool
}

// NullKey is the sentinel for a missing field or an explicit nil value.
var NullKey = Key{Kind: KindNull}

// Canonicalize takes a value extracted from a msgpack-decoded body and
// returns the canonical Key. Supported inputs:
//
//   - nil                → KindNull
//   - bool               → KindBool
//   - int8..int64        → KindInt64
//   - uint8..uint64      → KindUint64 (uint64 stays uint64 even when it
//                          fits in int64; cross-kind equality handles the
//                          overlap via Equal)
//   - float32/float64    → KindFloat64
//   - string             → KindString
//   - time.Time          → KindInt64 (Unix seconds, UTC)
//
// Unsupported types collapse to KindNull. This matches the filter package's
// behavior, where a non-comparable value never matches.
func Canonicalize(v any) Key {
	if v == nil {
		return NullKey
	}
	switch n := v.(type) {
	case bool:
		return Key{Kind: KindBool, B: n}
	case int8:
		return Key{Kind: KindInt64, I: int64(n)}
	case int16:
		return Key{Kind: KindInt64, I: int64(n)}
	case int32:
		return Key{Kind: KindInt64, I: int64(n)}
	case int64:
		return Key{Kind: KindInt64, I: n}
	case int:
		return Key{Kind: KindInt64, I: int64(n)}
	case uint8:
		return Key{Kind: KindUint64, U: uint64(n)}
	case uint16:
		return Key{Kind: KindUint64, U: uint64(n)}
	case uint32:
		return Key{Kind: KindUint64, U: uint64(n)}
	case uint64:
		return Key{Kind: KindUint64, U: n}
	case uint:
		return Key{Kind: KindUint64, U: uint64(n)}
	case float32:
		return Key{Kind: KindFloat64, F: float64(n)}
	case float64:
		return Key{Kind: KindFloat64, F: n}
	case string:
		return Key{Kind: KindString, S: n}
	case time.Time:
		return Key{Kind: KindInt64, I: n.UTC().Unix()}
	default:
		return NullKey
	}
}

// Equal returns true if two canonical keys compare equal under the filter
// semantics. Same-kind comparison is direct equality. Cross-kind numeric
// comparison promotes through the widest representation that loses no
// information; string vs numeric is never equal; null is only equal to null.
func Equal(a, b Key) bool {
	if a.Kind == b.Kind {
		switch a.Kind {
		case KindNull:
			return true
		case KindBool:
			return a.B == b.B
		case KindInt64:
			return a.I == b.I
		case KindUint64:
			return a.U == b.U
		case KindFloat64:
			return a.F == b.F
		case KindString:
			return a.S == b.S
		}
		return false
	}

	// Cross-kind numeric promotion. Anything involving a non-numeric kind
	// returns false here.
	if !isNumeric(a.Kind) || !isNumeric(b.Kind) {
		return false
	}

	// If either side is a float, promote both to float64 and require the
	// other side to convert losslessly.
	if a.Kind == KindFloat64 || b.Kind == KindFloat64 {
		af, ok := toFloat64Lossless(a)
		if !ok {
			return false
		}
		bf, ok := toFloat64Lossless(b)
		if !ok {
			return false
		}
		return af == bf
	}

	// Remaining mixed case: int64 vs uint64. Equal iff the int64 is
	// non-negative and the magnitudes match.
	if a.Kind == KindInt64 && b.Kind == KindUint64 {
		return a.I >= 0 && uint64(a.I) == b.U
	}
	if a.Kind == KindUint64 && b.Kind == KindInt64 {
		return b.I >= 0 && uint64(b.I) == a.U
	}
	return false
}

func isNumeric(k Kind) bool {
	return k == KindInt64 || k == KindUint64 || k == KindFloat64
}

// toFloat64Lossless converts a numeric Key to float64. Returns false when
// the conversion would lose precision (large int/uint magnitudes outside
// the 2^53 mantissa range).
func toFloat64Lossless(k Key) (float64, bool) {
	switch k.Kind {
	case KindFloat64:
		return k.F, true
	case KindInt64:
		f := float64(k.I)
		if int64(f) != k.I {
			return 0, false
		}
		return f, true
	case KindUint64:
		f := float64(k.U)
		if uint64(f) != k.U {
			return 0, false
		}
		return f, true
	default:
		return 0, false
	}
}

// IsNull reports whether k is the null sentinel.
func (k Key) IsNull() bool { return k.Kind == KindNull }
