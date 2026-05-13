package gateway

import (
	"testing"

	hydrapb "github.com/hydraide/hydraide/sdk/go/hydraidego/v3/hydraidepbgo"
)

// --- Builders -------------------------------------------------------------

func eq(path string, v any) *hydrapb.TreasureFilter {
	f := &hydrapb.TreasureFilter{
		Operator:       hydrapb.Relational_EQUAL,
		BytesFieldPath: bpStr(path),
	}
	switch x := v.(type) {
	case int32:
		f.CompareValue = &hydrapb.TreasureFilter_Int32Val{Int32Val: x}
	case int64:
		f.CompareValue = &hydrapb.TreasureFilter_Int64Val{Int64Val: x}
	case uint32:
		f.CompareValue = &hydrapb.TreasureFilter_Uint32Val{Uint32Val: x}
	case uint64:
		f.CompareValue = &hydrapb.TreasureFilter_Uint64Val{Uint64Val: x}
	case float64:
		f.CompareValue = &hydrapb.TreasureFilter_Float64Val{Float64Val: x}
	case string:
		f.CompareValue = &hydrapb.TreasureFilter_StringVal{StringVal: x}
	case bool:
		if x {
			f.CompareValue = &hydrapb.TreasureFilter_BoolVal{BoolVal: hydrapb.Boolean_TRUE}
		} else {
			f.CompareValue = &hydrapb.TreasureFilter_BoolVal{BoolVal: hydrapb.Boolean_FALSE}
		}
	default:
		panic("unsupported eq value type")
	}
	return f
}

func neq(path string, v int64) *hydrapb.TreasureFilter {
	return &hydrapb.TreasureFilter{
		Operator:       hydrapb.Relational_NOT_EQUAL,
		BytesFieldPath: bpStr(path),
		CompareValue:   &hydrapb.TreasureFilter_Int64Val{Int64Val: v},
	}
}

func gt(path string, v int64) *hydrapb.TreasureFilter {
	return &hydrapb.TreasureFilter{
		Operator:       hydrapb.Relational_GREATER_THAN,
		BytesFieldPath: bpStr(path),
		CompareValue:   &hydrapb.TreasureFilter_Int64Val{Int64Val: v},
	}
}

func lt(path string, v int64) *hydrapb.TreasureFilter {
	return &hydrapb.TreasureFilter{
		Operator:       hydrapb.Relational_LESS_THAN,
		BytesFieldPath: bpStr(path),
		CompareValue:   &hydrapb.TreasureFilter_Int64Val{Int64Val: v},
	}
}

func int64In(path string, vs ...int64) *hydrapb.TreasureFilter {
	return &hydrapb.TreasureFilter{
		Operator:       hydrapb.Relational_INT64_IN,
		BytesFieldPath: bpStr(path),
		Int64InVals:    vs,
	}
}

func stringIn(path string, vs ...string) *hydrapb.TreasureFilter {
	return &hydrapb.TreasureFilter{
		Operator:       hydrapb.Relational_STRING_IN,
		BytesFieldPath: bpStr(path),
		StringInVals:   vs,
	}
}

func andG(legs ...*hydrapb.TreasureFilter) *hydrapb.FilterGroup {
	return &hydrapb.FilterGroup{Logic: hydrapb.FilterLogic_AND, Filters: legs}
}

func orG(legs ...*hydrapb.TreasureFilter) *hydrapb.FilterGroup {
	return &hydrapb.FilterGroup{Logic: hydrapb.FilterLogic_OR, Filters: legs}
}

func withSubGroups(g *hydrapb.FilterGroup, subs ...*hydrapb.FilterGroup) *hydrapb.FilterGroup {
	g.SubGroups = append(g.SubGroups, subs...)
	return g
}

func withVector(g *hydrapb.FilterGroup) *hydrapb.FilterGroup {
	g.VectorFilters = append(g.VectorFilters, &hydrapb.VectorFilter{
		BytesFieldPath: "Embedding",
		QueryVector:    []float32{0.1, 0.2, 0.3},
	})
	return g
}

func withPhrase(g *hydrapb.FilterGroup) *hydrapb.FilterGroup {
	g.PhraseFilters = append(g.PhraseFilters, &hydrapb.PhraseFilter{
		BytesFieldPath: "Body",
		Words:          []string{"hello"},
	})
	return g
}

func withGeo(g *hydrapb.FilterGroup) *hydrapb.FilterGroup {
	g.GeoDistanceFilters = append(g.GeoDistanceFilters, &hydrapb.GeoDistanceFilter{})
	return g
}

func withNested(g *hydrapb.FilterGroup) *hydrapb.FilterGroup {
	g.NestedSliceWhereFilters = append(g.NestedSliceWhereFilters, &hydrapb.NestedSliceWhereFilter{
		SlicePath: "Items",
	})
	return g
}

