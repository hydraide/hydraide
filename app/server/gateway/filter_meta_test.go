package gateway

import (
	"math"
	"testing"

	hydrapb "github.com/hydraide/hydraide/generated/hydraidepbgo"
	"github.com/vmihailenco/msgpack/v5"
)

// --- Test helpers ---

func normalizeVec(vals []float32) []float32 {
	var norm float32
	for _, v := range vals {
		norm += v * v
	}
	if norm == 0 {
		return vals
	}
	invNorm := float32(1.0 / math.Sqrt(float64(norm)))
	result := make([]float32, len(vals))
	for i, v := range vals {
		result[i] = v * invNorm
	}
	return result
}

// --- hasAnyLabels tests ---

func TestHasAnyLabels_Nil(t *testing.T) {
	if hasAnyLabels(nil) {
		t.Error("expected false for nil group")
	}
}

func TestHasAnyLabels_Empty(t *testing.T) {
	fg := &hydrapb.FilterGroup{}
	if hasAnyLabels(fg) {
		t.Error("expected false for empty group")
	}
}

func TestHasAnyLabels_WithVector(t *testing.T) {
	fg := &hydrapb.FilterGroup{
		VectorFilters: []*hydrapb.VectorFilter{{BytesFieldPath: "Embedding"}},
	}
	if !hasAnyLabels(fg) {
		t.Error("expected true when VectorFilter present")
	}
}

func TestHasAnyLabels_WithLabel(t *testing.T) {
	label := "test"
	path := "Field"
	fg := &hydrapb.FilterGroup{
		Filters: []*hydrapb.TreasureFilter{{
			BytesFieldPath: &path,
			Label:          &label,
		}},
	}
	if !hasAnyLabels(fg) {
		t.Error("expected true when filter has label")
	}
}

func TestHasAnyLabels_Nested(t *testing.T) {
	label := "nested"
	path := "Field"
	fg := &hydrapb.FilterGroup{
		SubGroups: []*hydrapb.FilterGroup{{
			Filters: []*hydrapb.TreasureFilter{{
				BytesFieldPath: &path,
				Label:          &label,
			}},
		}},
	}
	if !hasAnyLabels(fg) {
		t.Error("expected true when nested subgroup has label")
	}
}

func TestHasAnyLabels_NoLabels(t *testing.T) {
	path := "Field"
	fg := &hydrapb.FilterGroup{
		Filters: []*hydrapb.TreasureFilter{{
			BytesFieldPath: &path,
		}},
	}
	if hasAnyLabels(fg) {
		t.Error("expected false when no labels and no vectors")
	}
}

// --- Vector score tests ---

func TestVectorScore_Captured(t *testing.T) {
	storedVec := normalizeVec([]float32{1.0, 0.0, 0.0})
	queryVec := normalizeVec([]float32{0.9, 0.1, 0.0})

	data := map[string]interface{}{"Embedding": []interface{}{storedVec[0], storedVec[1], storedVec[2]}}
	tr := newTreasureWithBytes(makeMsgpackBytesVal(t, data))

	fg := &hydrapb.FilterGroup{
		VectorFilters: []*hydrapb.VectorFilter{{
			BytesFieldPath: "Embedding",
			QueryVector:    queryVec,
			MinSimilarity:  0.5,
		}},
	}

	matched, meta := evaluateNativeFilterGroupWithMeta(tr, fg)
	if !matched {
		t.Fatal("expected vector filter to match")
	}
	if len(meta.vectorScores) != 1 {
		t.Fatalf("expected 1 vector score, got %d", len(meta.vectorScores))
	}
	if meta.vectorScores[0] < 0.5 || meta.vectorScores[0] > 1.0 {
		t.Errorf("unexpected score: %f", meta.vectorScores[0])
	}
}

func TestVectorScore_NoLabel(t *testing.T) {
	storedVec := normalizeVec([]float32{1.0, 0.0, 0.0})
	queryVec := normalizeVec([]float32{1.0, 0.0, 0.0})

	data := map[string]interface{}{"Embedding": []interface{}{storedVec[0], storedVec[1], storedVec[2]}}
	tr := newTreasureWithBytes(makeMsgpackBytesVal(t, data))

	fg := &hydrapb.FilterGroup{
		VectorFilters: []*hydrapb.VectorFilter{{
			BytesFieldPath: "Embedding",
			QueryVector:    queryVec,
			MinSimilarity:  0.5,
		}},
	}

	matched, meta := evaluateNativeFilterGroupWithMeta(tr, fg)
	if !matched {
		t.Fatal("expected match")
	}
	if len(meta.vectorScores) != 1 {
		t.Fatalf("expected 1 score, got %d", len(meta.vectorScores))
	}
	// No label, so matchedLabels should be empty
	if len(meta.matchedLabels) != 0 {
		t.Errorf("expected no labels, got %v", meta.matchedLabels)
	}
}

