package gateway

import (
	"testing"
	"time"

	"github.com/hydraide/hydraide/app/core/hydra/swamp/treasure"
	hydrapb "github.com/hydraide/hydraide/generated/hydraidepbgo"
)

// =============================================================================
// Test helpers for NestedSliceWhere / IN filters
// =============================================================================

// makeCampaignTreasure creates a treasure with a CampaignEntries slice
// simulating the real-world use case from the requirements.
func makeCampaignTreasure(t *testing.T, entries []map[string]interface{}) treasure.Treasure {
	t.Helper()
	entrySlice := make([]interface{}, len(entries))
	for i, e := range entries {
		entrySlice[i] = e
	}
	return makeSliceTreasure(t, map[string]interface{}{"CampaignEntries": entrySlice})
}

// makeNestedSliceWhereFilter creates a NestedSliceWhereFilter proto message.
func makeNestedSliceWhereFilter(mode hydrapb.NestedSliceWhereFilter_Mode, slicePath string, conditions *hydrapb.FilterGroup) *hydrapb.NestedSliceWhereFilter {
	return &hydrapb.NestedSliceWhereFilter{
		EvalMode:   mode,
		SlicePath:  slicePath,
		Conditions: conditions,
	}
}

// bytesFieldFilter creates a TreasureFilter with BytesFieldPath set.
func bytesFieldFilter(path string, op hydrapb.Relational_Operator, cv interface{}) *hydrapb.TreasureFilter {
	f := &hydrapb.TreasureFilter{
		Operator:       op,
		BytesFieldPath: &path,
	}
	switch v := cv.(type) {
	case string:
		f.CompareValue = &hydrapb.TreasureFilter_StringVal{StringVal: v}
	case int8:
		f.CompareValue = &hydrapb.TreasureFilter_Int8Val{Int8Val: int32(v)}
	case int32:
		f.CompareValue = &hydrapb.TreasureFilter_Int32Val{Int32Val: v}
	case int64:
		f.CompareValue = &hydrapb.TreasureFilter_Int64Val{Int64Val: v}
	case bool:
		bv := hydrapb.Boolean_FALSE
		if v {
			bv = hydrapb.Boolean_TRUE
		}
		f.CompareValue = &hydrapb.TreasureFilter_BoolVal{BoolVal: bv}
	}
	return f
}

// andConditions wraps filters into an AND FilterGroup.
func andConditions(filters ...*hydrapb.TreasureFilter) *hydrapb.FilterGroup {
	return &hydrapb.FilterGroup{
		Logic:   hydrapb.FilterLogic_AND,
		Filters: filters,
	}
}

// orConditions wraps filters into an OR FilterGroup.
func orConditions(filters ...*hydrapb.TreasureFilter) *hydrapb.FilterGroup {
	return &hydrapb.FilterGroup{
		Logic:   hydrapb.FilterLogic_OR,
		Filters: filters,
	}
}

// =============================================================================
// NestedSliceWhere (ANY mode) tests
// =============================================================================

func TestNestedSliceWhere_BasicMatch(t *testing.T) {
	// 3 entries, one matches all conditions (Status=1, CampaignID="camp-1", NextSendAt=100)
	tr := makeCampaignTreasure(t, []map[string]interface{}{
		{"Status": int8(1), "CampaignID": "camp-1", "NextSendAt": int64(100)},
		{"Status": int8(2), "CampaignID": "camp-2", "NextSendAt": int64(200)},
		{"Status": int8(3), "CampaignID": "camp-3", "NextSendAt": int64(300)},
	})
	nf := makeNestedSliceWhereFilter(hydrapb.NestedSliceWhereFilter_ANY, "CampaignEntries",
		andConditions(
			bytesFieldFilter("Status", hydrapb.Relational_EQUAL, int8(1)),
			bytesFieldFilter("CampaignID", hydrapb.Relational_EQUAL, "camp-1"),
			bytesFieldFilter("NextSendAt", hydrapb.Relational_LESS_THAN_OR_EQUAL, int64(150)),
		),
	)
	fg := &hydrapb.FilterGroup{
		Logic:                    hydrapb.FilterLogic_AND,
		NestedSliceWhereFilters: []*hydrapb.NestedSliceWhereFilter{nf},
	}
	if !evaluateNativeFilterGroup(tr, fg) {
		t.Error("expected WHERE(Status=1, CampaignID=camp-1, NextSendAt<=150) to match first entry")
	}
}

func TestNestedSliceWhere_NoSingleElementMatchesAll(t *testing.T) {
	// Each condition matches a DIFFERENT element — should fail
	tr := makeCampaignTreasure(t, []map[string]interface{}{
		{"Status": int8(1), "CampaignID": "camp-2", "NextSendAt": int64(999)}, // Status=1 but wrong CampaignID
		{"Status": int8(2), "CampaignID": "camp-1", "NextSendAt": int64(999)}, // CampaignID=camp-1 but wrong Status
		{"Status": int8(3), "CampaignID": "camp-3", "NextSendAt": int64(100)}, // NextSendAt<=150 but wrong Status
	})
	nf := makeNestedSliceWhereFilter(hydrapb.NestedSliceWhereFilter_ANY, "CampaignEntries",
		andConditions(
			bytesFieldFilter("Status", hydrapb.Relational_EQUAL, int8(1)),
			bytesFieldFilter("CampaignID", hydrapb.Relational_EQUAL, "camp-1"),
			bytesFieldFilter("NextSendAt", hydrapb.Relational_LESS_THAN_OR_EQUAL, int64(150)),
		),
	)
	fg := &hydrapb.FilterGroup{
		Logic:                    hydrapb.FilterLogic_AND,
		NestedSliceWhereFilters: []*hydrapb.NestedSliceWhereFilter{nf},
	}
	if evaluateNativeFilterGroup(tr, fg) {
		t.Error("expected WHERE to fail when no single element matches all conditions")
	}
}

func TestNestedSliceWhere_EmptySlice(t *testing.T) {
	tr := makeSliceTreasure(t, map[string]interface{}{"CampaignEntries": []interface{}{}})
	nf := makeNestedSliceWhereFilter(hydrapb.NestedSliceWhereFilter_ANY, "CampaignEntries",
		andConditions(bytesFieldFilter("Status", hydrapb.Relational_EQUAL, int8(1))),
	)
	fg := &hydrapb.FilterGroup{
		Logic:                    hydrapb.FilterLogic_AND,
		NestedSliceWhereFilters: []*hydrapb.NestedSliceWhereFilter{nf},
	}
	if evaluateNativeFilterGroup(tr, fg) {
		t.Error("expected WHERE on empty slice to return false")
	}
}

func TestNestedSliceWhere_MissingField(t *testing.T) {
	tr := makeSliceTreasure(t, map[string]interface{}{"OtherField": "value"})
	nf := makeNestedSliceWhereFilter(hydrapb.NestedSliceWhereFilter_ANY, "CampaignEntries",
		andConditions(bytesFieldFilter("Status", hydrapb.Relational_EQUAL, int8(1))),
	)
	fg := &hydrapb.FilterGroup{
		Logic:                    hydrapb.FilterLogic_AND,
		NestedSliceWhereFilters: []*hydrapb.NestedSliceWhereFilter{nf},
	}
	if evaluateNativeFilterGroup(tr, fg) {
		t.Error("expected WHERE on missing slice field to return false")
	}
}

func TestNestedSliceWhere_NilElements(t *testing.T) {
	tr := makeSliceTreasure(t, map[string]interface{}{
		"CampaignEntries": []interface{}{
			nil,
			map[string]interface{}{"Status": int8(1), "CampaignID": "camp-1"},
			nil,
		},
	})
	nf := makeNestedSliceWhereFilter(hydrapb.NestedSliceWhereFilter_ANY, "CampaignEntries",
		andConditions(
			bytesFieldFilter("Status", hydrapb.Relational_EQUAL, int8(1)),
			bytesFieldFilter("CampaignID", hydrapb.Relational_EQUAL, "camp-1"),
		),
	)
	fg := &hydrapb.FilterGroup{
		Logic:                    hydrapb.FilterLogic_AND,
		NestedSliceWhereFilters: []*hydrapb.NestedSliceWhereFilter{nf},
	}
	if !evaluateNativeFilterGroup(tr, fg) {
		t.Error("expected WHERE to skip nil elements and match the valid one")
	}
}

func TestNestedSliceWhere_SingleCondition(t *testing.T) {
	tr := makeCampaignTreasure(t, []map[string]interface{}{
		{"Status": int8(2)},
		{"Status": int8(1)},
	})
	nf := makeNestedSliceWhereFilter(hydrapb.NestedSliceWhereFilter_ANY, "CampaignEntries",
		andConditions(bytesFieldFilter("Status", hydrapb.Relational_EQUAL, int8(1))),
	)
	fg := &hydrapb.FilterGroup{
		Logic:                    hydrapb.FilterLogic_AND,
		NestedSliceWhereFilters: []*hydrapb.NestedSliceWhereFilter{nf},
	}
	if !evaluateNativeFilterGroup(tr, fg) {
		t.Error("expected WHERE with single condition to work like ANY-match")
	}
}

func TestNestedSliceWhere_NestedPath(t *testing.T) {
	tr := makeSliceTreasure(t, map[string]interface{}{
		"Outer": map[string]interface{}{
			"Items": []interface{}{
				map[string]interface{}{"Name": "foo"},
				map[string]interface{}{"Name": "bar"},
			},
		},
	})
	nf := makeNestedSliceWhereFilter(hydrapb.NestedSliceWhereFilter_ANY, "Outer.Items",
		andConditions(bytesFieldFilter("Name", hydrapb.Relational_EQUAL, "bar")),
	)
	fg := &hydrapb.FilterGroup{
		Logic:                    hydrapb.FilterLogic_AND,
		NestedSliceWhereFilters: []*hydrapb.NestedSliceWhereFilter{nf},
	}
	if !evaluateNativeFilterGroup(tr, fg) {
		t.Error("expected WHERE with dot-separated SlicePath to work")
	}
}

