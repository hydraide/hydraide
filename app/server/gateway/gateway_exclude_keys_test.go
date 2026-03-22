package gateway

import (
	"testing"

	"github.com/hydraide/hydraide/app/core/hydra/swamp/treasure"
	hydrapb "github.com/hydraide/hydraide/generated/hydraidepbgo"
)

// --- Test helpers ---

func newTreasureWithKey(key string) treasure.Treasure {
	tr := treasure.New(noopSave)
	gid := tr.StartTreasureGuard(true)
	tr.BodySetKey(gid, key)
	tr.SetContentString(gid, "content-"+key)
	tr.ReleaseTreasureGuard(gid)
	return tr
}

func newTreasureWithKeyAndBytes(t *testing.T, key string, data map[string]interface{}) treasure.Treasure {
	t.Helper()
	tr := treasure.New(noopSave)
	gid := tr.StartTreasureGuard(true)
	tr.BodySetKey(gid, key)
	tr.SetContentByteArray(gid, makeMsgpackBytesVal(t, data))
	tr.ReleaseTreasureGuard(gid)
	return tr
}

// simulateExcludeKeysLoop simulates the gateway treasure loop pattern:
// exclude keys -> filter -> collect keys. Returns the collected keys.
func simulateExcludeKeysLoop(
	treasures []treasure.Treasure,
	excludeKeys []string,
	filters *hydrapb.FilterGroup,
	maxResults int32,
	keysOnly bool,
) []*hydrapb.Treasure {
	excludeMap := buildExcludeMap(excludeKeys)
	var result []*hydrapb.Treasure
	var matchCount int32

	for _, tr := range treasures {
		if excludeMap != nil {
			if _, excluded := excludeMap[tr.GetKey()]; excluded {
				continue
			}
		}
		if filters != nil && !evaluateNativeFilterGroup(tr, filters) {
			continue
		}
		if keysOnly {
			result = append(result, &hydrapb.Treasure{Key: tr.GetKey(), IsExist: true})
		} else {
			t := &hydrapb.Treasure{}
			treasureToKeyValuePair(tr, t)
			result = append(result, t)
		}
		matchCount++
		if maxResults > 0 && matchCount >= maxResults {
			break
		}
	}
	return result
}

// --- buildExcludeMap tests ---

func TestBuildExcludeMap_Nil(t *testing.T) {
	m := buildExcludeMap(nil)
	if m != nil {
		t.Error("expected nil for nil input")
	}
}

func TestBuildExcludeMap_Empty(t *testing.T) {
	m := buildExcludeMap([]string{})
	if m != nil {
		t.Error("expected nil for empty input")
	}
}

func TestBuildExcludeMap_Basic(t *testing.T) {
	m := buildExcludeMap([]string{"a", "b", "c"})
	if len(m) != 3 {
		t.Errorf("expected 3 entries, got %d", len(m))
	}
	if _, ok := m["b"]; !ok {
		t.Error("expected key 'b' in map")
	}
}

func TestBuildExcludeMap_Duplicates(t *testing.T) {
	m := buildExcludeMap([]string{"a", "a", "b"})
	if len(m) != 2 {
		t.Errorf("expected 2 entries for duplicated input, got %d", len(m))
	}
}

// --- ExcludeKeys tests ---

func TestExcludeKeys_Basic(t *testing.T) {
	treasures := []treasure.Treasure{
		newTreasureWithKey("k1"),
		newTreasureWithKey("k2"),
		newTreasureWithKey("k3"),
		newTreasureWithKey("k4"),
		newTreasureWithKey("k5"),
	}
	result := simulateExcludeKeysLoop(treasures, []string{"k2", "k4"}, nil, 0, false)
	if len(result) != 3 {
		t.Fatalf("expected 3 results, got %d", len(result))
	}
	keys := []string{result[0].Key, result[1].Key, result[2].Key}
	if keys[0] != "k1" || keys[1] != "k3" || keys[2] != "k5" {
		t.Errorf("unexpected keys: %v", keys)
	}
}