func bpStr(s string) *string { return &s }

// --- M1: empty filter ----------------------------------------------------

func TestPlanner_M1_NilFilter(t *testing.T) {
	if PlanFilter(nil).Mode != PlanModeBypass {
		t.Fatalf("nil filter must Bypass")
	}
}

func TestPlanner_M1_EmptyGroup(t *testing.T) {
	if PlanFilter(&hydrapb.FilterGroup{}).Mode != PlanModeBypass {
		t.Fatalf("empty group must Bypass")
	}
}

// --- M2: beacon + Equal --------------------------------------------------

func TestPlanner_M2_AsnEqual(t *testing.T) {
	p := PlanFilter(andG(eq("asn", int64(10))))
	if p.Mode != PlanModeAnd || len(p.Hints) != 1 {
		t.Fatalf("plan=%+v", p)
	}
	h := p.Hints[0]
	if h.FieldPath != "asn" || h.Op != HintEqual {
		t.Fatalf("hint=%+v", h)
	}
	if v, ok := h.Values[0].(int64); !ok || v != 10 {
		t.Fatalf("hint value=%v", h.Values[0])
	}
	// Residual is the original group minus the single leg, so empty.
	if !isEmptyGroup(p.Residual) {
		t.Fatalf("residual should be empty: %+v", p.Residual)
	}
}

func TestPlanner_M2_StatusEqual(t *testing.T) {
	p := PlanFilter(andG(eq("status", "ready")))
	if p.Mode != PlanModeAnd {
		t.Fatalf("mode=%v", p.Mode)
	}
	if p.Hints[0].FieldPath != "status" || p.Hints[0].Values[0] != "ready" {
		t.Fatalf("hint=%+v", p.Hints[0])
	}
}

// --- M5: two ANDed Equals ------------------------------------------------

func TestPlanner_M5_AsnAndStatus(t *testing.T) {
	p := PlanFilter(andG(eq("asn", int64(10)), eq("status", "ready")))
	if p.Mode != PlanModeAnd || len(p.Hints) != 1 {
		t.Fatalf("plan=%+v", p)
	}
	// First-indexable-wins heuristic.
	if p.Hints[0].FieldPath != "asn" {
		t.Fatalf("hint=%+v", p.Hints[0])
	}
	if len(p.Residual.GetFilters()) != 1 || p.Residual.GetFilters()[0].GetBytesFieldPath() != "status" {
		t.Fatalf("residual=%+v", p.Residual)
	}
}

func TestPlanner_M5_ThreeEquals(t *testing.T) {
	p := PlanFilter(andG(
		eq("asn", int64(10)),
		eq("status", "ready"),
		eq("category", "A"),
	))
	if p.Mode != PlanModeAnd || len(p.Residual.GetFilters()) != 2 {
		t.Fatalf("plan=%+v", p)
	}
}

// --- M6/M7: OR-union -----------------------------------------------------

func TestPlanner_M6_TwoAsnEquals(t *testing.T) {
	p := PlanFilter(orG(eq("asn", int64(10)), eq("asn", int64(20))))
	if p.Mode != PlanModeOrUnion || len(p.Hints) != 2 {
		t.Fatalf("plan=%+v", p)
	}
	if p.Hints[0].FieldPath != "asn" || p.Hints[1].FieldPath != "asn" {
		t.Fatalf("hints=%+v", p.Hints)
	}
	if p.Residual != nil {
		t.Fatalf("OrUnion residual must be nil, got %+v", p.Residual)
	}
}

func TestPlanner_M7_AsnOrStatus(t *testing.T) {
	p := PlanFilter(orG(eq("asn", int64(10)), eq("status", "ready")))
	if p.Mode != PlanModeOrUnion || len(p.Hints) != 2 {
		t.Fatalf("plan=%+v", p)
	}
	if p.Hints[0].FieldPath != "asn" || p.Hints[1].FieldPath != "status" {
		t.Fatalf("hints=%+v", p.Hints)
	}
}

// --- M8: IN --------------------------------------------------------------

func TestPlanner_M8_SmallInt64In(t *testing.T) {
	p := PlanFilter(andG(int64In("asn", 1, 2, 3)))
	if p.Mode != PlanModeAnd || len(p.Hints) != 1 {
		t.Fatalf("plan=%+v", p)
	}
	if p.Hints[0].Op != HintIn || len(p.Hints[0].Values) != 3 {
		t.Fatalf("hint=%+v", p.Hints[0])
	}
}

func TestPlanner_M8_StringIn(t *testing.T) {
	p := PlanFilter(andG(stringIn("status", "ready", "pending")))
	if p.Mode != PlanModeAnd || p.Hints[0].Op != HintIn {
		t.Fatalf("plan=%+v", p)
	}
	if p.Hints[0].Values[0] != "ready" || p.Hints[0].Values[1] != "pending" {
		t.Fatalf("values=%v", p.Hints[0].Values)
	}
}