func TestNestedSliceWhere_WithFilterOR(t *testing.T) {
	tr := makeCampaignTreasure(t, []map[string]interface{}{
		{"Status": int8(3), "CampaignID": "camp-1"},
	})
	// Status=1 OR Status=3 — the entry has Status=3 so it should match
	nf := makeNestedSliceWhereFilter(hydrapb.NestedSliceWhereFilter_ANY, "CampaignEntries",
		&hydrapb.FilterGroup{
			Logic: hydrapb.FilterLogic_AND,
			Filters: []*hydrapb.TreasureFilter{
				bytesFieldFilter("CampaignID", hydrapb.Relational_EQUAL, "camp-1"),
			},
			SubGroups: []*hydrapb.FilterGroup{
				orConditions(
					bytesFieldFilter("Status", hydrapb.Relational_EQUAL, int8(1)),
					bytesFieldFilter("Status", hydrapb.Relational_EQUAL, int8(3)),
				),
			},
		},
	)
	fg := &hydrapb.FilterGroup{
		Logic:                    hydrapb.FilterLogic_AND,
		NestedSliceWhereFilters: []*hydrapb.NestedSliceWhereFilter{nf},
	}
	if !evaluateNativeFilterGroup(tr, fg) {
		t.Error("expected WHERE with OR sub-condition to match")
	}
}

func TestNestedSliceWhere_WithStringIn(t *testing.T) {
	tr := makeCampaignTreasure(t, []map[string]interface{}{
		{"Status": int8(1), "CampaignID": "camp-2"},
	})
	campaignPath := "CampaignID"
	nf := makeNestedSliceWhereFilter(hydrapb.NestedSliceWhereFilter_ANY, "CampaignEntries",
		andConditions(
			bytesFieldFilter("Status", hydrapb.Relational_EQUAL, int8(1)),
			&hydrapb.TreasureFilter{
				Operator:       hydrapb.Relational_STRING_IN,
				BytesFieldPath: &campaignPath,
				StringInVals:   []string{"camp-1", "camp-2", "camp-3"},
			},
		),
	)
	fg := &hydrapb.FilterGroup{
		Logic:                    hydrapb.FilterLogic_AND,
		NestedSliceWhereFilters: []*hydrapb.NestedSliceWhereFilter{nf},
	}
	if !evaluateNativeFilterGroup(tr, fg) {
		t.Error("expected WHERE with STRING_IN to match camp-2")
	}
}

func TestNestedSliceWhere_ComposedWithTopLevelAND(t *testing.T) {
	tr := makeSliceTreasure(t, map[string]interface{}{
		"Active": true,
		"CampaignEntries": []interface{}{
			map[string]interface{}{"Status": int8(1)},
		},
	})
	activePath := "Active"
	fg := &hydrapb.FilterGroup{
		Logic: hydrapb.FilterLogic_AND,
		Filters: []*hydrapb.TreasureFilter{
			bytesFieldFilter("Active", hydrapb.Relational_EQUAL, true),
		},
		NestedSliceWhereFilters: []*hydrapb.NestedSliceWhereFilter{
			makeNestedSliceWhereFilter(hydrapb.NestedSliceWhereFilter_ANY, "CampaignEntries",
				andConditions(bytesFieldFilter("Status", hydrapb.Relational_EQUAL, int8(1))),
			),
		},
	}
	_ = activePath
	if !evaluateNativeFilterGroup(tr, fg) {
		t.Error("expected top-level AND with NestedSliceWhere to pass")
	}
}

func TestNestedSliceWhere_ComposedWithTopLevelAND_Fail(t *testing.T) {
	tr := makeSliceTreasure(t, map[string]interface{}{
		"Active": false,
		"CampaignEntries": []interface{}{
			map[string]interface{}{"Status": int8(1)},
		},
	})
	fg := &hydrapb.FilterGroup{
		Logic: hydrapb.FilterLogic_AND,
		Filters: []*hydrapb.TreasureFilter{
			bytesFieldFilter("Active", hydrapb.Relational_EQUAL, true),
		},
		NestedSliceWhereFilters: []*hydrapb.NestedSliceWhereFilter{
			makeNestedSliceWhereFilter(hydrapb.NestedSliceWhereFilter_ANY, "CampaignEntries",
				andConditions(bytesFieldFilter("Status", hydrapb.Relational_EQUAL, int8(1))),
			),
		},
	}
	if evaluateNativeFilterGroup(tr, fg) {
		t.Error("expected top-level AND to fail when Active=false")
	}
}

func TestNestedSliceWhere_ComposedWithTopLevelOR(t *testing.T) {
	tr := makeSliceTreasure(t, map[string]interface{}{
		"Active": false,
		"CampaignEntries": []interface{}{
			map[string]interface{}{"Status": int8(1)},
		},
	})
	fg := &hydrapb.FilterGroup{
		Logic: hydrapb.FilterLogic_OR,
		Filters: []*hydrapb.TreasureFilter{
			bytesFieldFilter("Active", hydrapb.Relational_EQUAL, true),
		},
		NestedSliceWhereFilters: []*hydrapb.NestedSliceWhereFilter{
			makeNestedSliceWhereFilter(hydrapb.NestedSliceWhereFilter_ANY, "CampaignEntries",
				andConditions(bytesFieldFilter("Status", hydrapb.Relational_EQUAL, int8(1))),
			),
		},
	}
	if !evaluateNativeFilterGroup(tr, fg) {
		t.Error("expected top-level OR to pass when NestedSliceWhere matches")
	}
}

func TestNestedSliceWhere_LabelTracking(t *testing.T) {
	tr := makeCampaignTreasure(t, []map[string]interface{}{
		{"Status": int8(1)},
	})
	label := "active-campaign"
	nf := makeNestedSliceWhereFilter(hydrapb.NestedSliceWhereFilter_ANY, "CampaignEntries",
		andConditions(bytesFieldFilter("Status", hydrapb.Relational_EQUAL, int8(1))),
	)
	nf.Label = &label
	fg := &hydrapb.FilterGroup{
		Logic:                    hydrapb.FilterLogic_AND,
		NestedSliceWhereFilters: []*hydrapb.NestedSliceWhereFilter{nf},
	}
	matched, meta := evaluateNativeFilterGroupWithMeta(tr, fg)
	if !matched {
		t.Error("expected match")
	}
	if meta == nil || len(meta.matchedLabels) == 0 {
		t.Error("expected label to be tracked")
	} else if meta.matchedLabels[0] != "active-campaign" {
		t.Errorf("expected label 'active-campaign', got %q", meta.matchedLabels[0])
	}
}

func TestNestedSliceWhere_LabelNotTrackedOnFail(t *testing.T) {
	tr := makeCampaignTreasure(t, []map[string]interface{}{
		{"Status": int8(2)},
	})
	label := "active-campaign"
	nf := makeNestedSliceWhereFilter(hydrapb.NestedSliceWhereFilter_ANY, "CampaignEntries",
		andConditions(bytesFieldFilter("Status", hydrapb.Relational_EQUAL, int8(1))),
	)
	nf.Label = &label
	fg := &hydrapb.FilterGroup{
		Logic:                    hydrapb.FilterLogic_AND,
		NestedSliceWhereFilters: []*hydrapb.NestedSliceWhereFilter{nf},
	}
	matched, meta := evaluateNativeFilterGroupWithMeta(tr, fg)
	if matched {
		t.Error("expected no match")
	}
	if meta != nil && len(meta.matchedLabels) > 0 {
		t.Error("expected no labels tracked on failure")
	}
}

// =============================================================================
// NestedSliceAll tests
// =============================================================================

func TestNestedSliceAll_AllMatch(t *testing.T) {
	tr := makeCampaignTreasure(t, []map[string]interface{}{
		{"Status": int8(3)},
		{"Status": int8(3)},
		{"Status": int8(3)},
	})
	nf := makeNestedSliceWhereFilter(hydrapb.NestedSliceWhereFilter_ALL, "CampaignEntries",
		andConditions(bytesFieldFilter("Status", hydrapb.Relational_EQUAL, int8(3))),
	)
	fg := &hydrapb.FilterGroup{
		Logic:                    hydrapb.FilterLogic_AND,
		NestedSliceWhereFilters: []*hydrapb.NestedSliceWhereFilter{nf},
	}
	if !evaluateNativeFilterGroup(tr, fg) {
		t.Error("expected ALL to pass when every element matches")
	}
}

func TestNestedSliceAll_OneDoesNotMatch(t *testing.T) {
	tr := makeCampaignTreasure(t, []map[string]interface{}{
		{"Status": int8(3)},
		{"Status": int8(1)},
		{"Status": int8(3)},
	})
	nf := makeNestedSliceWhereFilter(hydrapb.NestedSliceWhereFilter_ALL, "CampaignEntries",
		andConditions(bytesFieldFilter("Status", hydrapb.Relational_EQUAL, int8(3))),
	)
	fg := &hydrapb.FilterGroup{
		Logic:                    hydrapb.FilterLogic_AND,
		NestedSliceWhereFilters: []*hydrapb.NestedSliceWhereFilter{nf},
	}
	if evaluateNativeFilterGroup(tr, fg) {
		t.Error("expected ALL to fail when one element doesn't match")
	}
}

func TestNestedSliceAll_EmptySlice(t *testing.T) {
	tr := makeSliceTreasure(t, map[string]interface{}{"CampaignEntries": []interface{}{}})
	nf := makeNestedSliceWhereFilter(hydrapb.NestedSliceWhereFilter_ALL, "CampaignEntries",
		andConditions(bytesFieldFilter("Status", hydrapb.Relational_EQUAL, int8(1))),
	)
	fg := &hydrapb.FilterGroup{
		Logic:                    hydrapb.FilterLogic_AND,
		NestedSliceWhereFilters: []*hydrapb.NestedSliceWhereFilter{nf},
	}
	if !evaluateNativeFilterGroup(tr, fg) {
		t.Error("expected ALL on empty slice to return true (vacuous truth)")
	}
}

