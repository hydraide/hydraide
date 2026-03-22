package models

import (
	"log/slog"

	"github.com/hydraide/hydraide/sdk/go/hydraidego"
	"github.com/hydraide/hydraide/sdk/go/hydraidego/name"
	"github.com/hydraide/hydraide/sdk/go/hydraidego/utils/hydraidehelper"
	"github.com/hydraide/hydraide/sdk/go/hydraidego/utils/repo"
)

// --- SliceLen Examples ---

// SearchWithContacts finds domains where the LLMContacts slice has at least 1 entry.
// Uses the #len pseudo-field to compare slice length.
func (d *DizzletPayload) SearchWithContacts(r repo.Repo) ([]*DizzletPayload, error) {
	ctx, cancel := hydraidehelper.CreateHydraContext()
	defer cancel()

	h := r.GetHydraidego()
	swamp := name.New().Sanctuary("catalog").Realm("dizzlets").Swamp("com")
	index := &hydraidego.Index{
		IndexType:  hydraidego.IndexCreationTime,
		IndexOrder: hydraidego.IndexOrderDesc,
	}

	filters := hydraidego.FilterAND(
		hydraidego.FilterBytesFieldSliceLen(hydraidego.GreaterThan, "LLMContacts", 0),
	)

	var results []*DizzletPayload
	err := h.CatalogReadManyStream(ctx, swamp, index, filters, DizzletPayload{}, func(model any) error {
		results = append(results, model.(*DizzletPayload))
		return nil
	})
	if err != nil {
		slog.Error("SearchWithContacts", "error", err)
		return nil, err
	}
	return results, nil
}

// SearchWithManyCategories finds domains with 3 or more product categories.
func (d *DizzletPayload) SearchWithManyCategories(r repo.Repo) ([]*DizzletPayload, error) {
	ctx, cancel := hydraidehelper.CreateHydraContext()
	defer cancel()

	h := r.GetHydraidego()
	swamp := name.New().Sanctuary("catalog").Realm("dizzlets").Swamp("com")
	index := &hydraidego.Index{
		IndexType:  hydraidego.IndexCreationTime,
		IndexOrder: hydraidego.IndexOrderDesc,
	}

	filters := hydraidego.FilterAND(
		hydraidego.FilterBytesFieldSliceLen(hydraidego.GreaterThanOrEqual, "LLMProductCategories", 3),
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

// --- NestedSliceAny Examples ---

// SearchWithEmail finds domains where at least 1 contact person has a non-empty email.
// Uses the [*] wildcard syntax to iterate over struct slice elements.
func (d *DizzletPayload) SearchWithEmail(r repo.Repo) ([]*DizzletPayload, error) {
	ctx, cancel := hydraidehelper.CreateHydraContext()
	defer cancel()

	h := r.GetHydraidego()
	swamp := name.New().Sanctuary("catalog").Realm("dizzlets").Swamp("com")
	index := &hydraidego.Index{
		IndexType:  hydraidego.IndexCreationTime,
		IndexOrder: hydraidego.IndexOrderDesc,
	}

	filters := hydraidego.FilterAND(
		hydraidego.FilterBytesFieldNestedSliceAnyString("LLMContacts", "Email", hydraidego.IsNotEmpty, ""),
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

// SearchWithCEO finds domains where at least 1 contact has the Role "CEO".
func (d *DizzletPayload) SearchWithCEO(r repo.Repo) ([]*DizzletPayload, error) {
	ctx, cancel := hydraidehelper.CreateHydraContext()
	defer cancel()

	h := r.GetHydraidego()
	swamp := name.New().Sanctuary("catalog").Realm("dizzlets").Swamp("com")
	index := &hydraidego.Index{
		IndexType:  hydraidego.IndexCreationTime,
		IndexOrder: hydraidego.IndexOrderDesc,
	}

	filters := hydraidego.FilterAND(
		hydraidego.FilterBytesFieldNestedSliceAnyString("LLMContacts", "Role", hydraidego.Equal, "CEO"),
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

// SearchWithDomainEmail finds domains where at least 1 contact has a domain-matching email.
func (d *DizzletPayload) SearchWithDomainEmail(r repo.Repo) ([]*DizzletPayload, error) {
	ctx, cancel := hydraidehelper.CreateHydraContext()
	defer cancel()

	h := r.GetHydraidego()
	swamp := name.New().Sanctuary("catalog").Realm("dizzlets").Swamp("com")
	index := &hydraidego.Index{
		IndexType:  hydraidego.IndexCreationTime,
		IndexOrder: hydraidego.IndexOrderDesc,
	}

	filters := hydraidego.FilterAND(
		hydraidego.FilterBytesFieldNestedSliceAnyBool("LLMContacts", "IsDomainMatch", hydraidego.Equal, true),
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
