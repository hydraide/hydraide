package models

import (
	"log/slog"

	"github.com/hydraide/hydraide/sdk/go/hydraidego"
	"github.com/hydraide/hydraide/sdk/go/hydraidego/name"
	"github.com/hydraide/hydraide/sdk/go/hydraidego/utils/hydraidehelper"
	"github.com/hydraide/hydraide/sdk/go/hydraidego/utils/repo"
)

// --- ExcludeKeys Examples ---

// SearchWithExclusion demonstrates paginated search excluding already-seen results.
// Each subsequent call passes the previously seen keys so they don't appear again.
func (d *DizzletPayload) SearchWithExclusion(r repo.Repo, seenKeys []string) ([]*DizzletPayload, error) {
	ctx, cancel := hydraidehelper.CreateHydraContext()
	defer cancel()

	h := r.GetHydraidego()
	swamp := name.New().Sanctuary("catalog").Realm("dizzlets").Swamp("com")

	index := &hydraidego.Index{
		IndexType:   hydraidego.IndexCreationTime,
		IndexOrder:  hydraidego.IndexOrderDesc,
		MaxResults:  10,
		ExcludeKeys: seenKeys, // skip already-seen results
	}

	filters := hydraidego.FilterAND(
		hydraidego.FilterBytesFieldSliceContainsInt8("LLMSiteFunctions", int8(7)),
	)

	var results []*DizzletPayload
	err := h.CatalogReadManyStream(ctx, swamp, index, filters, DizzletPayload{}, func(model any) error {
		results = append(results, model.(*DizzletPayload))
		return nil
	})
	if err != nil {
		slog.Error("SearchWithExclusion", "error", err)
		return nil, err
	}
	return results, nil
}

// --- KeysOnly Examples ---

// DiscoverThenFetch demonstrates a two-phase search pattern:
// 1. KeysOnly search to discover matching keys (lightweight, no content transfer)
// 2. CatalogReadBatch to fetch full content for selected keys only
func (d *DizzletPayload) DiscoverThenFetch(r repo.Repo) ([]*DizzletPayload, error) {
	ctx, cancel := hydraidehelper.CreateHydraContext()
	defer cancel()

	h := r.GetHydraidego()
	swamp := name.New().Sanctuary("catalog").Realm("dizzlets").Swamp("com")

	// Phase 1: discover matching keys (lightweight — no content serialization)
	discoveryIndex := &hydraidego.Index{
		IndexType:  hydraidego.IndexCreationTime,
		IndexOrder: hydraidego.IndexOrderDesc,
		MaxResults: 1000,
		KeysOnly:   true, // only Key + IsExist, no content
	}

	filters := hydraidego.FilterAND(
		hydraidego.FilterBytesFieldSliceContainsInt8("LLMSiteFunctions", int8(7)),
		hydraidego.FilterBytesFieldSliceLen(hydraidego.GreaterThan, "LLMContacts", 0),
	)

	var matchedKeys []string
	err := h.CatalogReadManyStream(ctx, swamp, discoveryIndex, filters, DizzletPayload{}, func(model any) error {
		m := model.(*DizzletPayload)
		matchedKeys = append(matchedKeys, m.Domain)
		return nil
	})
	if err != nil {
		return nil, err
	}

	if len(matchedKeys) == 0 {
		return nil, nil
	}

	// Phase 2: fetch full content for the top 10 results only
	top := 10
	if len(matchedKeys) < top {
		top = len(matchedKeys)
	}

	var results []*DizzletPayload
	err = h.CatalogReadBatch(ctx, swamp, matchedKeys[:top], DizzletPayload{}, func(model any) error {
		results = append(results, model.(*DizzletPayload))
		return nil
	})
	if err != nil {
		return nil, err
	}
	return results, nil
}

// --- Combined ExcludeKeys + KeysOnly Example ---