func TestNestedSliceAll_MissingField(t *testing.T) {
	tr := makeSliceTreasure(t, map[string]interface{}{"OtherField": "value"})
	nf := makeNestedSliceWhereFilter(hydrapb.NestedSliceWhereFilter_ALL, "CampaignEntries",
		andConditions(bytesFieldFilter("Status", hydrapb.Relational_EQUAL, int8(1))),
	)
	fg := &hydrapb.FilterGroup{
		Logic:                    hydrapb.FilterLogic_AND,
		NestedSliceWhereFilters: []*hydrapb.NestedSliceWhereFilter{nf},
	}
	if !evaluateNativeFilterGroup(tr, fg) {
		t.Error("expected ALL on missing field to return true (no elements = vacuous truth)")
	}
}

func TestNestedSliceAll_SingleElement(t *testing.T) {
	tr := makeCampaignTreasure(t, []map[string]interface{}{
		{"Status": int8(1)},
	})
	nf := makeNestedSliceWhereFilter(hydrapb.NestedSliceWhereFilter_ALL, "CampaignEntries",
		andConditions(bytesFieldFilter("Status", hydrapb.Relational_EQUAL, int8(1))),
	)
	fg := &hydrapb.FilterGroup{
		Logic:                    hydrapb.FilterLogic_AND,
		NestedSliceWhereFilters: []*hydrapb.NestedSliceWhereFilter{nf},
	}
	if !evaluateNativeFilterGroup(tr, fg) {
		t.Error("expected ALL with single matching element to pass")
	}
}

func TestNestedSliceAll_SingleElementFails(t *testing.T) {
	tr := makeCampaignTreasure(t, []map[string]interface{}{
		{"Status": int8(2)},
	})
	nf := makeNestedSliceWhereFilter(hydrapb.NestedSliceWhereFilter_ALL, "CampaignEntries",
		andConditions(bytesFieldFilter("Status", hydrapb.Relational_EQUAL, int8(1))),
	)
	fg := &hydrapb.FilterGroup{
		Logic:                    hydrapb.FilterLogic_AND,
		NestedSliceWhereFilters: []*hydrapb.NestedSliceWhereFilter{nf},
	}
	if evaluateNativeFilterGroup(tr, fg) {
		t.Error("expected ALL with single non-matching element to fail")
	}
}

func TestNestedSliceAll_WithComplexConditions(t *testing.T) {
	tr := makeCampaignTreasure(t, []map[string]interface{}{
		{"Status": int8(3), "CampaignID": "camp-1"},
		{"Status": int8(3), "CampaignID": "camp-2"},
	})
	// ALL elements: Status=3 AND (CampaignID=camp-1 OR CampaignID=camp-2)
	nf := makeNestedSliceWhereFilter(hydrapb.NestedSliceWhereFilter_ALL, "CampaignEntries",
		&hydrapb.FilterGroup{
			Logic: hydrapb.FilterLogic_AND,
			Filters: []*hydrapb.TreasureFilter{
				bytesFieldFilter("Status", hydrapb.Relational_EQUAL, int8(3)),
			},
			SubGroups: []*hydrapb.FilterGroup{
				orConditions(
					bytesFieldFilter("CampaignID", hydrapb.Relational_EQUAL, "camp-1"),
					bytesFieldFilter("CampaignID", hydrapb.Relational_EQUAL, "camp-2"),
				),
			},
		},
	)
	fg := &hydrapb.FilterGroup{
		Logic:                    hydrapb.FilterLogic_AND,
		NestedSliceWhereFilters: []*hydrapb.NestedSliceWhereFilter{nf},
	}
	if !evaluateNativeFilterGroup(tr, fg) {
		t.Error("expected ALL with complex conditions to pass")
	}
}

// =============================================================================
// NestedSliceNone tests
// =============================================================================

func TestNestedSliceNone_NoMatch(t *testing.T) {
	tr := makeCampaignTreasure(t, []map[string]interface{}{
		{"Status": int8(2)},
		{"Status": int8(3)},
		{"Status": int8(2)},
	})
	nf := makeNestedSliceWhereFilter(hydrapb.NestedSliceWhereFilter_NONE, "CampaignEntries",
		andConditions(bytesFieldFilter("Status", hydrapb.Relational_EQUAL, int8(1))),
	)
	fg := &hydrapb.FilterGroup{
		Logic:                    hydrapb.FilterLogic_AND,
		NestedSliceWhereFilters: []*hydrapb.NestedSliceWhereFilter{nf},
	}
	if !evaluateNativeFilterGroup(tr, fg) {
		t.Error("expected NONE to pass when no element matches")
	}
}

func TestNestedSliceNone_OneMatches(t *testing.T) {
	tr := makeCampaignTreasure(t, []map[string]interface{}{
		{"Status": int8(2)},
		{"Status": int8(1)},
		{"Status": int8(3)},
	})
	nf := makeNestedSliceWhereFilter(hydrapb.NestedSliceWhereFilter_NONE, "CampaignEntries",
		andConditions(bytesFieldFilter("Status", hydrapb.Relational_EQUAL, int8(1))),
	)
	fg := &hydrapb.FilterGroup{
		Logic:                    hydrapb.FilterLogic_AND,
		NestedSliceWhereFilters: []*hydrapb.NestedSliceWhereFilter{nf},
	}
	if evaluateNativeFilterGroup(tr, fg) {
		t.Error("expected NONE to fail when one element matches")
	}
}

func TestNestedSliceNone_EmptySlice(t *testing.T) {
	tr := makeSliceTreasure(t, map[string]interface{}{"CampaignEntries": []interface{}{}})
	nf := makeNestedSliceWhereFilter(hydrapb.NestedSliceWhereFilter_NONE, "CampaignEntries",
		andConditions(bytesFieldFilter("Status", hydrapb.Relational_EQUAL, int8(1))),
	)
	fg := &hydrapb.FilterGroup{
		Logic:                    hydrapb.FilterLogic_AND,
		NestedSliceWhereFilters: []*hydrapb.NestedSliceWhereFilter{nf},
	}
	if !evaluateNativeFilterGroup(tr, fg) {
		t.Error("expected NONE on empty slice to return true")
	}
}

func TestNestedSliceNone_MissingField(t *testing.T) {
	tr := makeSliceTreasure(t, map[string]interface{}{"OtherField": "value"})
	nf := makeNestedSliceWhereFilter(hydrapb.NestedSliceWhereFilter_NONE, "CampaignEntries",
		andConditions(bytesFieldFilter("Status", hydrapb.Relational_EQUAL, int8(1))),
	)
	fg := &hydrapb.FilterGroup{
		Logic:                    hydrapb.FilterLogic_AND,
		NestedSliceWhereFilters: []*hydrapb.NestedSliceWhereFilter{nf},
	}
	if !evaluateNativeFilterGroup(tr, fg) {
		t.Error("expected NONE on missing field to return true")
	}
}

func TestNestedSliceNone_AllMatch(t *testing.T) {
	tr := makeCampaignTreasure(t, []map[string]interface{}{
		{"Status": int8(1)},
		{"Status": int8(1)},
	})
	nf := makeNestedSliceWhereFilter(hydrapb.NestedSliceWhereFilter_NONE, "CampaignEntries",
		andConditions(bytesFieldFilter("Status", hydrapb.Relational_EQUAL, int8(1))),
	)
	fg := &hydrapb.FilterGroup{
		Logic:                    hydrapb.FilterLogic_AND,
		NestedSliceWhereFilters: []*hydrapb.NestedSliceWhereFilter{nf},
	}
	if evaluateNativeFilterGroup(tr, fg) {
		t.Error("expected NONE to fail when all elements match")
	}
}

// =============================================================================
// NestedSliceCount tests
// =============================================================================

func TestNestedSliceCount_ExactMatch(t *testing.T) {
	tr := makeCampaignTreasure(t, []map[string]interface{}{
		{"Status": int8(1)},
		{"Status": int8(1)},
		{"Status": int8(2)},
	})
	nf := makeNestedSliceWhereFilter(hydrapb.NestedSliceWhereFilter_COUNT, "CampaignEntries",
		andConditions(bytesFieldFilter("Status", hydrapb.Relational_EQUAL, int8(1))),
	)
	nf.CountOperator = hydrapb.Relational_EQUAL
	nf.CountValue = 2
	fg := &hydrapb.FilterGroup{
		Logic:                    hydrapb.FilterLogic_AND,
		NestedSliceWhereFilters: []*hydrapb.NestedSliceWhereFilter{nf},
	}
	if !evaluateNativeFilterGroup(tr, fg) {
		t.Error("expected COUNT==2 to pass when 2 elements match")
	}
}

func TestNestedSliceCount_GreaterThan(t *testing.T) {
	tr := makeCampaignTreasure(t, []map[string]interface{}{
		{"Status": int8(1)},
		{"Status": int8(1)},
		{"Status": int8(2)},
	})
	nf := makeNestedSliceWhereFilter(hydrapb.NestedSliceWhereFilter_COUNT, "CampaignEntries",
		andConditions(bytesFieldFilter("Status", hydrapb.Relational_EQUAL, int8(1))),
	)
	nf.CountOperator = hydrapb.Relational_GREATER_THAN
	nf.CountValue = 1
	fg := &hydrapb.FilterGroup{
		Logic:                    hydrapb.FilterLogic_AND,
		NestedSliceWhereFilters: []*hydrapb.NestedSliceWhereFilter{nf},
	}
	if !evaluateNativeFilterGroup(tr, fg) {
		t.Error("expected COUNT>1 to pass when 2 elements match")
	}
}

func TestNestedSliceCount_LessThan(t *testing.T) {
	tr := makeCampaignTreasure(t, []map[string]interface{}{
		{"Status": int8(1)},
		{"Status": int8(2)},
		{"Status": int8(2)},
	})
	nf := makeNestedSliceWhereFilter(hydrapb.NestedSliceWhereFilter_COUNT, "CampaignEntries",
		andConditions(bytesFieldFilter("Status", hydrapb.Relational_EQUAL, int8(1))),
	)
	nf.CountOperator = hydrapb.Relational_LESS_THAN
	nf.CountValue = 2
	fg := &hydrapb.FilterGroup{
		Logic:                    hydrapb.FilterLogic_AND,
		NestedSliceWhereFilters: []*hydrapb.NestedSliceWhereFilter{nf},
	}
	if !evaluateNativeFilterGroup(tr, fg) {
		t.Error("expected COUNT<2 to pass when 1 element matches")
	}
}