func TestExcludeKeys_EmptyList(t *testing.T) {
	treasures := []treasure.Treasure{
		newTreasureWithKey("k1"),
		newTreasureWithKey("k2"),
		newTreasureWithKey("k3"),
	}
	result := simulateExcludeKeysLoop(treasures, []string{}, nil, 0, false)
	if len(result) != 3 {
		t.Errorf("expected 3 results with empty exclude list, got %d", len(result))
	}
}

func TestExcludeKeys_AllExcluded(t *testing.T) {
	treasures := []treasure.Treasure{
		newTreasureWithKey("k1"),
		newTreasureWithKey("k2"),
	}
	result := simulateExcludeKeysLoop(treasures, []string{"k1", "k2"}, nil, 0, false)
	if len(result) != 0 {
		t.Errorf("expected 0 results when all excluded, got %d", len(result))
	}
}

func TestExcludeKeys_NonExistentKeys(t *testing.T) {
	treasures := []treasure.Treasure{
		newTreasureWithKey("k1"),
		newTreasureWithKey("k2"),
		newTreasureWithKey("k3"),
	}
	result := simulateExcludeKeysLoop(treasures, []string{"k99", "k100"}, nil, 0, false)
	if len(result) != 3 {
		t.Errorf("expected 3 results when excluding non-existent keys, got %d", len(result))
	}
}

func TestExcludeKeys_WithFilter(t *testing.T) {
	// 5 treasures with Score field, 3 have Score > 50, exclude 1 of those
	treasures := []treasure.Treasure{
		newTreasureWithKeyAndBytes(t, "k1", map[string]interface{}{"Score": int64(80)}),
		newTreasureWithKeyAndBytes(t, "k2", map[string]interface{}{"Score": int64(30)}),
		newTreasureWithKeyAndBytes(t, "k3", map[string]interface{}{"Score": int64(90)}),
		newTreasureWithKeyAndBytes(t, "k4", map[string]interface{}{"Score": int64(10)}),
		newTreasureWithKeyAndBytes(t, "k5", map[string]interface{}{"Score": int64(70)}),
	}
	path := "Score"
	filters := &hydrapb.FilterGroup{
		Logic: hydrapb.FilterLogic_AND,
		Filters: []*hydrapb.TreasureFilter{{
			Operator:       hydrapb.Relational_GREATER_THAN,
			BytesFieldPath: &path,
			CompareValue:   &hydrapb.TreasureFilter_Int64Val{Int64Val: 50},
		}},
	}
	// 3 match filter (k1=80, k3=90, k5=70), exclude k3
	result := simulateExcludeKeysLoop(treasures, []string{"k3"}, filters, 0, false)
	if len(result) != 2 {
		t.Fatalf("expected 2 results (filter + exclude), got %d", len(result))
	}
	if result[0].Key != "k1" || result[1].Key != "k5" {
		t.Errorf("unexpected keys: %s, %s", result[0].Key, result[1].Key)
	}
}

func TestExcludeKeys_WithMaxResults(t *testing.T) {
	treasures := []treasure.Treasure{
		newTreasureWithKey("k1"),
		newTreasureWithKey("k2"),
		newTreasureWithKey("k3"),
		newTreasureWithKey("k4"),
		newTreasureWithKey("k5"),
	}
	// Exclude k2, maxResults=2 -> should get k1, k3
	result := simulateExcludeKeysLoop(treasures, []string{"k2"}, nil, 2, false)
	if len(result) != 2 {
		t.Fatalf("expected 2 results, got %d", len(result))
	}
	if result[0].Key != "k1" || result[1].Key != "k3" {
		t.Errorf("unexpected keys: %s, %s", result[0].Key, result[1].Key)
	}
}

// --- KeysOnly tests ---

