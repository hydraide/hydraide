package msgpackpatch

// MessagePack type code classifiers. These mirror the constants in
// github.com/vmihailenco/msgpack/v5/msgpcode but are kept local to avoid
// importing an internal-feeling subpackage and to make the type checks
// cheap and inlinable.
//
// Reference: https://github.com/msgpack/msgpack/blob/master/spec.md

// Container codes.
func isFixMap(c byte) bool   { return c >= 0x80 && c <= 0x8f }
func isFixArray(c byte) bool { return c >= 0x90 && c <= 0x9f }

const (
	codeNil      byte = 0xc0
	codeFalse    byte = 0xc2
	codeTrue     byte = 0xc3
	codeBin8     byte = 0xc4
	codeBin16    byte = 0xc5
	codeBin32    byte = 0xc6
	codeExt8     byte = 0xc7
	codeExt16    byte = 0xc8
	codeExt32    byte = 0xc9
	codeFloat32  byte = 0xca
	codeFloat64  byte = 0xcb
	codeUint8    byte = 0xcc
	codeUint16   byte = 0xcd
	codeUint32   byte = 0xce
	codeUint64   byte = 0xcf
	codeInt8     byte = 0xd0
	codeInt16    byte = 0xd1
	codeInt32    byte = 0xd2
	codeInt64    byte = 0xd3
	codeFixExt1  byte = 0xd4
	codeFixExt2  byte = 0xd5
	codeFixExt4  byte = 0xd6
	codeFixExt8  byte = 0xd7
	codeFixExt16 byte = 0xd8
	codeStr8     byte = 0xd9
	codeStr16    byte = 0xda
	codeStr32    byte = 0xdb
	codeArray16  byte = 0xdc
	codeArray32  byte = 0xdd
	codeMap16    byte = 0xde
	codeMap32    byte = 0xdf
)

func isFixStr(c byte) bool      { return c >= 0xa0 && c <= 0xbf }
func isPositiveFix(c byte) bool { return c <= 0x7f }
func isNegativeFix(c byte) bool { return c >= 0xe0 }

func isMapCode(c byte) bool {
	return isFixMap(c) || c == codeMap16 || c == codeMap32
}

func isArrayCode(c byte) bool {
	return isFixArray(c) || c == codeArray16 || c == codeArray32
}

func isStringCode(c byte) bool {
	return isFixStr(c) || c == codeStr8 || c == codeStr16 || c == codeStr32
}

func isIntegerCode(c byte) bool {
	if isPositiveFix(c) || isNegativeFix(c) {
		return true
	}
	switch c {
	case codeUint8, codeUint16, codeUint32, codeUint64,
		codeInt8, codeInt16, codeInt32, codeInt64:
		return true
	}
	return false
}

func isFloatCode(c byte) bool {
	return c == codeFloat32 || c == codeFloat64
}

func isNumericCode(c byte) bool {
	return isIntegerCode(c) || isFloatCode(c)
}