func TestNestedSliceCount_Zero(t *testing.T) {
	tr := makeCampaignTreasure(t, []map[string]interface{}{
		{"Status": int8(2)},
		{"Status": int8(3)},
	})
	nf := makeNestedSliceWhereFilter(hydrapb.NestedSliceWhereFilter_COUNT, "CampaignEntries",
		andConditions(bytesFieldFilter("Status", hydrapb.Relational_EQUAL, int8(1))),
	)
	nf.CountOperator = hydrapb.Relational_EQUAL
	nf.CountValue = 0
	fg := &hydrapb.FilterGroup{
		Logic:                    hydrapb.FilterLogic_AND,
		NestedSliceWhereFilters: []*hydrapb.NestedSliceWhereFilter{nf},
	}
	if !evaluateNativeFilterGroup(tr, fg) {
		t.Error("expected COUNT==0 to pass when no elements match")
	}
}

func TestNestedSliceCount_GreaterThanAll_Fail(t *testing.T) {
	tr := makeCampaignTreasure(t, []map[string]interface{}{
		{"Status": int8(1)},
		{"Status": int8(1)},
		{"Status": int8(1)},
	})
	nf := makeNestedSliceWhereFilter(hydrapb.NestedSliceWhereFilter_COUNT, "CampaignEntries",
		andConditions(bytesFieldFilter("Status", hydrapb.Relational_EQUAL, int8(1))),
	)
	nf.CountOperator = hydrapb.Relational_GREATER_THAN
	nf.CountValue = 3
	fg := &hydrapb.FilterGroup{
		Logic:                    hydrapb.FilterLogic_AND,
		NestedSliceWhereFilters: []*hydrapb.NestedSliceWhereFilter{nf},
	}
	if evaluateNativeFilterGroup(tr, fg) {
		t.Error("expected COUNT>3 to fail when only 3 elements match")
	}
}

func TestNestedSliceCount_EmptySlice_Equal0(t *testing.T) {
	tr := makeSliceTreasure(t, map[string]interface{}{"CampaignEntries": []interface{}{}})
	nf := makeNestedSliceWhereFilter(hydrapb.NestedSliceWhereFilter_COUNT, "CampaignEntries",
		andConditions(bytesFieldFilter("Status", hydrapb.Relational_EQUAL, int8(1))),
	)
	nf.CountOperator = hydrapb.Relational_EQUAL
	nf.CountValue = 0
	fg := &hydrapb.FilterGroup{
		Logic:                    hydrapb.FilterLogic_AND,
		NestedSliceWhereFilters: []*hydrapb.NestedSliceWhereFilter{nf},
	}
	if !evaluateNativeFilterGroup(tr, fg) {
		t.Error("expected COUNT==0 on empty slice to pass")
	}
}

func TestNestedSliceCount_EmptySlice_GT0_Fail(t *testing.T) {
	tr := makeSliceTreasure(t, map[string]interface{}{"CampaignEntries": []interface{}{}})
	nf := makeNestedSliceWhereFilter(hydrapb.NestedSliceWhereFilter_COUNT, "CampaignEntries",
		andConditions(bytesFieldFilter("Status", hydrapb.Relational_EQUAL, int8(1))),
	)
	nf.CountOperator = hydrapb.Relational_GREATER_THAN
	nf.CountValue = 0
	fg := &hydrapb.FilterGroup{
		Logic:                    hydrapb.FilterLogic_AND,
		NestedSliceWhereFilters: []*hydrapb.NestedSliceWhereFilter{nf},
	}
	if evaluateNativeFilterGroup(tr, fg) {
		t.Error("expected COUNT>0 on empty slice to fail")
	}
}

// =============================================================================
// STRING_IN tests
// =============================================================================

func TestStringIn_Match(t *testing.T) {
	tr := makeSliceTreasure(t, map[string]interface{}{"CampaignID": "camp-2"})
	path := "CampaignID"
	f := &hydrapb.TreasureFilter{
		Operator:       hydrapb.Relational_STRING_IN,
		BytesFieldPath: &path,
		StringInVals:   []string{"camp-1", "camp-2", "camp-3"},
	}
	fg := &hydrapb.FilterGroup{Logic: hydrapb.FilterLogic_AND, Filters: []*hydrapb.TreasureFilter{f}}
	if !evaluateNativeFilterGroup(tr, fg) {
		t.Error("expected STRING_IN to match camp-2")
	}
}

func TestStringIn_NoMatch(t *testing.T) {
	tr := makeSliceTreasure(t, map[string]interface{}{"CampaignID": "camp-99"})
	path := "CampaignID"
	f := &hydrapb.TreasureFilter{
		Operator:       hydrapb.Relational_STRING_IN,
		BytesFieldPath: &path,
		StringInVals:   []string{"camp-1", "camp-2", "camp-3"},
	}
	fg := &hydrapb.FilterGroup{Logic: hydrapb.FilterLogic_AND, Filters: []*hydrapb.TreasureFilter{f}}
	if evaluateNativeFilterGroup(tr, fg) {
		t.Error("expected STRING_IN to not match camp-99")
	}
}

func TestStringIn_EmptyVals(t *testing.T) {
	tr := makeSliceTreasure(t, map[string]interface{}{"CampaignID": "camp-1"})
	path := "CampaignID"
	f := &hydrapb.TreasureFilter{
		Operator:       hydrapb.Relational_STRING_IN,
		BytesFieldPath: &path,
		StringInVals:   []string{},
	}
	fg := &hydrapb.FilterGroup{Logic: hydrapb.FilterLogic_AND, Filters: []*hydrapb.TreasureFilter{f}}
	if evaluateNativeFilterGroup(tr, fg) {
		t.Error("expected STRING_IN with empty vals to return false")
	}
}

func TestStringIn_NilField(t *testing.T) {
	tr := makeSliceTreasure(t, map[string]interface{}{"OtherField": "value"})
	path := "CampaignID"
	f := &hydrapb.TreasureFilter{
		Operator:       hydrapb.Relational_STRING_IN,
		BytesFieldPath: &path,
		StringInVals:   []string{"camp-1"},
	}
	fg := &hydrapb.FilterGroup{Logic: hydrapb.FilterLogic_AND, Filters: []*hydrapb.TreasureFilter{f}}
	if evaluateNativeFilterGroup(tr, fg) {
		t.Error("expected STRING_IN on missing field to return false")
	}
}

func TestStringIn_NonStringField(t *testing.T) {
	tr := makeSliceTreasure(t, map[string]interface{}{"Status": int8(1)})
	path := "Status"
	f := &hydrapb.TreasureFilter{
		Operator:       hydrapb.Relational_STRING_IN,
		BytesFieldPath: &path,
		StringInVals:   []string{"1"},
	}
	fg := &hydrapb.FilterGroup{Logic: hydrapb.FilterLogic_AND, Filters: []*hydrapb.TreasureFilter{f}}
	if evaluateNativeFilterGroup(tr, fg) {
		t.Error("expected STRING_IN on int field to return false")
	}
}

func TestStringIn_SingleValue(t *testing.T) {
	tr := makeSliceTreasure(t, map[string]interface{}{"CampaignID": "camp-1"})
	path := "CampaignID"
	f := &hydrapb.TreasureFilter{
		Operator:       hydrapb.Relational_STRING_IN,
		BytesFieldPath: &path,
		StringInVals:   []string{"camp-1"},
	}
	fg := &hydrapb.FilterGroup{Logic: hydrapb.FilterLogic_AND, Filters: []*hydrapb.TreasureFilter{f}}
	if !evaluateNativeFilterGroup(tr, fg) {
		t.Error("expected STRING_IN with single value to match")
	}
}

func TestStringIn_WithWildcard(t *testing.T) {
	tr := makeSliceTreasure(t, map[string]interface{}{
		"Items": []interface{}{
			map[string]interface{}{"ID": "a"},
			map[string]interface{}{"ID": "b"},
			map[string]interface{}{"ID": "c"},
		},
	})
	path := "Items[*].ID"
	f := &hydrapb.TreasureFilter{
		Operator:       hydrapb.Relational_STRING_IN,
		BytesFieldPath: &path,
		StringInVals:   []string{"b", "d"},
	}
	fg := &hydrapb.FilterGroup{Logic: hydrapb.FilterLogic_AND, Filters: []*hydrapb.TreasureFilter{f}}
	if !evaluateNativeFilterGroup(tr, fg) {
		t.Error("expected STRING_IN with [*] wildcard to match 'b'")
	}
}

func TestStringIn_WithWildcard_NoMatch(t *testing.T) {
	tr := makeSliceTreasure(t, map[string]interface{}{
		"Items": []interface{}{
			map[string]interface{}{"ID": "a"},
			map[string]interface{}{"ID": "c"},
		},
	})
	path := "Items[*].ID"
	f := &hydrapb.TreasureFilter{
		Operator:       hydrapb.Relational_STRING_IN,
		BytesFieldPath: &path,
		StringInVals:   []string{"b", "d"},
	}
	fg := &hydrapb.FilterGroup{Logic: hydrapb.FilterLogic_AND, Filters: []*hydrapb.TreasureFilter{f}}
	if evaluateNativeFilterGroup(tr, fg) {
		t.Error("expected STRING_IN with [*] wildcard to not match")
	}
}

// =============================================================================
// INT32_IN tests
// =============================================================================

func TestInt32In_Match(t *testing.T) {
	tr := makeSliceTreasure(t, map[string]interface{}{"Status": int8(5)})
	path := "Status"
	f := &hydrapb.TreasureFilter{
		Operator:       hydrapb.Relational_INT32_IN,
		BytesFieldPath: &path,
		Int32InVals:    []int32{3, 5, 7},
	}
	fg := &hydrapb.FilterGroup{Logic: hydrapb.FilterLogic_AND, Filters: []*hydrapb.TreasureFilter{f}}
	if !evaluateNativeFilterGroup(tr, fg) {
		t.Error("expected INT32_IN to match 5")
	}
}