// IncrementalSearch demonstrates multiple rounds of search with a growing exclusion list.
// Each round discovers new keys, appends them to the exclusion list, and repeats.
func (d *DizzletPayload) IncrementalSearch(r repo.Repo, rounds int) ([]string, error) {
	ctx, cancel := hydraidehelper.CreateHydraContext()
	defer cancel()

	h := r.GetHydraidego()
	swamp := name.New().Sanctuary("catalog").Realm("dizzlets").Swamp("com")

	filters := hydraidego.FilterAND(
		hydraidego.FilterBytesFieldSliceContainsInt8("LLMSiteFunctions", int8(7)),
	)

	var allKeys []string

	for round := 0; round < rounds; round++ {
		index := &hydraidego.Index{
			IndexType:   hydraidego.IndexCreationTime,
			IndexOrder:  hydraidego.IndexOrderDesc,
			MaxResults:  10,
			ExcludeKeys: allKeys, // exclude all previously found keys
			KeysOnly:    true,    // only need keys, not content
		}

		var roundKeys []string
		err := h.CatalogReadManyStream(ctx, swamp, index, filters, DizzletPayload{}, func(model any) error {
			m := model.(*DizzletPayload)
			roundKeys = append(roundKeys, m.Domain)
			return nil
		})
		if err != nil {
			return nil, err
		}

		if len(roundKeys) == 0 {
			break // no more results
		}

		allKeys = append(allKeys, roundKeys...)
	}

	return allKeys, nil
}

// --- IncludedKeys Examples ---

// SearchWithinSubset searches only within a pre-computed candidate list.
// Only domains in candidateKeys are evaluated against the filters.
func (d *DizzletPayload) SearchWithinSubset(r repo.Repo, candidateKeys []string) ([]*DizzletPayload, error) {
	ctx, cancel := hydraidehelper.CreateHydraContext()
	defer cancel()

	h := r.GetHydraidego()
	swamp := name.New().Sanctuary("catalog").Realm("dizzlets").Swamp("com")

	index := &hydraidego.Index{
		IndexType:    hydraidego.IndexCreationTime,
		IndexOrder:   hydraidego.IndexOrderDesc,
		IncludedKeys: candidateKeys, // only search within these keys
	}

	filters := hydraidego.FilterAND(
		hydraidego.FilterBytesFieldSliceContainsInt8("LLMSiteFunctions", int8(7)),
		hydraidego.FilterBytesFieldSliceLen(hydraidego.GreaterThan, "LLMContacts", 0),
	)

	var results []*DizzletPayload
	err := h.CatalogReadManyStream(ctx, swamp, index, filters, DizzletPayload{}, func(model any) error {
		results = append(results, model.(*DizzletPayload))
		return nil
	})
	if err != nil {
		return nil, err
	}
	return results, nil
}

// SearchSubsetExcludingSeen combines IncludedKeys with ExcludeKeys.
// Searches within candidates but excludes already-seen results.
func (d *DizzletPayload) SearchSubsetExcludingSeen(r repo.Repo, candidateKeys []string, seenKeys []string) ([]*DizzletPayload, error) {
	ctx, cancel := hydraidehelper.CreateHydraContext()
	defer cancel()

	h := r.GetHydraidego()
	swamp := name.New().Sanctuary("catalog").Realm("dizzlets").Swamp("com")

	index := &hydraidego.Index{
		IndexType:    hydraidego.IndexCreationTime,
		IndexOrder:   hydraidego.IndexOrderDesc,
		IncludedKeys: candidateKeys,
		ExcludeKeys:  seenKeys,
		MaxResults:   10,
	}

	filters := hydraidego.FilterAND(
		hydraidego.FilterBytesFieldSliceContainsInt8("LLMSiteFunctions", int8(7)),
	)

	var results []*DizzletPayload
	err := h.CatalogReadManyStream(ctx, swamp, index, filters, DizzletPayload{}, func(model any) error {
		results = append(results, model.(*DizzletPayload))
		return nil
	})
	if err != nil {
		return nil, err
	}
	return results, nil
}