func TestKeysOnly_Basic(t *testing.T) {
	treasures := []treasure.Treasure{newTreasureWithKey("k1")}
	result := simulateExcludeKeysLoop(treasures, nil, nil, 0, true)
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	if result[0].Key != "k1" {
		t.Errorf("expected key 'k1', got '%s'", result[0].Key)
	}
	if !result[0].IsExist {
		t.Error("expected IsExist=true")
	}
	// Verify no content is set
	if result[0].GetStringVal() != "" {
		t.Error("expected no content in KeysOnly mode")
	}
}

func TestKeysOnly_False(t *testing.T) {
	treasures := []treasure.Treasure{newTreasureWithKey("k1")}
	result := simulateExcludeKeysLoop(treasures, nil, nil, 0, false)
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	if result[0].Key != "k1" {
		t.Errorf("expected key 'k1', got '%s'", result[0].Key)
	}
	// Full treasure should have content
	if result[0].GetStringVal() != "content-k1" {
		t.Errorf("expected content 'content-k1', got '%s'", result[0].GetStringVal())
	}
}

func TestKeysOnly_WithExcludeKeys(t *testing.T) {
	treasures := []treasure.Treasure{
		newTreasureWithKey("k1"),
		newTreasureWithKey("k2"),
		newTreasureWithKey("k3"),
	}
	result := simulateExcludeKeysLoop(treasures, []string{"k2"}, nil, 0, true)
	if len(result) != 2 {
		t.Fatalf("expected 2 results, got %d", len(result))
	}
	// Both should be keys-only
	for _, r := range result {
		if !r.IsExist {
			t.Errorf("expected IsExist=true for key %s", r.Key)
		}
		if r.GetStringVal() != "" {
			t.Errorf("expected no content for key %s in KeysOnly mode", r.Key)
		}
	}
	if result[0].Key != "k1" || result[1].Key != "k3" {
		t.Errorf("unexpected keys: %s, %s", result[0].Key, result[1].Key)
	}
}

// --- Benchmarks ---

func BenchmarkBuildExcludeMap_100(b *testing.B) {
	keys := make([]string, 100)
	for i := range keys {
		keys[i] = "key-" + string(rune('A'+i%26)) + string(rune('0'+i/26))
	}
	for b.Loop() {
		buildExcludeMap(keys)
	}
}

func BenchmarkBuildExcludeMap_10000(b *testing.B) {
	keys := make([]string, 10000)
	for i := range keys {
		keys[i] = "key-" + string(rune('A'+i%26)) + string(rune('0'+i/26))
	}
	for b.Loop() {
		buildExcludeMap(keys)
	}
}

func BenchmarkExcludeKeyLookup_InMap(b *testing.B) {
	keys := make([]string, 10000)
	for i := range keys {
		keys[i] = "key-" + string(rune('A'+i%26)) + string(rune('0'+i/26))
	}
	m := buildExcludeMap(keys)
	target := keys[5000] // key that exists
	for b.Loop() {
		_, _ = m[target]
	}
}

func BenchmarkExcludeKeyLookup_NotInMap(b *testing.B) {
	keys := make([]string, 10000)
	for i := range keys {
		keys[i] = "key-" + string(rune('A'+i%26)) + string(rune('0'+i/26))
	}
	m := buildExcludeMap(keys)
	for b.Loop() {
		_, _ = m["nonexistent-key"]
	}
}

func BenchmarkKeysOnlyProto(b *testing.B) {
	tr := newTreasureWithKey("benchmark-key")
	for b.Loop() {
		_ = &hydrapb.Treasure{Key: tr.GetKey(), IsExist: true}
	}
}

func BenchmarkFullTreasureConversion(b *testing.B) {
	tr := newTreasureWithKey("benchmark-key")
	for b.Loop() {
		t := &hydrapb.Treasure{}
		treasureToKeyValuePair(tr, t)
	}
}