func TestVectorScore_WithLabel(t *testing.T) {
	storedVec := normalizeVec([]float32{1.0, 0.0, 0.0})
	queryVec := normalizeVec([]float32{1.0, 0.0, 0.0})

	data := map[string]interface{}{"Embedding": []interface{}{storedVec[0], storedVec[1], storedVec[2]}}
	tr := newTreasureWithBytes(makeMsgpackBytesVal(t, data))

	label := "semantic"
	fg := &hydrapb.FilterGroup{
		VectorFilters: []*hydrapb.VectorFilter{{
			BytesFieldPath: "Embedding",
			QueryVector:    queryVec,
			MinSimilarity:  0.5,
			Label:          &label,
		}},
	}

	matched, meta := evaluateNativeFilterGroupWithMeta(tr, fg)
	if !matched {
		t.Fatal("expected match")
	}
	if len(meta.matchedLabels) != 1 || meta.matchedLabels[0] != "semantic" {
		t.Errorf("expected ['semantic'], got %v", meta.matchedLabels)
	}
}

// --- Labeled filter tests ---

func TestLabeledFilter_AND_Single(t *testing.T) {
	tr := makeSliceTreasure(t, map[string]interface{}{"Funcs": []interface{}{int8(1), int8(7)}})
	path := "Funcs"
	label := "booking"
	fg := &hydrapb.FilterGroup{
		Filters: []*hydrapb.TreasureFilter{{
			Operator:       hydrapb.Relational_SLICE_CONTAINS,
			BytesFieldPath: &path,
			CompareValue:   &hydrapb.TreasureFilter_Int8Val{Int8Val: 7},
			Label:          &label,
		}},
	}

	matched, meta := evaluateNativeFilterGroupWithMeta(tr, fg)
	if !matched {
		t.Fatal("expected match")
	}
	if len(meta.matchedLabels) != 1 || meta.matchedLabels[0] != "booking" {
		t.Errorf("expected ['booking'], got %v", meta.matchedLabels)
	}
}

func TestLabeledFilter_AND_Multiple(t *testing.T) {
	tr := makeSliceTreasure(t, map[string]interface{}{
		"Funcs":          []interface{}{int8(7)},
		"geo_latitude":   47.497,
		"geo_longitude":  19.040,
	})
	funcPath := "Funcs"
	bookingLabel := "booking"
	budapestLabel := "budapest"
	fg := &hydrapb.FilterGroup{
		Filters: []*hydrapb.TreasureFilter{{
			Operator:       hydrapb.Relational_SLICE_CONTAINS,
			BytesFieldPath: &funcPath,
			CompareValue:   &hydrapb.TreasureFilter_Int8Val{Int8Val: 7},
			Label:          &bookingLabel,
		}},
		GeoDistanceFilters: []*hydrapb.GeoDistanceFilter{{
			LatFieldPath: "geo_latitude",
			LngFieldPath: "geo_longitude",
			RefLatitude:  47.497,
			RefLongitude: 19.040,
			RadiusKm:     50.0,
			Mode:         hydrapb.GeoDistanceMode_INSIDE,
			Label:        &budapestLabel,
		}},
	}

	matched, meta := evaluateNativeFilterGroupWithMeta(tr, fg)
	if !matched {
		t.Fatal("expected match")
	}
	if len(meta.matchedLabels) != 2 {
		t.Fatalf("expected 2 labels, got %d: %v", len(meta.matchedLabels), meta.matchedLabels)
	}
}

