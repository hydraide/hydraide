package msgpackpatch

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParsePath_Simple(t *testing.T) {
	cases := []struct {
		in       string
		segments []Segment
	}{
		{"Foo", []Segment{{Kind: SegField, Field: "Foo"}}},
		{"Foo.Bar", []Segment{
			{Kind: SegField, Field: "Foo"},
			{Kind: SegField, Field: "Bar"},
		}},
		{"a.b.c.d", []Segment{
			{Kind: SegField, Field: "a"},
			{Kind: SegField, Field: "b"},
			{Kind: SegField, Field: "c"},
			{Kind: SegField, Field: "d"},
		}},
		{"Tags[3]", []Segment{
			{Kind: SegField, Field: "Tags"},
			{Kind: SegIndex, Index: 3},
		}},
		{"Tags[-1]", []Segment{
			{Kind: SegField, Field: "Tags"},
			{Kind: SegIndex, Index: -1},
		}},
		{"Tags[]", []Segment{
			{Kind: SegField, Field: "Tags"},
			{Kind: SegAppend},
		}},
		{"a.b[0].c", []Segment{
			{Kind: SegField, Field: "a"},
			{Kind: SegField, Field: "b"},
			{Kind: SegIndex, Index: 0},
			{Kind: SegField, Field: "c"},
		}},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			p, err := ParsePath(tc.in)
			require.NoError(t, err)
			assert.Equal(t, tc.segments, p.Segments)
		})
	}
}

func TestParsePath_Invalid(t *testing.T) {
	bad := []string{
		"",
		".",
		".foo",
		"foo.",
		"foo..bar",
		"foo[",
		"foo]",
		"foo[abc]",
		"foo[1",
		"foo[1]extra",
		"[]",
		"foo[*]",   // wildcards not allowed in mutation paths
		"foo.#len", // pseudo-fields not allowed in mutation paths
	}
	for _, s := range bad {
		t.Run(s, func(t *testing.T) {
			_, err := ParsePath(s)
			assert.ErrorIs(t, err, ErrPathInvalid, "input %q", s)
		})
	}
}

func TestResolve_TopLevelExisting(t *testing.T) {
	blob := mustEncode(t, map[string]any{"name": "alice"})
	skel, err := Parse(blob)
	require.NoError(t, err)

	p, err := ParsePath("name")
	require.NoError(t, err)
	c, err := p.Resolve(skel)
	require.NoError(t, err)

	assert.Same(t, skel, c.Parent)
	assert.Equal(t, SegField, c.Final.Kind)
	assert.Equal(t, "name", c.Final.Field)
	assert.NotNil(t, c.Target)
	assert.Equal(t, KindLeaf, c.Target.Kind)
	assert.Equal(t, 0, c.TargetIdx)
	assert.Equal(t, -1, c.MissingAt)
}

func TestResolve_TopLevelMissing(t *testing.T) {
	blob := mustEncode(t, map[string]any{"name": "alice"})
	skel, err := Parse(blob)
	require.NoError(t, err)

	p, err := ParsePath("missing")
	require.NoError(t, err)
	c, err := p.Resolve(skel)
	require.NoError(t, err)

	assert.Same(t, skel, c.Parent)
	assert.Nil(t, c.Target)
	assert.Equal(t, -1, c.TargetIdx)
	// MissingAt == 0 means the final segment (index 0) is missing — but the parent exists.
	// Auto-create / SET layer can use Parent + Final to insert.
	assert.Equal(t, 0, c.MissingAt)
}

func TestResolve_Nested(t *testing.T) {
	blob := mustEncode(t, map[string]any{
		"outer": map[string]any{"inner": "deep"},
	})
	skel, err := Parse(blob)
	require.NoError(t, err)

	p, err := ParsePath("outer.inner")
	require.NoError(t, err)
	c, err := p.Resolve(skel)
	require.NoError(t, err)

	require.NotNil(t, c.Parent)
	assert.Equal(t, KindMap, c.Parent.Kind)
	assert.Equal(t, "inner", c.Parent.MapFields[0].Key)
	assert.NotNil(t, c.Target)
	assert.Equal(t, KindLeaf, c.Target.Kind)
}

