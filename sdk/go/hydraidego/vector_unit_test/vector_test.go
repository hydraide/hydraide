package vector_unit_test

import (
	"math"
	"testing"

	"github.com/hydraide/hydraide/generated/hydraidepbgo"
	"github.com/hydraide/hydraide/sdk/go/hydraidego"
)

// --- NormalizeVector tests ---

func TestNormalizeVector_Basic(t *testing.T) {
	v := hydraidego.NormalizeVector([]float32{3.0, 4.0})
	if v == nil {
		t.Fatal("expected non-nil result")
	}
	if diff := math.Abs(float64(v[0]) - 0.6); diff > 0.001 {
		t.Errorf("v[0] = %f, want ~0.6", v[0])
	}
	if diff := math.Abs(float64(v[1]) - 0.8); diff > 0.001 {
		t.Errorf("v[1] = %f, want ~0.8", v[1])
	}

	var norm float32
	for _, x := range v {
		norm += x * x
	}
	if diff := math.Abs(float64(norm) - 1.0); diff > 0.001 {
		t.Errorf("norm = %f, want ~1.0", norm)
	}
}

func TestNormalizeVector_AlreadyNormalized(t *testing.T) {
	v := hydraidego.NormalizeVector([]float32{1.0, 0.0, 0.0})
	if v == nil {
		t.Fatal("expected non-nil result")
	}
	if v[0] != 1.0 || v[1] != 0.0 || v[2] != 0.0 {
		t.Errorf("already-normalized vector should remain unchanged, got %v", v)
	}
}

func TestNormalizeVector_ZeroVector(t *testing.T) {
	v := hydraidego.NormalizeVector([]float32{0.0, 0.0, 0.0})
	if v != nil {
		t.Error("expected nil for zero vector")
	}
}

func TestNormalizeVector_Empty(t *testing.T) {
	v := hydraidego.NormalizeVector([]float32{})
	if v != nil {
		t.Error("expected nil for empty vector")
	}
}

func TestNormalizeVector_Nil(t *testing.T) {
	v := hydraidego.NormalizeVector(nil)
	if v != nil {
		t.Error("expected nil for nil input")
	}
}

func TestNormalizeVector_DoesNotMutateOriginal(t *testing.T) {
	original := []float32{3.0, 4.0}
	_ = hydraidego.NormalizeVector(original)
	if original[0] != 3.0 || original[1] != 4.0 {
		t.Error("NormalizeVector should not mutate the original slice")
	}
}

func TestNormalizeVector_HighDimensional(t *testing.T) {
	v := make([]float32, 384)
	for i := range v {
		v[i] = float32(i) + 1.0
	}
	result := hydraidego.NormalizeVector(v)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	var norm float32
	for _, x := range result {
		norm += x * x
	}
	if diff := math.Abs(float64(norm) - 1.0); diff > 0.001 {
		t.Errorf("norm of 384-dim vector = %f, want ~1.0", norm)
	}
}

// --- CosineSimilarity tests ---

func TestCosineSimilarity_Identical(t *testing.T) {
	v := []float32{1.0, 2.0, 3.0}
	sim := hydraidego.CosineSimilarity(v, v)
	if diff := math.Abs(float64(sim) - 1.0); diff > 0.001 {
		t.Errorf("identical vectors should have similarity ~1.0, got %f", sim)
	}
}

func TestCosineSimilarity_Orthogonal(t *testing.T) {
	a := []float32{1.0, 0.0, 0.0}
	b := []float32{0.0, 1.0, 0.0}
	sim := hydraidego.CosineSimilarity(a, b)
	if diff := math.Abs(float64(sim)); diff > 0.001 {
		t.Errorf("orthogonal vectors should have similarity ~0.0, got %f", sim)
	}
}

func TestCosineSimilarity_Opposite(t *testing.T) {
	a := []float32{1.0, 0.0}
	b := []float32{-1.0, 0.0}
	sim := hydraidego.CosineSimilarity(a, b)
	if diff := math.Abs(float64(sim) + 1.0); diff > 0.001 {
		t.Errorf("opposite vectors should have similarity ~-1.0, got %f", sim)
	}
}

func TestCosineSimilarity_DifferentMagnitudes(t *testing.T) {
	a := []float32{1.0, 0.0}
	b := []float32{100.0, 0.0}
	sim := hydraidego.CosineSimilarity(a, b)
	if diff := math.Abs(float64(sim) - 1.0); diff > 0.001 {
		t.Errorf("parallel vectors of different magnitude should have similarity ~1.0, got %f", sim)
	}
}

func TestCosineSimilarity_DimensionMismatch(t *testing.T) {
	sim := hydraidego.CosineSimilarity([]float32{1.0, 0.0}, []float32{1.0, 0.0, 0.0})
	if sim != 0 {
		t.Errorf("dimension mismatch should return 0, got %f", sim)
	}
}

