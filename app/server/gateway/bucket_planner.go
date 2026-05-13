package gateway

import (
	hydrapb "github.com/hydraide/hydraide/sdk/go/hydraidego/v3/hydraidepbgo"
)

// PlanMode is the execution strategy chosen by PlanFilter for a single
// FilterGroup. Bypass means "fall through to the beacon-walk + per-row
// predicate evaluation that exists today"; And and OrUnion route
// through the bucket index and apply a residual predicate to the
// candidate set.
type PlanMode uint8

const (
	PlanModeBypass PlanMode = iota
	PlanModeAnd
	PlanModeOrUnion
)

// HintOp is the operator of a single bucket lookup. v1 supports
// equality and IN; range is reserved for v1.1.
type HintOp uint8

const (
	HintEqual HintOp = iota + 1
	HintIn
)

// BucketHint describes a single bucket lookup. The gateway resolves
// the swamp's bucket for FieldPath and asks it for the union of
// Values (Op==HintIn) or just the single Values[0] (Op==HintEqual).
type BucketHint struct {
	FieldPath string
	Op        HintOp
	Values    []any
}

// Plan is the planner's output. For And, Hints has exactly one entry
// (the selected indexable leg) and Residual is the original group
// with that leg removed. For OrUnion, Hints holds every leg and
// Residual is nil (the union itself is the answer; the gateway still
// applies any global IncludeKeys/ExcludeKeys filters, but no further
// per-treasure predicate is needed).
type Plan struct {
	Mode     PlanMode
	Hints    []BucketHint
	Residual *hydrapb.FilterGroup
}

// PlanFilter analyses a FilterGroup and returns an execution Plan.
// Selection heuristic in v1 is "first indexable leg wins"; second-pass
// selectivity via bucket.CountForValue is a later optimisation.
//
// The function never returns nil; an absent filter yields Bypass.
func PlanFilter(group *hydrapb.FilterGroup) Plan {
	if isEmptyGroup(group) {
		return Plan{Mode: PlanModeBypass}
	}

	logic := group.GetLogic()

	if logic == hydrapb.FilterLogic_AND {
		return planAnd(group)
	}
	return planOr(group)
}

func planAnd(group *hydrapb.FilterGroup) Plan {
	for i, leg := range group.GetFilters() {
		hint, ok := indexableHint(leg)
		if !ok {
			continue
		}
		return Plan{
			Mode:     PlanModeAnd,
			Hints:    []BucketHint{hint},
			Residual: removeFilterAt(group, i),
		}
	}

	// No indexable leaf at this level; a sub-group might collapse to
	// an OR-union we can use as the candidate set. Only the first such
	// sub-group is consumed; the rest stay in the residual.
	for i, sub := range group.GetSubGroups() {
		subPlan := PlanFilter(sub)
		if subPlan.Mode != PlanModeOrUnion {
			continue
		}
		return Plan{
			Mode:     PlanModeAnd,
			Hints:    subPlan.Hints,
			Residual: removeSubGroupAt(group, i),
		}
	}

	return Plan{Mode: PlanModeBypass}
}

func planOr(group *hydrapb.FilterGroup) Plan {
	// Any non-indexable presence under OR forces bypass: phrase /
	// vector / geo / nested-slice / sub-group legs all fail the "every
	// leg indexable" precondition.
	if len(group.GetSubGroups()) > 0 ||
		len(group.GetPhraseFilters()) > 0 ||
		len(group.GetVectorFilters()) > 0 ||
		len(group.GetGeoDistanceFilters()) > 0 ||
		len(group.GetNestedSliceWhereFilters()) > 0 {
		return Plan{Mode: PlanModeBypass}
	}

	hints := make([]BucketHint, 0, len(group.GetFilters()))
	for _, leg := range group.GetFilters() {
		hint, ok := indexableHint(leg)
		if !ok {
			return Plan{Mode: PlanModeBypass}
		}
		hints = append(hints, hint)
	}
	if len(hints) == 0 {
		return Plan{Mode: PlanModeBypass}
	}
	return Plan{Mode: PlanModeOrUnion, Hints: hints, Residual: nil}
}

// isEmptyGroup is true when the group has no filters of any kind. We
// treat nil and "zero-everything" identically.
func isEmptyGroup(g *hydrapb.FilterGroup) bool {
	if g == nil {
		return true
	}
	return len(g.GetFilters()) == 0 &&
		len(g.GetSubGroups()) == 0 &&
		len(g.GetPhraseFilters()) == 0 &&
		len(g.GetVectorFilters()) == 0 &&
		len(g.GetGeoDistanceFilters()) == 0 &&
		len(g.GetNestedSliceWhereFilters()) == 0
}