func TestInt32In_NoMatch(t *testing.T) {
	tr := makeSliceTreasure(t, map[string]interface{}{"Status": int8(9)})
	path := "Status"
	f := &hydrapb.TreasureFilter{
		Operator:       hydrapb.Relational_INT32_IN,
		BytesFieldPath: &path,
		Int32InVals:    []int32{3, 5, 7},
	}
	fg := &hydrapb.FilterGroup{Logic: hydrapb.FilterLogic_AND, Filters: []*hydrapb.TreasureFilter{f}}
	if evaluateNativeFilterGroup(tr, fg) {
		t.Error("expected INT32_IN to not match 9")
	}
}

func TestInt32In_EmptyVals(t *testing.T) {
	tr := makeSliceTreasure(t, map[string]interface{}{"Status": int8(5)})
	path := "Status"
	f := &hydrapb.TreasureFilter{
		Operator:       hydrapb.Relational_INT32_IN,
		BytesFieldPath: &path,
		Int32InVals:    []int32{},
	}
	fg := &hydrapb.FilterGroup{Logic: hydrapb.FilterLogic_AND, Filters: []*hydrapb.TreasureFilter{f}}
	if evaluateNativeFilterGroup(tr, fg) {
		t.Error("expected INT32_IN with empty vals to return false")
	}
}

func TestInt32In_NilField(t *testing.T) {
	tr := makeSliceTreasure(t, map[string]interface{}{"OtherField": "value"})
	path := "Status"
	f := &hydrapb.TreasureFilter{
		Operator:       hydrapb.Relational_INT32_IN,
		BytesFieldPath: &path,
		Int32InVals:    []int32{1, 2},
	}
	fg := &hydrapb.FilterGroup{Logic: hydrapb.FilterLogic_AND, Filters: []*hydrapb.TreasureFilter{f}}
	if evaluateNativeFilterGroup(tr, fg) {
		t.Error("expected INT32_IN on missing field to return false")
	}
}

// =============================================================================
// INT64_IN tests
// =============================================================================

func TestInt64In_Match(t *testing.T) {
	tr := makeSliceTreasure(t, map[string]interface{}{"Timestamp": int64(1000)})
	path := "Timestamp"
	f := &hydrapb.TreasureFilter{
		Operator:       hydrapb.Relational_INT64_IN,
		BytesFieldPath: &path,
		Int64InVals:    []int64{999, 1000, 1001},
	}
	fg := &hydrapb.FilterGroup{Logic: hydrapb.FilterLogic_AND, Filters: []*hydrapb.TreasureFilter{f}}
	if !evaluateNativeFilterGroup(tr, fg) {
		t.Error("expected INT64_IN to match 1000")
	}
}

func TestInt64In_NoMatch(t *testing.T) {
	tr := makeSliceTreasure(t, map[string]interface{}{"Timestamp": int64(500)})
	path := "Timestamp"
	f := &hydrapb.TreasureFilter{
		Operator:       hydrapb.Relational_INT64_IN,
		BytesFieldPath: &path,
		Int64InVals:    []int64{999, 1000, 1001},
	}
	fg := &hydrapb.FilterGroup{Logic: hydrapb.FilterLogic_AND, Filters: []*hydrapb.TreasureFilter{f}}
	if evaluateNativeFilterGroup(tr, fg) {
		t.Error("expected INT64_IN to not match 500")
	}
}

func TestInt64In_WithTime(t *testing.T) {
	// time.Time stored as Unix seconds (int64)
	unixSec := int64(1712534400) // 2024-04-08 00:00:00 UTC
	tr := makeSliceTreasure(t, map[string]interface{}{"NextSendAt": unixSec})
	path := "NextSendAt"
	f := &hydrapb.TreasureFilter{
		Operator:       hydrapb.Relational_INT64_IN,
		BytesFieldPath: &path,
		Int64InVals:    []int64{1712534400, 1712620800},
	}
	fg := &hydrapb.FilterGroup{Logic: hydrapb.FilterLogic_AND, Filters: []*hydrapb.TreasureFilter{f}}
	if !evaluateNativeFilterGroup(tr, fg) {
		t.Error("expected INT64_IN to match Unix timestamp")
	}
}

// =============================================================================
// Integration tests — real-world scenarios
// =============================================================================

func TestIntegration_WorkerQuery(t *testing.T) {
	// Worker query: Status=1 AND CampaignID∈{camp-1,camp-2} AND NextSendAt<=150 AND NextSendAt>0
	tr := makeSliceTreasure(t, map[string]interface{}{
		"CampaignEntries": []interface{}{
			map[string]interface{}{
				"Status":     int8(2),
				"CampaignID": "camp-1",
				"NextSendAt": int64(100),
			},
			map[string]interface{}{
				"Status":     int8(1),
				"CampaignID": "camp-2",
				"NextSendAt": int64(120),
			},
			map[string]interface{}{
				"Status":     int8(1),
				"CampaignID": "camp-3",
				"NextSendAt": int64(50),
			},
		},
	})

	campaignPath := "CampaignID"
	nf := makeNestedSliceWhereFilter(hydrapb.NestedSliceWhereFilter_ANY, "CampaignEntries",
		&hydrapb.FilterGroup{
			Logic: hydrapb.FilterLogic_AND,
			Filters: []*hydrapb.TreasureFilter{
				bytesFieldFilter("Status", hydrapb.Relational_EQUAL, int8(1)),
				{
					Operator:       hydrapb.Relational_STRING_IN,
					BytesFieldPath: &campaignPath,
					StringInVals:   []string{"camp-1", "camp-2"},
				},
				bytesFieldFilter("NextSendAt", hydrapb.Relational_LESS_THAN_OR_EQUAL, int64(150)),
				bytesFieldFilter("NextSendAt", hydrapb.Relational_GREATER_THAN, int64(0)),
			},
		},
	)
	fg := &hydrapb.FilterGroup{
		Logic:                    hydrapb.FilterLogic_AND,
		NestedSliceWhereFilters: []*hydrapb.NestedSliceWhereFilter{nf},
	}
	if !evaluateNativeFilterGroup(tr, fg) {
		t.Error("expected worker query to match entry[1]: Status=1, CampaignID=camp-2, NextSendAt=120")
	}
}

func TestIntegration_WorkerQuery_NoMatch(t *testing.T) {
	// No entry satisfies all conditions at once
	tr := makeSliceTreasure(t, map[string]interface{}{
		"CampaignEntries": []interface{}{
			map[string]interface{}{
				"Status":     int8(1),
				"CampaignID": "camp-3", // not in IN set
				"NextSendAt": int64(100),
			},
			map[string]interface{}{
				"Status":     int8(2), // not Active
				"CampaignID": "camp-1",
				"NextSendAt": int64(100),
			},
		},
	})

	campaignPath := "CampaignID"
	nf := makeNestedSliceWhereFilter(hydrapb.NestedSliceWhereFilter_ANY, "CampaignEntries",
		&hydrapb.FilterGroup{
			Logic: hydrapb.FilterLogic_AND,
			Filters: []*hydrapb.TreasureFilter{
				bytesFieldFilter("Status", hydrapb.Relational_EQUAL, int8(1)),
				{
					Operator:       hydrapb.Relational_STRING_IN,
					BytesFieldPath: &campaignPath,
					StringInVals:   []string{"camp-1", "camp-2"},
				},
				bytesFieldFilter("NextSendAt", hydrapb.Relational_LESS_THAN_OR_EQUAL, int64(150)),
				bytesFieldFilter("NextSendAt", hydrapb.Relational_GREATER_THAN, int64(0)),
			},
		},
	)
	fg := &hydrapb.FilterGroup{
		Logic:                    hydrapb.FilterLogic_AND,
		NestedSliceWhereFilters: []*hydrapb.NestedSliceWhereFilter{nf},
	}
	if evaluateNativeFilterGroup(tr, fg) {
		t.Error("expected worker query to fail when no entry satisfies all conditions")
	}
}

func TestIntegration_DashboardCount(t *testing.T) {
	// Dashboard: count active domains in campaign-123
	tr := makeSliceTreasure(t, map[string]interface{}{
		"CampaignEntries": []interface{}{
			map[string]interface{}{"Status": int8(1), "CampaignID": "campaign-123"},
			map[string]interface{}{"Status": int8(1), "CampaignID": "campaign-456"},
			map[string]interface{}{"Status": int8(3), "CampaignID": "campaign-123"},
		},
	})

	// WHERE: at least one active entry for campaign-123
	nf := makeNestedSliceWhereFilter(hydrapb.NestedSliceWhereFilter_ANY, "CampaignEntries",
		andConditions(
			bytesFieldFilter("Status", hydrapb.Relational_EQUAL, int8(1)),
			bytesFieldFilter("CampaignID", hydrapb.Relational_EQUAL, "campaign-123"),
		),
	)
	fg := &hydrapb.FilterGroup{
		Logic:                    hydrapb.FilterLogic_AND,
		NestedSliceWhereFilters: []*hydrapb.NestedSliceWhereFilter{nf},
	}
	if !evaluateNativeFilterGroup(tr, fg) {
		t.Error("expected dashboard query to find active entry in campaign-123")
	}
}

func TestIntegration_NestedSliceWhereInFilterAND(t *testing.T) {
	// Combine top-level bytes filter with NestedSliceWhere
	tr := makeSliceTreasure(t, map[string]interface{}{
		"DomainName": "example.com",
		"CampaignEntries": []interface{}{
			map[string]interface{}{"Status": int8(1), "CampaignID": "camp-1"},
		},
	})
	fg := &hydrapb.FilterGroup{
		Logic: hydrapb.FilterLogic_AND,
		Filters: []*hydrapb.TreasureFilter{
			bytesFieldFilter("DomainName", hydrapb.Relational_CONTAINS, "example"),
		},
		NestedSliceWhereFilters: []*hydrapb.NestedSliceWhereFilter{
			makeNestedSliceWhereFilter(hydrapb.NestedSliceWhereFilter_ANY, "CampaignEntries",
				andConditions(
					bytesFieldFilter("Status", hydrapb.Relational_EQUAL, int8(1)),
					bytesFieldFilter("CampaignID", hydrapb.Relational_EQUAL, "camp-1"),
				),
			),
		},
	}
	if !evaluateNativeFilterGroup(tr, fg) {
		t.Error("expected combined AND of bytes filter + NestedSliceWhere to pass")
	}
}