func TestLabeledFilter_OR_AllMatching(t *testing.T) {
	tr := makeSliceTreasure(t, map[string]interface{}{
		"Sectors": []interface{}{int8(1), int8(6)},
	})
	path := "Sectors"
	hospLabel := "hospitality"
	healthLabel := "health"
	fg := &hydrapb.FilterGroup{
		Logic: hydrapb.FilterLogic_OR,
		Filters: []*hydrapb.TreasureFilter{
			{
				Operator:       hydrapb.Relational_SLICE_CONTAINS,
				BytesFieldPath: &path,
				CompareValue:   &hydrapb.TreasureFilter_Int8Val{Int8Val: 1},
				Label:          &hospLabel,
			},
			{
				Operator:       hydrapb.Relational_SLICE_CONTAINS,
				BytesFieldPath: &path,
				CompareValue:   &hydrapb.TreasureFilter_Int8Val{Int8Val: 6},
				Label:          &healthLabel,
			},
		},
	}

	matched, meta := evaluateNativeFilterGroupWithMeta(tr, fg)
	if !matched {
		t.Fatal("expected match")
	}
	// Both branches should match — OR evaluates all with meta
	if len(meta.matchedLabels) != 2 {
		t.Errorf("expected 2 labels (both OR branches), got %v", meta.matchedLabels)
	}
}

func TestLabeledFilter_OR_PartialMatch(t *testing.T) {
	tr := makeSliceTreasure(t, map[string]interface{}{
		"Sectors": []interface{}{int8(6)}, // only health, no hospitality
	})
	path := "Sectors"
	hospLabel := "hospitality"
	healthLabel := "health"
	fg := &hydrapb.FilterGroup{
		Logic: hydrapb.FilterLogic_OR,
		Filters: []*hydrapb.TreasureFilter{
			{
				Operator:       hydrapb.Relational_SLICE_CONTAINS,
				BytesFieldPath: &path,
				CompareValue:   &hydrapb.TreasureFilter_Int8Val{Int8Val: 1},
				Label:          &hospLabel,
			},
			{
				Operator:       hydrapb.Relational_SLICE_CONTAINS,
				BytesFieldPath: &path,
				CompareValue:   &hydrapb.TreasureFilter_Int8Val{Int8Val: 6},
				Label:          &healthLabel,
			},
		},
	}

	matched, meta := evaluateNativeFilterGroupWithMeta(tr, fg)
	if !matched {
		t.Fatal("expected match")
	}
	if len(meta.matchedLabels) != 1 || meta.matchedLabels[0] != "health" {
		t.Errorf("expected ['health'], got %v", meta.matchedLabels)
	}
}

func TestLabeledFilter_MixedLabeledUnlabeled(t *testing.T) {
	tr := makeSliceTreasure(t, map[string]interface{}{
		"Funcs":          []interface{}{int8(7)},
		"geo_latitude":   47.497,
		"geo_longitude":  19.040,
	})
	funcPath := "Funcs"
	bookingLabel := "booking"
	fg := &hydrapb.FilterGroup{
		Filters: []*hydrapb.TreasureFilter{{
			Operator:       hydrapb.Relational_SLICE_CONTAINS,
			BytesFieldPath: &funcPath,
			CompareValue:   &hydrapb.TreasureFilter_Int8Val{Int8Val: 7},
			Label:          &bookingLabel,
		}},
		GeoDistanceFilters: []*hydrapb.GeoDistanceFilter{{
			LatFieldPath: "geo_latitude",
			LngFieldPath: "geo_longitude",
			RefLatitude:  47.497,
			RefLongitude: 19.040,
			RadiusKm:     50.0,
			Mode:         hydrapb.GeoDistanceMode_INSIDE,
			// NO LABEL
		}},
	}

	matched, meta := evaluateNativeFilterGroupWithMeta(tr, fg)
	if !matched {
		t.Fatal("expected match")
	}
	// Only the labeled filter should appear
	if len(meta.matchedLabels) != 1 || meta.matchedLabels[0] != "booking" {
		t.Errorf("expected ['booking'] only, got %v", meta.matchedLabels)
	}
}