func TestResolve_MissingIntermediate(t *testing.T) {
	blob := mustEncode(t, map[string]any{"a": map[string]any{"b": int8(1)}})
	skel, err := Parse(blob)
	require.NoError(t, err)

	// path: a.x.y — "x" doesn't exist under "a", so resolution can't continue.
	p, err := ParsePath("a.x.y")
	require.NoError(t, err)
	c, err := p.Resolve(skel)
	require.NoError(t, err)

	// MissingAt == 1 means segment index 1 ("x") was the first missing segment.
	assert.Equal(t, 1, c.MissingAt)
	assert.Nil(t, c.Target)
	// Parent = the "a" map (deepest existing container along the path).
	require.NotNil(t, c.Parent)
	assert.Equal(t, KindMap, c.Parent.Kind)
}

func TestResolve_ArrayIndex(t *testing.T) {
	blob := mustEncode(t, map[string]any{
		"tags": []any{"a", "b", "c"},
	})
	skel, err := Parse(blob)
	require.NoError(t, err)

	p, err := ParsePath("tags[1]")
	require.NoError(t, err)
	c, err := p.Resolve(skel)
	require.NoError(t, err)

	assert.NotNil(t, c.Target)
	assert.Equal(t, 1, c.TargetIdx)
	assert.Equal(t, SegIndex, c.Final.Kind)
	assert.Equal(t, KindArray, c.Parent.Kind)
}

func TestResolve_ArrayIndexNegative(t *testing.T) {
	blob := mustEncode(t, map[string]any{
		"tags": []any{"a", "b", "c"},
	})
	skel, err := Parse(blob)
	require.NoError(t, err)

	p, err := ParsePath("tags[-1]")
	require.NoError(t, err)
	c, err := p.Resolve(skel)
	require.NoError(t, err)

	require.NotNil(t, c.Target)
	// -1 should resolve to index 2 in a 3-element array.
	assert.Equal(t, 2, c.TargetIdx)
}

func TestResolve_ArrayIndexOutOfRange(t *testing.T) {
	blob := mustEncode(t, map[string]any{
		"tags": []any{"a", "b"},
	})
	skel, err := Parse(blob)
	require.NoError(t, err)

	p, err := ParsePath("tags[5]")
	require.NoError(t, err)
	_, err = p.Resolve(skel)
	assert.ErrorIs(t, err, ErrPathInvalid)
}

func TestResolve_AppendMarker(t *testing.T) {
	blob := mustEncode(t, map[string]any{
		"tags": []any{"a"},
	})
	skel, err := Parse(blob)
	require.NoError(t, err)

	p, err := ParsePath("tags[]")
	require.NoError(t, err)
	c, err := p.Resolve(skel)
	require.NoError(t, err)

	assert.Equal(t, SegAppend, c.Final.Kind)
	assert.Equal(t, KindArray, c.Parent.Kind)
	// Append marker has no Target.
	assert.Nil(t, c.Target)
}

func TestResolve_AppendOnMissingArray(t *testing.T) {
	blob := mustEncode(t, map[string]any{"name": "alice"})
	skel, err := Parse(blob)
	require.NoError(t, err)

	// "tags" doesn't exist; tags[] is an APPEND target on a missing array.
	// Resolver should signal MissingAt at the field segment (auto-create candidate).
	p, err := ParsePath("tags[]")
	require.NoError(t, err)
	c, err := p.Resolve(skel)
	require.NoError(t, err)

	assert.Equal(t, 0, c.MissingAt)
	assert.Nil(t, c.Target)
}

func TestResolve_FieldAccessOnNonMap(t *testing.T) {
	blob := mustEncode(t, map[string]any{"name": "alice"})
	skel, err := Parse(blob)
	require.NoError(t, err)

	// "name" is a string leaf; trying to traverse into it as map is a type error.
	p, err := ParsePath("name.something")
	require.NoError(t, err)
	_, err = p.Resolve(skel)
	assert.ErrorIs(t, err, ErrTypeMismatch)
}

func TestResolve_IndexAccessOnNonArray(t *testing.T) {
	blob := mustEncode(t, map[string]any{"name": "alice"})
	skel, err := Parse(blob)
	require.NoError(t, err)

	p, err := ParsePath("name[0]")
	require.NoError(t, err)
	_, err = p.Resolve(skel)
	assert.ErrorIs(t, err, ErrTypeMismatch)
}