// =============================================================================
// hasAnyLabels tests for NestedSliceWhereFilter
// =============================================================================

func TestHasAnyLabels_NestedSliceWhereWithLabel(t *testing.T) {
	label := "my-label"
	fg := &hydrapb.FilterGroup{
		NestedSliceWhereFilters: []*hydrapb.NestedSliceWhereFilter{
			{SlicePath: "Items", Label: &label},
		},
	}
	if !hasAnyLabels(fg) {
		t.Error("expected hasAnyLabels to return true when NestedSliceWhereFilter has label")
	}
}

func TestHasAnyLabels_NestedSliceWhereWithoutLabel(t *testing.T) {
	fg := &hydrapb.FilterGroup{
		NestedSliceWhereFilters: []*hydrapb.NestedSliceWhereFilter{
			{SlicePath: "Items"},
		},
	}
	if hasAnyLabels(fg) {
		t.Error("expected hasAnyLabels to return false when NestedSliceWhereFilter has no label")
	}
}

// =============================================================================
// No conditions edge case
// =============================================================================

func TestNestedSliceWhere_NoConditions(t *testing.T) {
	tr := makeCampaignTreasure(t, []map[string]interface{}{
		{"Status": int8(1)},
	})
	// No conditions = every element matches
	nf := makeNestedSliceWhereFilter(hydrapb.NestedSliceWhereFilter_ANY, "CampaignEntries", nil)
	fg := &hydrapb.FilterGroup{
		Logic:                    hydrapb.FilterLogic_AND,
		NestedSliceWhereFilters: []*hydrapb.NestedSliceWhereFilter{nf},
	}
	if !evaluateNativeFilterGroup(tr, fg) {
		t.Error("expected WHERE with no conditions to pass (all elements match vacuously)")
	}
}

func TestNestedSliceNone_NoConditions(t *testing.T) {
	tr := makeCampaignTreasure(t, []map[string]interface{}{
		{"Status": int8(1)},
	})
	// No conditions = every element matches → NONE should be false
	nf := makeNestedSliceWhereFilter(hydrapb.NestedSliceWhereFilter_NONE, "CampaignEntries", nil)
	fg := &hydrapb.FilterGroup{
		Logic:                    hydrapb.FilterLogic_AND,
		NestedSliceWhereFilters: []*hydrapb.NestedSliceWhereFilter{nf},
	}
	if evaluateNativeFilterGroup(tr, fg) {
		t.Error("expected NONE with no conditions to fail (all elements match vacuously)")
	}
}

// =============================================================================
// Nested NestedSliceWhere — slice-within-slice
// =============================================================================

func TestNestedSliceWhere_NestedSliceWithinSlice(t *testing.T) {
	// CampaignEntries[].SentTemplates[] — nested slice inside a nested slice
	// We use [*] wildcard inside the conditions to check SentTemplates
	tr := makeSliceTreasure(t, map[string]interface{}{
		"CampaignEntries": []interface{}{
			map[string]interface{}{
				"Status":     int8(1),
				"CampaignID": "camp-1",
				"SentTemplates": []interface{}{
					map[string]interface{}{"TemplateID": "tmpl-A", "SentAt": int64(100)},
					map[string]interface{}{"TemplateID": "tmpl-B", "SentAt": int64(200)},
				},
			},
			map[string]interface{}{
				"Status":        int8(2),
				"CampaignID":    "camp-2",
				"SentTemplates": []interface{}{},
			},
		},
	})

	// Find a CampaignEntry that is Active AND has SentTemplates with tmpl-B
	sentTemplatePath := "SentTemplates[*].TemplateID"
	nf := makeNestedSliceWhereFilter(hydrapb.NestedSliceWhereFilter_ANY, "CampaignEntries",
		andConditions(
			bytesFieldFilter("Status", hydrapb.Relational_EQUAL, int8(1)),
			&hydrapb.TreasureFilter{
				Operator:       hydrapb.Relational_EQUAL,
				BytesFieldPath: &sentTemplatePath,
				CompareValue:   &hydrapb.TreasureFilter_StringVal{StringVal: "tmpl-B"},
			},
		),
	)
	fg := &hydrapb.FilterGroup{
		Logic:                    hydrapb.FilterLogic_AND,
		NestedSliceWhereFilters: []*hydrapb.NestedSliceWhereFilter{nf},
	}
	if !evaluateNativeFilterGroup(tr, fg) {
		t.Error("expected nested slice-within-slice to find Active entry with tmpl-B")
	}
}

func TestNestedSliceWhere_NestedSliceWithinSlice_NoMatch(t *testing.T) {
	tr := makeSliceTreasure(t, map[string]interface{}{
		"CampaignEntries": []interface{}{
			map[string]interface{}{
				"Status":     int8(1),
				"CampaignID": "camp-1",
				"SentTemplates": []interface{}{
					map[string]interface{}{"TemplateID": "tmpl-A"},
				},
			},
			map[string]interface{}{
				"Status":     int8(2),
				"CampaignID": "camp-2",
				"SentTemplates": []interface{}{
					map[string]interface{}{"TemplateID": "tmpl-B"}, // has tmpl-B but Status=2
				},
			},
		},
	})

	// Active AND has tmpl-B — entry[0] is Active but has tmpl-A, entry[1] has tmpl-B but is not Active
	sentTemplatePath := "SentTemplates[*].TemplateID"
	nf := makeNestedSliceWhereFilter(hydrapb.NestedSliceWhereFilter_ANY, "CampaignEntries",
		andConditions(
			bytesFieldFilter("Status", hydrapb.Relational_EQUAL, int8(1)),
			&hydrapb.TreasureFilter{
				Operator:       hydrapb.Relational_EQUAL,
				BytesFieldPath: &sentTemplatePath,
				CompareValue:   &hydrapb.TreasureFilter_StringVal{StringVal: "tmpl-B"},
			},
		),
	)
	fg := &hydrapb.FilterGroup{
		Logic:                    hydrapb.FilterLogic_AND,
		NestedSliceWhereFilters: []*hydrapb.NestedSliceWhereFilter{nf},
	}
	if evaluateNativeFilterGroup(tr, fg) {
		t.Error("expected nested slice-within-slice to fail: no single entry is both Active and has tmpl-B")
	}
}

// =============================================================================
// INT32_IN / INT64_IN with [*] wildcard
// =============================================================================

func TestInt32In_WithWildcard(t *testing.T) {
	tr := makeSliceTreasure(t, map[string]interface{}{
		"Items": []interface{}{
			map[string]interface{}{"Status": int8(1)},
			map[string]interface{}{"Status": int8(5)},
			map[string]interface{}{"Status": int8(9)},
		},
	})
	path := "Items[*].Status"
	f := &hydrapb.TreasureFilter{
		Operator:       hydrapb.Relational_INT32_IN,
		BytesFieldPath: &path,
		Int32InVals:    []int32{5, 7},
	}
	fg := &hydrapb.FilterGroup{Logic: hydrapb.FilterLogic_AND, Filters: []*hydrapb.TreasureFilter{f}}
	if !evaluateNativeFilterGroup(tr, fg) {
		t.Error("expected INT32_IN with [*] wildcard to match Status=5")
	}
}

func TestInt32In_WithWildcard_NoMatch(t *testing.T) {
	tr := makeSliceTreasure(t, map[string]interface{}{
		"Items": []interface{}{
			map[string]interface{}{"Status": int8(1)},
			map[string]interface{}{"Status": int8(3)},
		},
	})
	path := "Items[*].Status"
	f := &hydrapb.TreasureFilter{
		Operator:       hydrapb.Relational_INT32_IN,
		BytesFieldPath: &path,
		Int32InVals:    []int32{5, 7},
	}
	fg := &hydrapb.FilterGroup{Logic: hydrapb.FilterLogic_AND, Filters: []*hydrapb.TreasureFilter{f}}
	if evaluateNativeFilterGroup(tr, fg) {
		t.Error("expected INT32_IN with [*] wildcard to not match")
	}
}

func TestInt64In_WithWildcard(t *testing.T) {
	tr := makeSliceTreasure(t, map[string]interface{}{
		"Events": []interface{}{
			map[string]interface{}{"Timestamp": int64(100)},
			map[string]interface{}{"Timestamp": int64(200)},
			map[string]interface{}{"Timestamp": int64(300)},
		},
	})
	path := "Events[*].Timestamp"
	f := &hydrapb.TreasureFilter{
		Operator:       hydrapb.Relational_INT64_IN,
		BytesFieldPath: &path,
		Int64InVals:    []int64{200, 400},
	}
	fg := &hydrapb.FilterGroup{Logic: hydrapb.FilterLogic_AND, Filters: []*hydrapb.TreasureFilter{f}}
	if !evaluateNativeFilterGroup(tr, fg) {
		t.Error("expected INT64_IN with [*] wildcard to match Timestamp=200")
	}
}

// =============================================================================
// NestedSliceCount with multiple conditions
// =============================================================================

func TestNestedSliceCount_MultipleConditions(t *testing.T) {
	// Count entries where Status=1 AND CampaignID starts with "camp-"
	tr := makeCampaignTreasure(t, []map[string]interface{}{
		{"Status": int8(1), "CampaignID": "camp-abc"},
		{"Status": int8(1), "CampaignID": "other-xyz"}, // doesn't start with camp-
		{"Status": int8(2), "CampaignID": "camp-def"},   // wrong status
		{"Status": int8(1), "CampaignID": "camp-ghi"},
	})
	nf := makeNestedSliceWhereFilter(hydrapb.NestedSliceWhereFilter_COUNT, "CampaignEntries",
		andConditions(
			bytesFieldFilter("Status", hydrapb.Relational_EQUAL, int8(1)),
			bytesFieldFilter("CampaignID", hydrapb.Relational_STARTS_WITH, "camp-"),
		),
	)
	nf.CountOperator = hydrapb.Relational_EQUAL
	nf.CountValue = 2 // camp-abc and camp-ghi
	fg := &hydrapb.FilterGroup{
		Logic:                    hydrapb.FilterLogic_AND,
		NestedSliceWhereFilters: []*hydrapb.NestedSliceWhereFilter{nf},
	}
	if !evaluateNativeFilterGroup(tr, fg) {
		t.Error("expected COUNT==2 for entries matching Status=1 AND CampaignID starts with camp-")
	}
}