func TestLabeledFilter_NestedGroup(t *testing.T) {
	tr := makeSliceTreasure(t, map[string]interface{}{
		"Funcs":   []interface{}{int8(7)},
		"Sectors": []interface{}{int8(6)},
	})
	funcPath := "Funcs"
	sectorPath := "Sectors"
	bookingLabel := "booking"
	healthLabel := "health"

	fg := &hydrapb.FilterGroup{
		Logic: hydrapb.FilterLogic_AND,
		Filters: []*hydrapb.TreasureFilter{{
			Operator:       hydrapb.Relational_SLICE_CONTAINS,
			BytesFieldPath: &funcPath,
			CompareValue:   &hydrapb.TreasureFilter_Int8Val{Int8Val: 7},
			Label:          &bookingLabel,
		}},
		SubGroups: []*hydrapb.FilterGroup{{
			Logic: hydrapb.FilterLogic_OR,
			Filters: []*hydrapb.TreasureFilter{{
				Operator:       hydrapb.Relational_SLICE_CONTAINS,
				BytesFieldPath: &sectorPath,
				CompareValue:   &hydrapb.TreasureFilter_Int8Val{Int8Val: 6},
				Label:          &healthLabel,
			}},
		}},
	}

	matched, meta := evaluateNativeFilterGroupWithMeta(tr, fg)
	if !matched {
		t.Fatal("expected match")
	}
	if len(meta.matchedLabels) != 2 {
		t.Fatalf("expected 2 labels from nested groups, got %v", meta.matchedLabels)
	}
}

func TestNoLabels_FastPath(t *testing.T) {
	tr := makeSliceTreasure(t, map[string]interface{}{"Funcs": []interface{}{int8(7)}})
	path := "Funcs"
	fg := &hydrapb.FilterGroup{
		Filters: []*hydrapb.TreasureFilter{{
			Operator:       hydrapb.Relational_SLICE_CONTAINS,
			BytesFieldPath: &path,
			CompareValue:   &hydrapb.TreasureFilter_Int8Val{Int8Val: 7},
			// NO LABEL
		}},
	}

	// Fast path: hasAnyLabels should be false
	if hasAnyLabels(fg) {
		t.Error("expected hasAnyLabels=false for unlabeled filters without vectors")
	}

	// Regular evaluation works
	if !evaluateNativeFilterGroup(tr, fg) {
		t.Error("expected match on fast path")
	}
}

// --- Integration: compound with scores and labels ---

func TestCompound_VectorAndSliceWithLabels(t *testing.T) {
	storedVec := normalizeVec([]float32{1.0, 0.0, 0.0})
	data := map[string]interface{}{
		"Embedding": []interface{}{storedVec[0], storedVec[1], storedVec[2]},
		"Funcs":     []interface{}{int8(7)},
	}
	tr := newTreasureWithBytes(makeMsgpackBytesVal(t, data))

	queryVec := normalizeVec([]float32{0.95, 0.05, 0.0})
	funcPath := "Funcs"
	semanticLabel := "semantic"
	bookingLabel := "booking"

	fg := &hydrapb.FilterGroup{
		Filters: []*hydrapb.TreasureFilter{{
			Operator:       hydrapb.Relational_SLICE_CONTAINS,
			BytesFieldPath: &funcPath,
			CompareValue:   &hydrapb.TreasureFilter_Int8Val{Int8Val: 7},
			Label:          &bookingLabel,
		}},
		VectorFilters: []*hydrapb.VectorFilter{{
			BytesFieldPath: "Embedding",
			QueryVector:    queryVec,
			MinSimilarity:  0.5,
			Label:          &semanticLabel,
		}},
	}

	matched, meta := evaluateNativeFilterGroupWithMeta(tr, fg)
	if !matched {
		t.Fatal("expected match")
	}
	if len(meta.vectorScores) != 1 {
		t.Fatalf("expected 1 vector score, got %d", len(meta.vectorScores))
	}
	if meta.vectorScores[0] < 0.9 {
		t.Errorf("expected high similarity, got %f", meta.vectorScores[0])
	}
	if len(meta.matchedLabels) != 2 {
		t.Fatalf("expected 2 labels, got %v", meta.matchedLabels)
	}
}

// --- Benchmarks ---

func BenchmarkFilterEval_WithoutMeta(b *testing.B) {
	data := map[string]interface{}{"Funcs": []interface{}{int8(1), int8(7)}}
	encoded, _ := msgpack.Marshal(data)
	bytesVal := append([]byte{msgpackMagic0, msgpackMagic1}, encoded...)
	tr := newTreasureWithBytes(bytesVal)
	path := "Funcs"
	fg := &hydrapb.FilterGroup{
		Filters: []*hydrapb.TreasureFilter{{
			Operator:       hydrapb.Relational_SLICE_CONTAINS,
			BytesFieldPath: &path,
			CompareValue:   &hydrapb.TreasureFilter_Int8Val{Int8Val: 7},
		}},
	}
	for b.Loop() {
		evaluateNativeFilterGroup(tr, fg)
	}
}

