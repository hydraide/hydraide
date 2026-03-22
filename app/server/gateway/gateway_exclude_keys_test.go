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

// simulateKeysLoop simulates the gateway treasure loop pattern:
// include keys -> exclude keys -> filter -> collect. Returns the collected treasures.
func simulateKeysLoop(
	treasures []treasure.Treasure,
	includedKeys []string,
	excludeKeys []string,
	filters *hydrapb.FilterGroup,
	maxResults int32,
	keysOnly bool,
) []*hydrapb.Treasure {
	includeMap := buildKeySet(includedKeys)
	excludeMap := buildKeySet(excludeKeys)
	var result []*hydrapb.Treasure
	var matchCount int32

	for _, tr := range treasures {
		key := tr.GetKey()
		if includeMap != nil {
			if _, included := includeMap[key]; !included {
				continue
			}
		}
		if excludeMap != nil {
			if _, excluded := excludeMap[key]; excluded {
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

// --- buildKeySet tests ---

func TestBuildExcludeMap_Nil(t *testing.T) {
	m := buildKeySet(nil)
	if m != nil {
		t.Error("expected nil for nil input")
	}
}

func TestBuildExcludeMap_Empty(t *testing.T) {
	m := buildKeySet([]string{})
	if m != nil {
		t.Error("expected nil for empty input")
	}
}

func TestBuildExcludeMap_Basic(t *testing.T) {
	m := buildKeySet([]string{"a", "b", "c"})
	if len(m) != 3 {
		t.Errorf("expected 3 entries, got %d", len(m))
	}
	if _, ok := m["b"]; !ok {
		t.Error("expected key 'b' in map")
	}
}

func TestBuildExcludeMap_Duplicates(t *testing.T) {
	m := buildKeySet([]string{"a", "a", "b"})
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
	result := simulateKeysLoop(treasures, nil, []string{"k2", "k4"}, nil, 0, false)
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
	result := simulateKeysLoop(treasures, nil, []string{}, nil, 0, false)
	if len(result) != 3 {
		t.Errorf("expected 3 results with empty exclude list, got %d", len(result))
	}
}

func TestExcludeKeys_AllExcluded(t *testing.T) {
	treasures := []treasure.Treasure{
		newTreasureWithKey("k1"),
		newTreasureWithKey("k2"),
	}
	result := simulateKeysLoop(treasures, nil, []string{"k1", "k2"}, nil, 0, false)
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
	result := simulateKeysLoop(treasures, nil, []string{"k99", "k100"}, nil, 0, false)
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
	result := simulateKeysLoop(treasures, nil, []string{"k3"}, filters, 0, false)
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
	result := simulateKeysLoop(treasures, nil, []string{"k2"}, nil, 2, false)
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
	result := simulateKeysLoop(treasures, nil, nil, nil, 0, true)
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
	result := simulateKeysLoop(treasures, nil, nil, nil, 0, false)
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
	result := simulateKeysLoop(treasures, nil, []string{"k2"}, nil, 0, true)
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

// --- IncludedKeys tests ---

func TestIncludedKeys_Basic(t *testing.T) {
	treasures := []treasure.Treasure{
		newTreasureWithKey("k1"),
		newTreasureWithKey("k2"),
		newTreasureWithKey("k3"),
		newTreasureWithKey("k4"),
		newTreasureWithKey("k5"),
	}
	result := simulateKeysLoop(treasures, []string{"k2", "k4"}, nil, nil, 0, false)
	if len(result) != 2 {
		t.Fatalf("expected 2 results, got %d", len(result))
	}
	if result[0].Key != "k2" || result[1].Key != "k4" {
		t.Errorf("unexpected keys: %s, %s", result[0].Key, result[1].Key)
	}
}

func TestIncludedKeys_EmptyList(t *testing.T) {
	treasures := []treasure.Treasure{
		newTreasureWithKey("k1"),
		newTreasureWithKey("k2"),
		newTreasureWithKey("k3"),
	}
	result := simulateKeysLoop(treasures, []string{}, nil, nil, 0, false)
	if len(result) != 3 {
		t.Errorf("expected 3 results with empty include list (no restriction), got %d", len(result))
	}
}

func TestIncludedKeys_AllIncluded(t *testing.T) {
	treasures := []treasure.Treasure{
		newTreasureWithKey("k1"),
		newTreasureWithKey("k2"),
	}
	result := simulateKeysLoop(treasures, []string{"k1", "k2"}, nil, nil, 0, false)
	if len(result) != 2 {
		t.Errorf("expected 2 results, got %d", len(result))
	}
}

func TestIncludedKeys_NonExistentKeys(t *testing.T) {
	treasures := []treasure.Treasure{
		newTreasureWithKey("k1"),
		newTreasureWithKey("k2"),
		newTreasureWithKey("k3"),
	}
	result := simulateKeysLoop(treasures, []string{"k99", "k100"}, nil, nil, 0, false)
	if len(result) != 0 {
		t.Errorf("expected 0 results when include list has no matching keys, got %d", len(result))
	}
}

func TestIncludedKeys_WithExcludeKeys(t *testing.T) {
	treasures := []treasure.Treasure{
		newTreasureWithKey("k1"),
		newTreasureWithKey("k2"),
		newTreasureWithKey("k3"),
		newTreasureWithKey("k4"),
	}
	// Include k1,k2,k3 then exclude k2 -> k1, k3
	result := simulateKeysLoop(treasures, []string{"k1", "k2", "k3"}, []string{"k2"}, nil, 0, false)
	if len(result) != 2 {
		t.Fatalf("expected 2 results (include then exclude), got %d", len(result))
	}
	if result[0].Key != "k1" || result[1].Key != "k3" {
		t.Errorf("unexpected keys: %s, %s", result[0].Key, result[1].Key)
	}
}

func TestIncludedKeys_WithFilter(t *testing.T) {
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
	// Include k1,k2,k3 (3 match filter: k1=80, k3=90), k2=30 fails filter
	result := simulateKeysLoop(treasures, []string{"k1", "k2", "k3"}, nil, filters, 0, false)
	if len(result) != 2 {
		t.Fatalf("expected 2 results (include + filter), got %d", len(result))
	}
	if result[0].Key != "k1" || result[1].Key != "k3" {
		t.Errorf("unexpected keys: %s, %s", result[0].Key, result[1].Key)
	}
}

func TestIncludedKeys_FullPipeline(t *testing.T) {
	treasures := []treasure.Treasure{
		newTreasureWithKeyAndBytes(t, "k1", map[string]interface{}{"Score": int64(80)}),
		newTreasureWithKeyAndBytes(t, "k2", map[string]interface{}{"Score": int64(90)}),
		newTreasureWithKeyAndBytes(t, "k3", map[string]interface{}{"Score": int64(70)}),
		newTreasureWithKeyAndBytes(t, "k4", map[string]interface{}{"Score": int64(60)}),
		newTreasureWithKeyAndBytes(t, "k5", map[string]interface{}{"Score": int64(95)}),
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
	// Include k1,k2,k3,k4 (exclude k5), exclude k4, filter >50, maxResults=2
	// Pipeline: include [k1,k2,k3,k4] -> exclude k4 -> [k1,k2,k3] -> filter >50 -> [k1=80,k2=90,k3=70] -> max 2 -> [k1,k2]
	result := simulateKeysLoop(treasures, []string{"k1", "k2", "k3", "k4"}, []string{"k4"}, filters, 2, false)
	if len(result) != 2 {
		t.Fatalf("expected 2 results (full pipeline), got %d", len(result))
	}
	if result[0].Key != "k1" || result[1].Key != "k2" {
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
		buildKeySet(keys)
	}
}

func BenchmarkBuildExcludeMap_10000(b *testing.B) {
	keys := make([]string, 10000)
	for i := range keys {
		keys[i] = "key-" + string(rune('A'+i%26)) + string(rune('0'+i/26))
	}
	for b.Loop() {
		buildKeySet(keys)
	}
}

func BenchmarkExcludeKeyLookup_InMap(b *testing.B) {
	keys := make([]string, 10000)
	for i := range keys {
		keys[i] = "key-" + string(rune('A'+i%26)) + string(rune('0'+i/26))
	}
	m := buildKeySet(keys)
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
	m := buildKeySet(keys)
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