func TestNestedSliceCount_GreaterThanOrEqual(t *testing.T) {
	tr := makeCampaignTreasure(t, []map[string]interface{}{
		{"Status": int8(1)},
		{"Status": int8(1)},
		{"Status": int8(1)},
	})
	nf := makeNestedSliceWhereFilter(hydrapb.NestedSliceWhereFilter_COUNT, "CampaignEntries",
		andConditions(bytesFieldFilter("Status", hydrapb.Relational_EQUAL, int8(1))),
	)
	nf.CountOperator = hydrapb.Relational_GREATER_THAN_OR_EQUAL
	nf.CountValue = 3
	fg := &hydrapb.FilterGroup{
		Logic:                    hydrapb.FilterLogic_AND,
		NestedSliceWhereFilters: []*hydrapb.NestedSliceWhereFilter{nf},
	}
	if !evaluateNativeFilterGroup(tr, fg) {
		t.Error("expected COUNT>=3 to pass when exactly 3 match")
	}
}

// =============================================================================
// Multiple NestedSliceWhereFilters in same FilterGroup
// =============================================================================

func TestMultipleNestedSliceWhere_AND(t *testing.T) {
	// Two independent NestedSliceWhere filters AND-ed together
	tr := makeSliceTreasure(t, map[string]interface{}{
		"CampaignEntries": []interface{}{
			map[string]interface{}{"Status": int8(1), "CampaignID": "camp-1"},
		},
		"Tags": []interface{}{
			map[string]interface{}{"Name": "priority", "Value": "high"},
		},
	})
	fg := &hydrapb.FilterGroup{
		Logic: hydrapb.FilterLogic_AND,
		NestedSliceWhereFilters: []*hydrapb.NestedSliceWhereFilter{
			makeNestedSliceWhereFilter(hydrapb.NestedSliceWhereFilter_ANY, "CampaignEntries",
				andConditions(bytesFieldFilter("Status", hydrapb.Relational_EQUAL, int8(1))),
			),
			makeNestedSliceWhereFilter(hydrapb.NestedSliceWhereFilter_ANY, "Tags",
				andConditions(bytesFieldFilter("Value", hydrapb.Relational_EQUAL, "high")),
			),
		},
	}
	if !evaluateNativeFilterGroup(tr, fg) {
		t.Error("expected two AND-ed NestedSliceWhere filters to both pass")
	}
}

func TestMultipleNestedSliceWhere_AND_OneFails(t *testing.T) {
	tr := makeSliceTreasure(t, map[string]interface{}{
		"CampaignEntries": []interface{}{
			map[string]interface{}{"Status": int8(1)},
		},
		"Tags": []interface{}{
			map[string]interface{}{"Name": "priority", "Value": "low"},
		},
	})
	fg := &hydrapb.FilterGroup{
		Logic: hydrapb.FilterLogic_AND,
		NestedSliceWhereFilters: []*hydrapb.NestedSliceWhereFilter{
			makeNestedSliceWhereFilter(hydrapb.NestedSliceWhereFilter_ANY, "CampaignEntries",
				andConditions(bytesFieldFilter("Status", hydrapb.Relational_EQUAL, int8(1))),
			),
			makeNestedSliceWhereFilter(hydrapb.NestedSliceWhereFilter_ANY, "Tags",
				andConditions(bytesFieldFilter("Value", hydrapb.Relational_EQUAL, "high")),
			),
		},
	}
	if evaluateNativeFilterGroup(tr, fg) {
		t.Error("expected AND to fail when second NestedSliceWhere doesn't match")
	}
}

// =============================================================================
// SDK → Proto conversion tests
// =============================================================================

func TestSDK_FilterNestedSliceWhere_ProtoConversion(t *testing.T) {
	// Use the SDK to build a filter, convert to proto, verify structure
	sdkFilter := filterNestedSliceWhereSDK("CampaignEntries",
		filterBytesFieldInt8SDK(hydrapb.Relational_EQUAL, "Status", 1),
		filterBytesFieldStringSDK(hydrapb.Relational_EQUAL, "CampaignID", "camp-1"),
	)
	pg := convertNestedSliceWhereToProto(sdkFilter)

	if pg.SlicePath != "CampaignEntries" {
		t.Errorf("expected SlicePath=CampaignEntries, got %s", pg.SlicePath)
	}
	if pg.EvalMode != hydrapb.NestedSliceWhereFilter_ANY {
		t.Errorf("expected ANY mode, got %v", pg.EvalMode)
	}
	if pg.Conditions == nil || len(pg.Conditions.Filters) != 2 {
		t.Errorf("expected 2 conditions, got %v", pg.Conditions)
	}
}

func TestSDK_FilterNestedSliceAll_ProtoConversion(t *testing.T) {
	sdkFilter := filterNestedSliceAllSDK("Items",
		filterBytesFieldInt8SDK(hydrapb.Relational_EQUAL, "Done", 1),
	)
	pg := convertNestedSliceWhereToProto(sdkFilter)

	if pg.EvalMode != hydrapb.NestedSliceWhereFilter_ALL {
		t.Errorf("expected ALL mode, got %v", pg.EvalMode)
	}
}

func TestSDK_FilterNestedSliceNone_ProtoConversion(t *testing.T) {
	sdkFilter := filterNestedSliceNoneSDK("Items",
		filterBytesFieldInt8SDK(hydrapb.Relational_EQUAL, "Deleted", 1),
	)
	pg := convertNestedSliceWhereToProto(sdkFilter)

	if pg.EvalMode != hydrapb.NestedSliceWhereFilter_NONE {
		t.Errorf("expected NONE mode, got %v", pg.EvalMode)
	}
}

func TestSDK_FilterNestedSliceCount_ProtoConversion(t *testing.T) {
	sdkFilter := filterNestedSliceCountSDK("Items", hydrapb.Relational_GREATER_THAN_OR_EQUAL, 3,
		filterBytesFieldInt8SDK(hydrapb.Relational_EQUAL, "Active", 1),
	)
	pg := convertNestedSliceWhereToProto(sdkFilter)

	if pg.EvalMode != hydrapb.NestedSliceWhereFilter_COUNT {
		t.Errorf("expected COUNT mode, got %v", pg.EvalMode)
	}
	if pg.CountOperator != hydrapb.Relational_GREATER_THAN_OR_EQUAL {
		t.Errorf("expected GTE operator, got %v", pg.CountOperator)
	}
	if pg.CountValue != 3 {
		t.Errorf("expected count=3, got %d", pg.CountValue)
	}
}

func TestSDK_FilterBytesFieldStringIn_ProtoConversion(t *testing.T) {
	path := "CampaignID"
	f := &Filter{
		operator:       stringInOp,
		bytesFieldPath: &path,
		stringInVals:   []string{"a", "b", "c"},
	}
	pf := convertFilterToProtoTest(f)

	if pf.Operator != hydrapb.Relational_STRING_IN {
		t.Errorf("expected STRING_IN, got %v", pf.Operator)
	}
	if len(pf.StringInVals) != 3 || pf.StringInVals[0] != "a" {
		t.Errorf("expected StringInVals=[a,b,c], got %v", pf.StringInVals)
	}
}

func TestSDK_FilterBytesFieldInt32In_ProtoConversion(t *testing.T) {
	path := "Status"
	f := &Filter{
		operator:       int32InOp,
		bytesFieldPath: &path,
		int32InVals:    []int32{1, 2, 3},
	}
	pf := convertFilterToProtoTest(f)

	if pf.Operator != hydrapb.Relational_INT32_IN {
		t.Errorf("expected INT32_IN, got %v", pf.Operator)
	}
	if len(pf.Int32InVals) != 3 {
		t.Errorf("expected 3 Int32InVals, got %d", len(pf.Int32InVals))
	}
}

func TestSDK_FilterBytesFieldInt64In_ProtoConversion(t *testing.T) {
	path := "Timestamp"
	f := &Filter{
		operator:       int64InOp,
		bytesFieldPath: &path,
		int64InVals:    []int64{100, 200, 300},
	}
	pf := convertFilterToProtoTest(f)

	if pf.Operator != hydrapb.Relational_INT64_IN {
		t.Errorf("expected INT64_IN, got %v", pf.Operator)
	}
	if len(pf.Int64InVals) != 3 {
		t.Errorf("expected 3 Int64InVals, got %d", len(pf.Int64InVals))
	}
}

func TestSDK_FilterBytesFieldTime_ProducesInt64(t *testing.T) {
	path := "NextSendAt"
	unixSec := int64(1712534400)
	// FilterBytesFieldTime should produce the same as FilterBytesFieldInt64
	f := &Filter{
		operator:       lessThanOrEqualOp,
		bytesFieldPath: &path,
		int64Val:       &unixSec,
	}
	pf := convertFilterToProtoTest(f)

	if pf.Operator != hydrapb.Relational_LESS_THAN_OR_EQUAL {
		t.Errorf("expected LESS_THAN_OR_EQUAL, got %v", pf.Operator)
	}
	cv, ok := pf.CompareValue.(*hydrapb.TreasureFilter_Int64Val)
	if !ok {
		t.Fatal("expected Int64Val compare value")
	}
	if cv.Int64Val != 1712534400 {
		t.Errorf("expected Unix timestamp 1712534400, got %d", cv.Int64Val)
	}
}

// --- SDK test helpers ---
// These simulate SDK construction without importing the SDK package
// (we're in the gateway package, so we mirror the essential SDK types)

type Filter struct {
	operator       hydrapb.Relational_Operator
	bytesFieldPath *string
	int8Val        *int32
	stringVal      *string
	int64Val       *int64
	stringInVals   []string
	int32InVals    []int32
	int64InVals    []int64
	label          *string
	treasureKey    *string
}

type NestedSliceWhereFilterSDK struct {
	mode          hydrapb.NestedSliceWhereFilter_Mode
	slicePath     string
	conditions    []*Filter
	countOperator *hydrapb.Relational_Operator
	countValue    *int32
}