// indexableHint returns a BucketHint and true when the leg is a body-
// field equality or IN against a simple value type. Returns false for
// every other operator (NOT_EQUAL, range, CONTAINS, IS_EMPTY, etc.),
// for filters without BytesFieldPath, and for filters that compare
// against Treasure metadata (CreatedAt/UpdatedAt/ExpiredAt).
func indexableHint(f *hydrapb.TreasureFilter) (BucketHint, bool) {
	if f == nil {
		return BucketHint{}, false
	}
	path := f.GetBytesFieldPath()
	if path == "" {
		return BucketHint{}, false
	}
	switch f.GetOperator() {
	case hydrapb.Relational_EQUAL:
		v, ok := compareValueToAny(f)
		if !ok {
			return BucketHint{}, false
		}
		return BucketHint{FieldPath: path, Op: HintEqual, Values: []any{v}}, true
	case hydrapb.Relational_STRING_IN:
		if len(f.GetStringInVals()) == 0 {
			return BucketHint{}, false
		}
		vals := make([]any, len(f.GetStringInVals()))
		for i, s := range f.GetStringInVals() {
			vals[i] = s
		}
		return BucketHint{FieldPath: path, Op: HintIn, Values: vals}, true
	case hydrapb.Relational_INT32_IN:
		if len(f.GetInt32InVals()) == 0 {
			return BucketHint{}, false
		}
		vals := make([]any, len(f.GetInt32InVals()))
		for i, n := range f.GetInt32InVals() {
			vals[i] = int64(n)
		}
		return BucketHint{FieldPath: path, Op: HintIn, Values: vals}, true
	case hydrapb.Relational_INT64_IN:
		if len(f.GetInt64InVals()) == 0 {
			return BucketHint{}, false
		}
		vals := make([]any, len(f.GetInt64InVals()))
		for i, n := range f.GetInt64InVals() {
			vals[i] = n
		}
		return BucketHint{FieldPath: path, Op: HintIn, Values: vals}, true
	}
	return BucketHint{}, false
}

// compareValueToAny pulls the set field of the CompareValue oneof and
// returns it as the canonical Go type the bucket expects. Timestamp
// variants are intentionally excluded — they compare against Treasure
// metadata, not body fields.
func compareValueToAny(f *hydrapb.TreasureFilter) (any, bool) {
	switch cv := f.GetCompareValue().(type) {
	case *hydrapb.TreasureFilter_Int8Val:
		return int64(cv.Int8Val), true
	case *hydrapb.TreasureFilter_Int16Val:
		return int64(cv.Int16Val), true
	case *hydrapb.TreasureFilter_Int32Val:
		return int64(cv.Int32Val), true
	case *hydrapb.TreasureFilter_Int64Val:
		return cv.Int64Val, true
	case *hydrapb.TreasureFilter_Uint8Val:
		return uint64(cv.Uint8Val), true
	case *hydrapb.TreasureFilter_Uint16Val:
		return uint64(cv.Uint16Val), true
	case *hydrapb.TreasureFilter_Uint32Val:
		return uint64(cv.Uint32Val), true
	case *hydrapb.TreasureFilter_Uint64Val:
		return cv.Uint64Val, true
	case *hydrapb.TreasureFilter_Float32Val:
		return float64(cv.Float32Val), true
	case *hydrapb.TreasureFilter_Float64Val:
		return cv.Float64Val, true
	case *hydrapb.TreasureFilter_StringVal:
		return cv.StringVal, true
	case *hydrapb.TreasureFilter_BoolVal:
		// Boolean_TRUE = 0, Boolean_FALSE = 1 in the proto enum.
		return cv.BoolVal == hydrapb.Boolean_TRUE, true
	default:
		return nil, false
	}
}

// removeFilterAt returns a shallow copy of group with the i-th
// Filters entry removed. Other slices (SubGroups, PhraseFilters,
// VectorFilters, GeoDistanceFilters, NestedSliceWhereFilters) are
// reference-copied — the residual is read-only from the gateway's
// perspective.
func removeFilterAt(group *hydrapb.FilterGroup, i int) *hydrapb.FilterGroup {
	src := group.GetFilters()
	if i < 0 || i >= len(src) {
		return cloneGroupHeader(group)
	}
	out := cloneGroupHeader(group)
	if len(src) <= 1 {
		out.Filters = nil
	} else {
		out.Filters = make([]*hydrapb.TreasureFilter, 0, len(src)-1)
		out.Filters = append(out.Filters, src[:i]...)
		out.Filters = append(out.Filters, src[i+1:]...)
	}
	return out
}

func removeSubGroupAt(group *hydrapb.FilterGroup, i int) *hydrapb.FilterGroup {
	src := group.GetSubGroups()
	if i < 0 || i >= len(src) {
		return cloneGroupHeader(group)
	}
	out := cloneGroupHeader(group)
	if len(src) <= 1 {
		out.SubGroups = nil
	} else {
		out.SubGroups = make([]*hydrapb.FilterGroup, 0, len(src)-1)
		out.SubGroups = append(out.SubGroups, src[:i]...)
		out.SubGroups = append(out.SubGroups, src[i+1:]...)
	}
	return out
}

func cloneGroupHeader(g *hydrapb.FilterGroup) *hydrapb.FilterGroup {
	return &hydrapb.FilterGroup{
		Logic:                   g.GetLogic(),
		Filters:                 g.GetFilters(),
		SubGroups:               g.GetSubGroups(),
		PhraseFilters:           g.GetPhraseFilters(),
		VectorFilters:           g.GetVectorFilters(),
		GeoDistanceFilters:      g.GetGeoDistanceFilters(),
		NestedSliceWhereFilters: g.GetNestedSliceWhereFilters(),
	}
}
