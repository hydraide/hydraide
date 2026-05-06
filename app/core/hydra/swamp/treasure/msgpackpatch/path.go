package msgpackpatch

import (
	"fmt"
	"strconv"
	"strings"
)

// SegmentKind identifies what a path segment refers to.
type SegmentKind uint8

const (
	// SegField is a named field in a map (e.g. "Foo").
	SegField SegmentKind = iota
	// SegIndex is a numeric index into an array (e.g. "[3]" or "[-1]").
	// Negative indices count from the end and are resolved at navigation time.
	SegIndex
	// SegAppend is the "[]" marker, valid only as the final segment of an
	// APPEND or PREPEND path. It refers to the position past the last element.
	SegAppend
)

// Segment is one step in a path expression.
type Segment struct {
	Kind  SegmentKind
	Field string // SegField only
	Index int    // SegIndex only (may be negative; resolved at navigation)
}

// Path is a parsed mutation path expression.
type Path struct {
	Segments []Segment
}

// ParsePath parses an expression like "Foo.Bar[3].Baz" or "Tags[]" into a
// sequence of Segments. Wildcards ("[*]") and pseudo-fields ("#len") are
// rejected because they have no meaning in a mutation context.
//
// Grammar (informal):
//
//	path     := segment ('.' segment)*
//	segment  := name | name '[' index ']' | name '[]'
//	name     := [A-Za-z0-9_] (no '.', '[', ']')
//	index    := '-'? [0-9]+
//
// All non-empty inputs that don't match return ErrPathInvalid.
func ParsePath(s string) (*Path, error) {
	if s == "" {
		return nil, fmt.Errorf("%w: empty path", ErrPathInvalid)
	}

	var segs []Segment
	parts := strings.Split(s, ".")
	for _, part := range parts {
		if part == "" {
			return nil, fmt.Errorf("%w: empty segment in %q", ErrPathInvalid, s)
		}
		if strings.HasPrefix(part, "#") {
			return nil, fmt.Errorf("%w: pseudo-field %q not allowed", ErrPathInvalid, part)
		}
		if err := parseSegmentInto(part, &segs); err != nil {
			return nil, err
		}
	}
	return &Path{Segments: segs}, nil
}

// parseSegmentInto handles a single dot-delimited part, which may contain
// zero or more bracket sub-expressions, e.g. "Tags", "Tags[3]", "Tags[]".
// Multiple bracket suffixes on one part (e.g. "m[0][1]") are also accepted.
func parseSegmentInto(part string, out *[]Segment) error {
	// Find first '[' to split name from bracket suffixes.
	br := strings.IndexByte(part, '[')
	if br < 0 {
		// Plain field name. Reject anything containing ']'.
		if strings.ContainsAny(part, "[]") {
			return fmt.Errorf("%w: malformed segment %q", ErrPathInvalid, part)
		}
		*out = append(*out, Segment{Kind: SegField, Field: part})
		return nil
	}
	name := part[:br]
	if name == "" {
		return fmt.Errorf("%w: bracket without name in %q", ErrPathInvalid, part)
	}
	if strings.ContainsAny(name, "[]") {
		return fmt.Errorf("%w: malformed segment %q", ErrPathInvalid, part)
	}
	*out = append(*out, Segment{Kind: SegField, Field: name})

	// Walk bracket suffixes: "[N]" or "[]".
	rest := part[br:]
	for len(rest) > 0 {
		if rest[0] != '[' {
			return fmt.Errorf("%w: trailing %q after brackets", ErrPathInvalid, rest)
		}
		end := strings.IndexByte(rest, ']')
		if end < 0 {
			return fmt.Errorf("%w: unclosed bracket in %q", ErrPathInvalid, part)
		}
		inner := rest[1:end]
		if inner == "" {
			*out = append(*out, Segment{Kind: SegAppend})
		} else if inner == "*" {
			return fmt.Errorf("%w: wildcard [*] not allowed", ErrPathInvalid)
		} else {
			n, err := strconv.Atoi(inner)
			if err != nil {
				return fmt.Errorf("%w: non-integer index %q", ErrPathInvalid, inner)
			}
			*out = append(*out, Segment{Kind: SegIndex, Index: n})
		}
		rest = rest[end+1:]
	}
	return nil
}