const (
	stringInOp         = hydrapb.Relational_STRING_IN
	int32InOp          = hydrapb.Relational_INT32_IN
	int64InOp          = hydrapb.Relational_INT64_IN
	lessThanOrEqualOp  = hydrapb.Relational_LESS_THAN_OR_EQUAL
)

func filterBytesFieldInt8SDK(op hydrapb.Relational_Operator, path string, val int8) *Filter {
	v := int32(val)
	return &Filter{operator: op, bytesFieldPath: &path, int8Val: &v}
}

func filterBytesFieldStringSDK(op hydrapb.Relational_Operator, path string, val string) *Filter {
	return &Filter{operator: op, bytesFieldPath: &path, stringVal: &val}
}

func filterNestedSliceWhereSDK(slicePath string, conditions ...*Filter) *NestedSliceWhereFilterSDK {
	return &NestedSliceWhereFilterSDK{mode: hydrapb.NestedSliceWhereFilter_ANY, slicePath: slicePath, conditions: conditions}
}

func filterNestedSliceAllSDK(slicePath string, conditions ...*Filter) *NestedSliceWhereFilterSDK {
	return &NestedSliceWhereFilterSDK{mode: hydrapb.NestedSliceWhereFilter_ALL, slicePath: slicePath, conditions: conditions}
}

func filterNestedSliceNoneSDK(slicePath string, conditions ...*Filter) *NestedSliceWhereFilterSDK {
	return &NestedSliceWhereFilterSDK{mode: hydrapb.NestedSliceWhereFilter_NONE, slicePath: slicePath, conditions: conditions}
}

func filterNestedSliceCountSDK(slicePath string, op hydrapb.Relational_Operator, count int32, conditions ...*Filter) *NestedSliceWhereFilterSDK {
	return &NestedSliceWhereFilterSDK{
		mode: hydrapb.NestedSliceWhereFilter_COUNT, slicePath: slicePath, conditions: conditions,
		countOperator: &op, countValue: &count,
	}
}

func convertFilterToProtoTest(f *Filter) *hydrapb.TreasureFilter {
	pf := &hydrapb.TreasureFilter{
		Operator:       f.operator,
		BytesFieldPath: f.bytesFieldPath,
	}
	if f.int8Val != nil {
		pf.CompareValue = &hydrapb.TreasureFilter_Int8Val{Int8Val: *f.int8Val}
	} else if f.stringVal != nil {
		pf.CompareValue = &hydrapb.TreasureFilter_StringVal{StringVal: *f.stringVal}
	} else if f.int64Val != nil {
		pf.CompareValue = &hydrapb.TreasureFilter_Int64Val{Int64Val: *f.int64Val}
	}
	if len(f.stringInVals) > 0 {
		pf.StringInVals = f.stringInVals
	}
	if len(f.int32InVals) > 0 {
		pf.Int32InVals = f.int32InVals
	}
	if len(f.int64InVals) > 0 {
		pf.Int64InVals = f.int64InVals
	}
	return pf
}

func convertNestedSliceWhereToProto(nf *NestedSliceWhereFilterSDK) *hydrapb.NestedSliceWhereFilter {
	result := &hydrapb.NestedSliceWhereFilter{
		EvalMode:  nf.mode,
		SlicePath: nf.slicePath,
	}
	if len(nf.conditions) > 0 {
		fg := &hydrapb.FilterGroup{Logic: hydrapb.FilterLogic_AND}
		for _, c := range nf.conditions {
			fg.Filters = append(fg.Filters, convertFilterToProtoTest(c))
		}
		result.Conditions = fg
	}
	if nf.countOperator != nil {
		result.CountOperator = *nf.countOperator
	}
	if nf.countValue != nil {
		result.CountValue = *nf.countValue
	}
	return result
}

// =============================================================================
// time.Time in msgpack — regression tests for FilterBytesFieldTime
// =============================================================================

// TestNestedSliceWhere_TimeField_MsgpackExtension reproduces the bug where
// FilterBytesFieldTime produces an Int64Val comparison, but msgpack stores
// time.Time as ext type -1 (timestamp extension), not as int64.
// The server must convert time.Time → Unix seconds before comparing.
func TestNestedSliceWhere_TimeField_MsgpackExtension(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	fiveMinAgo := now.Add(-5 * time.Minute)

	// time.Time fields in msgpack are stored as timestamp extension, not int64
	tr := makeSliceTreasure(t, map[string]interface{}{
		"CampaignEntries": []interface{}{
			map[string]interface{}{
				"Status":     int8(1),
				"CampaignID": "camp-active",
				"NextSendAt": fiveMinAgo, // time.Time — msgpack ext type -1
			},
		},
	})

	campaignPath := "CampaignID"
	nf := makeNestedSliceWhereFilter(hydrapb.NestedSliceWhereFilter_ANY, "CampaignEntries",
		&hydrapb.FilterGroup{
			Logic: hydrapb.FilterLogic_AND,
			Filters: []*hydrapb.TreasureFilter{
				bytesFieldFilter("Status", hydrapb.Relational_EQUAL, int8(1)),
				{
					Operator:       hydrapb.Relational_STRING_IN,
					BytesFieldPath: &campaignPath,
					StringInVals:   []string{"camp-active"},
				},
				// FilterBytesFieldTime(LTE, "NextSendAt", now) → Int64Val = now.Unix()
				bytesFieldFilter("NextSendAt", hydrapb.Relational_LESS_THAN_OR_EQUAL, now.Unix()),
				// FilterBytesFieldTime(GT, "NextSendAt", time.Time{}) → Int64Val = 0
				bytesFieldFilter("NextSendAt", hydrapb.Relational_GREATER_THAN, int64(0)),
			},
		},
	)
	fg := &hydrapb.FilterGroup{
		Logic:                    hydrapb.FilterLogic_AND,
		NestedSliceWhereFilters: []*hydrapb.NestedSliceWhereFilter{nf},
	}
	if !evaluateNativeFilterGroup(tr, fg) {
		t.Error("expected NestedSliceWhere with time.Time field (msgpack ext) to match when NextSendAt is 5min ago")
	}
}

func TestNestedSliceWhere_TimeField_ZeroTime(t *testing.T) {
	// Zero time.Time should be > 0 Unix seconds check should fail
	tr := makeSliceTreasure(t, map[string]interface{}{
		"CampaignEntries": []interface{}{
			map[string]interface{}{
				"Status":     int8(1),
				"CampaignID": "camp-active",
				"NextSendAt": time.Time{}, // zero time
			},
		},
	})

	nf := makeNestedSliceWhereFilter(hydrapb.NestedSliceWhereFilter_ANY, "CampaignEntries",
		andConditions(
			bytesFieldFilter("Status", hydrapb.Relational_EQUAL, int8(1)),
			bytesFieldFilter("NextSendAt", hydrapb.Relational_GREATER_THAN, int64(0)),
		),
	)
	fg := &hydrapb.FilterGroup{
		Logic:                    hydrapb.FilterLogic_AND,
		NestedSliceWhereFilters: []*hydrapb.NestedSliceWhereFilter{nf},
	}
	if evaluateNativeFilterGroup(tr, fg) {
		t.Error("expected zero time.Time to fail GT 0 check")
	}
}

func TestNestedSliceWhere_TimeField_FutureTime(t *testing.T) {
	// NextSendAt is in the future — LTE now should fail
	now := time.Now().UTC().Truncate(time.Second)
	futureTime := now.Add(1 * time.Hour)

	tr := makeSliceTreasure(t, map[string]interface{}{
		"CampaignEntries": []interface{}{
			map[string]interface{}{
				"Status":     int8(1),
				"NextSendAt": futureTime,
			},
		},
	})

	nf := makeNestedSliceWhereFilter(hydrapb.NestedSliceWhereFilter_ANY, "CampaignEntries",
		andConditions(
			bytesFieldFilter("Status", hydrapb.Relational_EQUAL, int8(1)),
			bytesFieldFilter("NextSendAt", hydrapb.Relational_LESS_THAN_OR_EQUAL, now.Unix()),
		),
	)
	fg := &hydrapb.FilterGroup{
		Logic:                    hydrapb.FilterLogic_AND,
		NestedSliceWhereFilters: []*hydrapb.NestedSliceWhereFilter{nf},
	}
	if evaluateNativeFilterGroup(tr, fg) {
		t.Error("expected future NextSendAt to fail LTE now check")
	}
}

func TestTopLevel_TimeField_MsgpackExtension(t *testing.T) {
	// Same bug at top-level (not nested) — time.Time in BytesVal
	now := time.Now().UTC().Truncate(time.Second)
	fiveMinAgo := now.Add(-5 * time.Minute)

	tr := makeSliceTreasure(t, map[string]interface{}{
		"NextSendAt": fiveMinAgo,
	})

	fg := &hydrapb.FilterGroup{
		Logic: hydrapb.FilterLogic_AND,
		Filters: []*hydrapb.TreasureFilter{
			bytesFieldFilter("NextSendAt", hydrapb.Relational_LESS_THAN_OR_EQUAL, now.Unix()),
		},
	}
	if !evaluateNativeFilterGroup(tr, fg) {
		t.Error("expected top-level time.Time field (msgpack ext) to match LTE now")
	}
}

func TestAnyMatch_TimeField_Wildcard(t *testing.T) {
	// time.Time with [*] wildcard path (evaluateAnyMatch path)
	now := time.Now().UTC().Truncate(time.Second)
	fiveMinAgo := now.Add(-5 * time.Minute)

	tr := makeSliceTreasure(t, map[string]interface{}{
		"Events": []interface{}{
			map[string]interface{}{"At": now.Add(1 * time.Hour)},  // future
			map[string]interface{}{"At": fiveMinAgo},               // past
		},
	})

	path := "Events[*].At"
	fg := &hydrapb.FilterGroup{
		Logic: hydrapb.FilterLogic_AND,
		Filters: []*hydrapb.TreasureFilter{
			bytesFieldFilter("Events[*].At", hydrapb.Relational_LESS_THAN_OR_EQUAL, now.Unix()),
		},
	}
	_ = path
	if !evaluateNativeFilterGroup(tr, fg) {
		t.Error("expected [*] wildcard time.Time to match when at least one is <= now")
	}
}