func BenchmarkFilterEval_WithMeta_NoLabels(b *testing.B) {
	storedVec := normalizeVec([]float32{1.0, 0.0, 0.0})
	data := map[string]interface{}{"Embedding": []interface{}{storedVec[0], storedVec[1], storedVec[2]}}
	encoded, _ := msgpack.Marshal(data)
	bytesVal := append([]byte{msgpackMagic0, msgpackMagic1}, encoded...)
	tr := newTreasureWithBytes(bytesVal)
	queryVec := normalizeVec([]float32{1.0, 0.0, 0.0})
	fg := &hydrapb.FilterGroup{
		VectorFilters: []*hydrapb.VectorFilter{{
			BytesFieldPath: "Embedding",
			QueryVector:    queryVec,
			MinSimilarity:  0.5,
		}},
	}
	for b.Loop() {
		evaluateNativeFilterGroupWithMeta(tr, fg)
	}
}

func BenchmarkFilterEval_WithMeta_WithLabels(b *testing.B) {
	storedVec := normalizeVec([]float32{1.0, 0.0, 0.0})
	data := map[string]interface{}{
		"Embedding": []interface{}{storedVec[0], storedVec[1], storedVec[2]},
		"Funcs":     []interface{}{int8(7)},
	}
	encoded, _ := msgpack.Marshal(data)
	bytesVal := append([]byte{msgpackMagic0, msgpackMagic1}, encoded...)
	tr := newTreasureWithBytes(bytesVal)
	queryVec := normalizeVec([]float32{1.0, 0.0, 0.0})
	funcPath := "Funcs"
	semLabel := "semantic"
	bookLabel := "booking"
	fg := &hydrapb.FilterGroup{
		Filters: []*hydrapb.TreasureFilter{{
			Operator:       hydrapb.Relational_SLICE_CONTAINS,
			BytesFieldPath: &funcPath,
			CompareValue:   &hydrapb.TreasureFilter_Int8Val{Int8Val: 7},
			Label:          &bookLabel,
		}},
		VectorFilters: []*hydrapb.VectorFilter{{
			BytesFieldPath: "Embedding",
			QueryVector:    queryVec,
			MinSimilarity:  0.5,
			Label:          &semLabel,
		}},
	}
	for b.Loop() {
		evaluateNativeFilterGroupWithMeta(tr, fg)
	}
}

func BenchmarkFilterEval_OR_WithMeta(b *testing.B) {
	data := map[string]interface{}{"Sectors": []interface{}{int8(1), int8(6)}}
	encoded, _ := msgpack.Marshal(data)
	bytesVal := append([]byte{msgpackMagic0, msgpackMagic1}, encoded...)
	tr := newTreasureWithBytes(bytesVal)
	path := "Sectors"
	labels := []string{"a", "b", "c", "d", "e"}
	var filters []*hydrapb.TreasureFilter
	for i, l := range labels {
		l := l
		filters = append(filters, &hydrapb.TreasureFilter{
			Operator:       hydrapb.Relational_SLICE_CONTAINS,
			BytesFieldPath: &path,
			CompareValue:   &hydrapb.TreasureFilter_Int8Val{Int8Val: int32(i + 1)},
			Label:          &l,
		})
	}
	fg := &hydrapb.FilterGroup{Logic: hydrapb.FilterLogic_OR, Filters: filters}
	for b.Loop() {
		evaluateNativeFilterGroupWithMeta(tr, fg)
	}
}

func BenchmarkVectorFilterWithScore(b *testing.B) {
	storedVec := normalizeVec([]float32{1.0, 0.0, 0.0})
	data := map[string]interface{}{"Embedding": []interface{}{storedVec[0], storedVec[1], storedVec[2]}}
	encoded, _ := msgpack.Marshal(data)
	bytesVal := append([]byte{msgpackMagic0, msgpackMagic1}, encoded...)
	tr := newTreasureWithBytes(bytesVal)
	vf := &hydrapb.VectorFilter{
		BytesFieldPath: "Embedding",
		QueryVector:    normalizeVec([]float32{1.0, 0.0, 0.0}),
		MinSimilarity:  0.5,
	}
	for b.Loop() {
		evaluateNativeVectorFilterWithScore(tr, vf)
	}
}