// --- M9: AND + vector ----------------------------------------------------

func TestPlanner_M9_AsnAndVector(t *testing.T) {
	g := andG(eq("asn", int64(10)))
	withVector(g)
	p := PlanFilter(g)
	if p.Mode != PlanModeAnd {
		t.Fatalf("mode=%v", p.Mode)
	}
	if len(p.Residual.GetVectorFilters()) != 1 {
		t.Fatalf("vector residual missing: %+v", p.Residual)
	}
}

// --- M10: AND containing OR-union sub --------------------------------------

func TestPlanner_M10_OrUnionInAnd(t *testing.T) {
	sub := orG(eq("asn", int64(10)), eq("asn", int64(20)))
	g := andG(eq("status", "ready"))
	withSubGroups(g, sub)
	p := PlanFilter(g)
	if p.Mode != PlanModeAnd {
		t.Fatalf("mode=%v", p.Mode)
	}
	// AND has a directly-indexable leaf (status), so the heuristic
	// picks that first; the OR sub-group stays in residual.
	if p.Hints[0].FieldPath != "status" {
		t.Fatalf("hint=%+v", p.Hints[0])
	}
	if len(p.Residual.GetSubGroups()) != 1 {
		t.Fatalf("sub residual missing: %+v", p.Residual)
	}
}

func TestPlanner_M10_OnlyOrSubGroup(t *testing.T) {
	// AND with no indexable leaf but a fully-indexable OR sub-group
	// is consumed as the AND candidate set.
	sub := orG(eq("asn", int64(10)), eq("asn", int64(20)))
	g := withSubGroups(andG(), sub)
	withVector(g) // a non-indexable AND sibling
	p := PlanFilter(g)
	if p.Mode != PlanModeAnd || len(p.Hints) != 2 {
		t.Fatalf("plan=%+v", p)
	}
	if len(p.Residual.GetVectorFilters()) != 1 {
		t.Fatalf("vector residual missing: %+v", p.Residual)
	}
}

// --- M11/M12/M14/M15: range — bypass in v1 -------------------------------

func TestPlanner_M11_GreaterThan(t *testing.T) {
	if PlanFilter(andG(gt("score", 100))).Mode != PlanModeBypass {
		t.Fatalf("range alone must Bypass in v1")
	}
}

func TestPlanner_M11_LessThan(t *testing.T) {
	if PlanFilter(andG(lt("score", 50))).Mode != PlanModeBypass {
		t.Fatalf("range alone must Bypass in v1")
	}
}

func TestPlanner_M14_AsnAndRange(t *testing.T) {
	p := PlanFilter(andG(eq("asn", int64(10)), gt("score", 100)))
	if p.Mode != PlanModeAnd {
		t.Fatalf("mode=%v", p.Mode)
	}
	if p.Hints[0].FieldPath != "asn" {
		t.Fatalf("hint=%+v", p.Hints[0])
	}
	// Range stays in residual.
	if len(p.Residual.GetFilters()) != 1 || p.Residual.GetFilters()[0].GetOperator() != hydrapb.Relational_GREATER_THAN {
		t.Fatalf("residual=%+v", p.Residual)
	}
}

func TestPlanner_M15_TwoRangeSameField(t *testing.T) {
	if PlanFilter(andG(gt("score", 100), lt("score", 200))).Mode != PlanModeBypass {
		t.Fatalf("two ranges must Bypass in v1")
	}
}

// --- M13: mixed OR (Bypass v1) -------------------------------------------

func TestPlanner_M13_EqualOrRange(t *testing.T) {
	if PlanFilter(orG(eq("asn", int64(10)), gt("score", 100))).Mode != PlanModeBypass {
		t.Fatalf("OR with non-indexable leg must Bypass")
	}
}

// --- M16: NOT_EQUAL — bypass when alone ----------------------------------

func TestPlanner_M16_NotEqualAlone(t *testing.T) {
	if PlanFilter(andG(neq("asn", 10))).Mode != PlanModeBypass {
		t.Fatalf("NOT_EQUAL alone must Bypass")
	}
}

// --- M17: vector / geo / phrase alone -------------------------------------

func TestPlanner_M17_VectorOnly(t *testing.T) {
	g := withVector(&hydrapb.FilterGroup{Logic: hydrapb.FilterLogic_AND})
	if PlanFilter(g).Mode != PlanModeBypass {
		t.Fatalf("vector alone must Bypass")
	}
}

