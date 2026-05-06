package msgpackpatch

import (
	"bytes"
	"fmt"

	"github.com/vmihailenco/msgpack/v5"
)

// numericClass categorizes a msgpack numeric leaf for INC purposes. Mixing
// classes (e.g. float delta on an int target) is rejected as a type mismatch.
type numericClass uint8

const (
	classNone numericClass = iota
	classInt
	classUint
	classFloat
)

// classifyNumericCode returns the numeric class implied by a leading msgpack
// type code. Positive-fixint encodes a small unsigned int but Go decodes it
// uniformly via DecodeInt64 / DecodeUint64; we treat it as uint here so that
// INC on a positive-fixint target keeps the smallest sane encoding.
func classifyNumericCode(c byte) numericClass {
	if isFloatCode(c) {
		return classFloat
	}
	switch c {
	case codeInt8, codeInt16, codeInt32, codeInt64:
		return classInt
	case codeUint8, codeUint16, codeUint32, codeUint64:
		return classUint
	}
	if isPositiveFix(c) {
		return classUint
	}
	if isNegativeFix(c) {
		return classInt
	}
	return classNone
}

// readNumericLeaf decodes a single numeric msgpack value into either an int64,
// uint64, or float64 slot, depending on the leading code's class.
//
// The returned class indicates which slot is meaningful. classNone means the
// blob is not numeric.
func readNumericLeaf(raw []byte) (i int64, u uint64, f float64, class numericClass, err error) {
	if len(raw) == 0 {
		return 0, 0, 0, classNone, fmt.Errorf("%w: empty numeric leaf", ErrInvalidMsgpack)
	}
	class = classifyNumericCode(raw[0])
	if class == classNone {
		return 0, 0, 0, classNone, nil
	}
	dec := msgpack.NewDecoder(bytes.NewReader(raw))
	switch class {
	case classInt:
		i, err = dec.DecodeInt64()
	case classUint:
		u, err = dec.DecodeUint64()
	case classFloat:
		f, err = dec.DecodeFloat64()
	}
	if err != nil {
		return 0, 0, 0, classNone, fmt.Errorf("%w: %v", ErrInvalidMsgpack, err)
	}
	return i, u, f, class, nil
}

// encodeIntWithCode emits the msgpack encoding of n that preserves the given
// type code. Falls back to int8/16/32/64 sized encoding for non-typed codes
// (positive/negative fixint inputs are widened to a fixed-width type so the
// resulting blob is unambiguous).
func encodeIntWithCode(code byte, n int64) ([]byte, error) {
	var buf bytes.Buffer
	enc := msgpack.NewEncoder(&buf)
	switch code {
	case codeInt8:
		if err := enc.EncodeInt8(int8(n)); err != nil {
			return nil, err
		}
	case codeInt16:
		if err := enc.EncodeInt16(int16(n)); err != nil {
			return nil, err
		}
	case codeInt32:
		if err := enc.EncodeInt32(int32(n)); err != nil {
			return nil, err
		}
	case codeInt64:
		if err := enc.EncodeInt64(n); err != nil {
			return nil, err
		}
	default:
		// Positive/negative fixint targets get widened to int64 to keep the
		// implementation predictable; SDKs round-trip this transparently.
		if err := enc.EncodeInt64(n); err != nil {
			return nil, err
		}
	}
	return buf.Bytes(), nil
}

func encodeUintWithCode(code byte, n uint64) ([]byte, error) {
	var buf bytes.Buffer
	enc := msgpack.NewEncoder(&buf)
	switch code {
	case codeUint8:
		if err := enc.EncodeUint8(uint8(n)); err != nil {
			return nil, err
		}
	case codeUint16:
		if err := enc.EncodeUint16(uint16(n)); err != nil {
			return nil, err
		}
	case codeUint32:
		if err := enc.EncodeUint32(uint32(n)); err != nil {
			return nil, err
		}
	case codeUint64:
		if err := enc.EncodeUint64(n); err != nil {
			return nil, err
		}
	default:
		if err := enc.EncodeUint64(n); err != nil {
			return nil, err
		}
	}
	return buf.Bytes(), nil
}

func encodeFloatWithCode(code byte, n float64) ([]byte, error) {
	var buf bytes.Buffer
	enc := msgpack.NewEncoder(&buf)
	if code == codeFloat32 {
		if err := enc.EncodeFloat32(float32(n)); err != nil {
			return nil, err
		}
	} else {
		if err := enc.EncodeFloat64(n); err != nil {
			return nil, err
		}
	}
	return buf.Bytes(), nil
}
