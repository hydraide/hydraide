package valuecanon

import (
	"testing"
	"time"
)

func TestCanonicalize_Nil(t *testing.T) {
	k := Canonicalize(nil)
	if k.Kind != KindNull {
		t.Fatalf("nil should canonicalize to KindNull, got %v", k.Kind)
	}
	if !k.IsNull() {
		t.Fatalf("IsNull should return true")
	}
}

func TestCanonicalize_Bool(t *testing.T) {
	if k := Canonicalize(true); k.Kind != KindBool || !k.B {
		t.Fatalf("true canonicalization wrong: %+v", k)
	}
	if k := Canonicalize(false); k.Kind != KindBool || k.B {
		t.Fatalf("false canonicalization wrong: %+v", k)
	}
}

func TestValuecanon_IntVariants(t *testing.T) {
	want := Key{Kind: KindInt64, I: 42}
	values := []any{int8(42), int16(42), int32(42), int64(42), int(42)}
	for _, v := range values {
		got := Canonicalize(v)
		if got != want {
			t.Errorf("int variant %T(%v) → %+v, want %+v", v, v, got, want)
		}
	}
}

func TestValuecanon_UintVariants(t *testing.T) {
	want := Key{Kind: KindUint64, U: 42}
	values := []any{uint8(42), uint16(42), uint32(42), uint64(42), uint(42)}
	for _, v := range values {
		got := Canonicalize(v)
		if got != want {
			t.Errorf("uint variant %T(%v) → %+v, want %+v", v, v, got, want)
		}
	}
}

func TestValuecanon_FloatVariants(t *testing.T) {
	got32 := Canonicalize(float32(1.5))
	got64 := Canonicalize(float64(1.5))
	if got32.Kind != KindFloat64 || got32.F != 1.5 {
		t.Errorf("float32 → %+v", got32)
	}
	if got64.Kind != KindFloat64 || got64.F != 1.5 {
		t.Errorf("float64 → %+v", got64)
	}
}

func TestValuecanon_String(t *testing.T) {
	k := Canonicalize("hello")
	if k.Kind != KindString || k.S != "hello" {
		t.Fatalf("string canonicalization wrong: %+v", k)
	}
}

func TestValuecanon_Time(t *testing.T) {
	tm := time.Date(2026, 5, 12, 10, 0, 0, 0, time.UTC)
	k := Canonicalize(tm)
	if k.Kind != KindInt64 {
		t.Fatalf("time.Time should canonicalize to KindInt64, got %v", k.Kind)
	}
	if k.I != tm.Unix() {
		t.Fatalf("time.Time → %d, want %d", k.I, tm.Unix())
	}
}

func TestValuecanon_UnsupportedTypeIsNull(t *testing.T) {
	k := Canonicalize(struct{ A int }{1})
	if k.Kind != KindNull {
		t.Fatalf("unsupported type should fall back to KindNull, got %v", k.Kind)
	}
}

func TestValuecanon_NullSentinel(t *testing.T) {
	a := Canonicalize(nil)
	b := NullKey
	if !Equal(a, b) {
		t.Fatalf("Canonicalize(nil) should equal NullKey")
	}
}

func TestEqual_SameKindSameValue(t *testing.T) {
	cases := []Key{
		{Kind: KindBool, B: true},
		{Kind: KindInt64, I: 42},
		{Kind: KindUint64, U: 42},
		{Kind: KindFloat64, F: 1.5},
		{Kind: KindString, S: "x"},
		NullKey,
	}
	for _, k := range cases {
		if !Equal(k, k) {
			t.Errorf("Equal(%+v, %+v) should be true", k, k)
		}
	}
}

func TestEqual_SameKindDifferentValue(t *testing.T) {
	pairs := [][2]Key{
		{{Kind: KindBool, B: true}, {Kind: KindBool, B: false}},
		{{Kind: KindInt64, I: 1}, {Kind: KindInt64, I: 2}},
		{{Kind: KindUint64, U: 1}, {Kind: KindUint64, U: 2}},
		{{Kind: KindFloat64, F: 1.0}, {Kind: KindFloat64, F: 2.0}},
		{{Kind: KindString, S: "a"}, {Kind: KindString, S: "b"}},
	}
	for _, p := range pairs {
		if Equal(p[0], p[1]) {
			t.Errorf("Equal(%+v, %+v) should be false", p[0], p[1])
		}
	}
}