func TestCosineSimilarity_ZeroVector(t *testing.T) {
	sim := hydraidego.CosineSimilarity([]float32{0.0, 0.0}, []float32{1.0, 0.0})
	if sim != 0 {
		t.Errorf("zero vector should return 0, got %f", sim)
	}
}

func TestCosineSimilarity_Empty(t *testing.T) {
	sim := hydraidego.CosineSimilarity([]float32{}, []float32{})
	if sim != 0 {
		t.Errorf("empty vectors should return 0, got %f", sim)
	}
}

// --- FilterVector builder tests ---

func TestFilterVector_Builder(t *testing.T) {
	qv := []float32{0.1, 0.2, 0.3}
	vf := hydraidego.FilterVector("Embedding", qv, 0.75)
	if vf == nil {
		t.Fatal("expected non-nil VectorFilter")
	}
}

func TestFilterVector_ForKey(t *testing.T) {
	qv := []float32{0.1, 0.2, 0.3}
	vf := hydraidego.FilterVector("Embedding", qv, 0.70).ForKey("MainProfile")
	if vf == nil {
		t.Fatal("expected non-nil VectorFilter")
	}
}

func TestFilterVector_IsFilterItem(t *testing.T) {
	vf := hydraidego.FilterVector("Embedding", []float32{0.1}, 0.5)
	var _ hydraidego.FilterItem = vf
}

// --- FilterAND / FilterOR with VectorFilter ---

func TestFilterAND_WithVectorFilter(t *testing.T) {
	qv := []float32{0.1, 0.2}
	group := hydraidego.FilterAND(
		hydraidego.FilterInt32(hydraidego.Equal, 42),
		hydraidego.FilterVector("Embedding", qv, 0.70),
	)
	if group == nil {
		t.Fatal("expected non-nil FilterGroup")
	}
}

func TestFilterOR_WithVectorFilter(t *testing.T) {
	qv := []float32{0.1, 0.2}
	group := hydraidego.FilterOR(
		hydraidego.FilterString(hydraidego.Equal, "test"),
		hydraidego.FilterVector("Embedding", qv, 0.60),
		hydraidego.FilterPhrase("WordIndex", "hello", "world"),
	)
	if group == nil {
		t.Fatal("expected non-nil FilterGroup")
	}
}

// --- convertFilterGroupToProto (tested via exported function behavior) ---
// Since convertFilterGroupToProto is unexported, we verify the full flow by
// testing that FilterGroups with VectorFilters compile and produce valid types.

func TestFilterGroupWithAllTypes_Compiles(t *testing.T) {
	qv := []float32{0.1}
	group := hydraidego.FilterAND(
		hydraidego.FilterInt32(hydraidego.Equal, 1),
		hydraidego.FilterBytesFieldString(hydraidego.Equal, "Category", "business"),
		hydraidego.FilterPhrase("WordIndex", "hello"),
		hydraidego.FilterVector("Embedding", qv, 0.5),
		hydraidego.FilterOR(
			hydraidego.FilterString(hydraidego.Equal, "a"),
			hydraidego.FilterVector("Vec2", qv, 0.3),
		),
	)
	if group == nil {
		t.Fatal("expected non-nil FilterGroup")
	}
}

// Verify the proto VectorFilter type exists and has the expected fields.
func TestProtoVectorFilter_Fields(t *testing.T) {
	vf := &hydraidepbgo.VectorFilter{
		BytesFieldPath: "Embedding",
		QueryVector:    []float32{0.1, 0.2, 0.3},
		MinSimilarity:  0.75,
	}

	if vf.BytesFieldPath != "Embedding" {
		t.Errorf("BytesFieldPath = %q, want %q", vf.BytesFieldPath, "Embedding")
	}
	if len(vf.QueryVector) != 3 {
		t.Errorf("QueryVector len = %d, want 3", len(vf.QueryVector))
	}
	if vf.MinSimilarity != 0.75 {
		t.Errorf("MinSimilarity = %f, want 0.75", vf.MinSimilarity)
	}
}

func TestProtoVectorFilter_TreasureKey(t *testing.T) {
	key := "Profile"
	vf := &hydraidepbgo.VectorFilter{
		BytesFieldPath: "Embedding",
		QueryVector:    []float32{0.1},
		MinSimilarity:  0.5,
		TreasureKey:    &key,
	}

	if vf.TreasureKey == nil || *vf.TreasureKey != "Profile" {
		t.Error("expected TreasureKey = 'Profile'")
	}
}

func TestProtoFilterGroup_VectorFilters(t *testing.T) {
	fg := &hydraidepbgo.FilterGroup{
		Logic: hydraidepbgo.FilterLogic_AND,
		VectorFilters: []*hydraidepbgo.VectorFilter{
			{BytesFieldPath: "Embedding", QueryVector: []float32{0.1}, MinSimilarity: 0.5},
		},
	}

	if len(fg.VectorFilters) != 1 {
		t.Fatalf("expected 1 VectorFilter, got %d", len(fg.VectorFilters))
	}
}