// Cursor is the result of resolving a path against a Skeleton. It describes
// where in the tree the final segment refers to, even if the target doesn't
// yet exist (the SET / APPEND auto-create case).
type Cursor struct {
	// Parent is the deepest container along the path whose existence is
	// confirmed. For a top-level field, Parent is the root map. For a
	// missing-intermediate path, Parent is the deepest existing ancestor.
	Parent *Skeleton

	// Final is the final segment of the path. Together with Parent it
	// uniquely describes the target slot.
	Final Segment

	// Target is the existing skeleton at the path, or nil if the target
	// doesn't exist (final segment missing) or for SegAppend (which has
	// no target by definition).
	Target *Skeleton

	// TargetIdx is the index of Target within Parent.MapFields (when Final
	// is SegField) or Parent.ArrayItems (when Final is SegIndex). -1 if
	// Target is nil.
	TargetIdx int

	// MissingAt is the index of the first missing path segment, or -1 if
	// the entire path resolved to an existing target. When MissingAt ==
	// len(path.Segments)-1 only the final segment is missing (typical
	// SET-on-new-field). When MissingAt < len-1 an intermediate is missing
	// (auto-create candidate for SET).
	MissingAt int

	// ResolvedFinalIndex stores the absolute (non-negative) array index
	// that Final.Index resolved to, when Final.Kind == SegIndex.
	ResolvedFinalIndex int
}

// Resolve walks the path through root and returns a Cursor describing the
// outcome.
//
// Returns ErrTypeMismatch when the path attempts to traverse into a node of
// the wrong kind (field-access into a leaf, index-access into a map, etc.).
// Returns ErrPathInvalid when an array index is out of range for an existing
// array (this is a hard error, not a "missing" signal).
//
// A non-existent leaf field is NOT an error; it produces a Cursor with
// Target=nil and MissingAt set, to support SET / APPEND auto-create.
func (p *Path) Resolve(root *Skeleton) (*Cursor, error) {
	if root == nil || len(p.Segments) == 0 {
		return nil, fmt.Errorf("%w: empty path or nil root", ErrPathInvalid)
	}

	current := root
	for i, seg := range p.Segments {
		isFinal := i == len(p.Segments)-1

		switch seg.Kind {
		case SegField:
			if current.Kind != KindMap {
				return nil, fmt.Errorf("%w: field %q on non-map at depth %d",
					ErrTypeMismatch, seg.Field, i)
			}
			idx := findField(current, seg.Field)
			if idx < 0 {
				// Missing field. If final, this is an auto-create slot.
				// If intermediate, it's still surfaced via MissingAt; the SET
				// op layer decides whether to create or error.
				return &Cursor{
					Parent:    current,
					Final:     seg,
					Target:    nil,
					TargetIdx: -1,
					MissingAt: i,
				}, nil
			}
			if isFinal {
				return &Cursor{
					Parent:    current,
					Final:     seg,
					Target:    current.MapFields[idx].Value,
					TargetIdx: idx,
					MissingAt: -1,
				}, nil
			}
			current = current.MapFields[idx].Value

		case SegIndex:
			if current.Kind != KindArray {
				return nil, fmt.Errorf("%w: index on non-array at depth %d",
					ErrTypeMismatch, i)
			}
			idx, err := resolveIndex(seg.Index, len(current.ArrayItems))
			if err != nil {
				return nil, err
			}
			if isFinal {
				return &Cursor{
					Parent:             current,
					Final:              seg,
					Target:             current.ArrayItems[idx],
					TargetIdx:          idx,
					MissingAt:          -1,
					ResolvedFinalIndex: idx,
				}, nil
			}
			current = current.ArrayItems[idx]

		case SegAppend:
			if !isFinal {
				return nil, fmt.Errorf("%w: [] marker must be final segment", ErrPathInvalid)
			}
			if current.Kind != KindArray {
				return nil, fmt.Errorf("%w: [] on non-array at depth %d",
					ErrTypeMismatch, i)
			}
			return &Cursor{
				Parent:    current,
				Final:     seg,
				Target:    nil,
				TargetIdx: -1,
				MissingAt: -1,
			}, nil
		}
	}
	// Unreachable in correct paths.
	return nil, fmt.Errorf("%w: walk fell through", ErrPathInvalid)
}

// findField returns the index of name in m.MapFields, or -1 if absent.
func findField(m *Skeleton, name string) int {
	for i, f := range m.MapFields {
		if f.Key == name {
			return i
		}
	}
	return -1
}

// resolveIndex turns a possibly-negative index into a non-negative one,
// checking array bounds.
func resolveIndex(want, length int) (int, error) {
	if want < 0 {
		want = length + want
	}
	if want < 0 || want >= length {
		return 0, fmt.Errorf("%w: index out of range", ErrPathInvalid)
	}
	return want, nil
}