func TestPlanner_M17_GeoOnly(t *testing.T) {
	g := withGeo(&hydrapb.FilterGroup{Logic: hydrapb.FilterLogic_AND})
	if PlanFilter(g).Mode != PlanModeBypass {
		t.Fatalf("geo alone must Bypass")
	}
}

func TestPlanner_M17_PhraseOnly(t *testing.T) {
	g := withPhrase(&hydrapb.FilterGroup{Logic: hydrapb.FilterLogic_AND})
	if PlanFilter(g).Mode != PlanModeBypass {
		t.Fatalf("phrase alone must Bypass")
	}
}

// --- M18: AND + phrase ---------------------------------------------------

func TestPlanner_M18_AsnAndPhrase(t *testing.T) {
	g := withPhrase(andG(eq("asn", int64(10))))
	p := PlanFilter(g)
	if p.Mode != PlanModeAnd {
		t.Fatalf("mode=%v", p.Mode)
	}
	if len(p.Residual.GetPhraseFilters()) != 1 {
		t.Fatalf("phrase residual missing: %+v", p.Residual)
	}
}

func TestPlanner_M18_AsnAndNested(t *testing.T) {
	g := withNested(andG(eq("asn", int64(10))))
	p := PlanFilter(g)
	if p.Mode != PlanModeAnd || len(p.Residual.GetNestedSliceWhereFilters()) != 1 {
		t.Fatalf("plan=%+v", p)
	}
}

// --- M20: AND containing a mixed OR sub-group ----------------------------

func TestPlanner_M20_BypassOrInAnd(t *testing.T) {
	// (asn=10 OR score>5) is a mixed OR (one indexable leg + one
	// range leg), so the inner sub-group itself is Bypass. The outer
	// AND has a directly-indexable leaf (status), so it picks status
	// and the OR sub-group stays in residual untouched.
	sub := orG(eq("asn", int64(10)), gt("score", 5))
	g := andG(eq("status", "ready"))
	withSubGroups(g, sub)
	p := PlanFilter(g)
	if p.Mode != PlanModeAnd {
		t.Fatalf("mode=%v", p.Mode)
	}
	if p.Hints[0].FieldPath != "status" {
		t.Fatalf("hint=%+v", p.Hints[0])
	}
	if len(p.Residual.GetSubGroups()) != 1 {
		t.Fatalf("sub residual missing: %+v", p.Residual)
	}
}

// --- M21: deeply nested with phrase in top-level OR ----------------------

func TestPlanner_M21_DeepNestedBypass(t *testing.T) {
	inner := andG(eq("asn", int64(10)))
	withVector(inner)
	top := withPhrase(orG())
	withSubGroups(top, inner)
	if PlanFilter(top).Mode != PlanModeBypass {
		t.Fatalf("top-level OR with phrase must Bypass")
	}
}

// --- M22: AND with NOT_EQUAL residual ------------------------------------

func TestPlanner_M22_NotResidual(t *testing.T) {
	p := PlanFilter(andG(eq("asn", int64(10)), neq("status", 0)))
	if p.Mode != PlanModeAnd {
		t.Fatalf("mode=%v", p.Mode)
	}
	if len(p.Residual.GetFilters()) != 1 || p.Residual.GetFilters()[0].GetOperator() != hydrapb.Relational_NOT_EQUAL {
		t.Fatalf("residual=%+v", p.Residual)
	}
}

// --- Indexability of missing BytesFieldPath ------------------------------

func TestPlanner_NoBytesFieldPathBypass(t *testing.T) {
	// An Equal filter without BytesFieldPath compares Treasure-level
	// values, not body fields. Not bucket-eligible.
	f := &hydrapb.TreasureFilter{
		Operator:     hydrapb.Relational_EQUAL,
		CompareValue: &hydrapb.TreasureFilter_Int64Val{Int64Val: 1},
	}
	p := PlanFilter(&hydrapb.FilterGroup{
		Logic:   hydrapb.FilterLogic_AND,
		Filters: []*hydrapb.TreasureFilter{f},
	})
	if p.Mode != PlanModeBypass {
		t.Fatalf("missing path must Bypass: %+v", p)
	}
}

func TestPlanner_TimestampValueBypass(t *testing.T) {
	// CreatedAt/UpdatedAt/ExpiredAt timestamp variants are not body
	// fields even with a path; the planner ignores them.
	f := &hydrapb.TreasureFilter{
		Operator:       hydrapb.Relational_EQUAL,
		BytesFieldPath: bpStr("x"),
		CompareValue:   &hydrapb.TreasureFilter_CreatedAtVal{},
	}
	p := PlanFilter(&hydrapb.FilterGroup{
		Logic:   hydrapb.FilterLogic_AND,
		Filters: []*hydrapb.TreasureFilter{f},
	})
	if p.Mode != PlanModeBypass {
		t.Fatalf("timestamp must Bypass: %+v", p)
	}
}