func TestValuecanon_IntVsUintBoundary(t *testing.T) {
	// Positive: int64(5) and uint64(5) compare equal.
	if !Equal(Canonicalize(int64(5)), Canonicalize(uint64(5))) {
		t.Errorf("int64(5) should equal uint64(5)")
	}
	// Negative int never equals any uint.
	if Equal(Canonicalize(int64(-1)), Canonicalize(uint64(0))) {
		t.Errorf("int64(-1) must not equal uint64(0)")
	}
	// Huge uint64 outside int64 range never equals any int64.
	huge := uint64(1) << 63
	if Equal(Canonicalize(int64(0)), Canonicalize(huge)) {
		t.Errorf("int64(0) must not equal uint64(1<<63)")
	}
}

func TestValuecanon_IntVsFloat(t *testing.T) {
	// Lossless: int64(5) vs float64(5.0).
	if !Equal(Canonicalize(int64(5)), Canonicalize(float64(5.0))) {
		t.Errorf("int64(5) should equal float64(5.0)")
	}
	// Lossy: int64 outside 2^53 mantissa range — strict inequality.
	big := int64(1<<53) + 1
	if Equal(Canonicalize(big), Canonicalize(float64(big))) {
		t.Errorf("int64(%d) must not equal float64(%d) under lossless rule", big, big)
	}
	// Fractional float: 5 vs 5.5 — not equal.
	if Equal(Canonicalize(int64(5)), Canonicalize(float64(5.5))) {
		t.Errorf("int64(5) must not equal float64(5.5)")
	}
}

func TestValuecanon_UintVsFloat(t *testing.T) {
	if !Equal(Canonicalize(uint64(5)), Canonicalize(float64(5.0))) {
		t.Errorf("uint64(5) should equal float64(5.0)")
	}
	if Equal(Canonicalize(uint64(5)), Canonicalize(float64(5.5))) {
		t.Errorf("uint64(5) must not equal float64(5.5)")
	}
}

func TestValuecanon_StringNotEqualToNumeric(t *testing.T) {
	pairs := [][2]any{
		{"5", int64(5)},
		{"5", uint64(5)},
		{"5.0", float64(5.0)},
		{"true", true},
	}
	for _, p := range pairs {
		if Equal(Canonicalize(p[0]), Canonicalize(p[1])) {
			t.Errorf("Equal(%v, %v) must be false across string boundary", p[0], p[1])
		}
	}
}

func TestValuecanon_NullEqualsNullOnly(t *testing.T) {
	n := NullKey
	for _, other := range []any{false, int64(0), uint64(0), float64(0), ""} {
		if Equal(n, Canonicalize(other)) {
			t.Errorf("NullKey must not equal %T(%v)", other, other)
		}
	}
	if !Equal(n, n) {
		t.Errorf("NullKey must equal itself")
	}
}

func TestKey_UsableAsMapKey(t *testing.T) {
	m := map[Key]int{}
	m[Canonicalize(int64(1))] = 10
	m[Canonicalize("x")] = 20
	m[NullKey] = 30
	if m[Canonicalize(int64(1))] != 10 {
		t.Errorf("int64 map lookup failed")
	}
	if m[Canonicalize("x")] != 20 {
		t.Errorf("string map lookup failed")
	}
	if m[NullKey] != 30 {
		t.Errorf("null map lookup failed")
	}
	// uint64(1) is a different Kind from int64(1), so a separate slot.
	m[Canonicalize(uint64(1))] = 40
	if m[Canonicalize(int64(1))] != 10 {
		t.Errorf("uint64 insert must not collide with int64 slot")
	}
	if m[Canonicalize(uint64(1))] != 40 {
		t.Errorf("uint64 map lookup failed")
	}
}
