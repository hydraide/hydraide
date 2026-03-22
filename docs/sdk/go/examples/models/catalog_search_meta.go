package models

import (
	"fmt"
	"sort"

	"github.com/hydraide/hydraide/sdk/go/hydraidego"
	"github.com/hydraide/hydraide/sdk/go/hydraidego/name"
	"github.com/hydraide/hydraide/sdk/go/hydraidego/utils/hydraidehelper"
	"github.com/hydraide/hydraide/sdk/go/hydraidego/utils/repo"
)

// --- SearchMeta Examples ---

// DizzletWithMeta extends DizzletPayload with a SearchMeta field.
// The searchMeta-tagged field is automatically populated during search/read responses.
// It is read-only: never processed during write operations (Set, Create, Update).
type DizzletWithMeta struct {
	Domain  string                  `hydraide:"key"`
	Payload []byte                  `hydraide:"value"`
	Meta    *hydraidego.SearchMeta  `hydraide:"searchMeta"` // auto-populated on read
}

// SearchResult wraps a domain with its search metadata for client-side ranking.
type SearchResult struct {
	Domain        string
	VectorScore   float32
	MatchedLabels []string
}

// SearchWithRelevance demonstrates vector similarity search with score capture.
// The Meta.VectorScores field is automatically populated via the searchMeta tag.
func (d *DizzletWithMeta) SearchWithRelevance(r repo.Repo, queryVec []float32) ([]SearchResult, error) {
	ctx, cancel := hydraidehelper.CreateHydraContext()
	defer cancel()

	h := r.GetHydraidego()
	swamp := name.New().Sanctuary("catalog").Realm("dizzlets").Swamp("com")

	index := &hydraidego.Index{
		IndexType:  hydraidego.IndexCreationTime,
		IndexOrder: hydraidego.IndexOrderDesc,
		MaxResults: 100,
	}

	normalizedVec := hydraidego.NormalizeVector(queryVec)
	filters := hydraidego.FilterAND(
		hydraidego.FilterVector("Embedding", normalizedVec, 0.6).WithLabel("semantic"),
		hydraidego.FilterBytesFieldSliceContainsInt8("LLMSiteFunctions", int8(7)).WithLabel("booking"),
	)

	var results []SearchResult
	err := h.CatalogReadManyStream(ctx, swamp, index, filters, DizzletWithMeta{},
		func(model any) error {
			m := model.(*DizzletWithMeta)
			result := SearchResult{Domain: m.Domain}
			if m.Meta != nil {
				if len(m.Meta.VectorScores) > 0 {
					result.VectorScore = m.Meta.VectorScores[0]
				}
				result.MatchedLabels = m.Meta.MatchedLabels
			}
			results = append(results, result)
			return nil
		})
	if err != nil {
		return nil, err
	}

	// Sort by relevance (highest score first)
	sort.Slice(results, func(i, j int) bool {
		return results[i].VectorScore > results[j].VectorScore
	})

	return results, nil
}

// SearchWithMatchLabels demonstrates OR search with labeled conditions.
// MatchedLabels shows which specific OR branches matched for each result.
func (d *DizzletWithMeta) SearchWithMatchLabels(r repo.Repo) error {
	ctx, cancel := hydraidehelper.CreateHydraContext()
	defer cancel()

	h := r.GetHydraidego()
	swamp := name.New().Sanctuary("catalog").Realm("dizzlets").Swamp("com")

	index := &hydraidego.Index{
		IndexType:  hydraidego.IndexCreationTime,
		IndexOrder: hydraidego.IndexOrderDesc,
		MaxResults: 50,
	}

	// OR search: match any of these sectors. Labels track which ones matched.
	// For a domain with sectors [1, 6]: Meta.MatchedLabels = ["hospitality", "health"]
	// For a domain with only sector [6]: Meta.MatchedLabels = ["health"]
	filters := hydraidego.FilterOR(
		hydraidego.FilterBytesFieldSliceContainsInt8("LLMIndustrySectors", int8(1)).WithLabel("hospitality"),
		hydraidego.FilterBytesFieldSliceContainsInt8("LLMIndustrySectors", int8(6)).WithLabel("health"),
		hydraidego.FilterBytesFieldSliceContainsInt8("LLMIndustrySectors", int8(3)).WithLabel("technology"),
	)

	return h.CatalogReadManyStream(ctx, swamp, index, filters, DizzletWithMeta{},
		func(model any) error {
			m := model.(*DizzletWithMeta)
			labels := "none"
			if m.Meta != nil && len(m.Meta.MatchedLabels) > 0 {
				labels = fmt.Sprintf("%v", m.Meta.MatchedLabels)
			}
			fmt.Printf("%s matched: %s\n", m.Domain, labels)
			return nil
		})
}

// SearchKeysOnlyWithScores demonstrates KeysOnly mode with vector scores.
// SearchMeta is populated even in KeysOnly mode — scores without content overhead.
func (d *DizzletWithMeta) SearchKeysOnlyWithScores(r repo.Repo, queryVec []float32) ([]SearchResult, error) {
	ctx, cancel := hydraidehelper.CreateHydraContext()
	defer cancel()

	h := r.GetHydraidego()
	swamp := name.New().Sanctuary("catalog").Realm("dizzlets").Swamp("com")

	index := &hydraidego.Index{
		IndexType:  hydraidego.IndexCreationTime,
		IndexOrder: hydraidego.IndexOrderDesc,
		MaxResults: 1000,
		KeysOnly:   true, // no content — just keys + searchMeta
	}

	normalizedVec := hydraidego.NormalizeVector(queryVec)
	filters := hydraidego.FilterAND(
		hydraidego.FilterBytesFieldSliceContainsInt8("LLMSiteFunctions", int8(7)).WithLabel("booking"),
		hydraidego.FilterVector("Embedding", normalizedVec, 0.5).WithLabel("semantic"),
	)

	var results []SearchResult
	err := h.CatalogReadManyStream(ctx, swamp, index, filters, DizzletWithMeta{},
		func(model any) error {
			m := model.(*DizzletWithMeta)
			result := SearchResult{Domain: m.Domain}
			if m.Meta != nil && len(m.Meta.VectorScores) > 0 {
				result.VectorScore = m.Meta.VectorScores[0]
			}
			results = append(results, result)
			return nil
		})
	if err != nil {
		return nil, err
	}

	// Sort by vector relevance, then fetch top 10 full content
	sort.Slice(results, func(i, j int) bool {
		return results[i].VectorScore > results[j].VectorScore
	})

	top := 10
	if len(results) < top {
		top = len(results)
	}

	// Phase 2: fetch full content for top results
	// topKeys := make([]string, top)
	// for i := 0; i < top; i++ { topKeys[i] = results[i].Domain }
	// h.CatalogReadBatch(ctx, swamp, topKeys, DizzletPayload{}, func(model any) error { ... })

	return results[:top], nil
}
