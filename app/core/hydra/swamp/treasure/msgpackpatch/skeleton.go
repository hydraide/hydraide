// Package msgpackpatch provides type-preserving structural mutation of
// MessagePack-encoded blobs. It walks a blob's structural skeleton without
// materializing leaf values, allowing patches to splice in pre-encoded bytes
// while leaving untouched fields bit-identical to the original.
//
// The package is intentionally swamp-independent so it can be unit-tested in
// isolation and reused outside the core engine.
package msgpackpatch

import (
	"bytes"
	"errors"
	"fmt"
	"io"

	"github.com/vmihailenco/msgpack/v5"
)

// Kind identifies the structural category of a Skeleton node.
type Kind uint8

const (
	KindLeaf Kind = iota
	KindMap
	KindArray
)

// Skeleton is the parsed structural representation of a msgpack blob.
//
// Leaf nodes record only a byte range into the original blob plus the leading
// msgpack type code; the value bytes are NOT materialized. Container nodes
// (map / array) recursively hold child Skeletons, preserving original order.
//
// The byte ranges into the original blob enable splice-style mutation where
// untouched leaves are memcpy'd verbatim, preserving exact type encoding
// (e.g. int8 stays int8, time.Time stays its canonical extension encoding).
type Skeleton struct {
	Kind Kind

	// Leaf only.
	LeafStart int  // inclusive byte offset into the original blob (when RawBytes is nil)
	LeafEnd   int  // exclusive byte offset into the original blob (when RawBytes is nil)
	LeafCode  byte // first byte of the leaf encoding (msgpack type code)

	// RawBytes, when non-nil, supersedes LeafStart/LeafEnd as the leaf's
	// byte source. This is set by mutation ops (SET, INC, etc.) to splice
	// in a new value without modifying the original blob.
	RawBytes []byte

	// Map only. Ordered list of (key, value) entries.
	// V1 supports string keys only; non-string-keyed maps return ErrNonStringKey.
	MapFields []MapField

	// Array only. Ordered child skeletons.
	ArrayItems []*Skeleton
}

// MapField is a single entry in a map skeleton.
type MapField struct {
	Key   string
	Value *Skeleton
}

// ErrNonStringKey is returned when the parser encounters a map key that is not
// a msgpack string. V1 only supports string-keyed maps, which matches all
// SDK-emitted blobs.
var ErrNonStringKey = errors.New("msgpackpatch: non-string map key not supported")

// ErrInvalidMsgpack indicates a malformed or truncated msgpack blob.
var ErrInvalidMsgpack = errors.New("msgpackpatch: invalid msgpack data")

// ErrPathInvalid indicates a malformed path string or an unresolvable
// structural reference (e.g. array index out of range).
var ErrPathInvalid = errors.New("msgpackpatch: invalid path")

// ErrTypeMismatch indicates a path traversal that crosses a type boundary
// (e.g. field access on a leaf, index access on a map).
var ErrTypeMismatch = errors.New("msgpackpatch: type mismatch")

// Parse walks the given msgpack blob and returns its structural skeleton.
// The returned Skeleton holds byte ranges into blob; the caller must not
// mutate blob while the Skeleton is in use.
func Parse(blob []byte) (*Skeleton, error) {
	if len(blob) == 0 {
		return nil, ErrInvalidMsgpack
	}
	r := bytes.NewReader(blob)
	dec := msgpack.NewDecoder(r)
	total := len(blob)

	skel, err := parseNode(dec, r, total)
	if err != nil {
		return nil, err
	}
	if r.Len() != 0 {
		return nil, fmt.Errorf("%w: %d trailing bytes", ErrInvalidMsgpack, r.Len())
	}
	return skel, nil
}

// pos returns the current absolute byte offset into the original blob,
// derived from the bytes.Reader's remaining length.
func pos(r *bytes.Reader, total int) int {
	return total - r.Len()
}

func parseNode(dec *msgpack.Decoder, r *bytes.Reader, total int) (*Skeleton, error) {
	startPos := pos(r, total)
	code, err := dec.PeekCode()
	if err != nil {
		return nil, wrapInvalid(err)
	}

	switch {
	case isMapCode(code):
		return parseMap(dec, r, total)
	case isArrayCode(code):
		return parseArray(dec, r, total)
	default:
		// Leaf: skip the value and record the byte range it occupied.
		if err := dec.Skip(); err != nil {
			return nil, wrapInvalid(err)
		}
		endPos := pos(r, total)
		return &Skeleton{
			Kind:      KindLeaf,
			LeafStart: startPos,
			LeafEnd:   endPos,
			LeafCode:  code,
		}, nil
	}
}

func parseMap(dec *msgpack.Decoder, r *bytes.Reader, total int) (*Skeleton, error) {
	n, err := dec.DecodeMapLen()
	if err != nil {
		return nil, wrapInvalid(err)
	}
	skel := &Skeleton{Kind: KindMap, MapFields: make([]MapField, 0, n)}
	for i := 0; i < n; i++ {
		// V1: keys must be msgpack strings.
		code, err := dec.PeekCode()
		if err != nil {
			return nil, wrapInvalid(err)
		}
		if !isStringCode(code) {
			return nil, ErrNonStringKey
		}
		key, err := dec.DecodeString()
		if err != nil {
			return nil, wrapInvalid(err)
		}
		val, err := parseNode(dec, r, total)
		if err != nil {
			return nil, err
		}
		skel.MapFields = append(skel.MapFields, MapField{Key: key, Value: val})
	}
	return skel, nil
}

func parseArray(dec *msgpack.Decoder, r *bytes.Reader, total int) (*Skeleton, error) {
	n, err := dec.DecodeArrayLen()
	if err != nil {
		return nil, wrapInvalid(err)
	}
	skel := &Skeleton{Kind: KindArray, ArrayItems: make([]*Skeleton, 0, n)}
	for i := 0; i < n; i++ {
		val, err := parseNode(dec, r, total)
		if err != nil {
			return nil, err
		}
		skel.ArrayItems = append(skel.ArrayItems, val)
	}
	return skel, nil
}

// leafBytes returns the raw msgpack bytes backing a leaf skeleton: either the
// post-mutation RawBytes if set, otherwise the slice into the original blob.
func leafBytes(s *Skeleton, orig []byte) []byte {
	if s.RawBytes != nil {
		return s.RawBytes
	}
	return orig[s.LeafStart:s.LeafEnd]
}

func wrapInvalid(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		return fmt.Errorf("%w: unexpected EOF", ErrInvalidMsgpack)
	}
	return fmt.Errorf("%w: %v", ErrInvalidMsgpack, err)
}
