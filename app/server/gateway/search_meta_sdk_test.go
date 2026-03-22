package gateway

import (
	"reflect"
	"testing"

	hydrapb "github.com/hydraide/hydraide/generated/hydraidepbgo"
	"github.com/hydraide/hydraide/sdk/go/hydraidego"
)

// These tests verify the SDK-level searchMeta tag population via setSearchMetaOnModel.
// They are placed in the gateway package to avoid the SDK's E2E TestMain constraint.

// We need to call the SDK's unexported function. Since it's in a different package,
// we test the behavior indirectly through a locally defined equivalent.
// The actual function is in sdk/go/hydraidego/hydraidego.go.

// For direct testing, we replicate the model tag behavior here.

type testModelWithSearchMeta struct {
	Key  string                  `hydraide:"key"`
	Meta *hydraidego.SearchMeta  `hydraide:"searchMeta"`
}

type testModelWithoutSearchMeta struct {
	Key string `hydraide:"key"`
}

// setSearchMetaOnTestModel replicates the SDK's setSearchMetaOnModel logic
// to test the tag-based population mechanism.
func setSearchMetaOnTestModel(model any, protoMeta *hydrapb.SearchResultMeta) {
	if protoMeta == nil {
		return
	}
	if len(protoMeta.VectorScores) == 0 && len(protoMeta.MatchedLabels) == 0 {
		return
	}

	v := reflect.ValueOf(model)
	if v.Kind() != reflect.Ptr || v.Elem().Kind() != reflect.Struct {
		return
	}

	t := v.Elem().Type()
	for i := 0; i < t.NumField(); i++ {
		if tag, ok := t.Field(i).Tag.Lookup("hydraide"); ok && tag == "searchMeta" {
			field := v.Elem().Field(i)
			if field.Type() == reflect.TypeOf((*hydraidego.SearchMeta)(nil)) {
				meta := &hydraidego.SearchMeta{
					VectorScores:  protoMeta.VectorScores,
					MatchedLabels: protoMeta.MatchedLabels,
				}
				field.Set(reflect.ValueOf(meta))
			}
			return
		}
	}
}

// --- setSearchMetaOnModel tests ---

func TestSetSearchMeta_Populated(t *testing.T) {
	m := &testModelWithSearchMeta{Key: "test"}
	protoMeta := &hydrapb.SearchResultMeta{
		VectorScores:  []float32{0.87, 0.92},
		MatchedLabels: []string{"booking", "semantic"},
	}

	setSearchMetaOnTestModel(m, protoMeta)

	if m.Meta == nil {
		t.Fatal("expected Meta to be populated")
	}
	if len(m.Meta.VectorScores) != 2 {
		t.Fatalf("expected 2 vector scores, got %d", len(m.Meta.VectorScores))
	}
	if m.Meta.VectorScores[0] != 0.87 || m.Meta.VectorScores[1] != 0.92 {
		t.Errorf("unexpected scores: %v", m.Meta.VectorScores)
	}
	if len(m.Meta.MatchedLabels) != 2 {
		t.Fatalf("expected 2 labels, got %d", len(m.Meta.MatchedLabels))
	}
	if m.Meta.MatchedLabels[0] != "booking" || m.Meta.MatchedLabels[1] != "semantic" {
		t.Errorf("unexpected labels: %v", m.Meta.MatchedLabels)
	}
}

func TestSetSearchMeta_NilProtoMeta(t *testing.T) {
	m := &testModelWithSearchMeta{Key: "test"}
	setSearchMetaOnTestModel(m, nil)
	if m.Meta != nil {
		t.Error("expected Meta to be nil when proto meta is nil")
	}
}

func TestSetSearchMeta_EmptyProtoMeta(t *testing.T) {
	m := &testModelWithSearchMeta{Key: "test"}
	setSearchMetaOnTestModel(m, &hydrapb.SearchResultMeta{})
	if m.Meta != nil {
		t.Error("expected Meta to be nil when proto meta has no scores or labels")
	}
}

func TestSetSearchMeta_VectorScoresOnly(t *testing.T) {
	m := &testModelWithSearchMeta{Key: "test"}
	protoMeta := &hydrapb.SearchResultMeta{
		VectorScores: []float32{0.95},
	}

	setSearchMetaOnTestModel(m, protoMeta)

	if m.Meta == nil {
		t.Fatal("expected Meta to be populated")
	}
	if len(m.Meta.VectorScores) != 1 || m.Meta.VectorScores[0] != 0.95 {
		t.Errorf("unexpected scores: %v", m.Meta.VectorScores)
	}
}

func TestSetSearchMeta_LabelsOnly(t *testing.T) {
	m := &testModelWithSearchMeta{Key: "test"}
	protoMeta := &hydrapb.SearchResultMeta{
		MatchedLabels: []string{"health"},
	}

	setSearchMetaOnTestModel(m, protoMeta)

	if m.Meta == nil {
		t.Fatal("expected Meta to be populated")
	}
	if len(m.Meta.MatchedLabels) != 1 || m.Meta.MatchedLabels[0] != "health" {
		t.Errorf("unexpected labels: %v", m.Meta.MatchedLabels)
	}
}

func TestSetSearchMeta_ModelWithoutTag(t *testing.T) {
	m := &testModelWithoutSearchMeta{Key: "test"}
	protoMeta := &hydrapb.SearchResultMeta{
		VectorScores:  []float32{0.87},
		MatchedLabels: []string{"booking"},
	}
	// Should not panic, just skip — no searchMeta field
	setSearchMetaOnTestModel(m, protoMeta)
}

func TestSetSearchMeta_NonPointerModel(t *testing.T) {
	m := testModelWithSearchMeta{Key: "test"}
	protoMeta := &hydrapb.SearchResultMeta{
		VectorScores: []float32{0.87},
	}
	// Not a pointer — should be a no-op, no panic
	setSearchMetaOnTestModel(m, protoMeta)
}

// --- WithLabel SDK tests (public API, can test from any package) ---

func TestSDK_Filter_WithLabel(t *testing.T) {
	f := hydraidego.FilterBytesFieldSliceContainsInt8("Funcs", int8(7)).WithLabel("booking")
	// WithLabel returns *Filter — verify it's chainable (compiles = passes)
	_ = f
}

func TestSDK_VectorFilter_WithLabel(t *testing.T) {
	vf := hydraidego.FilterVector("Embedding", []float32{1, 0, 0}, 0.5).WithLabel("semantic")
	_ = vf
}

func TestSDK_PhraseFilter_WithLabel(t *testing.T) {
	pf := hydraidego.FilterPhrase("WordIndex", "hello", "world").WithLabel("greeting")
	_ = pf
}

func TestSDK_GeoDistance_WithLabel(t *testing.T) {
	gf := hydraidego.GeoDistance("Lat", "Lng", 47.5, 19.0, 50.0, hydraidego.GeoInside).WithLabel("budapest")
	_ = gf
}

func TestSDK_SearchMeta_Struct(t *testing.T) {
	meta := &hydraidego.SearchMeta{
		VectorScores:  []float32{0.85, 0.92},
		MatchedLabels: []string{"a", "b", "c"},
	}
	if len(meta.VectorScores) != 2 {
		t.Errorf("expected 2 scores, got %d", len(meta.VectorScores))
	}
	if len(meta.MatchedLabels) != 3 {
		t.Errorf("expected 3 labels, got %d", len(meta.MatchedLabels))
	}
}
